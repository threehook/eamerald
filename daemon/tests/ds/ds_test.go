package ds_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/threehook/eamerald/internal/fs"
	azc "github.com/threehook/eamerald/mrld/clients/authorizer"
	dsc "github.com/threehook/eamerald/mrld/clients/directory"
	"github.com/threehook/eamerald/mrld/constants"

	client "github.com/aserto-dev/go-aserto"
	dsr "github.com/aserto-dev/go-directory/aserto/directory/reader/v3"
	assets_test "github.com/threehook/eamerald/daemon/tests/assets"
	tc "github.com/threehook/eamerald/daemon/tests/common"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestDirectory(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	t.Logf("\nTEST CONTAINER IMAGE: %q\n", tc.TestImage())

	req := testcontainers.ContainerRequest{
		Image:        tc.TestImage(),
		ExposedPorts: tc.TestExposedPorts,
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

	t.Run("testDirectory", testDirectory(dsConfig, azConfig))
}

const (
	pgNetworkAlias = "postgres"
	pgUser         = "postgres"
	pgPassword     = "postgres"
	pgDatabase     = "eamerald"
)

// startPostgresOnNetwork starts a postgres:16-alpine container reachable from other containers on network as
// pgNetworkAlias:5432, and returns the DSN eameraldd should use to reach it over that same network.
func startPostgresOnNetwork(ctx context.Context, t *testing.T, networkName string) string {
	t.Helper()

	pgReq := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     pgUser,
			"POSTGRES_PASSWORD": pgPassword,
			"POSTGRES_DB":       pgDatabase,
		},
		Networks:       []string{networkName},
		NetworkAliases: map[string][]string{networkName: {pgNetworkAlias}},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60 * time.Second),
	}

	pg, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: pgReq,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { testcontainers.CleanupContainer(t, pg) })

	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", pgUser, pgPassword, net.JoinHostPort(pgNetworkAlias, "5432"), pgDatabase)
}

// TestDirectoryPostgres runs the exact same subtests as TestDirectory (testDirectory, unchanged) against an
// eamerald instance configured with directory.backend: postgres instead of boltdb, proving the two backends
// behave identically for the directory service's Check/Checks path.
func TestDirectoryPostgres(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	pgNet, err := network.New(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgNet.Remove(ctx) })

	dsn := startPostgresOnNetwork(ctx, t, pgNet.Name)

	t.Logf("\nTEST CONTAINER IMAGE: %q\n", tc.TestImage())

	req := testcontainers.ContainerRequest{
		Image:        tc.TestImage(),
		ExposedPorts: tc.TestExposedPorts,
		Env: map[string]string{
			constants.EnvEameraldCertsDir:         constants.DefCertsDir,
			constants.EnvEameraldDBDir:            constants.DefDBDir,
			constants.EnvEameraldDecisionsDir:     constants.DefDecisionsDir,
			constants.EnvEameraldDirectoryBackend: "postgres",
			constants.EnvEameraldPostgresDSN:      dsn,
		},
		Networks: []string{pgNet.Name},
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

	t.Run("testDirectory", testDirectory(dsConfig, azConfig))
}

func testDirectory(dsConfig *dsc.Config, azConfig *azc.Config) func(*testing.T) {
	return func(t *testing.T) {
		opts := []client.ConnectionOption{
			client.WithAddr(dsConfig.Host),
			client.WithInsecure(true),
		}

		conn, err := client.NewConnection(opts...)
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })

		ctx, cancel := context.WithTimeout(t.Context(), dsConfig.Timeout)
		t.Cleanup(cancel)

		t.Run("", tc.InstallTemplate(ctx, dsConfig, azConfig, "../../../templates/gdrive.json"))

		tests := []struct {
			name string
			test func(*testing.T)
		}{
			{"TestCheck", testCheck(ctx, dsr.NewReaderClient(conn))},
			{"TestChecks", testChecks(ctx, dsr.NewReaderClient(conn))},
		}

		for _, testCase := range tests {
			t.Run(testCase.name, testCase.test)
		}
	}
}
