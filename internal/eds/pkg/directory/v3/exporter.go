package v3

import (
	"encoding/json"

	dse "github.com/aserto-dev/go-directory/aserto/directory/exporter/v3"
	"github.com/aserto-dev/go-directory/pkg/pb"
	"github.com/threehook/eamerald/internal/eds/pkg/ds"
	"github.com/threehook/eamerald/internal/eds/pkg/store"

	"github.com/rs/zerolog"
)

type Exporter struct {
	logger *zerolog.Logger
	store  store.Store
}

var _ dse.ExporterServer = (*Exporter)(nil)

func NewExporter(logger *zerolog.Logger, store store.Store) *Exporter {
	return &Exporter{
		logger: logger,
		store:  store,
	}
}

func (s *Exporter) Export(req *dse.ExportRequest, stream dse.Exporter_ExportServer) error {
	logger := s.logger.With().Str("method", "Export").Interface("req", req).Logger()

	err := s.store.View(stream.Context(), func(tx store.Tx) error {
		// stats mode, short circuits when enabled
		if req.GetOptions()&uint32(dse.Option_OPTION_STATS) != 0 {
			if err := exportStats(tx, stream, req.GetOptions()); err != nil {
				logger.Error().Err(err).Msg("export_stats")
				return err
			}

			return nil
		}

		if req.GetOptions()&uint32(dse.Option_OPTION_DATA_OBJECTS) != 0 {
			if err := exportObjects(tx, stream); err != nil {
				logger.Error().Err(err).Msg("export_objects")
				return err
			}
		}

		if req.GetOptions()&uint32(dse.Option_OPTION_DATA_RELATIONS) != 0 {
			if err := exportRelations(tx, stream); err != nil {
				logger.Error().Err(err).Msg("export_relations")
				return err
			}
		}

		return nil
	})

	return err
}

func exportObjects(tx store.Tx, stream dse.Exporter_ExportServer) error {
	iter, err := tx.ScanObjects(stream.Context(), "", "")
	if err != nil {
		return err
	}
	defer iter.Close()

	for iter.Next() {
		obj := ds.PatchObjectRead(iter.Value())
		if err := stream.Send(&dse.ExportResponse{Msg: &dse.ExportResponse_Object{Object: obj}}); err != nil {
			return err
		}
	}

	return nil
}

func exportRelations(tx store.Tx, stream dse.Exporter_ExportServer) error {
	iter, err := tx.ScanRelations(stream.Context(), store.ByObject, store.RelationFilter{}, "")
	if err != nil {
		return err
	}
	defer iter.Close()

	for iter.Next() {
		if err := stream.Send(&dse.ExportResponse{Msg: &dse.ExportResponse_Relation{Relation: iter.Value()}}); err != nil {
			return err
		}
	}

	return nil
}

func exportStats(tx store.Tx, stream dse.Exporter_ExportServer, opts uint32) error {
	stats := ds.NewStats()

	// object stats.
	if opts&uint32(dse.Option_OPTION_DATA_OBJECTS) != 0 {
		if err := stats.CountObjects(stream.Context(), tx); err != nil {
			return err
		}
	}

	// relation stats.
	if opts&uint32(dse.Option_OPTION_DATA_RELATIONS) != 0 {
		if err := stats.CountRelations(stream.Context(), tx); err != nil {
			return err
		}
	}

	buf, err := json.Marshal(stats.Stats)
	if err != nil {
		return err
	}

	resp := pb.NewStruct()
	if err := resp.UnmarshalJSON(buf); err != nil {
		return err
	}

	if err := stream.Send(&dse.ExportResponse{Msg: &dse.ExportResponse_Stats{Stats: resp}}); err != nil {
		return err
	}

	return nil
}
