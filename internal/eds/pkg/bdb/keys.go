package bdb

// Byte values must match ds.TypeIDSeparator / ds.InstanceSeparator exactly: they are part of the on-disk key encoding of existing BoltDB stores.
const (
	typeIDSeparator   byte = ':'
	instanceSeparator byte = '|'
)

// objectKey builds the "type:id" key used for the objects bucket.
func objectKey(objectType, objectID string) []byte {
	key := make([]byte, 0, len(objectType)+len(objectID)+1)
	key = append(key, objectType...)
	key = append(key, typeIDSeparator)
	key = append(key, objectID...)

	return key
}
