package ds

// model contains relation related items.

import (
	"bytes"
	"strings"

	"github.com/aserto-dev/azm/safe"
	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	dsr "github.com/aserto-dev/go-directory/aserto/directory/reader/v3"
	"github.com/threehook/eamerald/internal/eds/pkg/store"
)

const maxRelationIdentifierSize = 384

// Relation identifier.
type relation struct {
	*safe.SafeRelation
}

// Relation selector.
type relations struct {
	*safe.SafeRelations // implements Validate
	relation            // implements Filter
}

func Relation(i *dsc.Relation) *relation {
	return &relation{safe.Relation(&dsc.RelationIdentifier{
		ObjectType:      i.GetObjectType(),
		ObjectId:        i.GetObjectId(),
		Relation:        i.GetRelation(),
		SubjectType:     i.GetSubjectType(),
		SubjectId:       i.GetSubjectId(),
		SubjectRelation: i.GetSubjectRelation(),
	})}
}

func RelationIdentifier(i *dsc.RelationIdentifier) *relation {
	return &relation{&safe.SafeRelation{
		RelationIdentifier: i,
		HasSubjectRelation: i.GetSubjectRelation() != "",
	}}
}

func GetRelation(i *dsr.GetRelationRequest) *relations {
	r := safe.GetRelation(i)
	return &relations{r, relation{r.SafeRelation}}
}

func GetRelations(i *dsr.GetRelationsRequest) *relations {
	r := safe.GetRelations(i)
	return &relations{r, relation{r.SafeRelation}}
}

func (i *relation) Key() []byte {
	return i.ObjKey()
}

func (i *relation) ObjKey() []byte {
	buf := newRelationBuffer()

	buf.WriteString(i.GetObjectType())
	buf.WriteByte(TypeIDSeparator)
	buf.WriteString(i.GetObjectId())

	buf.WriteByte(InstanceSeparator)
	buf.WriteString(i.GetRelation())
	buf.WriteByte(InstanceSeparator)

	buf.WriteString(i.GetSubjectType())
	buf.WriteByte(TypeIDSeparator)
	buf.WriteString(i.GetSubjectId())

	if i.GetSubjectRelation() != "" {
		buf.WriteByte(InstanceSeparator)
		buf.WriteString(i.GetSubjectRelation())
	}

	return buf.Bytes()
}

func (i *relation) SubKey() []byte {
	buf := newRelationBuffer()

	buf.WriteString(i.GetSubjectType())
	buf.WriteByte(TypeIDSeparator)
	buf.WriteString(i.GetSubjectId())

	buf.WriteByte(InstanceSeparator)
	buf.WriteString(i.GetRelation())
	buf.WriteByte(InstanceSeparator)

	buf.WriteString(i.GetObjectType())
	buf.WriteByte(TypeIDSeparator)
	buf.WriteString(i.GetObjectId())

	if i.GetSubjectRelation() != "" {
		buf.WriteByte(InstanceSeparator)
		buf.WriteString(i.GetSubjectRelation())
	}

	return buf.Bytes()
}

// PathAndFilter returns the direction and filter for a singleton relation lookup (GetRelation), erroring when neither
// the object nor the subject identifier is complete.
func (i *relation) PathAndFilter() (store.Direction, store.RelationFilter, error) {
	dir, ok := i.direction()
	if !ok {
		return dir, store.RelationFilter{}, ErrNoCompleteObjectIdentifier
	}

	return dir, i.asFilter(), nil
}

// Filter returns the direction, filter and value-filter closure for the check/graph hot path (RelationReader), when
// neither the object nor the subject identifier is complete it falls back to an unfiltered object-indexed scan,
// matched entirely by the value filter.
func (i *relation) Filter() (store.Direction, store.RelationFilter, func(*dsc.RelationIdentifier) bool) {
	dir, _ := i.direction()
	return dir, i.asFilter(), i.identifierValueFilter()
}

// RelationValueFilter returns the direction, filter and value-filter closure for a paginated relation scan
// (GetRelations), with the same fallback as Filter.
func (i *relation) RelationValueFilter() (store.Direction, store.RelationFilter, func(*dsc.Relation) bool) {
	dir, _ := i.direction()
	return dir, i.asFilter(), i.relationValueFilter()
}

// relLike is satisfied by both *dsc.Relation and *dsc.RelationIdentifier, which share this field-getter shape.
type relLike interface {
	GetObjectType() string
	GetObjectId() string
	GetRelation() string
	GetSubjectType() string
	GetSubjectId() string
	GetSubjectRelation() string
}

// valueFilter builds a closure that accepts an item iff every field i has set matches the corresponding field on item;
// shared by identifierValueFilter (candidate *dsc.RelationIdentifier rows) and relationValueFilter (scanned *dsc.Relation rows).
func valueFilter[T relLike](i *relation) func(T) bool {
	return func(item T) bool {
		if fv := i.GetObjectType(); fv != "" && strings.Compare(item.GetObjectType(), fv) != 0 {
			return false
		}

		if fv := i.GetObjectId(); fv != "" && strings.Compare(fv, item.GetObjectId()) != 0 {
			return false
		}

		if fv := i.GetRelation(); fv != "" && strings.Compare(item.GetRelation(), fv) != 0 {
			return false
		}

		if fv := i.GetSubjectType(); fv != "" && strings.Compare(item.GetSubjectType(), fv) != 0 {
			return false
		}

		if fv := i.GetSubjectId(); fv != "" && strings.Compare(fv, item.GetSubjectId()) != 0 {
			return false
		}

		if i.HasSubjectRelation && strings.Compare(item.GetSubjectRelation(), i.GetSubjectRelation()) != 0 {
			return false
		}

		return true
	}
}

func (i *relation) identifierValueFilter() func(*dsc.RelationIdentifier) bool {
	return valueFilter[*dsc.RelationIdentifier](i)
}

func (i *relation) relationValueFilter() func(*dsc.Relation) bool {
	return valueFilter[*dsc.Relation](i)
}

// direction picks which index (object- or subject-indexed) a scan should be anchored on: object identifier first,
// subject identifier second, falling back to a full, unfiltered object-indexed scan when neither is complete (ok
// reports whether one of the two was actually complete).
func (i *relation) direction() (store.Direction, bool) {
	switch {
	case ObjectIdentifier(i.Object()).IsComplete():
		return store.ByObject, true
	case ObjectIdentifier(i.Subject()).IsComplete():
		return store.BySubject, true
	default:
		return store.ByObject, false
	}
}

func (i *relation) asFilter() store.RelationFilter {
	return store.RelationFilter{
		ObjectType:         i.GetObjectType(),
		ObjectID:           i.GetObjectId(),
		Relation:           i.GetRelation(),
		SubjectType:        i.GetSubjectType(),
		SubjectID:          i.GetSubjectId(),
		SubjectRelation:    i.GetSubjectRelation(),
		HasSubjectRelation: i.HasSubjectRelation,
	}
}

func newRelationBuffer() *bytes.Buffer {
	return bytes.NewBuffer(make([]byte, 0, maxRelationIdentifierSize))
}
