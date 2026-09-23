package bdb

import (
	"context"

	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	"github.com/threehook/eamerald/internal/eds/pkg/store"
)

func (t *tx) GetObject(ctx context.Context, objectType, objectID string) (*dsc.Object, error) {
	return Get[dsc.Object](ctx, t.tx, ObjectsPath, objectKey(objectType, objectID))
}

func (t *tx) SetObject(ctx context.Context, obj *dsc.Object) (*dsc.Object, error) {
	return Set[dsc.Object](ctx, t.tx, ObjectsPath, objectKey(obj.GetType(), obj.GetId()), obj)
}

func (t *tx) DeleteObject(ctx context.Context, objectType, objectID string) error {
	return Delete(ctx, t.tx, ObjectsPath, objectKey(objectType, objectID))
}

func (t *tx) ScanObjects(ctx context.Context, objectType, pageToken string) (store.Iterator[*dsc.Object], error) {
	opts := []ScanOption{}

	if objectType != "" {
		opts = append(opts, WithKeyFilter(objectKey(objectType, "")))
	}

	if pageToken != "" {
		opts = append(opts, WithPageToken(pageToken))
	}

	iter, err := NewScanIterator[dsc.Object](ctx, t.tx, ObjectsPath, opts...)
	if err != nil {
		return nil, err
	}

	return &objectIterator{iter: iter}, nil
}

type objectIterator struct {
	iter *ScanIterator[dsc.Object, *dsc.Object]
}

var _ store.Iterator[*dsc.Object] = (*objectIterator)(nil)

func (o *objectIterator) Next() bool         { return o.iter.Next() }
func (o *objectIterator) Value() *dsc.Object { return o.iter.Value() }
func (o *objectIterator) Token() string      { return string(o.iter.RawKey()) }

// Close is a no-op: a *bolt.Tx-backed cursor holds no resource beyond the enclosing transaction's lifetime.
func (o *objectIterator) Close() error { return nil }
