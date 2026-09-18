package eamerald

import (
	"context"

	"github.com/aserto-dev/go-authorizer/pkg/aerr"
	dsr "github.com/aserto-dev/go-directory/aserto/directory/reader/v3"
	dsa "github.com/authzen/access.go/api/access/v1"
	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/rs/zerolog"
	"github.com/threehook/eamerald/daemon/authorizer/builtins"
	"github.com/threehook/eamerald/daemon/authorizer/builtins/az"
	"github.com/threehook/eamerald/daemon/authorizer/builtins/ds"
	"github.com/threehook/eamerald/daemon/authorizer/plugins/adl_decision_logger"
	"github.com/threehook/eamerald/daemon/authorizer/plugins/edge"
	"github.com/threehook/eamerald/daemon/authorizer/plugins/entra"
	"github.com/threehook/eamerald/daemon/authorizer/plugins/git"
	"github.com/threehook/eamerald/daemon/authorizer/resolvers"
	"github.com/threehook/eamerald/internal/runtime"
	"github.com/threehook/eamerald/pkg/config"
	"google.golang.org/grpc"
)

var _ resolvers.RuntimeResolver = (*RuntimeResolver)(nil)

type RuntimeResolver struct {
	runtime *runtime.Runtime
}

func NewRuntimeResolver(
	ctx context.Context,
	logger *zerolog.Logger,
	cfg *config.Config,
	dsConn *grpc.ClientConn,
) (resolvers.RuntimeResolver, func(), error) {
	dsClient := dsr.NewReaderClient(dsConn)
	acClient := dsa.NewAccessClient(dsConn)

	rt, err := runtime.New(
		ctx, &cfg.OPA,

		// directory get functions
		runtime.WithBuiltin1(ds.RegisterIdentity(logger, builtins.DSIdentity, dsClient)),
		runtime.WithBuiltin1(ds.RegisterUser(logger, builtins.DSUser, dsClient)),
		runtime.WithBuiltin1(ds.RegisterObject(logger, builtins.DSObject, dsClient)),
		runtime.WithBuiltin1(ds.RegisterRelation(logger, builtins.DSRelation, dsClient)),
		runtime.WithBuiltin1(ds.RegisterRelations(logger, builtins.DSRelations, dsClient)),
		runtime.WithBuiltin1(ds.RegisterGraph(logger, builtins.DSGraph, dsClient)),

		// authorization check functions
		runtime.WithBuiltin1(ds.RegisterCheck(logger, builtins.DSCheck, dsClient)),
		runtime.WithBuiltin1(ds.RegisterChecks(logger, builtins.DSChecks, dsClient)),

		// authZen built-ins
		runtime.WithBuiltin1(az.RegisterEvaluation(logger, builtins.AZEvaluation, acClient)),
		runtime.WithBuiltin1(az.RegisterEvaluations(logger, builtins.AZEvaluations, acClient)),
		runtime.WithBuiltin1(az.RegisterSubjectSearch(logger, builtins.AZSubjectSearch, acClient)),
		runtime.WithBuiltin1(az.RegisterResourceSearch(logger, builtins.AZResourceSearch, acClient)),
		runtime.WithBuiltin1(az.RegisterActionSearch(logger, builtins.AZActionSearch, acClient)),

		// plugins
		runtime.WithPlugin(adl_decision_logger.PluginName, adl_decision_logger.NewFactory(logger.WithContext(ctx))),
		runtime.WithPlugin(edge.PluginName, edge.NewPluginFactory(ctx, cfg, logger)),
		runtime.WithPlugin(git.PluginName, git.NewPluginFactory(ctx, logger)),
		runtime.WithPlugin(entra.PluginName, entra.NewPluginFactory(ctx, logger, dsConn)),

		runtime.WithRegoVersion(ast.RegoV0),
	)
	if err != nil {
		return nil, func() {}, err
	}

	cleanupRuntime := func() {
		rt.Stop(ctx)
	}

	cleanup := func() {
		if cleanupRuntime != nil {
			cleanupRuntime()
		}
	}

	if err := rt.Start(ctx); err != nil {
		return nil, cleanup, err
	}

	if err := rt.CheckPluginsStatus(); err != nil {
		return nil, cleanup, aerr.ErrBadRuntime.Err(err)
	}

	return &RuntimeResolver{
		runtime: rt,
	}, cleanup, err
}

func (r *RuntimeResolver) GetRuntime(ctx context.Context) (*runtime.Runtime, error) {
	return r.runtime, nil
}
