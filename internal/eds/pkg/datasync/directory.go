package datasync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	dse "github.com/aserto-dev/go-directory/aserto/directory/exporter/v3"
	"github.com/threehook/eamerald/internal/eds/pkg/store"

	cuckoo "github.com/panmari/cuckoofilter"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Sync) syncDirectory(ctx context.Context, conn *grpc.ClientConn) error {
	runStartTime := time.Now().UTC()

	s.logger.Info().Str(syncStatus, syncStarted).Str("mode", s.options.Mode.RunMode()).Msg(syncRun)

	defer func() {
		close(s.errChan)
	}()

	// error spew.
	go func() {
		for e := range s.errChan {
			s.logger.Error().Err(e).Msg(syncRun)
		}
	}()

	g := new(errgroup.Group)

	g.Go(func() error {
		err := s.subscriber(ctx)
		if err != nil {
			s.logger.Error().Err(err).Str(syncStage, "subscriber").Msg(syncRun)
		}

		return err
	})

	g.Go(func() error {
		err := s.producer(ctx, conn)
		if err != nil {
			s.logger.Error().Err(err).Str(syncStage, "producer").Msg(syncRun)
		}

		return err
	})

	if err := g.Wait(); err != nil {
		s.logger.Error().Err(err).Msg(syncRun)
		return err
	}

	if Has(s.options.Mode, Diff) {
		if err := s.diff(ctx); err != nil {
			s.logger.Error().Err(err).Str(syncStage, "diff").Msg(syncRun)
			return err
		}
	}

	ts := <-s.tsChan
	if err := s.setWatermark(ts); err != nil {
		return err
	}

	runEndTime := time.Now().UTC()

	s.logger.Info().Str(syncStatus, syncFinished).Str("duration", runEndTime.Sub(runStartTime).String()).Msg(syncRun)

	return nil
}

func (s *Sync) producer(ctx context.Context, conn *grpc.ClientConn) error {
	s.logger.Info().Str(syncStatus, syncStarted).Msg(syncProducer)

	var recvCtr, objCtr, relCtr atomic.Int32

	defer func() {
		s.logger.Debug().Msg("producer closed export channel")
		close(s.exportChan)
	}()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ts := &timestamppb.Timestamp{}

	wm := s.getWatermark()
	if Has(s.options.Mode, Watermark) {
		ts = wm.Timestamp
	}

	if Has(s.options.Mode, Diff) {
		s.filter = cuckoo.NewFilter(wm.getFilterSize())
	}

	s.logger.Debug().Str("start_from", ts.String()).Msg(syncProducer)

	stream, err := dse.NewExporterClient(conn).Export(ctx, &dse.ExportRequest{
		Options:   uint32(dse.Option_OPTION_DATA),
		StartFrom: ts,
	})
	if err != nil {
		return err
	}

	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		recvCtr.Add(1)

		switch m := msg.GetMsg().(type) {
		case *dse.ExportResponse_Object:
			objCtr.Add(1)

			if Has(s.options.Mode, Diff) {
				s.filter.Insert(getObjectKey(m.Object))
			}
		case *dse.ExportResponse_Relation:
			relCtr.Add(1)

			if Has(s.options.Mode, Diff) {
				s.filter.Insert(getRelationKey(m.Relation))
			}
		default:
			s.logger.Debug().Msg("producer unknown message type")
			continue // do not send msg to exportChan when unknown.
		}

		s.exportChan <- msg
	}

	s.logger.Info().Str(syncStatus, syncFinished).
		Int32("received", recvCtr.Load()).
		Int32("objects", objCtr.Load()).
		Int32("relations", relCtr.Load()).
		Msg(syncProducer)

	return nil
}

