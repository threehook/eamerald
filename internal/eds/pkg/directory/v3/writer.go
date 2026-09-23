package v3

import (
	"context"

	"github.com/aserto-dev/azm/cache"
	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	dsw "github.com/aserto-dev/go-directory/aserto/directory/writer/v3"
	"github.com/aserto-dev/go-directory/pkg/derr"
	"github.com/aserto-dev/go-directory/pkg/validator"
	"github.com/threehook/eamerald/internal/eds/pkg/ds"
	"github.com/threehook/eamerald/internal/eds/pkg/store"

	"github.com/go-http-utils/headers"
	"github.com/grpc-ecosystem/go-grpc-middleware/util/metautils"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Writer struct {
	logger *zerolog.Logger
	store  store.Store
	mc     *cache.Cache
}

var _ dsw.WriterServer = (*Writer)(nil)

func NewWriter(logger *zerolog.Logger, store store.Store, mc *cache.Cache) *Writer {
	return &Writer{
		logger: logger,
		store:  store,
		mc:     mc,
	}
}

// SetObject.
func (s *Writer) SetObject(ctx context.Context, req *dsw.SetObjectRequest) (*dsw.SetObjectResponse, error) {
	resp := &dsw.SetObjectResponse{}

	if err := validator.SetObjectRequest(req); err != nil {
		return resp, err
	}

	obj := ds.Object(req.GetObject())
	if err := obj.Validate(s.mc); err != nil {
		// The object violates the model.
		return resp, err
	}

	etag := obj.Hash()

	err := s.store.Update(ctx, func(tx store.Tx) error {
		updObj, err := ds.UpdateMetadataObject(ctx, tx, req.GetObject())
		if err != nil {
			return err
		}

		// optimistic concurrency check
		ifMatchHeader := metautils.ExtractIncoming(ctx).Get(headers.IfMatch)
		// if the updReq.Etag == "" this means the this is an insert
		if ifMatchHeader != "" && updObj.GetEtag() != "" && ifMatchHeader != updObj.GetEtag() {
			return derr.ErrHashMismatch.Msgf("for object with type [%s] and id [%s]", updObj.GetType(), updObj.GetId())
		}

		if etag == updObj.GetEtag() {
			s.logger.Trace().Bytes("key", ds.Object(req.GetObject()).Key()).Str("etag-equal", etag).Msg("set_object")

			resp.Result = updObj

			return nil
		}

		updObj.Etag = etag

		objType, err := tx.SetObject(ctx, updObj)
		if err != nil {
			return err
		}

		resp.Result = objType

		return nil
	})

	return resp, err
}

func (s *Writer) DeleteObject(ctx context.Context, req *dsw.DeleteObjectRequest) (*dsw.DeleteObjectResponse, error) {
	resp := &dsw.DeleteObjectResponse{}

	if err := validator.DeleteObjectRequest(req); err != nil {
		return resp, err
	}

	objIdent := ds.ObjectIdentifier(&dsc.ObjectIdentifier{ObjectType: req.GetObjectType(), ObjectId: req.GetObjectId()})

	if err := objIdent.Validate(s.mc); err != nil {
		return resp, err
	}

	err := s.store.Update(ctx, func(tx store.Tx) error {
		objIdent := ds.ObjectIdentifier(&dsc.ObjectIdentifier{ObjectType: req.GetObjectType(), ObjectId: req.GetObjectId()})

		// optimistic concurrency check
		ifMatchHeader := metautils.ExtractIncoming(ctx).Get(headers.IfMatch)
		if ifMatchHeader != "" {
			obj := &dsc.Object{Type: req.GetObjectType(), Id: req.GetObjectId()}

			updObj, err := ds.UpdateMetadataObject(ctx, tx, obj)
			if err != nil {
				return err
			}

			if ifMatchHeader != updObj.GetEtag() {
				return derr.ErrHashMismatch.Msgf("for object with type [%s] and id [%s]", updObj.GetType(), updObj.GetId())
			}
		}

		if err := tx.DeleteObject(ctx, req.GetObjectType(), req.GetObjectId()); err != nil {
			return err
		}

		if req.GetWithRelations() {
			// incoming object relations of object instance (result.type == incoming.subject.type && result.key == incoming.subject.key)
			if err := deleteRelationsWithPrefix(ctx, tx, store.BySubject, objIdent.GetObjectType(), objIdent.GetObjectId()); err != nil {
				return err
			}
			// outgoing object relations of object instance (result.type == outgoing.object.type && result.key == outgoing.object.key)
			if err := deleteRelationsWithPrefix(ctx, tx, store.ByObject, objIdent.GetObjectType(), objIdent.GetObjectId()); err != nil {
				return err
			}
		}

		resp.Result = &emptypb.Empty{}

		return nil
	})

	return resp, err
}

// SetRelation.
func (s *Writer) SetRelation(ctx context.Context, req *dsw.SetRelationRequest) (*dsw.SetRelationResponse, error) {
	resp := &dsw.SetRelationResponse{}

	if err := validator.SetRelationRequest(req); err != nil {
		return resp, err
	}

	relation := ds.Relation(req.GetRelation())
	if err := relation.Validate(s.mc); err != nil {
		return resp, err
	}

	etag := relation.Hash()

	err := s.store.Update(ctx, func(tx store.Tx) error {
		updRel, err := ds.UpdateMetadataRelation(ctx, tx, req.GetRelation())
		if err != nil {
			return err
		}

		// optimistic concurrency check
		ifMatchHeader := metautils.ExtractIncoming(ctx).Get(headers.IfMatch)
		// if the updReq.Etag == "" this means the this is an insert
		if ifMatchHeader != "" && updRel.GetEtag() != "" && ifMatchHeader != updRel.GetEtag() {
			return derr.ErrHashMismatch.Msgf(
				"for relation with objectType [%s], objectId [%s], relation [%s], subjectType [%s], SubjectId [%s]",
				updRel.GetObjectType(), updRel.GetObjectId(), updRel.GetRelation(), updRel.GetSubjectType(), updRel.GetSubjectId(),
			)
		}

		if etag == updRel.GetEtag() {
			s.logger.Trace().Bytes("key", ds.Relation(req.GetRelation()).ObjKey()).Str("etag-equal", etag).Msg("set_relation")

			resp.Result = updRel

			return nil
		}

		updRel.Etag = etag

		objRel, err := tx.SetRelation(ctx, updRel)
		if err != nil {
			return err
		}

		resp.Result = objRel

		return nil
	})

	return resp, err
}

func (s *Writer) DeleteRelation(ctx context.Context, req *dsw.DeleteRelationRequest) (*dsw.DeleteRelationResponse, error) {
	resp := &dsw.DeleteRelationResponse{}

	if err := validator.DeleteRelationRequest(req); err != nil {
		return resp, err
	}

	rel := &dsc.Relation{
		ObjectType:      req.GetObjectType(),
		ObjectId:        req.GetObjectId(),
		Relation:        req.GetRelation(),
		SubjectType:     req.GetSubjectType(),
		SubjectId:       req.GetSubjectId(),
		SubjectRelation: req.GetSubjectRelation(),
	}

	rid := ds.Relation(rel)
	if err := rid.Validate(s.mc); err != nil {
		return resp, err
	}

	err := s.store.Update(ctx, func(tx store.Tx) error {
		// optimistic concurrency check
		ifMatchHeader := metautils.ExtractIncoming(ctx).Get(headers.IfMatch)
		if ifMatchHeader != "" {
			updRel, err := ds.UpdateMetadataRelation(ctx, tx, rel)
			if err != nil {
				return err
			}

			if ifMatchHeader != updRel.GetEtag() {
				return derr.ErrHashMismatch.Msgf(
					"for relation with objectType [%s], objectId [%s], relation [%s], subjectType [%s], SubjectId [%s]",
					rel.GetObjectType(), rel.GetObjectId(), rel.GetRelation(), rel.GetSubjectType(), rel.GetSubjectId(),
				)
			}
		}

		if err := tx.DeleteRelation(ctx, &dsc.RelationIdentifier{
			ObjectType:      req.GetObjectType(),
			ObjectId:        req.GetObjectId(),
			Relation:        req.GetRelation(),
			SubjectType:     req.GetSubjectType(),
			SubjectId:       req.GetSubjectId(),
			SubjectRelation: req.GetSubjectRelation(),
		}); err != nil {
			return err
		}

		resp.Result = &emptypb.Empty{}

		return nil
	})

	return resp, err
}
