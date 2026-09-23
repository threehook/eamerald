package ds

import (
	"bytes"
	"context"
	"hash/fnv"
	"strconv"

	"github.com/aserto-dev/azm/model"
	dsm "github.com/aserto-dev/go-directory/aserto/directory/model/v3"
	"github.com/threehook/eamerald/internal/eds/pkg/bdb"
	"github.com/threehook/eamerald/internal/eds/pkg/store"
)

type manifest struct {
	Metadata *dsm.Metadata
	Body     *dsm.Body
}

func Manifest(metadata *dsm.Metadata) *manifest {
	return &manifest{
		Metadata: metadata,
		Body:     &dsm.Body{},
	}
}

// Get, hydrates the manifest from the manifest table/bucket.
func (m *manifest) Get(ctx context.Context, tx store.Tx) (*manifest, error) {
	if ok, _ := tx.ManifestExists(ctx); !ok {
		return nil, bdb.ErrPathNotFound
	}

	metadata, err := tx.GetManifestMetadata(ctx)
	if err != nil {
		return nil, err
	}

	body, err := tx.GetManifestBody(ctx)
	if err != nil {
		return nil, err
	}

	return &manifest{Metadata: metadata, Body: body}, nil
}

// GetModel, hydrates the model cache from the manifest table/bucket.
func (m *manifest) GetModel(ctx context.Context, tx store.Tx) (*model.Model, error) {
	if ok, _ := tx.ManifestExists(ctx); !ok {
		return nil, bdb.ErrPathNotFound
	}

	mod, err := tx.GetManifestModel(ctx)
	if err != nil {
		return nil, err
	}

	return mod, nil
}

// Set, persists the manifest metadata and body.
func (m *manifest) Set(ctx context.Context, tx store.Tx, buf *bytes.Buffer) error {
	m.Body = &dsm.Body{Data: buf.Bytes()}

	return tx.SetManifest(ctx, m.Metadata, m.Body)
}

// SetModel, persists the model cache derived from the manifest.
func (m *manifest) SetModel(ctx context.Context, tx store.Tx, mod *model.Model) error {
	if mod.Metadata == nil {
		mod.Metadata = &model.Metadata{}
	}

	mod.Metadata.ETag = m.Metadata.GetEtag()
	mod.Metadata.UpdatedAt = m.Metadata.GetUpdatedAt().AsTime()

	return tx.SetManifestModel(ctx, mod)
}

// Delete
//
// !!! NOTE: delete manifest is a destructive operation !!!
//
// sets the manifest to an empty manifest, updates the model accordingly, deletes and recreates the objects and relations buckets.
func (m *manifest) Delete(ctx context.Context, tx store.Tx) error {
	return tx.DeleteManifest(ctx)
}

func (m *manifest) Hash() string {
	h := fnv.New64a()

	h.Reset()

	if _, err := h.Write(m.Body.GetData()); err != nil {
		return DefaultHash
	}

	return strconv.FormatUint(h.Sum64(), 10)
}
