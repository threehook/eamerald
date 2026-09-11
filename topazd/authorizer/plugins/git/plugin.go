package git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/bundle"
	"github.com/open-policy-agent/opa/v1/loader"
	"github.com/open-policy-agent/opa/v1/metrics"
	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

const (
	PluginName             string = "git"
	bundleName             string = "git"
	defaultRef             string = "refs/heads/main"
	defaultPollIntervalSec int    = 60
)

type GitAuthConfig struct {
	Username         string `json:"username"`           // basic-auth / PAT username for HTTPS remotes.
	Token            string `json:"token"`              // PAT / password for HTTPS remotes.
	SSHKeyPath       string `json:"ssh_key_path"`       // path to a private key file, for SSH remotes.
	SSHKeyPassphrase string `json:"ssh_key_passphrase"` // passphrase for the private key, if any.
	KnownHostsPath   string `json:"known_hosts_path"`   // optional known_hosts file used to verify the SSH host key.
}

type Config struct {
	Enabled bool `json:"enabled"`
	// Repo is the git remote URL (https:// or ssh://).
	Repo string `json:"repo"`
	// Ref is the git reference to track, e.g. "refs/heads/main" or "refs/tags/v1.0.0".
	Ref string `json:"ref"`
	// Path is the subdirectory within the repo containing the policy bundle; "" means repo root.
	Path string `json:"path"`
	// CacheDir is the local directory used to clone/checkout the repo; auto-derived when empty.
	CacheDir              string                     `json:"cache_dir"`
	PollIntervalSeconds   int                        `json:"poll_interval_seconds"`
	InsecureSkipTLSVerify bool                       `json:"insecure_skip_tls_verify"`
	Auth                  GitAuthConfig              `json:"auth"`
	SkipVerification      bool                       `json:"skip_verification"`
	VerificationConfig    *bundle.VerificationConfig `json:"verification_config"`
}

type Plugin struct {
	ctx      context.Context
	cancel   context.CancelFunc
	manager  *plugins.Manager
	logger   *zerolog.Logger
	config   *Config
	cacheDir string
	lastHash string
}

func newGitPlugin(logger *zerolog.Logger, cfg *Config, manager *plugins.Manager) *Plugin {
	newLogger := logger.With().Str("component", "git.plugin").Logger()

	syncContext, cancel := context.WithCancel(context.Background())

	return &Plugin{
		ctx:      syncContext,
		cancel:   cancel,
		logger:   &newLogger,
		manager:  manager,
		config:   cfg,
		cacheDir: cacheDir(cfg),
	}
}

// Start syncs synchronously, so the policy bundle is loaded before the
// authorizer starts serving, and fails the boot when it cannot be loaded.
// Reporting StateErr instead would hang startup permanently: the OPA runtime
// only becomes ready once every registered plugin reports StateOK, topaz's
// own services only start serving after the runtime is ready, and nothing
// retries the sync before that gate opens.
func (p *Plugin) Start(ctx context.Context) error {
	p.logger.Info().Str("id", p.manager.ID).Str("repo", p.config.Repo).Str("ref", p.config.Ref).Msg("GitPlugin.Start")

	if err := p.sync(ctx); err != nil {
		return errors.Wrap(err, "initial git sync failed")
	}

	p.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateOK})

	go p.scheduler()

	return nil
}

func (p *Plugin) Stop(ctx context.Context) {
	p.logger.Info().Str("id", p.manager.ID).Msg("GitPlugin.Stop")

	p.cancel()
	p.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateNotReady})
}

func (p *Plugin) Reconfigure(ctx context.Context, config any) {
	newConfig, ok := config.(*Config)
	if !ok {
		p.logger.Error().Str("config", "failed type assertion").Msg("GitPlugin.Reconfigure")
		return
	}

	p.logger.Trace().Str("id", p.manager.ID).Interface("cur", p.config).Interface("new", newConfig).Msg("GitPlugin.Reconfigure")

	p.config = newConfig
	p.cacheDir = cacheDir(newConfig)
}

func (p *Plugin) scheduler() {
	current := pollInterval(p.config)
	ticker := time.NewTicker(time.Duration(current) * time.Second)

	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			if next := pollInterval(p.config); next != current {
				current = next
				ticker.Reset(time.Duration(current) * time.Second)
			}

			if err := p.sync(p.ctx); err != nil {
				p.logger.Error().Err(err).Msg("git sync failed")
				p.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateErr, Message: err.Error()})

				continue
			}

			p.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateOK})
		}
	}
}

func (p *Plugin) sync(ctx context.Context) error {
	repo, err := p.openOrClone(ctx)
	if err != nil {
		return errors.Wrap(err, "open or clone git repo")
	}

	hash, err := p.fetchAndCheckout(ctx, repo)
	if err != nil {
		return errors.Wrap(err, "fetch and checkout git ref")
	}

	if hash == p.lastHash {
		p.logger.Debug().Str("hash", hash).Msg("git ref unchanged, skipping bundle activation")
		return nil
	}

	bundlePath := p.cacheDir
	if p.config.Path != "" {
		bundlePath = filepath.Join(p.cacheDir, p.config.Path)
	}

	b, err := loader.NewFileLoader().
		WithBundleVerificationConfig(p.config.VerificationConfig).
		WithSkipBundleVerification(p.config.SkipVerification).
		AsBundle(bundlePath)
	if err != nil {
		return errors.Wrapf(err, "build bundle from git checkout at '%s'", bundlePath)
	}

	if err := p.activate(ctx, b); err != nil {
		return errors.Wrap(err, "activate bundle")
	}

	p.logger.Info().Str("hash", hash).Str("path", bundlePath).Msg("activated bundle from git")

	p.lastHash = hash

	return nil
}

