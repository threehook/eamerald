package directory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aserto-dev/azm/cache"
	"github.com/aserto-dev/azm/model"
	dse "github.com/aserto-dev/go-directory/aserto/directory/exporter/v3"
	dsi "github.com/aserto-dev/go-directory/aserto/directory/importer/v3"
	dsm "github.com/aserto-dev/go-directory/aserto/directory/model/v3"
	dsr "github.com/aserto-dev/go-directory/aserto/directory/reader/v3"
	dsw "github.com/aserto-dev/go-directory/aserto/directory/writer/v3"
	dsa "github.com/authzen/access.go/api/access/v1"

	"github.com/threehook/eamerald/internal/adl"
	"github.com/threehook/eamerald/internal/eds/pkg/bdb"
	"github.com/threehook/eamerald/internal/eds/pkg/bdb/migrations/migrate"
	"github.com/threehook/eamerald/internal/eds/pkg/datasync"
	v3 "github.com/threehook/eamerald/internal/eds/pkg/directory/v3"
	"github.com/threehook/eamerald/internal/eds/pkg/pgdb"
	"github.com/threehook/eamerald/internal/eds/pkg/store"

	"github.com/Masterminds/semver/v3"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// required minimum schema version, when the current version is lower,
// migration will be invoked to update to the minimum schema version required.
const (
	schemaVersion string = "0.0.9"
)

// Backend names for Config.Backend. BackendBoltDB is the default when Backend is left empty.
const (
	BackendBoltDB   string = "boltdb"
	BackendPostgres string = "postgres"
)

var (
	ErrUnknownBackend     = fmt.Errorf("directory: unknown backend, expected %q or %q", BackendBoltDB, BackendPostgres)
	ErrPostgresDSNUnset   = errors.New("directory: backend is postgres but postgres.dsn is not set")
	ErrDataSyncBoltDBOnly = errors.New("directory: datasync is only supported with the boltdb backend")
)

type PostgresConfig struct {
	DSN string `json:"dsn"`
}

type Config struct {
	DBPath         string         `json:"db_path"`
	RequestTimeout time.Duration  `json:"request_timeout"`
	Seed           bool           `json:"seed_metadata"`
	EnableV2       bool           `json:"enable_v2"`
	Backend        string         `json:"backend"`
	Postgres       PostgresConfig `json:"postgres"`
}

type Directory struct {
	config *Config
	logger *zerolog.Logger

	store store.Store // backend-agnostic; used by the v3 servers.
	mc    *cache.Cache

	// boltStore is only non-nil when Backend == BackendBoltDB. datasync (the aserto-edge sync plugin) has no
	// Postgres equivalent yet: its watermark file is tied to a BoltDB file path and its row counts to raw
	// bbolt bucket stats. DataSyncClient reports that gap explicitly rather than wiring datasync against store.Store.
	boltStore *bdb.BoltDB

	exporter3 dse.ExporterServer
	importer3 dsi.ImporterServer
	model3    dsm.ModelServer
	reader3   dsr.ReaderServer
	writer3   dsw.WriterServer
	access1   dsa.AccessServer
}

var (
	directory *Directory
	once      sync.Once
)

func Get() (*Directory, error) {
	if directory != nil {
		return directory, nil
	}

	return nil, status.Error(codes.Internal, "directory not initialized")
}

func New(ctx context.Context, config *Config, logger *zerolog.Logger, adlLogger *adl.Logger) (*Directory, error) {
	var err error

	once.Do(func() {
		directory, err = newDirectory(ctx, config, logger, adlLogger)
	})

	return directory, err
}

// Open constructs a new, independent Directory instance, bypassing the process-wide singleton that New/Get share.
// eameraldd itself should keep using New/Get; Open is for tooling that needs more than one Directory alive in the
// same process (e.g. mrld-db migrate-to-postgres, which holds a BoltDB-backed source and a Postgres-backed
// destination open at once).
func Open(ctx context.Context, config *Config, logger *zerolog.Logger, adlLogger *adl.Logger) (*Directory, error) {
	return newDirectory(ctx, config, logger, adlLogger)
}

