//go:build integration

//nolint:funlen,goconst,dupl
package runtime_test

import (
	"context"
	"os"
	"testing"

	dsr "github.com/aserto-dev/go-directory/aserto/directory/reader/v3"
	dsa "github.com/authzen/access.go/api/access/v1"
	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/download"
	"github.com/open-policy-agent/opa/v1/plugins/bundle"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	runtime "github.com/threehook/eamerald/internal/runtime"
	"github.com/threehook/eamerald/pkg/config"
	"github.com/threehook/eamerald/daemon/authorizer/builtins"
	"github.com/threehook/eamerald/daemon/authorizer/builtins/az"
	"github.com/threehook/eamerald/daemon/authorizer/builtins/ds"
	"github.com/threehook/eamerald/daemon/authorizer/plugins/edge"
	"github.com/threehook/eamerald/daemon/authorizer/plugins/topaz_file_decision_logger"
	"google.golang.org/grpc"
)

// These tests pull a real policy bundle from ghcr.io and require a GH_TOKEN
// with read:packages scope, so they're gated behind the "integration" build
// tag: excluded from local/default `go test`/`make test` runs, and run
// separately in CI via `-tags=integration` where GH_TOKEN is always set.

func TestRemoteBundleV0(t *testing.T) {
	assert := require.New(t)

	var (
		logger *zerolog.Logger
		cfg    *config.Config
		dsConn *grpc.ClientConn
	)

	dsClient := dsr.NewReaderClient(dsConn)
	acClient := dsa.NewAccessClient(dsConn)

	tok := os.Getenv("GH_TOKEN")
	assert.NotEmpty(tok, "GH_TOKEN NOT SET")

	ctx, cancel := context.WithTimeout(t.Context(), testContextTimeout(t))
	t.Cleanup(cancel)

	r, err := runtime.New(
		t.Context(), &runtime.Config{
			Config: runtime.OPAConfig{
				Services: map[string]any{
					"ghcr": map[string]any{
						"url":  "https://ghcr.io",
						"type": "oci",
						"credentials": map[string]any{
							"bearer": map[string]any{
								"scheme": "Bearer",
								"token":  tok,
							},
						},
						"response_header_timeout_seconds": 5,
					},
				},
				Bundles: map[string]*bundle.Source{
					"testbundle": {
						Service:  "ghcr",
						Resource: "ghcr.io/aserto-policies/policy-peoplefinder-rbac:2",
						Persist:  false,
						Config: download.Config{
							Polling: testPollingConfig(),
						},
					},
				},
			},
		},
		runtime.WithBuiltin1(ds.RegisterIdentity(logger, builtins.DSIdentity, dsClient)),
		runtime.WithBuiltin1(ds.RegisterUser(logger, builtins.DSUser, dsClient)),
		runtime.WithBuiltin1(ds.RegisterObject(logger, builtins.DSObject, dsClient)),
		runtime.WithBuiltin1(ds.RegisterRelation(logger, builtins.DSRelation, dsClient)),
		runtime.WithBuiltin1(ds.RegisterRelations(logger, builtins.DSRelations, dsClient)),
		runtime.WithBuiltin1(ds.RegisterGraph(logger, builtins.DSGraph, dsClient)),
		runtime.WithBuiltin1(ds.RegisterCheck(logger, builtins.DSCheck, dsClient)),
		runtime.WithBuiltin1(ds.RegisterChecks(logger, builtins.DSChecks, dsClient)),
		runtime.WithBuiltin1(az.RegisterEvaluation(logger, builtins.AZEvaluation, acClient)),
		runtime.WithBuiltin1(az.RegisterEvaluations(logger, builtins.AZEvaluations, acClient)),
		runtime.WithBuiltin1(az.RegisterSubjectSearch(logger, builtins.AZSubjectSearch, acClient)),
		runtime.WithBuiltin1(az.RegisterResourceSearch(logger, builtins.AZResourceSearch, acClient)),
		runtime.WithBuiltin1(az.RegisterActionSearch(logger, builtins.AZActionSearch, acClient)),
		runtime.WithPlugin(topaz_file_decision_logger.PluginName, topaz_file_decision_logger.NewFactory(ctx)),
		runtime.WithPlugin(edge.PluginName, edge.NewPluginFactory(ctx, cfg, logger)),
		runtime.WithRegoVersion(ast.RegoV0),
	)

	assert.NoError(err)
	assert.NotNil(r)

	assert.NoError(
		r.Start(ctx),
	)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(t.Context(), testContextTimeout(t))
		r.Stop(cleanupCtx)
		cleanupCancel()
	})

	assert.Equal(ast.RegoV0, r.GetPluginsManager().ParserOptions().RegoVersion)

	assert.NoError(
		r.CheckPluginsStatus(),
	)

	b, err := r.GetBundles(ctx)
	assert.NoError(err)
	assert.Len(b, 1)
}