func (p *Plugin) openOrClone(ctx context.Context) (*gogit.Repository, error) {
	if _, err := os.Stat(filepath.Join(p.cacheDir, ".git")); err == nil {
		return gogit.PlainOpen(p.cacheDir)
	}

	auth, err := p.authMethod()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(p.cacheDir, 0o750); err != nil { //nolint:mnd
		return nil, errors.Wrap(err, "create cache dir")
	}

	return gogit.PlainCloneContext(ctx, p.cacheDir, false, &gogit.CloneOptions{
		URL:             p.config.Repo,
		Auth:            auth,
		NoCheckout:      true,
		Tags:            gogit.AllTags,
		InsecureSkipTLS: p.config.InsecureSkipTLSVerify,
	})
}

func (p *Plugin) fetchAndCheckout(ctx context.Context, repo *gogit.Repository) (string, error) {
	auth, err := p.authMethod()
	if err != nil {
		return "", err
	}

	err = repo.FetchContext(ctx, &gogit.FetchOptions{
		RemoteName: "origin",
		Auth:       auth,
		RefSpecs: []gitconfig.RefSpec{
			"+refs/heads/*:refs/remotes/origin/*",
			"+refs/tags/*:refs/tags/*",
		},
		Force:           true,
		Tags:            gogit.AllTags,
		InsecureSkipTLS: p.config.InsecureSkipTLSVerify,
	})
	if err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return "", errors.Wrap(err, "git fetch")
	}

	hash, err := resolveRef(repo, ref(p.config))
	if err != nil {
		return "", err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}

	if err := wt.Checkout(&gogit.CheckoutOptions{Hash: *hash, Force: true}); err != nil {
		return "", errors.Wrap(err, "git checkout")
	}

	return hash.String(), nil
}

// resolveRef resolves ref against the state fetched into refs/remotes/origin/*
// and refs/tags/* by fetchAndCheckout. Branch names are tried against the
// remote-tracking namespace first (and never against a local refs/heads/*
// ref, which we never write to and which would otherwise resolve to a stale
// commit left over from the initial clone).
func resolveRef(repo *gogit.Repository, ref string) (*plumbing.Hash, error) {
	name := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/tags/")

	candidates := []string{"refs/remotes/origin/" + name, "refs/tags/" + name, ref}

	var lastErr error

	for _, candidate := range candidates {
		hash, err := repo.ResolveRevision(plumbing.Revision(candidate))
		if err == nil {
			return hash, nil
		}

		lastErr = err
	}

	return nil, errors.Wrapf(lastErr, "resolve git ref '%s'", ref)
}

func (p *Plugin) activate(ctx context.Context, b *bundle.Bundle) error {
	params := storage.WriteParams
	params.Context = storage.NewContext()

	return storage.Txn(ctx, p.manager.Store, params, func(txn storage.Transaction) error {
		compiler := ast.NewCompiler().
			WithPathConflictsCheck(storage.NonEmpty(ctx, p.manager.Store, txn)).
			WithEnablePrintStatements(p.manager.EnablePrintStatements())

		if b.Manifest.Roots != nil {
			compiler = compiler.WithPathConflictsCheckRoots(*b.Manifest.Roots)
		}

		opts := &bundle.ActivateOpts{
			Ctx:             ctx,
			Store:           p.manager.Store,
			Txn:             txn,
			TxnCtx:          params.Context,
			Compiler:        compiler,
			Metrics:         metrics.New(),
			Bundles:         map[string]*bundle.Bundle{bundleName: b},
			ExternalSources: p.manager.GetExternalSources(),
			ParserOptions:   p.manager.ParserOptions(),
		}

		if err := bundle.Activate(opts); err != nil {
			return err
		}

		plugins.SetCompilerOnContext(params.Context, compiler)

		return nil
	})
}

// authMethod returns the transport.AuthMethod go-git's CloneOptions/FetchOptions expect.
func (p *Plugin) authMethod() (transport.AuthMethod, error) { //nolint:ireturn
	auth := p.config.Auth

	switch {
	case auth.SSHKeyPath != "":
		method, err := gitssh.NewPublicKeysFromFile("git", auth.SSHKeyPath, auth.SSHKeyPassphrase)
		if err != nil {
			return nil, errors.Wrap(err, "load ssh private key")
		}

		if auth.KnownHostsPath != "" {
			cb, err := gitssh.NewKnownHostsCallback(auth.KnownHostsPath)
			if err != nil {
				return nil, errors.Wrap(err, "load known_hosts file")
			}

			method.HostKeyCallback = cb
		}

		return method, nil

	case auth.Token != "":
		username := auth.Username
		if username == "" {
			username = "git"
		}

		return &githttp.BasicAuth{Username: username, Password: auth.Token}, nil

	default:
		return nil, nil //nolint:nilnil // no credentials configured; anonymous/public access.
	}
}

func ref(cfg *Config) string {
	if cfg.Ref == "" {
		return defaultRef
	}

	return cfg.Ref
}

func pollInterval(cfg *Config) int {
	if cfg.PollIntervalSeconds <= 0 {
		return defaultPollIntervalSec
	}

	return cfg.PollIntervalSeconds
}

func cacheDir(cfg *Config) string {
	if cfg.CacheDir != "" {
		return cfg.CacheDir
	}

	sum := sha256.Sum256([]byte(cfg.Repo + "|" + ref(cfg)))

	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}

	return filepath.Join(home, ".policy", "git", hex.EncodeToString(sum[:])[:16])
}
