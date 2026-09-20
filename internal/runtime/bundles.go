package runtime

import (
	"context"
	"encoding/base64"
	"hash/adler32"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/server/types"
	"github.com/open-policy-agent/opa/v1/storage"
	opaTopdown "github.com/open-policy-agent/opa/v1/topdown"
	"github.com/pkg/errors"
)

type PolicyItem struct {
	Name string
	ID   string
}

type Bundle struct {
	ID   string
	Name string
	Path string
}

type Module struct {
	ID      string
	Name    string
	Content string
	Rules   []string
}

func (r *Runtime) GetBundles(ctx context.Context) ([]*PolicyItem, error) {
	if r.opaInstance == nil {
		return []*PolicyItem{}, errors.Errorf("OPA instance not initialized")
	}

	if r.pluginsManager == nil {
		return []*PolicyItem{}, errors.Errorf("plugin manager not initialized")
	}

	results := make([]*PolicyItem, 0)

	bundles, err := getBundles(ctx, r)
	if err != nil {
		if strings.Contains(err.Error(), opaTopdown.CancelErr) {
			return results, nil
		}

		return results, errors.Wrapf(err, "get bundles")
	}

	for _, b := range bundles {
		results = append(results, &PolicyItem{
			ID:   b.ID,
			Name: b.Name,
		})
	}

	return results, nil
}

func getBundles(ctx context.Context, r *Runtime) ([]*Bundle, error) {
	const queryStmt = "data.system.bundles[x]"

	queryResults, err := r.Query(ctx, queryStmt, nil, false, false, false, types.ExplainOffV1)
	if err != nil {
		return []*Bundle{}, errors.Wrapf(err, "query bundles")
	}

	results := make([]*Bundle, 0)

	for _, rs := range queryResults.Result {
		v, ok := rs.Bindings["x"].(string)
		if !ok {
			r.Logger.Error().Msg("expected binding [x] not found")
			continue
		}

		path := strings.TrimPrefix(v, "./")

		id := calcID(v)

		name, err := r.GetPolicyRootForPath(ctx, path)
		if err != nil {
			return []*Bundle{}, errors.Wrapf(err, "get policy name")
		}

		results = append(results, &Bundle{
			ID:   id,
			Name: name,
			Path: path,
		})
	}

	return results, nil
}

func (r *Runtime) GetBundleByID(ctx context.Context, id string) (*Bundle, error) {
	bundles, err := getBundles(ctx, r)
	if err != nil {
		return &Bundle{}, err
	}

	for _, v := range bundles {
		if v.ID == id {
			return v, nil
		}
	}

	return &Bundle{}, errors.Errorf("bundle for policy id not found [%s]", id)
}

func calcID(v string) string {
	if _, err := uuid.Parse(v); err == nil {
		return v
	}

	return strconv.FormatUint(uint64(adler32.Checksum([]byte(v))), 10)
}

func (r *Runtime) GetPolicies(ctx context.Context, id string) ([]*PolicyItem, error) {
	policies := make([]*PolicyItem, 0)

	policyList, err := r.GetPolicyList(ctx, id, noFilter)
	if err != nil {
		return policies, err
	}

	for _, policy := range policyList {
		policies = append(
			policies,
			&PolicyItem{
				Name: policy.PackageName,
				ID:   encID(policy.Location),
			},
		)
	}

	// sort policies by their name.
	sort.Slice(policies, func(i, j int) bool {
		return policies[i].Name < policies[j].Name
	})

	return policies, nil
}

type Policy struct {
	PackageName string
	Location    string
}

func (p Policy) Name() string {
	s := strings.Split(p.PackageName, ".")
	if len(s) >= 1 {
		return s[0]
	}

	return ""
}

type PathFilterFn func(packageName string) bool

var noFilter PathFilterFn = func(packageName string) bool { return true }

