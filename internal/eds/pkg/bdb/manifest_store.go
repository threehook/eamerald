package bdb

import (
	"context"

	"github.com/aserto-dev/azm/model"
	dsm "github.com/aserto-dev/go-directory/aserto/directory/model/v3"
)

func (t *tx) ManifestExists(_ context.Context) (bool, error) {
	return BucketExists(t.tx, ManifestPath)
}

func (t *tx) GetManifestMetadata(ctx context.Context) (*dsm.Metadata, error) {
	return Get[dsm.Metadata](ctx, t.tx, ManifestPath, MetadataKey)
}

func (t *tx) GetManifestBody(ctx context.Context) (*dsm.Body, error) {
	return Get[dsm.Body](ctx, t.tx, ManifestPath, BodyKey)
}

func (t *tx) GetManifestModel(ctx context.Context) (*model.Model, error) {
	return GetAny[model.Model](ctx, t.tx, ManifestPath, ModelKey)
}

func (t *tx) SetManifest(ctx context.Context, metadata *dsm.Metadata, body *dsm.Body) error {
	if _, err := CreateBucket(t.tx, ManifestPath); err != nil {
		return err
	}

	if _, err := Set[dsm.Metadata](ctx, t.tx, ManifestPath, MetadataKey, metadata); err != nil {
		return err
	}

	if _, err := Set[dsm.Body](ctx, t.tx, ManifestPath, BodyKey, body); err != nil {
		return err
	}

	return nil
}

func (t *tx) SetManifestModel(ctx context.Context, mod *model.Model) error {
	_, err := SetAny(ctx, t.tx, ManifestPath, ModelKey, mod)
	return err
}

// DeleteManifest resets the manifest to empty and wipes all objects and relations, mirroring the original ds.manifest.Delete bucket reset exactly.
func (t *tx) DeleteManifest(_ context.Context) error {
	for _, path := range []Path{ManifestPath, ObjectsPath, RelationsObjPath, RelationsSubPath} {
		if err := DeleteBucket(t.tx, path); err != nil {
			return err
		}

		if _, err := CreateBucket(t.tx, path); err != nil {
			return err
		}
	}

	return nil
}
