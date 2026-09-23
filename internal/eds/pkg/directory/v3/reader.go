package v3

import (
	"context"

	"github.com/aserto-dev/azm/cache"
	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	dsr "github.com/aserto-dev/go-directory/aserto/directory/reader/v3"
	"github.com/aserto-dev/go-directory/pkg/validator"
	"github.com/pkg/errors"
	"github.com/threehook/eamerald/internal/eds/pkg/bdb"
	"github.com/threehook/eamerald/internal/eds/pkg/ds"
	"github.com/threehook/eamerald/internal/eds/pkg/store"
	"github.com/threehook/eamerald/internal/eds/pkg/x"

	"github.com/go-http-utils/headers"
	"github.com/rs/zerolog"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Reader struct {
	logger *zerolog.Logger
	store  store.Store
	mc     *cache.Cache
}

var _ dsr.ReaderServer = (*Reader)(nil)

func NewReader(logger *zerolog.Logger, store store.Store, mc *cache.Cache) *Reader {
	return &Reader{
		logger: logger,
		store:  store,
		mc:     mc,
	}
}

// GetObject, get single object instance.
func (s *Reader) GetObject(ctx context.Context, req *dsr.GetObjectRequest) (*dsr.GetObjectResponse, error) {
	resp := &dsr.GetObjectResponse{}

	if err := validator.GetObjectRequest(req); err != nil {
		return resp, err
	}

	objIdent := ds.ObjectIdentifier(&dsc.ObjectIdentifier{ObjectType: req.GetObjectType(), ObjectId: req.GetObjectId()})
	if err := objIdent.Validate(s.mc); err != nil {
		return resp, err
	}

	err := s.store.View(ctx, func(tx store.Tx) error {
		obj, err := tx.GetObject(ctx, req.GetObjectType(), req.GetObjectId())
		if err != nil {
			return err
		}

		inMD, _ := metadata.FromIncomingContext(ctx)
		// optimistic concurrency check
		if lo.Contains(inMD.Get(headers.IfNoneMatch), obj.GetEtag()) {
			_ = grpc.SetHeader(ctx, metadata.Pairs("x-http-code", "304"))

			return nil
		}

		if req.GetWithRelations() {
			// incoming object relations of object instance (result.type == incoming.subject.type && result.key == incoming.subject.key)
			incoming, err := relationsByObject(ctx, tx, store.BySubject, obj.GetType(), obj.GetId())
			if err != nil {
				return err
			}

			resp.Relations = append(resp.Relations, incoming...)

			// outgoing object relations of object instance (result.type == outgoing.object.type && result.key == outgoing.object.key)
			outgoing, err := relationsByObject(ctx, tx, store.ByObject, obj.GetType(), obj.GetId())
			if err != nil {
				return err
			}

			resp.Relations = append(resp.Relations, outgoing...)

			s.logger.Trace().Msg("get object with relations")
		}

		resp.Result = ds.PatchObjectRead(obj)

		return nil
	})

	return resp, err
}

// GetObjectMany, get multiple object instances by type+id, in a single request.
func (s *Reader) GetObjectMany(ctx context.Context, req *dsr.GetObjectManyRequest) (*dsr.GetObjectManyResponse, error) {
	resp := &dsr.GetObjectManyResponse{Results: []*dsc.Object{}}

	if err := validator.GetObjectManyRequest(req); err != nil {
		return resp, err
	}

	// validate all object identifiers first.
	for _, i := range req.GetParam() {
		if err := ds.ObjectIdentifier(i).Validate(s.mc); err != nil {
			return resp, err
		}
	}

	err := s.store.View(ctx, func(tx store.Tx) error {
		for _, i := range req.GetParam() {
			obj, err := tx.GetObject(ctx, i.GetObjectType(), i.GetObjectId())
			if err != nil {
				return err
			}

			resp.Results = append(resp.GetResults(), ds.PatchObjectRead(obj))
		}

		return nil
	})

	return resp, err
}

// GetObjects, gets (all) object instances, optionally filtered by object type, as a paginated array of objects.
func (s *Reader) GetObjects(ctx context.Context, req *dsr.GetObjectsRequest) (*dsr.GetObjectsResponse, error) {
	resp := &dsr.GetObjectsResponse{Results: []*dsc.Object{}, Page: &dsc.PaginationResponse{}}

	if err := validator.GetObjectsRequest(req); err != nil {
		return resp, err
	}

	if req.GetPage() == nil {
		req.Page = &dsc.PaginationRequest{Size: x.MaxPageSize}
	}

	objectType := ""

	if req.GetObjectType() != "" {
		oid := ds.ObjectIdentifier(&dsc.ObjectIdentifier{ObjectType: req.GetObjectType()})
		if err := ds.ObjectSelector(oid.ObjectIdentifier).Validate(s.mc); err != nil {
			return resp, err
		}

		objectType = req.GetObjectType()
	}

	err := s.store.View(ctx, func(tx store.Tx) error {
		iter, err := tx.ScanObjects(ctx, objectType, req.GetPage().GetToken())
		if err != nil {
			return err
		}
		defer iter.Close()

		values, nextToken := store.Page(iter, req.GetPage().GetSize())

		resp.Results = lo.Map(values, func(x *dsc.Object, _ int) *dsc.Object {
			return ds.PatchObjectRead(x)
		})

		resp.Page = &dsc.PaginationResponse{NextToken: nextToken}

		return nil
	})

	return resp, err
}

// GetRelation, get a single relation instance based on subject, relation, object filter.
func (s *Reader) GetRelation(ctx context.Context, req *dsr.GetRelationRequest) (*dsr.GetRelationResponse, error) {
	resp := &dsr.GetRelationResponse{
		Result:  &dsc.Relation{},
		Objects: map[string]*dsc.Object{},
	}

	if err := validator.GetRelationRequest(req); err != nil {
		return resp, err
	}

	getRelation := ds.GetRelation(req)
	if err := getRelation.Validate(s.mc); err != nil {
		return resp, err
	}

	dir, filter, err := getRelation.PathAndFilter()
	if err != nil {
		return resp, err
	}

	err = s.store.View(ctx, func(tx store.Tx) error {
		iter, err := tx.ScanRelations(ctx, dir, filter, "")
		if err != nil {
			return err
		}
		defer iter.Close()

		var relations []*dsc.Relation
		for iter.Next() {
			relations = append(relations, iter.Value())
		}

		if len(relations) == 0 {
			return bdb.ErrKeyNotFound
		}

		if len(relations) != 1 {
			return bdb.ErrMultipleResults
		}

		dbRel := relations[0]
		resp.Result = dbRel

		inMD, _ := metadata.FromIncomingContext(ctx)
		if lo.Contains(inMD.Get(headers.IfNoneMatch), dbRel.GetEtag()) {
			_ = grpc.SetHeader(ctx, metadata.Pairs("x-http-code", "304"))

			return nil
		}

		if req.GetWithObjects() {
			relations := []*dsc.Relation{resp.GetResult()}
			resp.Objects = s.getWithObjects(ctx, tx, relations)
		}

		return nil
	})

	return resp, err
}

// GetRelations, gets paginated set of relation instances based on subject, relation, object filter.
func (s *Reader) GetRelations(ctx context.Context, req *dsr.GetRelationsRequest) (*dsr.GetRelationsResponse, error) {
	resp := &dsr.GetRelationsResponse{
		Results: []*dsc.Relation{},
		Objects: map[string]*dsc.Object{},
		Page:    &dsc.PaginationResponse{},
	}

	if err := validator.GetRelationsRequest(req); err != nil {
		return resp, err
	}

	if req.GetPage() == nil {
		req.Page = &dsc.PaginationRequest{Size: x.MaxPageSize}
	}

	getRelations := ds.GetRelations(req)
	if err := getRelations.Validate(s.mc); err != nil {
		return resp, err
	}

	dir, filter, valueFilter := getRelations.RelationValueFilter()

	err := s.store.View(ctx, func(tx store.Tx) error {
		iter, err := tx.ScanRelations(ctx, dir, filter, req.GetPage().GetToken())
		if err != nil {
			return err
		}
		defer iter.Close()

		for iter.Next() {
			if !valueFilter(iter.Value()) {
				continue
			}

			resp.Results = append(resp.GetResults(), iter.Value())

			if int64(req.GetPage().GetSize()) == int64(len(resp.GetResults())) {
				if iter.Next() {
					resp.Page.NextToken = iter.Token()
				}

				break
			}
		}

		// Close before any further query on tx: a SQL-backed Tx can't issue a new query while this scan's
		// result set is still open, even mid-transaction.
		if err := iter.Close(); err != nil {
			return err
		}

		if req.GetWithObjects() {
			resp.Objects = s.getWithObjects(ctx, tx, resp.GetResults())
		}

		return nil
	})

	return resp, err
}

// Check, if subject is permitted to access resource (object).
func (s *Reader) Check(ctx context.Context, req *dsr.CheckRequest) (*dsr.CheckResponse, error) {
	resp := &dsr.CheckResponse{}

	if err := validator.CheckRequest(req); err != nil {
		resp.Check = false
		resp.Context = ds.SetContextWithReason(err)

		return resp, nil
	}

	check := ds.Check(req)
	if err := check.Validate(s.mc); err != nil {
		resp.Check = false

		if err := errors.Unwrap(err); err != nil {
			resp.Context = ds.SetContextWithReason(err)
			return resp, nil
		}

		resp.Context = ds.SetContextWithReason(err)

		return resp, nil
	}

	err := s.store.View(ctx, func(tx store.Tx) error {
		var err error

		resp, err = check.Exec(ctx, tx, s.mc)

		return err
	})
	if err != nil {
		resp.Context = ds.SetContextWithReason(err)
	}

	return resp, nil
}

// Checks, execute multiple check requests in parallel.
func (s *Reader) Checks(ctx context.Context, req *dsr.ChecksRequest) (*dsr.ChecksResponse, error) {
	resp := &dsr.ChecksResponse{}

	checks := ds.Checks(req)
	if err := checks.Validate(s.mc); err != nil {
		return resp, err
	}

	err := s.store.View(ctx, func(tx store.Tx) error {
		var err error

		resp, err = checks.Exec(ctx, tx, s.mc)

		return err
	})
	if err != nil {
		return resp, err
	}

	return resp, nil
}

// CheckPermission is obsolete, use Check instead.
func (s *Reader) CheckPermission(_ context.Context, _ *dsr.CheckPermissionRequest) (*dsr.CheckPermissionResponse, error) {
	return &dsr.CheckPermissionResponse{}, status.Error(codes.Unimplemented, "check permission is obsolete, use check instead")
}

// CheckRelation is obsolete, use Check instead.
func (s *Reader) CheckRelation(_ context.Context, _ *dsr.CheckRelationRequest) (*dsr.CheckRelationResponse, error) {
	return &dsr.CheckRelationResponse{}, status.Error(codes.Unimplemented, "check relation is obsolete, use check instead")
}

// GetGraph, return graph of connected objects and relations for requested anchor subject/object.
func (s *Reader) GetGraph(ctx context.Context, req *dsr.GetGraphRequest) (*dsr.GetGraphResponse, error) {
	resp := &dsr.GetGraphResponse{}

	if err := validator.GetGraphRequest(req); err != nil {
		return &dsr.GetGraphResponse{}, err
	}

	getGraph := ds.GetGraph(req)
	if err := getGraph.Validate(s.mc); err != nil {
		return resp, err
	}

	err := s.store.View(ctx, func(tx store.Tx) error {
		var err error

		results, err := getGraph.Exec(ctx, tx, s.mc)
		if err != nil {
			return err
		}

		resp = results

		return nil
	})

	return resp, err
}

func (*Reader) getWithObjects(ctx context.Context, tx store.Tx, relations []*dsc.Relation) map[string]*dsc.Object {
	objects := map[string]*dsc.Object{}

	for _, r := range relations {
		rel := ds.Relation(r)

		sub, err := tx.GetObject(ctx, rel.GetSubjectType(), rel.GetSubjectId())
		if err != nil {
			sub = &dsc.Object{Type: rel.GetSubjectType(), Id: rel.GetSubjectId()}
		}

		objects[ds.Object(sub).StrKey()] = sub

		obj, err := tx.GetObject(ctx, rel.GetObjectType(), rel.GetObjectId())
		if err != nil {
			obj = &dsc.Object{Type: rel.GetObjectType(), Id: rel.GetObjectId()}
		}

		objects[ds.Object(obj).StrKey()] = obj
	}

	return objects
}
