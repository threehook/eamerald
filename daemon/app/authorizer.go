package app

import (
	"context"

	dsa "github.com/authzen/access.go/api/access/v1"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/threehook/eamerald/daemon/authorizer/impl"
	"github.com/threehook/eamerald/daemon/authorizer/resolvers"
	"github.com/threehook/eamerald/daemon/service/builder"
	"github.com/threehook/eamerald/internal/adl"
	"github.com/threehook/eamerald/pkg/config"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/samber/lo"
	"google.golang.org/grpc"
)

const (
	authorizerService = "authorizer"
)

type Authorizer struct {
	Resolver         *resolvers.Resolvers
	AuthorizerServer *impl.AuthorizerServer
	AccessServer     *impl.AccessServer
	cfg              *builder.API
	logger           *zerolog.Logger
	opts             []grpc.ServerOption
	cleanupFunctions []func()
}

var _ builder.ServiceTypes = (*Authorizer)(nil)

func NewAuthorizer(
	ctx context.Context,
	cfg *builder.API,
	commonConfig *config.Common,
	authorizerOpts []grpc.ServerOption,
	logger *zerolog.Logger,
	adlLogger *adl.Logger,
) (*Authorizer,
	error,
) {
	if cfg.GRPC.Certs.HasCert() {
		tlsCreds, err := cfg.GRPC.Certs.ServerCredentials()
		if err != nil {
			return nil, errors.Wrap(err, "failed to calculate tls config")
		}

		tlsAuth := grpc.Creds(tlsCreds)
		authorizerOpts = append(authorizerOpts, tlsAuth)
	}

	authResolvers := resolvers.New()

	authServer, err := impl.NewAuthorizerServer(ctx, logger, commonConfig, authResolvers, adlLogger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create authorizer server")
	}

	return &Authorizer{
		cfg:              cfg,
		logger:           logger,
		opts:             authorizerOpts,
		Resolver:         authResolvers,
		AuthorizerServer: authServer,
		AccessServer:     impl.NewAccessServer(authServer),
	}, nil
}

func (e *Authorizer) AvailableServices() []string {
	return []string{authorizerService}
}

func (e *Authorizer) GetGRPCRegistrations(services ...string) builder.GRPCRegistrations {
	return func(server *grpc.Server) {
		if e.servesAccessAPI(services...) {
			dsa.RegisterAccessServer(server, e.AccessServer)
		}
	}
}

func (e *Authorizer) GetGatewayRegistration(port string, services ...string) builder.HandlerRegistrations {
	return func(ctx context.Context, mux *runtime.ServeMux, grpcEndpoint string, opts []grpc.DialOption) error {
		if e.servesAccessAPI(services...) {
			if err := dsa.RegisterAccessHandlerFromEndpoint(ctx, mux, grpcEndpoint, opts); err != nil {
				return err
			}
		}

		return nil
	}
}

func (e *Authorizer) Cleanups() []func() {
	return e.cleanupFunctions
}

func (e *Authorizer) Close() {
	for _, f := range e.cleanupFunctions {
		if f != nil {
			f()
		}
	}
}

// servesAccessAPI reports whether the policy-engine implementation of the AuthZEN Access API should be registered on this port.
//
// The directory registers its own graph-backed implementation alongside the reader, and gRPC panics on a duplicate service registration, so a
// deployment that puts the authorizer and the reader on one address gets the directory's.
// Serving policy decisions over AuthZEN then needs the two on separate addresses, which is how they are configured by default.
func (e *Authorizer) servesAccessAPI(services ...string) bool {
	if !lo.Contains(services, readerService) {
		return true
	}

	e.logger.Warn().Str("service", accessService).
		Msg("authorizer and reader share a grpc address; serving the access api from the directory")

	return false
}
