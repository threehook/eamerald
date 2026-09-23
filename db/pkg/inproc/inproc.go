package inproc

import (
	"context"
	"net"

	dse "github.com/aserto-dev/go-directory/aserto/directory/exporter/v3"
	dsi "github.com/aserto-dev/go-directory/aserto/directory/importer/v3"
	dsm "github.com/aserto-dev/go-directory/aserto/directory/model/v3"
	dsr "github.com/aserto-dev/go-directory/aserto/directory/reader/v3"
	dsw "github.com/aserto-dev/go-directory/aserto/directory/writer/v3"

	"github.com/rs/zerolog"
	"github.com/threehook/eamerald/internal/eds"
	"github.com/threehook/eamerald/internal/eds/pkg/directory"
	"github.com/threehook/eamerald/internal/grpc/middlewares/gerr"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize int = 1024 * 1024

// NewServer opens a Directory for cfg (through the process-wide singleton, see directory.New) and wraps it with an
// in-process gRPC server/client pair. Callers that already hold a *directory.Directory (e.g. because they opened it
// via directory.Open to run more than one in the same process) should use NewServerFromDirectory instead.
func NewServer(ctx context.Context, logger *zerolog.Logger, cfg *directory.Config) (*grpc.ClientConn, func()) {
	dsLogger := logger.With().Str("component", "ds").Logger()

	inProcDirectory, err := eds.New(ctx, cfg, &dsLogger, nil)
	if err != nil {
		logger.Error().Err(err).Msg("failed to start edge directory server")
	}

	return NewServerFromDirectory(inProcDirectory)
}

// NewServerFromDirectory wraps an already-open Directory with an in-process gRPC server/client pair, so it can be
// driven with the normal mrld/clients/directory.Client rather than calling its Reader3/Writer3/... servers directly.
func NewServerFromDirectory(dir *directory.Directory) (*grpc.ClientConn, func()) {
	listener := bufconn.Listen(bufSize)

	errMiddleware := gerr.NewErrorMiddleware()
	s := grpc.NewServer(
		grpc.UnaryInterceptor(errMiddleware.Unary()),
		grpc.StreamInterceptor(errMiddleware.Stream()),
	)

	dsm.RegisterModelServer(s, dir.Model3())
	dsr.RegisterReaderServer(s, dir.Reader3())
	dsw.RegisterWriterServer(s, dir.Writer3())
	dse.RegisterExporterServer(s, dir.Exporter3())
	dsi.RegisterImporterServer(s, dir.Importer3())

	go func() {
		if err := s.Serve(listener); err != nil {
			panic(err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), //nolint:staticcheck
	)
	if err != nil {
		panic(err)
	}

	return conn, s.GracefulStop
}