// GetPolicyList returns the list of policies loaded by the runtime for a given bundle, identified with the policy id.
func (r *Runtime) GetPolicyList(ctx context.Context, id string, fn PathFilterFn) ([]Policy, error) {
	policyList := make([]Policy, 0)

	if fn == nil {
		return policyList, errors.Errorf("path filter is nil")
	}

	err := storage.Txn(ctx, r.pluginsManager.Store, storage.TransactionParams{}, func(txn storage.Transaction) error {
		policiesList, errX := r.pluginsManager.Store.ListPolicies(ctx, txn)
		if errX != nil {
			return errors.Wrap(errX, "error listing policies from storage")
		}

		for _, v := range policiesList {
			buf, errX := r.pluginsManager.Store.GetPolicy(ctx, txn, v)
			if errX != nil {
				return errors.Wrap(errX, "store.GetPolicy")
			}

			module, errY := ast.ParseModuleWithOpts("", string(buf), ast.ParserOptions{RegoVersion: r.regoVersion})
			if errY != nil {
				return errors.Wrap(errY, "ast.ParseModule")
			}

			packageName := strings.TrimPrefix(module.Package.Path.String(), "data.")

			// filter out entries which do prefix the path specified
			if fn != nil && !fn(packageName) {
				continue
			}

			policyList = append(
				policyList,
				Policy{
					PackageName: packageName,
					Location:    v,
				},
			)
		}

		return nil
	})
	if err != nil {
		return []Policy{}, err
	}

	return policyList, nil
}

// GetPolicyPackages returns every distinct package path in the policy list,
// sorted, so that the result does not depend on the store's iteration order.
func (r *Runtime) GetPolicyPackages(ctx context.Context) ([]string, error) {
	packages := []string{}

	err := storage.Txn(ctx, r.pluginsManager.Store, storage.TransactionParams{}, func(txn storage.Transaction) error {
		policiesList, err := r.pluginsManager.Store.ListPolicies(ctx, txn)
		if err != nil {
			return errors.Wrap(err, "error listing policies from storage")
		}

		for _, id := range policiesList {
			pkg, err := r.getPackageFromPolicyID(ctx, id, txn)
			if err != nil {
				return err
			}

			if pkg != "" && !slices.Contains(packages, pkg) {
				packages = append(packages, pkg)
			}
		}

		slices.Sort(packages)

		return nil
	})

	return packages, err
}

// GetPolicyRoots returns every distinct package root in the policy list,
// sorted, so that the result does not depend on the store's iteration order.
func (r *Runtime) GetPolicyRoots(ctx context.Context) ([]string, error) {
	packages, err := r.GetPolicyPackages(ctx)
	if err != nil {
		return nil, err
	}

	return PolicyRoots(packages), nil
}

// PolicyRoots reduces package paths to their distinct roots, keeping the
// sorted order of the input.
func PolicyRoots(packages []string) []string {
	roots := []string{}

	for _, pkg := range packages {
		root, _, _ := strings.Cut(pkg, ".")

		if root != "" && !slices.Contains(roots, root) {
			roots = append(roots, root)
		}
	}

	return roots
}

// PDPPolicyRoot returns the policy the AuthZEN Access API falls back to when
// a request selects none of its own.
func (r *Runtime) PDPPolicyRoot(ctx context.Context) (string, error) {
	roots, err := r.GetPolicyRoots(ctx)
	if err != nil {
		return "", err
	}

	return r.SelectPolicyRoot(roots)
}

// SelectPolicyRoot picks, out of the loaded package roots, the one this
// instance serves when a request selects no policy of its own.
//
// Picking a root implicitly when several are loaded would make the decision
// depend on the store's iteration order, so that case is an error the
// operator resolves by setting opa.policy_root.
func (r *Runtime) SelectPolicyRoot(roots []string) (string, error) {
	configured := r.Config.PolicyRoot

	switch {
	case len(roots) == 0:
		return "", errors.New("no policy loaded")

	case configured != "":
		if !slices.Contains(roots, configured) {
			return "", errors.Errorf("opa.policy_root %q is not loaded; loaded policies are [%s]",
				configured, strings.Join(roots, ", "))
		}

		return configured, nil

	case len(roots) == 1:
		return roots[0], nil

	default:
		return "", errors.Errorf(
			"%d policies are loaded [%s] and the request selected none:"+
				" set opa.policy_root, or select one per request",
			len(roots), strings.Join(roots, ", "))
	}
}

