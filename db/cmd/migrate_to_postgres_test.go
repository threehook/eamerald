package cmd_test

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	dsr "github.com/aserto-dev/go-directory/aserto/directory/reader/v3"
	dsw "github.com/aserto-dev/go-directory/aserto/directory/writer/v3"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/threehook/eamerald/db/cmd"
	"github.com/threehook/eamerald/db/pkg/inproc"
	"github.com/threehook/eamerald/internal/eds"
	"github.com/threehook/eamerald/internal/eds/pkg/directory"
	dirclient "github.com/threehook/eamerald/mrld/clients/directory"
)

const (
	pgUser     = "postgres"
	pgPassword = "postgres"
	pgDatabase = "eamerald"

	idAlice = "alice"
	idDoc1  = "doc1"

	typeUser     = "user"
	typeResource = "resource"
)

// newPostgresDSN starts a fresh postgres:16-alpine testcontainer and returns a DSN for it, host-mapped so this
// test process (not another container) can reach it directly.
func newPostgresDSN(t *testing.T) string {
	t.Helper()

	ctx := t.Context()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     pgUser,
			"POSTGRES_PASSWORD": pgPassword,
			"POSTGRES_DB":       pgDatabase,
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60 * time.Second),
	}

	pg, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { testcontainers.CleanupContainer(t, pg) })

	host, err := pg.Host(ctx)
	require.NoError(t, err)

	port, err := pg.MappedPort(ctx, "5432")
	require.NoError(t, err)

	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", pgUser, pgPassword, net.JoinHostPort(host, port.Port()), pgDatabase)
}

func TestMigrateToPostgres(t *testing.T) {
	ctx := t.Context()
	logger := zerolog.New(io.Discard)

	dbFile := filepath.Join(t.TempDir(), "source.db")

	// Seed a BoltDB source: manifest + one object of each type + one relation between them.
	func() {
		srcDir, err := eds.Open(ctx, &directory.Config{
			DBPath: dbFile, RequestTimeout: 5 * time.Second, Backend: directory.BackendBoltDB,
		}, &logger, nil)
		require.NoError(t, err)

		defer srcDir.Close()

		srcConn, srcCleanup := inproc.NewServerFromDirectory(srcDir)
		defer srcCleanup()

		manifestFile, err := os.Open("../../templates/simple-rbac/manifest.yaml")
		require.NoError(t, err)

		defer manifestFile.Close()

		require.NoError(t, dirclient.New(srcConn).SetManifest(ctx, manifestFile))

		writer := dsw.NewWriterClient(srcConn)

		_, err = writer.SetObject(ctx, &dsw.SetObjectRequest{Object: &dsc.Object{Type: typeUser, Id: idAlice}})
		require.NoError(t, err)

		_, err = writer.SetObject(ctx, &dsw.SetObjectRequest{Object: &dsc.Object{Type: typeResource, Id: idDoc1}})
		require.NoError(t, err)

		_, err = writer.SetRelation(ctx, &dsw.SetRelationRequest{Relation: &dsc.Relation{
			ObjectType: typeResource, ObjectId: idDoc1, Relation: "owner", SubjectType: typeUser, SubjectId: idAlice,
		}})
		require.NoError(t, err)
	}()

	dsn := newPostgresDSN(t)

	migrateCmd := &cmd.MigrateToPostgresCmd{DBFile: dbFile, DSN: dsn}
	require.NoError(t, migrateCmd.Run(ctx))

	// Re-running against the same, now-populated destination must not fail or duplicate anything (upsert semantics).
	require.NoError(t, migrateCmd.Run(ctx))

	// Reconnect to the postgres destination fresh, mirroring how a real cutover would be verified, and check the
	// manifest, object and relation all made it across, and that the model is actually usable (Check evaluates).
	dstDir, err := eds.Open(ctx, &directory.Config{
		RequestTimeout: 5 * time.Second, Backend: directory.BackendPostgres, Postgres: directory.PostgresConfig{DSN: dsn},
	}, &logger, nil)
	require.NoError(t, err)

	defer dstDir.Close()

	dstConn, dstCleanup := inproc.NewServerFromDirectory(dstDir)
	defer dstCleanup()

	reader := dsr.NewReaderClient(dstConn)

	obj, err := reader.GetObject(ctx, &dsr.GetObjectRequest{ObjectType: typeUser, ObjectId: idAlice})
	require.NoError(t, err)
	require.Equal(t, idAlice, obj.GetResult().GetId())

	rel, err := reader.GetRelation(ctx, &dsr.GetRelationRequest{
		ObjectType: typeResource, ObjectId: idDoc1, Relation: "owner", SubjectType: typeUser, SubjectId: idAlice,
	})
	require.NoError(t, err)
	require.Equal(t, idAlice, rel.GetResult().GetSubjectId())

	checkResp, err := reader.Check(ctx, &dsr.CheckRequest{
		ObjectType: typeResource, ObjectId: idDoc1, Relation: "can_read", SubjectType: typeUser, SubjectId: idAlice,
	})
	require.NoError(t, err)
	require.True(t, checkResp.GetCheck(), "owner should be able to read the resource after migration")
}