func (s *Sync) subscriber(ctx context.Context) error {
	s.logger.Info().Str(syncStatus, syncStarted).Msg(syncSubscriber)

	var recvCtr, objCtr, relCtr, errCtr atomic.Int32

	ts := &timestamppb.Timestamp{}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	batchErr := s.store.Batch(ctx, func(tx store.Tx) error {
		for {
			msg, ok := <-s.exportChan
			if !ok {
				s.logger.Debug().Msg("export channel closed")
				break
			}

			recvCtr.Add(1)

			switch m := msg.GetMsg().(type) {
			case *dse.ExportResponse_Object:
				if err := s.objectSetHandler(ctx, tx, m.Object); err == nil {
					ts = maxTS(ts, m.Object.GetUpdatedAt())

					objCtr.Add(1)
				} else {
					s.logger.Error().Err(err).Msgf("failed to set object %v", m.Object)

					errCtr.Add(1)

					s.errChan <- err
				}

			case *dse.ExportResponse_Relation:
				if err := s.relationSetHandler(ctx, tx, m.Relation); err == nil {
					ts = maxTS(ts, m.Relation.GetUpdatedAt())

					relCtr.Add(1)
				} else {
					s.logger.Error().Err(err).Msgf("failed to set object %v", m.Relation)

					errCtr.Add(1)

					s.errChan <- err
				}

			default:
				s.logger.Debug().Msg("unknown message type")
			}
		}

		return nil
	})
	if batchErr != nil {
		return batchErr
	}

	s.tsChan <- ts

	s.logger.Info().Str(syncStatus, syncFinished).
		Int32("received", recvCtr.Load()).
		Int32("objects", objCtr.Load()).
		Int32("relations", relCtr.Load()).
		Int32("errors", errCtr.Load()).
		Msg(syncSubscriber)

	return nil
}

func (s *Sync) diff(ctx context.Context) error {
	s.logger.Info().Str(syncStatus, syncStarted).Msg(syncDifference)

	if s.filter == nil {
		return errors.New("filter not initialized") //nolint:err113
	}

	var objCtr, relCtr, errCtr atomic.Int32

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	batchErr := s.store.Batch(ctx, func(tx store.Tx) error {
		if err := s.diffObjects(ctx, tx, &objCtr, &errCtr); err != nil {
			return err
		}

		return s.diffRelations(ctx, tx, &relCtr, &errCtr)
	})
	if batchErr != nil {
		return batchErr
	}

	s.logger.Info().Str(syncStatus, syncFinished).
		Int32("delete_objects", objCtr.Load()).
		Int32("deleted_relations", relCtr.Load()).
		Int32("errors", errCtr.Load()).
		Msg(syncDifference)

	return nil
}

// diffObjects deletes every object the sync's cuckoo filter didn't see. The scan is fully drained and closed before any delete runs: a SQL-backed
// Tx can't issue a write on the same connection while a query's result set is still being read.
func (s *Sync) diffObjects(ctx context.Context, tx store.Tx, objCtr, errCtr *atomic.Int32) error {
	iter, err := tx.ScanObjects(ctx, "", "")
	if err != nil {
		return err
	}

	var stale []*dsc.Object

	for iter.Next() {
		if obj := iter.Value(); !s.filter.Lookup(getObjectKey(obj)) {
			stale = append(stale, obj)
		}
	}

	if err := iter.Close(); err != nil {
		return err
	}

	for _, obj := range stale {
		s.logger.Trace().Str("key", string(getObjectKey(obj))).Msg("delete")

		if err := s.objectDeleteHandler(ctx, tx, obj); err == nil {
			objCtr.Add(1)
		} else {
			s.logger.Error().Err(err).Msgf("failed to delete object %v", obj)

			errCtr.Add(1)

			s.errChan <- err
		}
	}

	return nil
}

// diffRelations is diffObjects' relation-side counterpart.
func (s *Sync) diffRelations(ctx context.Context, tx store.Tx, relCtr, errCtr *atomic.Int32) error {
	iter, err := tx.ScanRelations(ctx, store.ByObject, store.RelationFilter{}, "")
	if err != nil {
		return err
	}

	var stale []*dsc.Relation

	for iter.Next() {
		if rel := iter.Value(); !s.filter.Lookup(getRelationKey(rel)) {
			stale = append(stale, rel)
		}
	}

	if err := iter.Close(); err != nil {
		return err
	}

	for _, rel := range stale {
		s.logger.Trace().Str("key", string(getRelationKey(rel))).Msg("delete")

		if err := s.relationDeleteHandler(ctx, tx, rel); err == nil {
			relCtr.Add(1)
		} else {
			s.logger.Error().Err(err).Msgf("failed to delete relation %v", rel)

			s.errChan <- err

			errCtr.Add(1)
		}
	}

	return nil
}

func getObjectKey(obj *dsc.Object) []byte {
	return fmt.Appendf([]byte{}, "%s:%s", obj.GetType(), obj.GetId())
}

func getRelationKey(rel *dsc.Relation) []byte {
	return fmt.Appendf([]byte{}, "%s:%s#%s@%s:%s%s",
		rel.GetObjectType(), rel.GetObjectId(), rel.GetRelation(), rel.GetSubjectType(), rel.GetSubjectId(),
		lo.Ternary(rel.GetSubjectRelation() == "", "", "#"+rel.GetSubjectRelation()))
}
