package pgdb_test

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/aserto-dev/azm/graph"
	"github.com/aserto-dev/azm/model"
	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	dsm "github.com/aserto-dev/go-directory/aserto/directory/model/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/threehook/eamerald/internal/eds/pkg/pgdb"
	"github.com/threehook/eamerald/internal/eds/pkg/store"
)

const (
	typeUser     = "user"
	typeDocument = "document"

	idAlice  = "alice"
	idBob    = "bob"
	idReport = "report"

	relViewer = "viewer"
)

// newTestStore starts a fresh, migrated Postgres testcontainer and returns a connected *pgdb.Store, closing both on test cleanup.
func newTestStore(t *testing.T) *pgdb.Store {
	t.Helper()

	ctx := t.Context()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     pgUser,
			"POSTGRES_PASSWORD": pgPassword,
			"POSTGRES_DB":       pgDatabase,
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60 * time.Second),
	}

	pg, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { testcontainers.CleanupContainer(t, pg) })

	host, err := pg.Host(ctx)
	require.NoError(t, err)

	port, err := pg.MappedPort(ctx, "5432")
	require.NoError(t, err)

	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", pgUser, pgPassword, net.JoinHostPort(host, port.Port()), pgDatabase)

	require.NoError(t, pgdb.Migrate(dsn))

	s, err := pgdb.Open(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(s.Close)

	return s
}

func TestObjectCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	obj := &dsc.Object{Type: typeUser, Id: idAlice, Etag: "etag-1"}

	err := s.Update(ctx, func(tx store.Tx) error {
		_, err := tx.SetObject(ctx, obj)
		return err
	})
	require.NoError(t, err)

	err = s.View(ctx, func(tx store.Tx) error {
		got, err := tx.GetObject(ctx, typeUser, idAlice)
		require.NoError(t, err)
		require.Equal(t, idAlice, got.GetId())
		require.Equal(t, "etag-1", got.GetEtag())

		return nil
	})
	require.NoError(t, err)

	err = s.Update(ctx, func(tx store.Tx) error {
		return tx.DeleteObject(ctx, typeUser, idAlice)
	})
	require.NoError(t, err)

	err = s.View(ctx, func(tx store.Tx) error {
		_, err := tx.GetObject(ctx, typeUser, idAlice)
		require.ErrorIs(t, err, pgdb.ErrNotFound)

		return nil
	})
	require.NoError(t, err)
}

func TestScanObjects(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	err := s.Update(ctx, func(tx store.Tx) error {
		for _, obj := range []*dsc.Object{
			{Type: typeUser, Id: idAlice},
			{Type: typeUser, Id: idBob},
			{Type: "group", Id: "admins"},
		} {
			if _, err := tx.SetObject(ctx, obj); err != nil {
				return err
			}
		}

		return nil
	})
	require.NoError(t, err)

	err = s.View(ctx, func(tx store.Tx) error {
		iter, err := tx.ScanObjects(ctx, typeUser, "")
		require.NoError(t, err)

		defer iter.Close()

		var ids []string
		for iter.Next() {
			ids = append(ids, iter.Value().GetId())
		}

		require.ElementsMatch(t, []string{idAlice, idBob}, ids)

		return nil
	})
	require.NoError(t, err)

	// pagination: page size of 1, resuming from the returned token, over the full (unfiltered) scan.
	err = s.View(ctx, func(tx store.Tx) error {
		iter, err := tx.ScanObjects(ctx, "", "")
		require.NoError(t, err)

		first, token := store.Page(iter, 1)
		require.Len(t, first, 1)
		require.NotEmpty(t, token)
		require.NoError(t, iter.Close(), "must close before issuing another query on the same tx")

		iter2, err := tx.ScanObjects(ctx, "", token)
		require.NoError(t, err)

		defer iter2.Close()

		var rest []*dsc.Object
		for iter2.Next() {
			rest = append(rest, iter2.Value())
		}

		require.Len(t, rest, 2, "the remaining two objects, resumed from the token")

		return nil
	})
	require.NoError(t, err)
}

