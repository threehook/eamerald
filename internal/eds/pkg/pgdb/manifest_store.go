package pgdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aserto-dev/azm/model"
	dsm "github.com/aserto-dev/go-directory/aserto/directory/model/v3"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
)

const manifestName = "default"

func (t *tx) ManifestExists(ctx context.Context) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var exists bool
	if err := t.tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM manifest WHERE name = $1)`, manifestName).Scan(&exists); err != nil {
		return false, fmt.Errorf("manifest exists: %w", err)
	}

	return exists, nil
}

func (t *tx) GetManifestMetadata(ctx context.Context) (*dsm.Metadata, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var data []byte

	err := t.tx.QueryRow(ctx, `SELECT metadata FROM manifest WHERE name = $1`, manifestName).Scan(&data)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("get manifest metadata: %w", err)
	}

	md := &dsm.Metadata{}
	if err := proto.Unmarshal(data, md); err != nil {
		return nil, fmt.Errorf("unmarshal manifest metadata: %w", err)
	}

	return md, nil
}

func (t *tx) GetManifestBody(ctx context.Context) (*dsm.Body, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var data []byte

	err := t.tx.QueryRow(ctx, `SELECT body FROM manifest WHERE name = $1`, manifestName).Scan(&data)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("get manifest body: %w", err)
	}

	body := &dsm.Body{}
	if err := proto.Unmarshal(data, body); err != nil {
		return nil, fmt.Errorf("unmarshal manifest body: %w", err)
	}

	return body, nil
}

func (t *tx) GetManifestModel(ctx context.Context) (*model.Model, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var data []byte

	err := t.tx.QueryRow(ctx, `SELECT model FROM manifest WHERE name = $1`, manifestName).Scan(&data)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("get manifest model: %w", err)
	}

	var mod model.Model
	if err := json.Unmarshal(data, &mod); err != nil {
		return nil, fmt.Errorf("unmarshal manifest model: %w", err)
	}

	return &mod, nil
}

// SetManifest upserts the manifest row's metadata and body, defaulting model to an empty object on first insert.
// Every call site sets the manifest before the model within the same transaction, so SetManifestModel can assume the row already exists.
func (t *tx) SetManifest(ctx context.Context, metadata *dsm.Metadata, body *dsm.Body) error {
	metaBytes, err := proto.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal manifest metadata: %w", err)
	}

	bodyBytes, err := proto.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal manifest body: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	_, err = t.tx.Exec(ctx, `
		INSERT INTO manifest (name, metadata, body, model)
		VALUES ($1, $2, $3, '{}'::jsonb)
		ON CONFLICT (name) DO UPDATE SET metadata = EXCLUDED.metadata, body = EXCLUDED.body
	`, manifestName, metaBytes, bodyBytes)
	if err != nil {
		return fmt.Errorf("set manifest: %w", err)
	}

	return nil
}

func (t *tx) SetManifestModel(ctx context.Context, mod *model.Model) error {
	data, err := json.Marshal(mod)
	if err != nil {
		return fmt.Errorf("marshal manifest model: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	tag, err := t.tx.Exec(ctx, `UPDATE manifest SET model = $1 WHERE name = $2`, data, manifestName)
	if err != nil {
		return fmt.Errorf("set manifest model: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set manifest model: %w", ErrNotFound)
	}

	return nil
}

// DeleteManifest resets the manifest to empty and wipes all objects and relations, mirroring bdb's bucket-reset semantics.
// Uses DELETE rather than TRUNCATE: TRUNCATE takes an ACCESS EXCLUSIVE table lock, which would reintroduce the kind of contention this migration
// exists to remove.
func (t *tx) DeleteManifest(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, err := t.tx.Exec(ctx, `DELETE FROM manifest WHERE name = $1`, manifestName); err != nil {
		return fmt.Errorf("delete manifest: %w", err)
	}

	if _, err := t.tx.Exec(ctx, `DELETE FROM relations`); err != nil {
		return fmt.Errorf("delete relations: %w", err)
	}

	if _, err := t.tx.Exec(ctx, `DELETE FROM objects`); err != nil {
		return fmt.Errorf("delete objects: %w", err)
	}

	return nil
}
