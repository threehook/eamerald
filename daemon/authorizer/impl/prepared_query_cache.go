package impl

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/aserto-dev/go-authorizer/pkg/aerr"
	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/threehook/eamerald/internal/runtime"
	"github.com/threehook/eamerald/internal/tsync"
)

// preparedQueryCache memoizes rego.PreparedEvalQuery values keyed by the
// (policy path, decisions) tuple of an Is request.
//
// Why this exists: rego.New(...).PrepareForEval(ctx) parses the query and
// plans the topdown evaluation; for an Is() request the prepared query is a
// pure function of the policy path, decision names, the active OPA compiler
// (rebuilt on bundle reload), and the storage instance. Re-preparing per call
// burns CPU and serializes goroutines on the OPA compiler's internal
// structures — measurable under concurrent load.
//
// Invalidation: when the OPA plugin manager rebuilds the compiler (bundle
// reload / discovery update), we evict the entire cache. The watcher is
// registered lazily on first access so we don't need to plumb runtime
// lifecycle into the constructor.
//
// Concurrency: sync.Map for the read-mostly path; singleflight collapses
// concurrent misses for the same key into one PrepareForEval call so a
// thundering herd on first use doesn't multiply work.
//
// The loaded policy packages are cached here too. They are derived from the
// same compiler state - reading them parses every module in the store, which
// is far too much to redo per decision - so they are invalidated by the same
// trigger.
type preparedQueryCache struct {
	entries     tsync.Map[string, *rego.PreparedEvalQuery] // key (string) -> *rego.PreparedEvalQuery
	prepGroup   tsync.Group[string, *rego.PreparedEvalQuery]
	packages    atomic.Pointer[[]string]              // package paths of the loaded bundle
	watcherOnce tsync.Map[*plugins.Manager, struct{}] // key (*plugins.Manager) -> struct{} (one-time RegisterCompilerTrigger per runtime)
}

func newPreparedQueryCache() *preparedQueryCache {
	return &preparedQueryCache{}
}

// policyPackages returns the package paths of the loaded bundle, reading
// them from the store only after the compiler has been rotated.
func (c *preparedQueryCache) policyPackages(ctx context.Context, rt *runtime.Runtime) ([]string, error) {
	c.ensureCompilerWatcher(rt)

	if cached := c.packages.Load(); cached != nil {
		return *cached, nil
	}

	packages, err := rt.GetPolicyPackages(ctx)
	if err != nil {
		return nil, err
	}

	c.packages.Store(&packages)

	return packages, nil
}

// bindingName is the Rego variable the i-th decision rule is bound to. The
// query builder and everything reading results back out go through it, so
// the two cannot drift apart.
func bindingName(i int) string {
	return fmt.Sprintf("x%d", i)
}

// cacheKey returns a stable string for the (path, decisions) tuple.
// The decisions list ordering is preserved because the prepared query
// references them positionally as x0, x1, ... — reordering produces a
// semantically different query.
func cacheKey(path string, decisions []string) string {
	var b strings.Builder

	b.Grow(len(path) + 1 + len(decisions)*8)
	b.WriteString(path)
	b.WriteByte('\x1f') // unit separator: cannot appear in path or decision names

	for i, d := range decisions {
		if i > 0 {
			b.WriteByte('\x1e') // record separator
		}

		b.WriteString(d)
	}

	return b.String()
}

