package impl

import (
	"context"
	"time"

	"github.com/aserto-dev/go-authorizer/pkg/aerr"
	"github.com/threehook/eamerald/internal/adl"
	"github.com/threehook/eamerald/internal/runtime"

	"github.com/threehook/eamerald/daemon/authorizer/resolvers"
	"github.com/threehook/eamerald/pkg/config"

	"github.com/rs/zerolog"
)

const (
	InputUser     string = "user"
	InputIdentity string = "identity"
	InputResource string = "resource"

	// InputSubject, InputAction and InputContext are the elements of the
	// AuthZEN information model set by the Access API. InputResource is
	// shared with it: an AuthZEN resource flattens onto the same
	// input.resource a Topaz resource context used.
	InputSubject string = "subject"
	InputAction  string = "action"
	InputContext string = "context"
)

const cleanupTimeout = 30 * time.Second

type AuthorizerServer struct {
	logger      *zerolog.Logger
	jwtResolver *jwtResolver
	resolver    *resolvers.Resolvers
	adl         *adl.Logger

	// preparedQueries memoizes the rego.PreparedEvalQuery values produced for
	// each (policy path, decisions) tuple the Access API evaluates, plus the
	// loaded bundle's policy roots (see prepared_query_cache.go).
	preparedQueries *preparedQueryCache
}

func NewAuthorizerServer(
	ctx context.Context,
	logger *zerolog.Logger,
	cfg *config.Common,
	rf *resolvers.Resolvers,
	adlLogger *adl.Logger,
) (*AuthorizerServer, error) {
	newLogger := logger.With().Str("component", "api.grpc").Logger()

	jwtResolver, err := NewJWTResolver(ctx, &cfg.JWT)
	if err != nil {
		return nil, err
	}

	if err := jwtResolver.Start(ctx); err != nil {
		return nil, err
	}

	go func() { //nolint:gosec // G118 - cleanup cannot use request context as it is already cancelled.
		<-ctx.Done()

		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()

		_ = jwtResolver.Stop(cleanupCtx)
	}()

	return &AuthorizerServer{
		logger:          &newLogger,
		jwtResolver:     jwtResolver,
		resolver:        rf,
		adl:             adlLogger,
		preparedQueries: newPreparedQueryCache(),
	}, nil
}

func (s *AuthorizerServer) getRuntime(ctx context.Context) (*runtime.Runtime, error) {
	rt, err := s.resolver.GetRuntimeResolver().GetRuntime(ctx)
	if err != nil {
		return nil, aerr.ErrInvalidPolicyID.Msg("undefined policy context")
	}

	return rt, err
}
