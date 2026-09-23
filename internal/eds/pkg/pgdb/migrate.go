// Package pgdb will hold the PostgreSQL-backed implementation of the internal/eds/pkg/store interface (phase 3). For now it only provides the
// schema migration runner: it is not wired into any production code path.
package pgdb

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies every pending migration to the Postgres database at dsn.
func Migrate(dsn string) error {
	return withMigrator(dsn, func(m *migrate.Migrate) error {
		return ignoreNoChange(m.Up())
	})
}

// Down rolls back every migration, dropping the schema Migrate created. For
// tests and local development; not used by any production path.
func Down(dsn string) error {
	return withMigrator(dsn, func(m *migrate.Migrate) error {
		return ignoreNoChange(m.Down())
	})
}

func ignoreNoChange(err error) error {
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}

	return err
}

func withMigrator(dsn string, run func(*migrate.Migrate) error) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open postgres connection: %w", err)
	}
	defer db.Close()

	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
	if err != nil {
		return fmt.Errorf("create postgres migration driver: %w", err)
	}

	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()

	return run(m)
}
