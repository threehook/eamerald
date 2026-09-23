package v3

import (
	"context"

	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	"github.com/threehook/eamerald/internal/eds/pkg/store"
)

// deleteRelationsWithPrefix deletes every relation whose object/subject key (as used in the given direction) has objectType:objectID as a prefix,
// mirroring the original ObjFilter/SubFilter-style scan (type:id followed by a separator). Shared by Writer.DeleteObject and Importer's
// delete-with-relations handler.
//
// The matches are collected before any delete runs, rather than deleting row-by-row while the scan is still open: a
// SQL-backed Tx can't issue a write on the same connection while a query's result set is still being read.
func deleteRelationsWithPrefix(ctx context.Context, tx store.Tx, dir store.Direction, objectType, objectID string) error {
	relations, err := relationsByObject(ctx, tx, dir, objectType, objectID)
	if err != nil {
		return err
	}

	for _, rel := range relations {
		if err := tx.DeleteRelation(ctx, &dsc.RelationIdentifier{
			ObjectType:      rel.GetObjectType(),
			ObjectId:        rel.GetObjectId(),
			Relation:        rel.GetRelation(),
			SubjectType:     rel.GetSubjectType(),
			SubjectId:       rel.GetSubjectId(),
			SubjectRelation: rel.GetSubjectRelation(),
		}); err != nil {
			return err
		}
	}

	return nil
}

// relationsByObject collects every relation whose object/subject key (as used in the given direction) names objectType:objectID, using the same
// strict, separator-terminated prefix as deleteRelationsWithPrefix.
func relationsByObject(ctx context.Context, tx store.Tx, dir store.Direction, objectType, objectID string) ([]*dsc.Relation, error) {
	iter, err := tx.ScanRelations(ctx, dir, store.ObjectOnlyFilter(dir, objectType, objectID), "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var relations []*dsc.Relation
	for iter.Next() {
		relations = append(relations, iter.Value())
	}

	return relations, nil
}
