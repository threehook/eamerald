package bdb

import (
	"bytes"

	"github.com/aserto-dev/azm/mempool"
)

// maxRelationFilterSize mirrors ds.maxRelationIdentifierSize: relation keys (type:id|relation|type:id|subrelation) comfortably fit this bound.
const maxRelationFilterSize = 384

// relFilterBufPool pools the scratch buffer used to build relation scan prefixes on the check/graph hot path (ScanRelationsFiltered),
// avoiding a per-candidate-edge allocation.
var relFilterBufPool = mempool.NewPool[*bytes.Buffer](func() *bytes.Buffer {
	return bytes.NewBuffer(make([]byte, 0, maxRelationFilterSize))
})