func TestRelationCRUDAndDirections(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	rel := &dsc.Relation{
		ObjectType: typeDocument, ObjectId: idReport, Relation: relViewer,
		SubjectType: typeUser, SubjectId: idAlice, Etag: "etag-1",
	}
	ident := &dsc.RelationIdentifier{
		ObjectType: rel.GetObjectType(), ObjectId: rel.GetObjectId(), Relation: rel.GetRelation(),
		SubjectType: rel.GetSubjectType(), SubjectId: rel.GetSubjectId(),
	}

	err := s.Update(ctx, func(tx store.Tx) error {
		_, err := tx.SetRelation(ctx, rel)
		return err
	})
	require.NoError(t, err)

	err = s.View(ctx, func(tx store.Tx) error {
		got, err := tx.GetRelationExact(ctx, ident)
		require.NoError(t, err)
		require.Equal(t, "etag-1", got.GetEtag())

		exists, err := tx.RelationsExistForObject(ctx, store.ByObject, typeDocument, idReport)
		require.NoError(t, err)
		require.True(t, exists, "object-indexed existence check")

		exists, err = tx.RelationsExistForObject(ctx, store.BySubject, typeUser, idAlice)
		require.NoError(t, err)
		require.True(t, exists, "subject-indexed existence check")

		exists, err = tx.RelationsExistForObject(ctx, store.ByObject, typeDocument, "does-not-exist")
		require.NoError(t, err)
		require.False(t, exists)

		return nil
	})
	require.NoError(t, err)

	err = s.View(ctx, func(tx store.Tx) error {
		iter, err := tx.ScanRelations(ctx, store.ByObject, store.RelationFilter{ObjectType: typeDocument, ObjectID: idReport}, "")
		require.NoError(t, err)
		require.True(t, iter.Next(), "object-indexed scan should find the relation")
		require.Equal(t, idAlice, iter.Value().GetSubjectId())
		require.False(t, iter.Next())
		require.NoError(t, iter.Close())

		iter, err = tx.ScanRelations(ctx, store.BySubject, store.RelationFilter{SubjectType: typeUser, SubjectID: idAlice}, "")
		require.NoError(t, err)

		defer iter.Close()

		require.True(t, iter.Next(), "subject-indexed scan should find the relation")
		require.Equal(t, idReport, iter.Value().GetObjectId())
		require.False(t, iter.Next())

		return nil
	})
	require.NoError(t, err)

	err = s.Update(ctx, func(tx store.Tx) error {
		return tx.DeleteRelation(ctx, ident)
	})
	require.NoError(t, err)

	err = s.View(ctx, func(tx store.Tx) error {
		_, err := tx.GetRelationExact(ctx, ident)
		require.ErrorIs(t, err, pgdb.ErrNotFound)

		return nil
	})
	require.NoError(t, err)
}

func TestScanRelationsPartialAndPaged(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	err := s.Update(ctx, func(tx store.Tx) error {
		rels := []*dsc.Relation{
			{ObjectType: typeDocument, ObjectId: idReport, Relation: relViewer, SubjectType: typeUser, SubjectId: idAlice},
			{ObjectType: typeDocument, ObjectId: idReport, Relation: "editor", SubjectType: typeUser, SubjectId: idBob},
			{ObjectType: typeDocument, ObjectId: "other", Relation: relViewer, SubjectType: typeUser, SubjectId: idAlice},
		}
		for _, r := range rels {
			if _, err := tx.SetRelation(ctx, r); err != nil {
				return err
			}
		}

		return nil
	})
	require.NoError(t, err)

	err = s.View(ctx, func(tx store.Tx) error {
		// partial identifier: only object type+id set, matches both viewer and editor relations on the report object.
		iter, err := tx.ScanRelations(ctx, store.ByObject, store.RelationFilter{ObjectType: typeDocument, ObjectID: idReport}, "")
		require.NoError(t, err)

		var subjects []string
		for iter.Next() {
			subjects = append(subjects, iter.Value().GetSubjectId())
		}

		require.ElementsMatch(t, []string{idAlice, idBob}, subjects)
		require.NoError(t, iter.Close())

		// pagination over the subject-indexed scan for alice, one page at a time.
		iter, err = tx.ScanRelations(ctx, store.BySubject, store.RelationFilter{SubjectType: typeUser, SubjectID: idAlice}, "")
		require.NoError(t, err)

		first, token := store.Page[*dsc.Relation](iter, 1)
		require.Len(t, first, 1)
		require.NotEmpty(t, token)
		require.NoError(t, iter.Close())

		iter, err = tx.ScanRelations(ctx, store.BySubject, store.RelationFilter{SubjectType: typeUser, SubjectID: idAlice}, token)
		require.NoError(t, err)

		defer iter.Close()

		second, _ := store.Page[*dsc.Relation](iter, 1)
		require.Len(t, second, 1)
		require.NotEqual(t, first[0].GetObjectId(), second[0].GetObjectId(), "the two pages must not repeat a row")

		return nil
	})
	require.NoError(t, err)
}

