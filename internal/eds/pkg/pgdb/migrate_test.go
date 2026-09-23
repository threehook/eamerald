package pgdb_test

import (
	"database/sql"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/threehook/eamerald/internal/eds/pkg/pgdb"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

const (
	pgUser     = "postgres"
	pgPassword = "postgres"
	pgDatabase = "eamerald"
)

func TestMigrate(t *testing.T) {
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

	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", pgUser, pgPassword, net.JoinHostPort(host, port.Port()), pgDatabase)

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	tables := []string{"objects", "relations", "manifest"}

	require.NoError(t, pgdb.Migrate(dsn), "migrate up")

	for _, table := range tables {
		require.True(t, tableExists(t, db, table), "table %q should exist after migrate up", table)
	}

	require.NoError(t, pgdb.Migrate(dsn), "migrate up is idempotent when there is nothing pending")
	require.NoError(t, pgdb.Down(dsn), "migrate down")

	for _, table := range tables {
		require.False(t, tableExists(t, db, table), "table %q should not exist after migrate down", table)
	}
}

func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()

	var exists bool

	err := db.QueryRowContext(t.Context(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)`,
		table,
	).Scan(&exists)
	require.NoError(t, err)

	return exists
}
