package pgdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var errTokenSize = errors.New("decode page token: unexpected value count")

// encodeToken/decodeToken represent a keyset-pagination resume point as a JSON array of the ordering column values for the last row returned,
// avoiding any ambiguity from delimiter characters appearing inside real data values.
func encodeToken(vals ...string) string {
	b, err := json.Marshal(vals)
	if err != nil {
		return ""
	}

	return string(b)
}

func decodeToken(token string, n int) ([]string, error) {
	var vals []string
	if err := json.Unmarshal([]byte(token), &vals); err != nil {
		return nil, fmt.Errorf("decode page token: %w", err)
	}

	if len(vals) != n {
		return nil, fmt.Errorf("%w: expected %d, got %d", errTokenSize, n, len(vals))
	}

	return vals, nil
}

func whereClause(conds []string) string {
	if len(conds) == 0 {
		return ""
	}

	return " WHERE " + strings.Join(conds, " AND ")
}
