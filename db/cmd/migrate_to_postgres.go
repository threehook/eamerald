package cmd

import (
	"context"
	"fmt"
	"io"

	dse "github.com/aserto-dev/go-directory/aserto/directory/exporter/v3"
	"github.com/threehook/eamerald/db/pkg/inproc"
	"github.com/threehook/eamerald/internal/eds"
	"github.com/threehook/eamerald/internal/eds/pkg/directory"
	dsc "github.com/threehook/eamerald/mrld/clients/directory"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

type MigrateToPostgresCmd struct {
	DBFile string `arg:"" help:"boltdb database file to migrate from; pass a snapshot/copy, not the file eameraldd is actively serving" type:"existingfile"` //nolint:lll
	DSN    string `arg:"" help:"postgres connection string to migrate to, e.g. postgres://user:password@host:5432/dbname"`
}

// Run streams the manifest, objects and relations from a BoltDB snapshot into a Postgres database, applying schema
// migrations to the destination first. It reuses the existing Exporter/Importer/Model client-server plumbing
// unchanged, wired in-process via two bufconn gRPC connections rather than a real network hop.
//
// Safe to re-run after a failed attempt: every write is an upsert (Opcode_OPCODE_SET), never a fail-if-exists insert.
func (cmd *MigrateToPostgresCmd) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	logger := zerolog.New(io.Discard)

	srcDir, err := eds.Open(ctx, &directory.Config{
		DBPath:         cmd.DBFile,
		RequestTimeout: requestTimeout,
		Backend:        directory.BackendBoltDB,
	}, &logger, nil)
	if err != nil {
		return fmt.Errorf("open source (boltdb %q): %w", cmd.DBFile, err)
	}
	defer srcDir.Close()

	dstDir, err := eds.Open(ctx, &directory.Config{
		RequestTimeout: requestTimeout,
		Backend:        directory.BackendPostgres,
		Postgres:       directory.PostgresConfig{DSN: cmd.DSN},
	}, &logger, nil)
	if err != nil {
		return fmt.Errorf("open destination (postgres): %w", err)
	}
	defer dstDir.Close()

	srcConn, srcCleanup := inproc.NewServerFromDirectory(srcDir)
	defer srcCleanup()

	dstConn, dstCleanup := inproc.NewServerFromDirectory(dstDir)
	defer dstCleanup()

	src := dsc.New(srcConn)
	dst := dsc.New(dstConn)

	// Manifest first: object/relation validation on the destination depends on the model it defines.
	manifest, err := src.GetManifest(ctx)
	if err != nil {
		return fmt.Errorf("read source manifest: %w", err)
	}

	if err := dst.SetManifest(ctx, manifest); err != nil {
		return fmt.Errorf("write destination manifest: %w", err)
	}

	return migrateData(ctx, src, dst)
}

// migrateData streams objects and relations directly from the source's Exporter to the destination's Importer
// through an in-memory pipe - no intermediate file - reusing ExportToFile/ImportFromFile's existing stream handling
// unchanged, same as mrld-db's load/sync commands already do against a file or a remote directory.
func migrateData(ctx context.Context, src, dst *dsc.Client) error {
	pr, pw := io.Pipe()

	grp, grpCtx := errgroup.WithContext(ctx)

	grp.Go(func() error {
		err := src.ExportToFile(grpCtx, pw, uint32(dse.Option_OPTION_DATA))
		_ = pw.CloseWithError(err) // nil err closes the reader side with a plain io.EOF, as intended.

		return err
	})

	grp.Go(func() error {
		defer pr.Close()
		return dst.ImportFromFile(grpCtx, pr)
	})

	if err := grp.Wait(); err != nil {
		return fmt.Errorf("migrate objects/relations: %w", err)
	}

	return nil
}
