package pgdb

import (
	"context"
	"errors"
	"fmt"
	"sync"

	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	"github.com/threehook/eamerald/internal/eds/pkg/store"
)

// objectTokenSize is the number of ordering columns (object_type, object_id) a ScanObjects page token encodes.
const objectTokenSize = 2

func (t *tx) GetObject(ctx context.Context, objectType, objectID string) (*dsc.Object, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var data []byte

	err := t.tx.QueryRow(ctx, `SELECT data FROM objects WHERE object_type = $1 AND object_id = $2`, objectType, objectID).Scan(&data)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("get object: %w", err)
	}

	obj := &dsc.Object{}
	if err := proto.Unmarshal(data, obj); err != nil {
		return nil, fmt.Errorf("unmarshal object: %w", err)
	}

	return obj, nil
}

func (t *tx) SetObject(ctx context.Context, obj *dsc.Object) (*dsc.Object, error) {
	data, err := proto.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshal object: %w", err)
	}

	createdAt := obj.GetCreatedAt().AsTime() //nolint:staticcheck // Marked as deprecated

	t.mu.Lock()
	defer t.mu.Unlock()

	_, err = t.tx.Exec(ctx, `
		INSERT INTO objects (object_type, object_id, data, etag, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (object_type, object_id) DO UPDATE SET data = EXCLUDED.data, etag = EXCLUDED.etag, updated_at = EXCLUDED.updated_at
	`, obj.GetType(), obj.GetId(), data, obj.GetEtag(), createdAt, obj.GetUpdatedAt().AsTime())
	if err != nil {
		return nil, fmt.Errorf("set object: %w", err)
	}

	return obj, nil
}

func (t *tx) DeleteObject(ctx context.Context, objectType, objectID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, err := t.tx.Exec(ctx, `DELETE FROM objects WHERE object_type = $1 AND object_id = $2`, objectType, objectID); err != nil {
		return fmt.Errorf("delete object: %w", err)
	}

	return nil
}

func (t *tx) ScanObjects(ctx context.Context, objectType, pageToken string) (store.Iterator[*dsc.Object], error) {
	var (
		where []string
		args  []any
	)

	if objectType != "" {
		args = append(args, objectType)
		where = append(where, fmt.Sprintf("object_type = $%d", len(args)))
	}

	if pageToken != "" {
		vals, err := decodeToken(pageToken, objectTokenSize)
		if err != nil {
			return nil, err
		}

		args = append(args, vals[0], vals[1])
		// >=, not >: the token is the peeked-but-not-yet-returned row's own key (mirroring bbolt's Seek, which is
		// inclusive of the exact key), so it must be included in the resumed page.
		where = append(where, fmt.Sprintf("(object_type, object_id) >= ($%d, $%d)", len(args)-1, len(args)))
	}

	query := "SELECT object_type, object_id, data FROM objects" + whereClause(where) + " ORDER BY object_type, object_id"

	t.mu.Lock()
	defer t.mu.Unlock()

	rows, err := t.tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("scan objects: %w", err)
	}

	return &objectIterator{rows: rows, mu: t.mu}, nil
}

type objectIterator struct {
	rows       pgx.Rows
	mu         *sync.Mutex
	objectType string
	objectID   string
	value      *dsc.Object
}

var _ store.Iterator[*dsc.Object] = (*objectIterator)(nil)

func (o *objectIterator) Next() bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.rows.Next() {
		return false
	}

	var data []byte
	if err := o.rows.Scan(&o.objectType, &o.objectID, &data); err != nil {
		return false
	}

	obj := &dsc.Object{}
	if err := proto.Unmarshal(data, obj); err == nil {
		o.value = obj
	} else {
		o.value = &dsc.Object{}
	}

	return true
}

func (o *objectIterator) Value() *dsc.Object { return o.value }
func (o *objectIterator) Token() string      { return encodeToken(o.objectType, o.objectID) }

// Close releases the underlying pgx.Rows/connection. It must be called even when the iterator wasn't drained to completion (e.g. a caller stopping
// after one page): pgx does not release the connection until Close is called or Next returns false on its own.
func (o *objectIterator) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.rows.Close()

	return o.rows.Err()
}