func newDirectory(ctx context.Context, config *Config, logger *zerolog.Logger, adlLogger *adl.Logger) (*Directory, error) {
	newLogger := logger.With().Str("component", "directory").Logger()

	backend := config.Backend
	if backend == "" {
		backend = BackendBoltDB
	}

	mc := cache.New(&model.Model{})

	var (
		st        store.Store
		boltStore *bdb.BoltDB
	)

	switch backend {
	case BackendBoltDB:
		bStore, err := openBoltDB(config, logger, &newLogger, mc)
		if err != nil {
			return nil, err
		}

		st, boltStore = bStore, bStore

	case BackendPostgres:
		pStore, err := openPostgres(ctx, config)
		if err != nil {
			return nil, err
		}

		st = pStore

	default:
		return nil, fmt.Errorf("%w: got %q", ErrUnknownBackend, backend)
	}

	if err := loadModel(ctx, st, mc); err != nil {
		return nil, err
	}

	reader3 := v3.NewReader(logger, st, mc)
	writer3 := v3.NewWriter(logger, st, mc)
	exporter3 := v3.NewExporter(logger, st)
	importer3 := v3.NewImporter(logger, st, mc)

	access1 := v3.NewAccess(logger, reader3, adlLogger)

	dir := &Directory{
		config:    config,
		logger:    &newLogger,
		store:     st,
		mc:        mc,
		boltStore: boltStore,
		model3:    v3.NewModel(logger, st, mc),
		reader3:   reader3,
		writer3:   writer3,
		exporter3: exporter3,
		importer3: importer3,
		access1:   access1,
	}

	return dir, nil
}

// openBoltDB applies pending schema migrations (if any) and opens the BoltDB file, sharing mc as its model cache.
func openBoltDB(config *Config, logger *zerolog.Logger, newLogger *zerolog.Logger, mc *cache.Cache) (*bdb.BoltDB, error) {
	cfg := bdb.Config{
		DBPath:         config.DBPath,
		RequestTimeout: config.RequestTimeout,
	}

	if ok, err := migrate.CheckSchemaVersion(&cfg, logger, semver.MustParse(schemaVersion)); !ok {
		switch {
		case errors.Is(err, migrate.ErrDirectorySchemaUpdateRequired):
			if err := migrate.Migrate(&cfg, logger, semver.MustParse(schemaVersion)); err != nil {
				return nil, err
			}
		case errors.Is(err, migrate.ErrDirectorySchemaVersionHigher):
			return nil, err
		default:
			return nil, err
		}

		if ok, err := migrate.CheckSchemaVersion(&cfg, logger, semver.MustParse(schemaVersion)); !ok {
			return nil, err
		}
	}

	store, err := bdb.New(&cfg, newLogger, mc)
	if err != nil {
		return nil, err
	}

	if err := store.Open(); err != nil {
		return nil, err
	}

	return store, nil
}

// openPostgres applies pending schema migrations and opens a connection pool for config.Postgres.DSN.
func openPostgres(ctx context.Context, config *Config) (*pgdb.Store, error) {
	if config.Postgres.DSN == "" {
		return nil, ErrPostgresDSNUnset
	}

	if err := pgdb.Migrate(config.Postgres.DSN); err != nil {
		return nil, fmt.Errorf("directory: migrate postgres schema: %w", err)
	}

	return pgdb.Open(ctx, config.Postgres.DSN)
}

// loadModel hydrates mc from the manifest already stored in st, if any. Backend-agnostic: it reads the manifest
// purely through store.Tx, so it works identically whether st is a *bdb.BoltDB or a *pgdb.Store.
func loadModel(ctx context.Context, st store.Store, mc *cache.Cache) error {
	return st.View(ctx, func(tx store.Tx) error {
		exists, err := tx.ManifestExists(ctx)
		if err != nil {
			return err
		}

		if !exists {
			return nil
		}

		mod, err := tx.GetManifestModel(ctx)

		switch {
		case status.Code(err) == codes.NotFound:
			return nil
		case err != nil:
			return err
		}

		return mc.UpdateModel(mod)
	})
}

func (s *Directory) Close() {
	switch {
	case s.boltStore != nil:
		s.boltStore.Close()
	default:
		if closer, ok := s.store.(interface{ Close() }); ok {
			closer.Close()
		}
	}

	s.store = nil
	s.boltStore = nil
}

func (s *Directory) Exporter3() dse.ExporterServer {
	return s.exporter3
}

func (s *Directory) Importer3() dsi.ImporterServer {
	return s.importer3
}

func (s *Directory) Model3() dsm.ModelServer {
	return s.model3
}

func (s *Directory) Reader3() dsr.ReaderServer {
	return s.reader3
}

func (s *Directory) Writer3() dsw.WriterServer {
	return s.writer3
}

func (s *Directory) Access1() dsa.AccessServer {
	return s.access1
}

func (s *Directory) Logger() *zerolog.Logger {
	return s.logger
}

func (s *Directory) Config() Config {
	return *s.config
}

// DataSyncClient returns the aserto-edge sync plugin's client. It errors when the directory is backed by Postgres:
// datasync's watermark file is tied to a BoltDB file path and its stats are raw bbolt bucket stats, neither of
// which has a Postgres equivalent yet.
func (s *Directory) DataSyncClient() (datasync.SyncClient, error) {
	if s.boltStore == nil {
		return nil, ErrDataSyncBoltDBOnly
	}

	return datasync.New(s.logger, s.boltStore), nil
}
