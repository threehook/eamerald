package pgdb

import (
	"net/http"

	cerr "github.com/aserto-dev/errors"
	"google.golang.org/grpc/codes"
)

// ErrNotFound is returned by Get* methods when no row matches. It maps to codes.NotFound via status.Code(err), the same contract
// internal/eds/pkg/bdb's errors satisfy, so callers written against store.Tx don't need to know which backend they're talking to.
var ErrNotFound = cerr.NewAsertoError("E20150", codes.NotFound, http.StatusNotFound, "key not found")
