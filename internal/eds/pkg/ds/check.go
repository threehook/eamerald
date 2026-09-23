package ds

import (
	"context"

	"github.com/aserto-dev/azm/cache"
	"github.com/aserto-dev/azm/graph"
	"github.com/aserto-dev/azm/safe"
	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	dsr "github.com/aserto-dev/go-directory/aserto/directory/reader/v3"
	"github.com/aserto-dev/go-directory/pkg/derr"
	"github.com/aserto-dev/go-directory/pkg/prop"
	"github.com/threehook/eamerald/internal/eds/pkg/store"

	"google.golang.org/protobuf/types/known/structpb"
)

type check struct {
	*safe.SafeCheck
}

func Check(i *dsr.CheckRequest) *check {
	return &check{safe.Check(i)}
}

func (i *check) Exec(ctx context.Context, tx store.Tx, mc *cache.Cache) (*dsr.CheckResponse, error) {
	if err := i.RelationIdentifiersExist(ctx, tx); err != nil {
		return &dsr.CheckResponse{
			Check:   false,
			Context: SetContextWithReason(err),
		}, err
	}

	return mc.Check(i.CheckRequest, getRelations(ctx, tx))
}

func getRelations(ctx context.Context, tx store.Tx) graph.RelationReader {
	return func(r *dsc.RelationIdentifier, pool graph.RelationPool, out *[]*dsc.RelationIdentifier) error {
		dir, filter, valueFilter := RelationIdentifier(r).Filter()

		return tx.ScanRelationsFiltered(ctx, dir, filter, valueFilter, pool, out)
	}
}

func (i *check) RelationIdentifiersExist(ctx context.Context, tx store.Tx) error {
	if !i.relationIdentifierExist(ctx, tx, store.BySubject, i.SubjectType, i.SubjectId) {
		return derr.ErrObjectNotFound.Msgf("subject %s:%s", i.SubjectType, i.SubjectId)
	}

	if !i.relationIdentifierExist(ctx, tx, store.ByObject, i.ObjectType, i.ObjectId) {
		return derr.ErrObjectNotFound.Msgf("object %s:%s", i.ObjectType, i.ObjectId)
	}

	return nil
}

func (*check) relationIdentifierExist(ctx context.Context, tx store.Tx, dir store.Direction, objectType, objectID string) bool {
	exists, err := tx.RelationsExistForObject(ctx, dir, objectType, objectID)
	if err != nil {
		return false
	}

	return exists
}

func SetContextWithReason(err error) *structpb.Struct {
	return &structpb.Struct{
		Fields: map[string]*structpb.Value{
			prop.Reason: structpb.NewStringValue(err.Error()),
		},
	}
}
