package middlewares

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/threehook/eamerald/daemon/authentication"
	grpcutil "github.com/threehook/eamerald/internal/grpc"
	"github.com/threehook/eamerald/internal/grpc/middlewares/gerr"
	"github.com/threehook/eamerald/internal/grpc/middlewares/request"
	"github.com/threehook/eamerald/internal/grpc/middlewares/tracing"
	"github.com/threehook/eamerald/pkg/config"
	"google.golang.org/grpc"
)

func GetMiddlewaresForService(ctx context.Context, cfg *config.Config, logger *zerolog.Logger) ([]grpc.ServerOption, error) {
	var middlewareList grpcutil.Middlewares

	if len(cfg.Auth.APIKeys) > 0 {
		authMiddleware, err := authentication.NewAPIKeyAuthMiddleware(ctx, &cfg.Auth, logger)
		if err != nil {
			return nil, err
		}

		middlewareList = append(middlewareList, authMiddleware)
	}

	middlewareList = append(middlewareList,
		request.NewRequestIDMiddleware(),
		tracing.NewTracingMiddleware(logger),
		gerr.NewErrorMiddleware(),
	)

	unary, stream := middlewareList.AsGRPCOptions()
	opts := []grpc.ServerOption{unary, stream}

	return opts, nil
}
