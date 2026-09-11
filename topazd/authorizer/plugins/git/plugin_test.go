//nolint:testpackage
package git

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

const testRepoURL = "https://example.com/a.git"

// commitFile writes name=content in dir and commits it to repo, returning the new commit hash.
func commitFile(t *testing.T, repo *gogit.Repository, dir, name, content string) plumbing.Hash {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))

	wt, err := repo.Worktree()
	require.NoError(t, err)

	_, err = wt.Add(name)
	require.NoError(t, err)

	hash, err := wt.Commit("commit "+name+"="+content, &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com"},
	})
	require.NoError(t, err)

	return hash
}

func TestResolveRef(t *testing.T) {
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)

	oldHash := commitFile(t, repo, dir, "a.txt", "old")
	newHash := commitFile(t, repo, dir, "a.txt", "new")

	// Simulate the state fetchAndCheckout leaves behind: a stale local
	// refs/heads/main left over from the initial clone (never updated,
	// since fetch only ever writes refs/remotes/origin/* and refs/tags/*)
	// alongside an up-to-date refs/remotes/origin/main.
	require.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(defaultRef), oldHash)))
	require.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference("refs/remotes/origin/main", newHash)))

	tagHash := commitFile(t, repo, dir, "a.txt", "tagged")
	require.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference("refs/tags/v1", tagHash)))

	t.Run("prefers the remote-tracking branch over the stale local branch", func(t *testing.T) {
		hash, err := resolveRef(repo, defaultRef)
		require.NoError(t, err)
		require.Equal(t, newHash, *hash)
	})

	t.Run("resolves a bare branch name the same way", func(t *testing.T) {
		hash, err := resolveRef(repo, "main")
		require.NoError(t, err)
		require.Equal(t, newHash, *hash)
	})

	t.Run("resolves tags", func(t *testing.T) {
		hash, err := resolveRef(repo, "refs/tags/v1")
		require.NoError(t, err)
		require.Equal(t, tagHash, *hash)
	})

	t.Run("falls back to a raw commit hash", func(t *testing.T) {
		hash, err := resolveRef(repo, tagHash.String())
		require.NoError(t, err)
		require.Equal(t, tagHash, *hash)
	})

	t.Run("errors on an unresolvable ref", func(t *testing.T) {
		_, err := resolveRef(repo, "refs/heads/does-not-exist")
		require.Error(t, err)
	})
}

func TestPollInterval(t *testing.T) {
	require.Equal(t, defaultPollIntervalSec, pollInterval(&Config{}))
	require.Equal(t, 30, pollInterval(&Config{PollIntervalSeconds: 30}))
	require.Equal(t, defaultPollIntervalSec, pollInterval(&Config{PollIntervalSeconds: -1}))
}

func TestRef(t *testing.T) {
	require.Equal(t, defaultRef, ref(&Config{}))
	require.Equal(t, "refs/heads/dev", ref(&Config{Ref: "refs/heads/dev"}))
}

func TestCacheDir(t *testing.T) {
	require.Equal(t, "/custom/path", cacheDir(&Config{CacheDir: "/custom/path"}))

	c1 := cacheDir(&Config{Repo: testRepoURL, Ref: defaultRef})
	c2 := cacheDir(&Config{Repo: testRepoURL, Ref: defaultRef})
	require.Equal(t, c1, c2, "cache dir should be deterministic for the same repo+ref")

	c3 := cacheDir(&Config{Repo: "https://example.com/b.git", Ref: defaultRef})
	require.NotEqual(t, c1, c3, "cache dir should differ for a different repo")
}

