package pgdb

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/threehook/eamerald/internal/eds/pkg/store"
)

// Store is a store.Store implementation backed by a Postgres connection pool. It is not wired into any production code path yet (phase 4).
type Store struct {
	pool *pgxpool.Pool
}

var _ store.Store = (*Store)(nil)

// Open creates a connection pool for dsn and verifies it is reachable.
func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &Store{pool: pool}, nil
}

// Close releases the connection pool.
func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) View(ctx context.Context, fn func(store.Tx) error) error {
	return s.withTx(ctx, pgx.ReadOnly, fn)
}

func (s *Store) Update(ctx context.Context, fn func(store.Tx) error) error {
	return s.withTx(ctx, pgx.ReadWrite, fn)
}

// Batch runs fn in a regular read-write transaction: Postgres has no equivalent of bbolt's optimistic-batching Update mode,
// so this is the closest match.
func (s *Store) Batch(ctx context.Context, fn func(store.Tx) error) error {
	return s.withTx(ctx, pgx.ReadWrite, fn)
}

func (s *Store) withTx(ctx context.Context, mode pgx.TxAccessMode, fn func(store.Tx) error) error {
	pgTx, err := s.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: mode})
	if err != nil {
		return fmt.Errorf("begin postgres transaction: %w", err)
	}

	if err := fn(&tx{tx: pgTx, mu: &sync.Mutex{}}); err != nil {
		_ = pgTx.Rollback(ctx)
		return err
	}

	if err := pgTx.Commit(ctx); err != nil {
		return fmt.Errorf("commit postgres transaction: %w", err)
	}

	return nil
}

// tx implements store.Tx over a single pgx.Tx. Unlike a *bolt.Tx opened for reading, a single pgx.Tx (one connection) cannot serve two commands at
// once: the check/graph hot path (ds/checks.go's Checks) runs concurrent goroutines against the same store.Tx, so every wire operation - including
// iterator Next/Close - takes mu, turning concurrent access into safe interleaving rather than a "conn busy" error or corrupted reads.
type tx struct {
	tx pgx.Tx
	mu *sync.Mutex
}

var _ store.Tx = (*tx)(nil)