func TestRemoteBundleV1(t *testing.T) {
	assert := require.New(t)

	var (
		logger *zerolog.Logger
		cfg    *config.Config
		dsConn *grpc.ClientConn
	)

	dsClient := dsr.NewReaderClient(dsConn)
	acClient := dsa.NewAccessClient(dsConn)

	tok := os.Getenv("GH_TOKEN")
	assert.NotEmpty(tok, "GH_TOKEN NOT SET")

	ctx, cancel := context.WithTimeout(t.Context(), testContextTimeout(t))
	t.Cleanup(cancel)

	r, err := runtime.New(
		t.Context(), &runtime.Config{
			Config: runtime.OPAConfig{
				Services: map[string]any{
					"ghcr": map[string]any{
						"url":  "https://ghcr.io",
						"type": "oci",
						"credentials": map[string]any{
							"bearer": map[string]any{
								"scheme": "Bearer",
								"token":  tok,
							},
						},
						"response_header_timeout_seconds": 5,
					},
				},
				Bundles: map[string]*bundle.Source{
					"testbundle": {
						Service:  "ghcr",
						Resource: "ghcr.io/aserto-policies/policy-rebac:latest",
						Persist:  false,
						Config: download.Config{
							Polling: testPollingConfig(),
						},
					},
				},
			},
		},
		runtime.WithBuiltin1(ds.RegisterIdentity(logger, builtins.DSIdentity, dsClient)),
		runtime.WithBuiltin1(ds.RegisterUser(logger, builtins.DSUser, dsClient)),
		runtime.WithBuiltin1(ds.RegisterObject(logger, builtins.DSObject, dsClient)),
		runtime.WithBuiltin1(ds.RegisterRelation(logger, builtins.DSRelation, dsClient)),
		runtime.WithBuiltin1(ds.RegisterRelations(logger, builtins.DSRelations, dsClient)),
		runtime.WithBuiltin1(ds.RegisterGraph(logger, builtins.DSGraph, dsClient)),
		runtime.WithBuiltin1(ds.RegisterCheck(logger, builtins.DSCheck, dsClient)),
		runtime.WithBuiltin1(ds.RegisterChecks(logger, builtins.DSChecks, dsClient)),
		runtime.WithBuiltin1(az.RegisterEvaluation(logger, builtins.AZEvaluation, acClient)),
		runtime.WithBuiltin1(az.RegisterEvaluations(logger, builtins.AZEvaluations, acClient)),
		runtime.WithBuiltin1(az.RegisterSubjectSearch(logger, builtins.AZSubjectSearch, acClient)),
		runtime.WithBuiltin1(az.RegisterResourceSearch(logger, builtins.AZResourceSearch, acClient)),
		runtime.WithBuiltin1(az.RegisterActionSearch(logger, builtins.AZActionSearch, acClient)),
		runtime.WithPlugin(topaz_file_decision_logger.PluginName, topaz_file_decision_logger.NewFactory(ctx)),
		runtime.WithPlugin(edge.PluginName, edge.NewPluginFactory(ctx, cfg, logger)),
		runtime.WithRegoVersion(ast.RegoV1),
	)

	assert.NoError(err)
	assert.NotNil(r)

	assert.NoError(
		r.Start(ctx),
	)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(t.Context(), testContextTimeout(t))
		r.Stop(cleanupCtx)
		cleanupCancel()
	})

	assert.Equal(ast.RegoV1, r.GetPluginsManager().ParserOptions().RegoVersion)

	assert.NoError(
		r.CheckPluginsStatus(),
	)

	b, err := r.GetBundles(ctx)
	assert.NoError(err)
	assert.Len(b, 1)
}

func testPollingConfig() download.PollingConfig {
	return download.PollingConfig{
		MinDelaySeconds:           func() *int64 { v := int64(60); return &v }(),
		MaxDelaySeconds:           func() *int64 { v := int64(120); return &v }(),
		LongPollingTimeoutSeconds: func() *int64 { v := int64(360); return &v }(),
	}
}