func TestAuthMethod(t *testing.T) {
	t.Run("no credentials configured", func(t *testing.T) {
		p := &Plugin{config: &Config{}}
		auth, err := p.authMethod()
		require.NoError(t, err)
		require.Nil(t, auth)
	})

	t.Run("token auth defaults username to git", func(t *testing.T) {
		p := &Plugin{config: &Config{Auth: GitAuthConfig{Token: "tok"}}}
		auth, err := p.authMethod()
		require.NoError(t, err)

		basicAuth, ok := auth.(*githttp.BasicAuth)
		require.True(t, ok)
		require.Equal(t, "git", basicAuth.Username)
		require.Equal(t, "tok", basicAuth.Password)
	})

	t.Run("token auth respects a configured username", func(t *testing.T) {
		p := &Plugin{config: &Config{Auth: GitAuthConfig{Username: "me", Token: "tok"}}}
		auth, err := p.authMethod()
		require.NoError(t, err)

		basicAuth, ok := auth.(*githttp.BasicAuth)
		require.True(t, ok)
		require.Equal(t, "me", basicAuth.Username)
	})

	t.Run("ssh key auth", func(t *testing.T) {
		keyPath := writeTestSSHKey(t)

		p := &Plugin{config: &Config{Auth: GitAuthConfig{SSHKeyPath: keyPath}}}
		auth, err := p.authMethod()
		require.NoError(t, err)

		publicKeys, ok := auth.(*gitssh.PublicKeys)
		require.True(t, ok)
		require.Nil(t, publicKeys.HostKeyCallback, "no known_hosts_path configured, so no callback should be wired up")
	})

	t.Run("ssh key auth with known_hosts_path", func(t *testing.T) {
		keyPath := writeTestSSHKey(t)
		knownHostsPath := writeTestKnownHosts(t, "example.com")

		p := &Plugin{config: &Config{Auth: GitAuthConfig{SSHKeyPath: keyPath, KnownHostsPath: knownHostsPath}}}
		auth, err := p.authMethod()
		require.NoError(t, err)

		publicKeys, ok := auth.(*gitssh.PublicKeys)
		require.True(t, ok)
		require.NotNil(t, publicKeys.HostKeyCallback, "known_hosts_path should wire up a host key callback")
	})

	t.Run("ssh key auth with an unreadable known_hosts_path errors", func(t *testing.T) {
		keyPath := writeTestSSHKey(t)

		p := &Plugin{config: &Config{Auth: GitAuthConfig{
			SSHKeyPath:     keyPath,
			KnownHostsPath: filepath.Join(t.TempDir(), "does-not-exist"),
		}}}
		_, err := p.authMethod()
		require.Error(t, err)
	})
}

// writeTestKnownHosts generates a throwaway ed25519 host key and writes a
// known_hosts file containing a single entry for host, for use with
// gitssh.NewKnownHostsCallback.
func writeTestKnownHosts(t *testing.T, host string) string {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	signer, err := ssh.NewSignerFromKey(priv)
	require.NoError(t, err)

	line := host + " " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))

	path := filepath.Join(t.TempDir(), "known_hosts")
	require.NoError(t, os.WriteFile(path, []byte(line+"\n"), 0o600))

	return path
}

// writeTestSSHKey generates a throwaway ed25519 private key and writes it,
// PEM-encoded, to a temp file for use with gitssh.NewPublicKeysFromFile.
func writeTestSSHKey(t *testing.T) string {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	block, err := ssh.MarshalPrivateKey(priv, "")
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(block), 0o600))

	return path
}

func TestSync(t *testing.T) {
	remoteDir := t.TempDir()
	repo, err := gogit.PlainInit(remoteDir, false)
	require.NoError(t, err)

	firstHash := commitFile(t, repo, remoteDir, "policy.rego", "package test\n\ndefault allow = false\n")

	head, err := repo.Head()
	require.NoError(t, err)

	logger := zerolog.Nop()
	mgr, err := plugins.New([]byte("{}"), "test", inmem.New())
	require.NoError(t, err)

	cfg := &Config{
		Repo:             remoteDir,
		Ref:              head.Name().String(),
		CacheDir:         t.TempDir(),
		SkipVerification: true,
	}

	p := newGitPlugin(&logger, cfg, mgr)

	require.NoError(t, p.sync(t.Context()))
	require.Equal(t, firstHash.String(), p.lastHash)

	t.Run("re-syncing with no new commits is a no-op", func(t *testing.T) {
		require.NoError(t, p.sync(t.Context()))
		require.Equal(t, firstHash.String(), p.lastHash)
	})

	t.Run("a new commit on the tracked ref is picked up", func(t *testing.T) {
		secondHash := commitFile(t, repo, remoteDir, "policy.rego", "package test\n\ndefault allow = true\n")

		require.NoError(t, p.sync(t.Context()))
		require.Equal(t, secondHash.String(), p.lastHash)
		require.NotEqual(t, firstHash.String(), p.lastHash)
	})
}

func TestReconfigure(t *testing.T) {
	logger := zerolog.Nop()
	mgr, err := plugins.New([]byte("{}"), "test", inmem.New())
	require.NoError(t, err)

	p := newGitPlugin(&logger, &Config{Repo: testRepoURL}, mgr)
	originalCacheDir := p.cacheDir

	newCfg := &Config{Repo: "https://example.com/b.git"}
	p.Reconfigure(context.Background(), newCfg)

	require.Same(t, newCfg, p.config)
	require.NotEqual(t, originalCacheDir, p.cacheDir, "cacheDir should be recomputed for the new repo")

	t.Run("ignores a malformed config", func(t *testing.T) {
		p.Reconfigure(context.Background(), "not a *Config")
		require.Same(t, newCfg, p.config, "config should be left untouched")
	})
}
