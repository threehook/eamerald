// Package store defines the storage-backend-agnostic transaction interface used by the directory service.
// internal/eds/pkg/bdb is currently the sole implementation, backed by BoltDB.
package store

import (
	"context"

	"github.com/aserto-dev/azm/graph"
	"github.com/aserto-dev/azm/model"
	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	dsm "github.com/aserto-dev/go-directory/aserto/directory/model/v3"
)

// Direction selects which side of a relation a scan is anchored/indexed on.
type Direction int

const (
	ByObject Direction = iota
	BySubject
)

// RelationFilter is a partial relation identifier used to narrow a relation scan. Zero-value fields are treated as unset.
// HasSubjectRelation distinguishes an explicitly empty subject_relation from an unset one.
type RelationFilter struct {
	ObjectType         string
	ObjectID           string
	Relation           string
	SubjectType        string
	SubjectID          string
	SubjectRelation    string
	HasSubjectRelation bool
}

// ObjectOnlyFilter builds a RelationFilter naming only the object or subject side (per dir), for scans anchored on a single object identifier with
// no further narrowing by relation/subject fields.
func ObjectOnlyFilter(dir Direction, objectType, objectID string) RelationFilter {
	if dir == BySubject {
		return RelationFilter{SubjectType: objectType, SubjectID: objectID}
	}

	return RelationFilter{ObjectType: objectType, ObjectID: objectID}
}

// Store manages transactions against the directory's storage backend.
type Store interface {
	View(ctx context.Context, fn func(Tx) error) error
	Update(ctx context.Context, fn func(Tx) error) error
	Batch(ctx context.Context, fn func(Tx) error) error
}

// ObjectStore is the object-table slice of a transaction.
type ObjectStore interface {
	GetObject(ctx context.Context, objectType, objectID string) (*dsc.Object, error)
	SetObject(ctx context.Context, obj *dsc.Object) (*dsc.Object, error)
	DeleteObject(ctx context.Context, objectType, objectID string) error
	// ScanObjects iterates objects, optionally restricted to objectType ("" for all types), resuming from pageToken when non-empty.
	ScanObjects(ctx context.Context, objectType, pageToken string) (Iterator[*dsc.Object], error)
}

// RelationStore is the relation-table slice of a transaction.
type RelationStore interface {
	// GetRelationExact looks up a single relation by its full identifier.
	GetRelationExact(ctx context.Context, ident *dsc.RelationIdentifier) (*dsc.Relation, error)
	// SetRelation persists rel, writing both the object- and subject-indexed views.
	SetRelation(ctx context.Context, rel *dsc.Relation) (*dsc.Relation, error)
	// DeleteRelation removes a relation, both indexed views.
	DeleteRelation(ctx context.Context, ident *dsc.RelationIdentifier) error
	// ScanRelations iterates relations matching filter's populated fields, resuming from pageToken when non-empty. dir picks
	// which index the filter's fields are matched against.
	ScanRelations(ctx context.Context, dir Direction, filter RelationFilter, pageToken string) (Iterator[*dsc.Relation], error)
	// ScanRelationsFiltered is the pooled, zero-allocation scan used by the check/graph hot path: candidates matching
	// filter's prefix are decoded into pool-provided instances, filtered by valueFilter, and appended to out.
	ScanRelationsFiltered(
		ctx context.Context, dir Direction, filter RelationFilter,
		valueFilter func(*dsc.RelationIdentifier) bool, pool graph.RelationPool, out *[]*dsc.RelationIdentifier,
	) error
	// RelationsExistForObject reports whether any relation names objectType:objectID on the side identified by dir.
	RelationsExistForObject(ctx context.Context, dir Direction, objectType, objectID string) (bool, error)
}

// ManifestStore is the manifest-table slice of a transaction.
type ManifestStore interface {
	ManifestExists(ctx context.Context) (bool, error)
	GetManifestMetadata(ctx context.Context) (*dsm.Metadata, error)
	GetManifestBody(ctx context.Context) (*dsm.Body, error)
	GetManifestModel(ctx context.Context) (*model.Model, error)
	SetManifest(ctx context.Context, metadata *dsm.Metadata, body *dsm.Body) error
	SetManifestModel(ctx context.Context, mod *model.Model) error
	// DeleteManifest resets the manifest to empty and wipes all objects and relations.
	DeleteManifest(ctx context.Context) error
}

// Tx exposes the domain-level operations the directory service performs within a single storage transaction.
type Tx interface {
	ObjectStore
	RelationStore
	ManifestStore
}
