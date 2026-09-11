package runtime_test

import (
	"context"
	"os"
	"testing"
	"time"

	runtime "github.com/aserto-dev/topaz/internal/runtime"
	"github.com/aserto-dev/topaz/internal/runtime/testutil"
	"github.com/stretchr/testify/require"
)

const defaultTestContextTimeout = 60 * time.Second

func testContextTimeout(t *testing.T) time.Duration {
	t.Helper()

	str := os.Getenv("TEST_CONTEXT_TIMEOUT")
	if str == "" {
		return defaultTestContextTimeout
	}

	t.Logf("env TEST_CONTEXT_TIME=%s", str)

	parsed, err := time.ParseDuration(str)
	if err != nil {
		t.Logf("parsing TEST_CONTEXT_TIME failed %q", err.Error())
		return defaultTestContextTimeout
	}

	return parsed
}

func TestEmptyRuntime(t *testing.T) {
	assert := require.New(t)

	ctx, cancel := context.WithTimeout(t.Context(), testContextTimeout(t))
	t.Cleanup(cancel)

	r, err := runtime.New(ctx, &runtime.Config{})
	assert.NoError(err)
	assert.NotNil(r)

	assert.NoError(
		r.Start(ctx),
	)
	t.Cleanup(func() { r.Stop(ctx) })

	assert.NoError(
		r.CheckPluginsStatus(),
	)

	b, err := r.GetBundles(ctx)
	assert.NoError(err)
	assert.Empty(b)
}

func TestLocalBundle(t *testing.T) {
	assert := require.New(t)

	ctx, cancel := context.WithTimeout(t.Context(), testContextTimeout(t))
	t.Cleanup(cancel)

	r, err := runtime.New(ctx, &runtime.Config{
		LocalBundles: runtime.LocalBundlesConfig{
			Paths: []string{testutil.AssetSimpleBundle()},
		},
	})
	assert.NoError(err)
	assert.NotNil(r)

	assert.NoError(
		r.Start(ctx),
	)
	t.Cleanup(func() { r.Stop(ctx) })

	assert.NoError(
		r.CheckPluginsStatus(),
	)

	b, err := r.GetBundles(ctx)
	assert.NoError(err)
	assert.Len(b, 1)
}

func TestFailingLocalBundle(t *testing.T) {
	assert := require.New(t)

	ctx, cancel := context.WithTimeout(t.Context(), testContextTimeout(t))
	t.Cleanup(cancel)

	r, err := runtime.New(ctx, &runtime.Config{
		LocalBundles: runtime.LocalBundlesConfig{
			Paths: []string{testutil.AssetBuiltinsBundle()},
		},
	})
	assert.NoError(err)
	assert.NotNil(r)

	assert.Error(
		r.Start(ctx),
	)
	t.Cleanup(func() { r.Stop(ctx) })

	assert.Error(
		r.CheckPluginsStatus(),
	)

	b, err := r.GetBundles(ctx)
	assert.Error(err)
	assert.Empty(b)
}
