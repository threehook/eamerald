package datasync

import (
	"context"

	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	"github.com/aserto-dev/go-directory/pkg/derr"
	"github.com/aserto-dev/go-directory/pkg/validator"
	"github.com/threehook/eamerald/internal/eds/pkg/ds"
	"github.com/threehook/eamerald/internal/eds/pkg/store"
)

//nolint:dupl // structurally mirrors relationSetHandler; Object and Relation share no common interface to unify against.
func (s *Sync) objectSetHandler(ctx context.Context, tx store.Tx, req *dsc.Object) error {
	s.logger.Debug().Interface("object", req).Msg("ImportObject")

	if req == nil {
		return derr.ErrInvalidObject.Msg("nil")
	}

	if err := validator.Object(req); err != nil {
		return err
	}

	obj := ds.Object(req)
	if err := obj.Validate(s.store.MC()); err != nil {
		// The object violates the model.
		return err
	}

	etag := obj.Hash()

	updReq, err := ds.UpdateMetadataObject(ctx, tx, req)
	if err != nil {
		return err
	}

	if etag == updReq.GetEtag() {
		s.logger.Trace().Bytes("key", obj.Key()).Str("etag-equal", etag).Msg("ImportObject")
		return nil
	}

	updReq.Etag = etag

	if _, err := tx.SetObject(ctx, updReq); err != nil {
		return derr.ErrInvalidObject.Msg("set")
	}

	return nil
}

func (s *Sync) objectDeleteHandler(ctx context.Context, tx store.Tx, req *dsc.Object) error {
	s.logger.Debug().Interface("object", req).Msg("ImportObject")

	if req == nil {
		return derr.ErrInvalidObject.Msg("nil")
	}

	if err := validator.Object(req); err != nil {
		return err
	}

	obj := ds.Object(req)
	if err := obj.Validate(s.store.MC()); err != nil {
		return err
	}

	if err := tx.DeleteObject(ctx, req.GetType(), req.GetId()); err != nil {
		return derr.ErrInvalidObject.Msg("delete")
	}

	return nil
}

//nolint:dupl // structurally mirrors objectSetHandler; Object and Relation share no common interface to unify against.
func (s *Sync) relationSetHandler(ctx context.Context, tx store.Tx, req *dsc.Relation) error {
	s.logger.Debug().Interface("relation", req).Msg("ImportRelation")

	if req == nil {
		return derr.ErrInvalidRelation.Msg("nil")
	}

	if err := validator.Relation(req); err != nil {
		return err
	}

	rel := ds.Relation(req)
	if err := rel.Validate(s.store.MC()); err != nil {
		return err
	}

	etag := rel.Hash()

	updReq, err := ds.UpdateMetadataRelation(ctx, tx, req)
	if err != nil {
		return err
	}

	if etag == updReq.GetEtag() {
		s.logger.Trace().Bytes("key", rel.ObjKey()).Str("etag-equal", etag).Msg("ImportRelation")
		return nil
	}

	updReq.Etag = etag

	if _, err := tx.SetRelation(ctx, updReq); err != nil {
		return derr.ErrInvalidRelation.Msg("set")
	}

	return nil
}

func (s *Sync) relationDeleteHandler(ctx context.Context, tx store.Tx, req *dsc.Relation) error {
	s.logger.Debug().Interface("relation", req).Msg("ImportRelation")

	if req == nil {
		return derr.ErrInvalidRelation.Msg("nil")
	}

	if err := validator.Relation(req); err != nil {
		return err
	}

	rel := ds.Relation(req)
	if err := rel.Validate(s.store.MC()); err != nil {
		return err
	}

	if err := tx.DeleteRelation(ctx, &dsc.RelationIdentifier{
		ObjectType:      req.GetObjectType(),
		ObjectId:        req.GetObjectId(),
		Relation:        req.GetRelation(),
		SubjectType:     req.GetSubjectType(),
		SubjectId:       req.GetSubjectId(),
		SubjectRelation: req.GetSubjectRelation(),
	}); err != nil {
		return derr.ErrInvalidRelation.Msg("delete")
	}

	return nil
}
