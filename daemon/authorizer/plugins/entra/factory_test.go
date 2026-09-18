//nolint:testpackage
package entra

import (
	"context"
	"testing"

	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/threehook/eamerald/daemon/authorizer/plugins/noop"
)

func TestFactoryValidate(t *testing.T) {
	factory := NewPluginFactory(context.Background(), &zerolog.Logger{}, nil)
	mgr, err := plugins.New([]byte("{}"), "test", inmem.New())
	require.NoError(t, err)

	t.Run("valid config", func(t *testing.T) {
		raw := []byte(`{"enabled": true, "tenant_id": "t1", "client_id": "c1", "client_secret": "s1", "poll_interval_seconds": 120}`)

		parsed, err := factory.Validate(mgr, raw)
		require.NoError(t, err)

		cfg, ok := parsed.(*Config)
		require.True(t, ok)
		require.True(t, cfg.Enabled)
		require.Equal(t, "t1", cfg.TenantID)
		require.Equal(t, "c1", cfg.ClientID)
		require.Equal(t, "s1", cfg.ClientSecret)
		require.Equal(t, 120, cfg.PollIntervalSeconds)
	})

	t.Run("malformed json", func(t *testing.T) {
		_, err := factory.Validate(mgr, []byte(`not json`))
		require.Error(t, err)
	})

	t.Run("unknown field", func(t *testing.T) {
		_, err := factory.Validate(mgr, []byte(`{"enabled": true, "not_a_real_field": "x"}`))
		require.Error(t, err)
	})
}

func TestFactoryNew(t *testing.T) {
	factory := NewPluginFactory(context.Background(), &zerolog.Logger{}, nil)
	mgr, err := plugins.New([]byte("{}"), "test", inmem.New())
	require.NoError(t, err)

	t.Run("disabled config returns a noop plugin", func(t *testing.T) {
		p := factory.New(mgr, &Config{Enabled: false})

		_, ok := p.(*noop.Noop)
		require.True(t, ok)
	})

	t.Run("enabled config returns a real plugin", func(t *testing.T) {
		p := factory.New(mgr, &Config{Enabled: true, TenantID: "t1", ClientID: "c1", ClientSecret: "s1"})

		_, ok := p.(*Plugin)
		require.True(t, ok)
	})

	t.Run("panics on a malformed config", func(t *testing.T) {
		require.Panics(t, func() {
			factory.New(mgr, "not a *Config")
		})
	})
}
