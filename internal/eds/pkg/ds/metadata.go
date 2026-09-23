package ds

import (
	"context"
	"time"

	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	"github.com/threehook/eamerald/internal/eds/pkg/store"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func UpdateMetadataObject(ctx context.Context, tx store.Tx, msg *dsc.Object) (*dsc.Object, error) {
	// get timestamp once for transaction.
	ts := timestamppb.New(time.Now().UTC())

	// get current instance.
	cur, err := tx.GetObject(ctx, msg.GetType(), msg.GetId())

	switch {
	case status.Code(err) == codes.NotFound:
		// new instance, set created_at timestamp.
		msg.CreatedAt = ts //nolint:staticcheck
		// if new instance set Etag to empty string.
		msg.Etag = ""

	case err != nil:
		return nil, err
	default:
		// existing instance, propagate created_at timestamp.
		msg.CreatedAt = cur.GetCreatedAt() //nolint:staticcheck
	}

	// always set updated_at timestamp.
	msg.UpdatedAt = ts

	if cur.GetEtag() != "" {
		msg.Etag = cur.GetEtag()
	}

	return msg, nil
}

func UpdateMetadataRelation(ctx context.Context, tx store.Tx, msg *dsc.Relation) (*dsc.Relation, error) {
	// get timestamp once for transaction.
	ts := timestamppb.New(time.Now().UTC())

	// get current instance.
	cur, err := tx.GetRelationExact(ctx, &dsc.RelationIdentifier{
		ObjectType:      msg.GetObjectType(),
		ObjectId:        msg.GetObjectId(),
		Relation:        msg.GetRelation(),
		SubjectType:     msg.GetSubjectType(),
		SubjectId:       msg.GetSubjectId(),
		SubjectRelation: msg.GetSubjectRelation(),
	})

	switch {
	case status.Code(err) == codes.NotFound:
		// new instance, set created_at timestamp.
		msg.CreatedAt = ts //nolint:staticcheck
		// if new instance set Etag to empty string.
		msg.Etag = ""

	case err != nil:
		return nil, err
	default:
		// existing instance, propagate created_at timestamp.
		msg.CreatedAt = cur.GetCreatedAt() //nolint:staticcheck
	}

	// always set updated_at timestamp.
	msg.UpdatedAt = ts

	if cur.GetEtag() != "" {
		msg.Etag = cur.GetEtag()
	}

	return msg, nil
}