// getOrPrepare returns a PreparedEvalQuery for (path, decisions) against the
// given runtime. parsedQuery must be the already-validated AST body for the
// joined query string; the function does not re-parse.
//
// preparedQueryFactory builds the rego.New(...) options when called — only
// invoked on cache miss. This keeps the singleflight key path allocation-
// free for the hot read.
func (c *preparedQueryCache) getOrPrepare(
	ctx context.Context,
	rt *runtime.Runtime,
	key string,
	preparedQueryFactory func(ctx context.Context) (rego.PreparedEvalQuery, error),
) (rego.PreparedEvalQuery, error) {
	c.ensureCompilerWatcher(rt)

	if v, ok := c.entries.Load(key); ok {
		return *v, nil
	}

	// Collapse concurrent misses on the same key.
	v, err, _ := c.prepGroup.Do(key, func() (*rego.PreparedEvalQuery, error) {
		// Re-check under the singleflight: a concurrent call may have
		// just populated the entry while we were waiting to enter Do.
		if entry, ok := c.entries.Load(key); ok {
			return entry, nil
		}

		pq, err := preparedQueryFactory(context.WithoutCancel(ctx))
		if err != nil {
			return nil, err
		}

		stored := &pq
		c.entries.Store(key, stored)

		return stored, nil
	})
	if err != nil {
		return rego.PreparedEvalQuery{}, err
	}

	return *v, nil
}

// decisionQuery returns the prepared query that binds each decision rule
// under data.<path> to x0, x1, ... in request order.
//
// The query body and its prepared form depend only on the policy path and
// the decisions list — both stable for the lifetime of the active OPA
// compiler — so repeated calls for the same tuple skip the parse + plan work
// and stop fighting each other on the compiler's internal locks. The cache
// is invalidated whenever the compiler is rotated (bundle reload).
//
// Is() and the AuthZEN Evaluation API share this shape: a single-action
// evaluation is the one-decision case, and both therefore share cache entries.
func (c *preparedQueryCache) decisionQuery(
	ctx context.Context, rt *runtime.Runtime, path string, decisions []string,
) (rego.PreparedEvalQuery, error) {
	return c.getOrPrepare(ctx, rt, cacheKey(path, decisions), func(ctx context.Context) (rego.PreparedEvalQuery, error) {
		queryStmt := strings.Builder{}

		for i, decision := range decisions {
			rule := fmt.Sprintf("data.%s.%s\n", path, decision)

			if ok, err := rt.ValidateRule(rule); !ok {
				return rego.PreparedEvalQuery{}, aerr.ErrBadQuery.Err(err).Msgf("invalid rule: %q", rule)
			}

			q := fmt.Sprintf("%s = %s\n", bindingName(i), rule)

			if _, err := rt.ValidateQuery(q); err != nil {
				return rego.PreparedEvalQuery{}, aerr.ErrBadQuery.Err(err).Msgf("invalid query: %q", q)
			}

			queryStmt.WriteString(q)
		}

		pq, err := rt.ValidateQuery(queryStmt.String())
		if err != nil {
			return rego.PreparedEvalQuery{}, aerr.ErrBadQuery.Err(err).Msgf("invalid query batch: %q", queryStmt.String())
		}

		prepared, err := rego.New(
			rego.Compiler(rt.GetPluginsManager().GetCompiler()),
			rego.Store(rt.GetPluginsManager().Store),
			rego.ParsedQuery(pq),
		).PrepareForEval(ctx)
		if err != nil {
			return rego.PreparedEvalQuery{}, aerr.ErrBadQuery.Err(err).Msg(queryStmt.String())
		}

		return prepared, nil
	})
}

// ensureCompilerWatcher registers (exactly once per runtime) a callback on
// the runtime's plugins manager that drops the entire cache whenever the
// OPA compiler is replaced. Compiler replacement happens on bundle activation,
// discovery updates, or any other policy mutation; the prepared queries
// reference compiler state that becomes invalid afterward.
func (c *preparedQueryCache) ensureCompilerWatcher(rt *runtime.Runtime) {
	if rt == nil {
		return
	}

	pm := rt.GetPluginsManager()
	if pm == nil {
		return
	}

	if _, alreadyRegistered := c.watcherOnce.LoadOrStore(pm, struct{}{}); alreadyRegistered {
		return
	}

	// Discard everything when the compiler rotates. We don't try to be
	// precise — bundle reloads are rare relative to Is() rate.
	pm.RegisterCompilerTrigger(func(_ storage.Transaction) {
		c.packages.Store(nil)

		c.entries.Range(func(k string, _ *rego.PreparedEvalQuery) bool {
			c.entries.Clear()
			return true
		})
	})
}
