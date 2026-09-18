package template_no_tls_test

import (
	"context"
	"testing"
	"time"

	azc "github.com/threehook/eamerald/cli/clients/authorizer"
	dsc "github.com/threehook/eamerald/cli/clients/directory"
	"github.com/threehook/eamerald/cli/x"
	assets_test "github.com/threehook/eamerald/daemon/tests/assets"
	tc "github.com/threehook/eamerald/daemon/tests/common"
	"github.com/threehook/eamerald/internal/fs"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestTemplatesNoTLS(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	t.Logf("\nTEST CONTAINER IMAGE: %q\n", tc.TestImage())

	req := testcontainers.ContainerRequest{
		Image:        tc.TestImage(),
		ExposedPorts: []string{"9292/tcp"},
		Env: map[string]string{
			x.EnvEameraldCertsDir:     x.DefCertsDir,
			x.EnvEameraldDBDir:        x.DefDBDir,
			x.EnvEameraldDecisionsDir: x.DefDecisionsDir,
		},
		Files: []testcontainers.ContainerFile{
			{
				Reader:            assets_test.ConfigNoTLSReader(),
				ContainerFilePath: tc.TestConfigFilePath,
				FileMode:          int64(fs.FileModeOwnerRWX),
			},
		},
		WaitingFor: wait.ForAll(
			wait.ForExposedPort(),
			wait.ForLog("Starting 0.0.0.0:9292 gRPC server"),
		).WithStartupTimeoutDefault(tc.TestStartupTimeout),
	}

	topaz, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          false,
	})
	require.NoError(t, err)

	if err := topaz.Start(ctx); err != nil {
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		testcontainers.CleanupContainer(t, topaz)
		cancel()
	})

	addr, err := tc.MappedAddr(ctx, topaz, "9292")
	require.NoError(t, err)

	dsConfig := &dsc.Config{
		Host:      addr,
		Insecure:  false,
		Plaintext: true,
		Timeout:   10 * time.Second,
	}

	azConfig := &azc.Config{
		Host:      addr,
		Insecure:  false,
		Plaintext: true,
		Timeout:   10 * time.Second,
	}

	for _, tmpl := range tcs {
		t.Run("testTemplatesWithNoTLS", tc.InstallTemplate(ctx, dsConfig, azConfig, tmpl))
	}
}

var tcs = []string{
	"../../../assets/acmecorp.json",
	"../../../assets/peoplefinder.json",

	"../../../assets/citadel.json",
	"../../../assets/api-auth.json",
	"../../../assets/api-gateway.json",
	"../../../assets/gdrive.json",
	"../../../assets/github.json",
	"../../../assets/multi-tenant.json",
	"../../../assets/simple-rbac.json",
	"../../../assets/slack.json",
	"../../../assets/todo.json",
}
