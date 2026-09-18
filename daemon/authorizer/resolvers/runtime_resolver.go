package resolvers

import (
	"context"

	"github.com/threehook/eamerald/internal/runtime"
)

type RuntimeResolver interface {
	GetRuntime(ctx context.Context) (*runtime.Runtime, error)
}
