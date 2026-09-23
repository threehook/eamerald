package bdb

import (
	"bytes"
	"context"
	"strings"

	"github.com/aserto-dev/azm/graph"
	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	"github.com/threehook/eamerald/internal/eds/pkg/store"
)

// isSet mirrors ds.IsSet (and azm/safe's IsSet): a field counts as set only if it is non-empty once trimmed.
func isSet(s string) bool {
	return strings.TrimSpace(s) != ""
}

// relationIdent is satisfied structurally by both *dsc.Relation and *dsc.RelationIdentifier, which share this field-getter shape.
type relationIdent interface {
	GetObjectType() string
	GetObjectId() string
	GetRelation() string
	GetSubjectType() string
	GetSubjectId() string
	GetSubjectRelation() string
}

// writeObjKey/writeSubKey build the object-indexed / subject-indexed primary key for a fully specified relation:
//
//	obj:  obj_type:obj_id|relation|sub_type:sub_id(|sub_relation)
//	sub:  sub_type:sub_id|relation|obj_type:obj_id(|sub_relation)
func writeObjKey(buf *bytes.Buffer, r relationIdent) {
	buf.WriteString(r.GetObjectType())
	buf.WriteByte(typeIDSeparator)
	buf.WriteString(r.GetObjectId())
	buf.WriteByte(instanceSeparator)
	buf.WriteString(r.GetRelation())
	buf.WriteByte(instanceSeparator)
	buf.WriteString(r.GetSubjectType())
	buf.WriteByte(typeIDSeparator)
	buf.WriteString(r.GetSubjectId())

	if sr := r.GetSubjectRelation(); sr != "" {
		buf.WriteByte(instanceSeparator)
		buf.WriteString(sr)
	}
}

func writeSubKey(buf *bytes.Buffer, r relationIdent) {
	buf.WriteString(r.GetSubjectType())
	buf.WriteByte(typeIDSeparator)
	buf.WriteString(r.GetSubjectId())
	buf.WriteByte(instanceSeparator)
	buf.WriteString(r.GetRelation())
	buf.WriteByte(instanceSeparator)
	buf.WriteString(r.GetObjectType())
	buf.WriteByte(typeIDSeparator)
	buf.WriteString(r.GetObjectId())

	if sr := r.GetSubjectRelation(); sr != "" {
		buf.WriteByte(instanceSeparator)
		buf.WriteString(sr)
	}
}

func objKeyOf(r relationIdent) []byte {
	var buf bytes.Buffer

	writeObjKey(&buf, r)

	return buf.Bytes()
}

func subKeyOf(r relationIdent) []byte {
	var buf bytes.Buffer

	writeSubKey(&buf, r)

	return buf.Bytes()
}

// writeObjFilter/writeSubFilter write a prefix matching filter's populated leading fields, in the same field order as writeObjKey/writeSubKey.
// They write nothing when the leading type+id pair is not fully set, matching the original ObjFilter/SubFilter's "only called when complete"
// precondition: an empty prefix means "scan everything" to the caller.
//
// format: obj_type:obj_id|relation|sub_type:sub_id(|sub_relation).
func writeObjFilter(buf *bytes.Buffer, f store.RelationFilter) {
	if !isSet(f.ObjectType) || !isSet(f.ObjectID) {
		return
	}

	buf.WriteString(f.ObjectType)
	buf.WriteByte(typeIDSeparator)
	buf.WriteString(f.ObjectID)
	buf.WriteByte(instanceSeparator)

	if !isSet(f.Relation) {
		return
	}

	buf.WriteString(f.Relation)
	buf.WriteByte(instanceSeparator)

	if !isSet(f.SubjectType) {
		return
	}

	buf.WriteString(f.SubjectType)
	buf.WriteByte(typeIDSeparator)

	if !isSet(f.SubjectID) {
		return
	}

	buf.WriteString(f.SubjectID)
}

