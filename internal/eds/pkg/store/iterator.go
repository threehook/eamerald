package store

// Iterator moves forward over a decoded sequence of rows. Token reports a resume point for the row last returned by
// Value, suitable for a later ScanObjects/ScanRelations pageToken argument. Close must be called once the caller is
// done with the iterator, whether or not it was drained to completion: a SQL-backed implementation may hold a
// connection or result set open until then. A BoltDB-backed implementation may treat it as a no-op.
type Iterator[T any] interface {
	Next() bool
	Value() T
	Token() string
	Close() error
}

// Page consumes up to pageSize items from it, returning them along with a continuation token when more items remain
// (empty when the scan is exhausted). Call it once per page, mirroring a single bounded scan.
func Page[T any](it Iterator[T], pageSize int32) ([]T, string) {
	var (
		items []T
		token string
	)

	for it.Next() {
		items = append(items, it.Value())

		if len(items) == int(pageSize) {
			if it.Next() {
				token = it.Token()
			}

			break
		}
	}

	return items, token
}