// relationIdentifierPool is a minimal graph.RelationPool for exercising ScanRelationsFiltered; it doesn't need to
// actually recycle instances for the test to be meaningful.
type relationIdentifierPool struct{}

func (relationIdentifierPool) Get() *dsc.RelationIdentifier { return &dsc.RelationIdentifier{} }
func (relationIdentifierPool) Put(*dsc.RelationIdentifier)  {}

var _ graph.RelationPool = relationIdentifierPool{}

func TestScanRelationsFiltered(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	err := s.Update(ctx, func(tx store.Tx) error {
		rels := []*dsc.Relation{
			{ObjectType: typeDocument, ObjectId: idReport, Relation: relViewer, SubjectType: typeUser, SubjectId: idAlice},
			{ObjectType: typeDocument, ObjectId: idReport, Relation: "editor", SubjectType: typeUser, SubjectId: idBob},
			{ObjectType: typeDocument, ObjectId: "other", Relation: relViewer, SubjectType: typeUser, SubjectId: "carol"},
		}
		for _, r := range rels {
			if _, err := tx.SetRelation(ctx, r); err != nil {
				return err
			}
		}

		return nil
	})
	require.NoError(t, err)

	err = s.View(ctx, func(tx store.Tx) error {
		var out []*dsc.RelationIdentifier

		// wildcard subject: any relation on document:report, filtered client-side by relation=relViewer as the
		// check/graph hot path does.
		filter := store.RelationFilter{ObjectType: typeDocument, ObjectID: idReport}
		valueFilter := func(r *dsc.RelationIdentifier) bool { return r.GetRelation() == relViewer }

		err := tx.ScanRelationsFiltered(ctx, store.ByObject, filter, valueFilter, relationIdentifierPool{}, &out)
		require.NoError(t, err)
		require.Len(t, out, 1)
		require.Equal(t, idAlice, out[0].GetSubjectId())

		return nil
	})
	require.NoError(t, err)
}

func TestManifestGetSetDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	err := s.View(ctx, func(tx store.Tx) error {
		exists, err := tx.ManifestExists(ctx)
		require.NoError(t, err)
		require.False(t, exists)

		_, err = tx.GetManifestMetadata(ctx)
		require.ErrorIs(t, err, pgdb.ErrNotFound)

		return nil
	})
	require.NoError(t, err)

	md := &dsm.Metadata{Etag: "manifest-etag-1"}
	body := &dsm.Body{Data: []byte("manifest body")}
	mod := &model.Model{Version: 1}

	err = s.Update(ctx, func(tx store.Tx) error {
		if err := tx.SetManifest(ctx, md, body); err != nil {
			return err
		}

		return tx.SetManifestModel(ctx, mod)
	})
	require.NoError(t, err)

	err = s.View(ctx, func(tx store.Tx) error {
		exists, err := tx.ManifestExists(ctx)
		require.NoError(t, err)
		require.True(t, exists)

		gotMD, err := tx.GetManifestMetadata(ctx)
		require.NoError(t, err)
		require.Equal(t, "manifest-etag-1", gotMD.GetEtag())

		gotBody, err := tx.GetManifestBody(ctx)
		require.NoError(t, err)
		require.Equal(t, []byte("manifest body"), gotBody.GetData())

		gotModel, err := tx.GetManifestModel(ctx)
		require.NoError(t, err)
		require.NotNil(t, gotModel)

		return nil
	})
	require.NoError(t, err)

	// SetManifestModel before SetManifest (no row yet) must fail loudly rather than silently no-op.
	err = s.Update(ctx, func(tx store.Tx) error {
		return tx.SetManifestModel(ctx, mod)
	})
	require.NoError(t, err) // the manifest row from above still exists; this just re-sets the model.

	err = s.Update(ctx, func(tx store.Tx) error {
		return tx.DeleteManifest(ctx)
	})
	require.NoError(t, err)

	err = s.View(ctx, func(tx store.Tx) error {
		exists, err := tx.ManifestExists(ctx)
		require.NoError(t, err)
		require.False(t, exists)

		return nil
	})
	require.NoError(t, err)
}

func TestSetManifestModelWithoutManifestFails(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	err := s.Update(ctx, func(tx store.Tx) error {
		return tx.SetManifestModel(ctx, &model.Model{})
	})
	require.Error(t, err)
	require.ErrorIs(t, err, pgdb.ErrNotFound, "setting a model with no prior SetManifest call should fail loudly")
}