// format: sub_type:sub_id|relation|obj_type:obj_id(|sub_relation).
func writeSubFilter(buf *bytes.Buffer, f store.RelationFilter) {
	if !isSet(f.SubjectType) || !isSet(f.SubjectID) {
		return
	}

	buf.WriteString(f.SubjectType)
	buf.WriteByte(typeIDSeparator)
	buf.WriteString(f.SubjectID)
	buf.WriteByte(instanceSeparator)

	if !isSet(f.Relation) {
		return
	}

	buf.WriteString(f.Relation)
	buf.WriteByte(instanceSeparator)

	if !isSet(f.ObjectType) {
		return
	}

	buf.WriteString(f.ObjectType)
	buf.WriteByte(typeIDSeparator)

	if !isSet(f.ObjectID) {
		return
	}

	buf.WriteString(f.ObjectID)
}

func directionPath(dir store.Direction) Path {
	if dir == store.BySubject {
		return RelationsSubPath
	}

	return RelationsObjPath
}

func (t *tx) GetRelationExact(ctx context.Context, ident *dsc.RelationIdentifier) (*dsc.Relation, error) {
	return Get[dsc.Relation](ctx, t.tx, RelationsObjPath, objKeyOf(ident))
}

func (t *tx) SetRelation(ctx context.Context, rel *dsc.Relation) (*dsc.Relation, error) {
	objRel, err := Set[dsc.Relation](ctx, t.tx, RelationsObjPath, objKeyOf(rel), rel)
	if err != nil {
		return nil, err
	}

	if _, err := Set[dsc.Relation](ctx, t.tx, RelationsSubPath, subKeyOf(rel), rel); err != nil {
		return nil, err
	}

	return objRel, nil
}

func (t *tx) DeleteRelation(ctx context.Context, ident *dsc.RelationIdentifier) error {
	if err := Delete(ctx, t.tx, RelationsObjPath, objKeyOf(ident)); err != nil {
		return err
	}

	return Delete(ctx, t.tx, RelationsSubPath, subKeyOf(ident))
}

func (t *tx) ScanRelations(
	ctx context.Context, dir store.Direction, filter store.RelationFilter, pageToken string,
) (store.Iterator[*dsc.Relation], error) {
	var buf bytes.Buffer

	if dir == store.BySubject {
		writeSubFilter(&buf, filter)
	} else {
		writeObjFilter(&buf, filter)
	}

	opts := []ScanOption{WithKeyFilter(buf.Bytes())}
	if pageToken != "" {
		opts = append(opts, WithPageToken(pageToken))
	}

	iter, err := NewScanIterator[dsc.Relation](ctx, t.tx, directionPath(dir), opts...)
	if err != nil {
		return nil, err
	}

	return &relationIterator{iter: iter}, nil
}

func (t *tx) ScanRelationsFiltered(
	ctx context.Context, dir store.Direction, filter store.RelationFilter,
	valueFilter func(*dsc.RelationIdentifier) bool, pool graph.RelationPool, out *[]*dsc.RelationIdentifier,
) error {
	buf := relFilterBufPool.Get()
	defer func() {
		buf.Reset()
		relFilterBufPool.Put(buf)
	}()

	if dir == store.BySubject {
		writeSubFilter(buf, filter)
	} else {
		writeObjFilter(buf, filter)
	}

	return ScanWithFilter(ctx, t.tx, directionPath(dir), buf.Bytes(), valueFilter, pool, out)
}

func (t *tx) RelationsExistForObject(ctx context.Context, dir store.Direction, objectType, objectID string) (bool, error) {
	var buf bytes.Buffer

	filter := store.ObjectOnlyFilter(dir, objectType, objectID)
	if dir == store.BySubject {
		writeSubFilter(&buf, filter)
	} else {
		writeObjFilter(&buf, filter)
	}

	return KeyPrefixExists[dsc.Relation](ctx, t.tx, directionPath(dir), buf.Bytes())
}

type relationIterator struct {
	iter *ScanIterator[dsc.Relation, *dsc.Relation]
}

var _ store.Iterator[*dsc.Relation] = (*relationIterator)(nil)

func (r *relationIterator) Next() bool           { return r.iter.Next() }
func (r *relationIterator) Value() *dsc.Relation { return r.iter.Value() }
func (r *relationIterator) Token() string        { return string(r.iter.RawKey()) }

// Close is a no-op: a *bolt.Tx-backed cursor holds no resource beyond the enclosing transaction's lifetime.
func (r *relationIterator) Close() error { return nil }
