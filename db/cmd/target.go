package cmd

import (
	"strings"

	"github.com/threehook/eamerald/internal/eds/pkg/directory"
)

// isPostgresDSN reports whether target is a postgres connection string rather than a BoltDB file path.
func isPostgresDSN(target string) bool {
	return strings.HasPrefix(target, "postgres://") || strings.HasPrefix(target, "postgresql://")
}

// configForTarget builds a directory.Config for target: a postgres connection string, or a BoltDB file path.
func configForTarget(target string) *directory.Config {
	if isPostgresDSN(target) {
		return &directory.Config{
			Backend:        directory.BackendPostgres,
			Postgres:       directory.PostgresConfig{DSN: target},
			RequestTimeout: requestTimeout,
		}
	}

	return &directory.Config{
		Backend:        directory.BackendBoltDB,
		DBPath:         target,
		RequestTimeout: requestTimeout,
	}
}
