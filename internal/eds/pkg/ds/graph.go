package ds

import (
	"context"

	"github.com/aserto-dev/azm/cache"
	"github.com/aserto-dev/azm/safe"
	dsr "github.com/aserto-dev/go-directory/aserto/directory/reader/v3"
	"github.com/threehook/eamerald/internal/eds/pkg/store"
)

type getGraph struct {
	*safe.SafeGetGraph
}

func GetGraph(i *dsr.GetGraphRequest) *getGraph {
	return &getGraph{safe.GetGraph(i)}
}

func (i *getGraph) Exec(ctx context.Context, tx store.Tx, mc *cache.Cache) (*dsr.GetGraphResponse, error) {
	return mc.GetGraph(i.GetGraphRequest, getRelations(ctx, tx))
}