// GetPolicyRootForPath returns the package root name from the policy list (not from the .manifest file) based on the given path.
func (r *Runtime) GetPolicyRootForPath(ctx context.Context, path string) (string, error) {
	var policyName string

	err := storage.Txn(ctx, r.pluginsManager.Store, storage.TransactionParams{}, func(txn storage.Transaction) error {
		policiesList, errX := r.pluginsManager.Store.ListPolicies(ctx, txn)
		if errX != nil {
			return errors.Wrap(errX, "error listing policies from storage")
		}

		for _, v := range policiesList {
			// filter out entries which do not belong to policy.
			trimmedPath := strings.TrimPrefix(v, "/")

			trimmedRequestPath := strings.TrimPrefix(path, "/")
			if !strings.HasPrefix(trimmedPath, trimmedRequestPath) {
				continue
			}

			policyRoot, err := r.getRootFromPolicyID(ctx, v, txn)
			if err != nil {
				return err
			}

			if policyRoot != "" {
				policyName = policyRoot
				break
			}
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	return policyName, nil
}

func (r *Runtime) getRootFromPolicyID(ctx context.Context, policyID string, txn storage.Transaction) (string, error) {
	packageName, err := r.getPackageFromPolicyID(ctx, policyID, txn)
	if err != nil {
		return "", err
	}

	root, _, _ := strings.Cut(packageName, ".")

	return root, nil
}

func (r *Runtime) getPackageFromPolicyID(ctx context.Context, policyID string, txn storage.Transaction) (string, error) {
	buf, err := r.pluginsManager.Store.GetPolicy(ctx, txn, policyID)
	if err != nil {
		return "", errors.Wrap(err, "store.GetPolicy")
	}

	module, err := ast.ParseModuleWithOpts("", string(buf), ast.ParserOptions{RegoVersion: r.regoVersion})
	if err != nil {
		return "", errors.Wrap(err, "ast.ParseModule")
	}

	return strings.TrimPrefix(module.Package.Path.String(), "data."), nil
}

func policyExists(ctx context.Context, r *Runtime, id string) bool {
	err := storage.Txn(ctx, r.pluginsManager.Store, storage.TransactionParams{}, func(txn storage.Transaction) error {
		_, err := r.pluginsManager.Store.GetPolicy(ctx, txn, id)
		return err
	})

	return err == nil
}

func (r *Runtime) GetModule(ctx context.Context, id string) (*Module, error) {
	pid := decID(id)

	if !policyExists(ctx, r, pid) {
		return &Module{}, errors.Errorf("policy not found [%s]", pid)
	}

	module, err := getModule(ctx, r, pid)
	if err != nil {
		return &Module{}, err
	}

	return module, nil
}

func getModule(ctx context.Context, r *Runtime, id string) (*Module, error) {
	mod := &Module{}

	err := storage.Txn(ctx, r.pluginsManager.Store, storage.TransactionParams{}, func(txn storage.Transaction) error {
		policy, err := r.pluginsManager.Store.GetPolicy(ctx, txn, id)
		if err != nil {
			return errors.Wrap(err, "failed to get policy")
		}

		module, err := ast.ParseModuleWithOpts("", string(policy), ast.ParserOptions{RegoVersion: r.regoVersion})
		if err != nil {
			return errors.Wrap(err, "parse module")
		}

		name := strings.TrimPrefix(module.Package.Path.String(), "data.")

		rules := []string{}
		for _, rule := range module.Rules {
			rules = append(rules, rule.Head.Name.String())
		}

		mod.ID = encID(id)
		mod.Name = name
		mod.Content = string(policy)
		mod.Rules = rules

		return nil
	})

	return mod, err
}

// decID decode policy ID (base64 -> string).
func decID(id string) string {
	b, err := base64.URLEncoding.DecodeString(id)
	if err != nil {
		return ""
	}

	return string(b)
}

// encID encode policy ID (string -> base64).
func encID(id string) string {
	return base64.URLEncoding.EncodeToString([]byte(id))
}
