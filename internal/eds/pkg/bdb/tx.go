package bdb

import (
	"context"

	"github.com/threehook/eamerald/internal/eds/pkg/store"
	bolt "go.etcd.io/bbolt"
)

var _ store.Store = (*BoltDB)(nil)

// View, Update and Batch adapt bbolt's transaction methods to store.Store, wrapping each *bolt.Tx in the store.Tx implementation below.
func (s *BoltDB) View(_ context.Context, fn func(store.Tx) error) error {
	return s.db.View(func(btx *bolt.Tx) error {
		return fn(&tx{tx: btx})
	})
}

func (s *BoltDB) Update(_ context.Context, fn func(store.Tx) error) error {
	return s.db.Update(func(btx *bolt.Tx) error {
		return fn(&tx{tx: btx})
	})
}

func (s *BoltDB) Batch(_ context.Context, fn func(store.Tx) error) error {
	return s.db.Batch(func(btx *bolt.Tx) error {
		return fn(&tx{tx: btx})
	})
}

// tx implements store.Tx over a single *bolt.Tx. It carries no mutable state of its own, so it is safe for concurrent
// use by multiple goroutines when wrapping a read-only (View) transaction, matching bbolt's own guarantee.
type tx struct {
	tx *bolt.Tx
}

var _ store.Tx = (*tx)(nil)
