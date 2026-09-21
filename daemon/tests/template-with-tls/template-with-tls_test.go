package template_with_tls_test

import (
	"context"
	"testing"
	"time"

	assets_test "github.com/threehook/eamerald/daemon/tests/assets"
	tc "github.com/threehook/eamerald/daemon/tests/common"
	"github.com/threehook/eamerald/internal/fs"
	azc "github.com/threehook/eamerald/mrld/clients/authorizer"
	dsc "github.com/threehook/eamerald/mrld/clients/directory"
	"github.com/threehook/eamerald/mrld/constants"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestTemplates(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	t.Logf("\nTEST CONTAINER IMAGE: %q\n", tc.TestImage())

	req := testcontainers.ContainerRequest{
		Image:        tc.TestImage(),
		ExposedPorts: []string{"9292/tcp"},
		Env: map[string]string{
			constants.EnvEameraldCertsDir:     constants.DefCertsDir,
			constants.EnvEameraldDBDir:        constants.DefDBDir,
			constants.EnvEameraldDecisionsDir: constants.DefDecisionsDir,
		},
		Files: []testcontainers.ContainerFile{
			{
				Reader:            assets_test.ConfigWithTLSReader(),
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
		Insecure:  true,
		Plaintext: false,
		Timeout:   10 * time.Second,
	}

	azConfig := &azc.Config{
		Host:      addr,
		Insecure:  true,
		Plaintext: false,
		Timeout:   10 * time.Second,
	}

	for _, tmpl := range tcs {
		t.Run("testTemplate", tc.InstallTemplate(ctx, dsConfig, azConfig, tmpl))
	}
}

var tcs = []string{
	"../../../templates/acmecorp.json",
	"../../../templates/peoplefinder.json",

	"../../../templates/citadel.json",
	"../../../templates/api-auth.json",
	"../../../templates/api-gateway.json",
	"../../../templates/gdrive.json",
	"../../../templates/github.json",
	"../../../templates/multi-tenant.json",
	"../../../templates/simple-rbac.json",
	"../../../templates/slack.json",
	"../../../templates/todo.json",
}
