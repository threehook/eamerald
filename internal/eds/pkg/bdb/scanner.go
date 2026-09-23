package bdb

import (
	"bytes"
	"context"

	"github.com/aserto-dev/azm/graph"
	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	bolt "go.etcd.io/bbolt"
)

type Iterator[T any, M Message[T]] interface {
	Next() bool       // move cursor to next element.
	RawKey() []byte   // return raw key ([]byte).
	RawValue() []byte // return raw value ([]byte).
	Key() string      // return key (string).
	Value() M         // return typed value (M).
	Delete() error    // delete element underneath cursor.
}

type ScanIterator[T any, M Message[T]] struct {
	ctx   context.Context
	tx    *bolt.Tx
	c     *bolt.Cursor
	args  *ScanArgs
	init  bool
	key   []byte
	value []byte
}

type ScanOption func(*ScanArgs)

type ScanArgs struct {
	startToken []byte
	keyFilter  []byte
}

func WithPageToken(token string) ScanOption {
	return func(a *ScanArgs) {
		a.startToken = []byte(token)
	}
}

func WithKeyFilter(filter []byte) ScanOption {
	return func(a *ScanArgs) {
		a.keyFilter = filter
	}
}

func NewScanIterator[T any, M Message[T]](ctx context.Context, tx *bolt.Tx, path Path, opts ...ScanOption) (*ScanIterator[T, M], error) {
	args := &ScanArgs{startToken: nil, keyFilter: nil}
	for _, opt := range opts {
		opt(args)
	}

	if len(args.startToken) == 0 && len(args.keyFilter) != 0 {
		args.startToken = args.keyFilter
	}

	b, err := SetBucket(tx, path)
	if err != nil {
		return nil, errors.Wrapf(ErrPathNotFound, "path [%s]", path)
	}

	return &ScanIterator[T, M]{ctx: ctx, tx: tx, c: b.Cursor(), args: args, init: false}, nil
}

func (s *ScanIterator[T, M]) Next() bool {
	if s.init {
		s.key, s.value = s.c.Next()
	}

	if !s.init {
		if s.args.startToken == nil {
			s.key, s.value = s.c.First()
		} else {
			s.key, s.value = s.c.Seek(s.args.startToken)
		}

		s.init = true
	}

	return s.key != nil && bytes.HasPrefix(s.key, s.args.keyFilter)
}

func (s *ScanIterator[T, M]) RawKey() []byte {
	return s.key
}

func (s *ScanIterator[T, M]) RawValue() []byte {
	return s.value
}

func (s *ScanIterator[T, M]) Key() string {
	return string(s.key)
}

func (s *ScanIterator[T, M]) Value() M {
	msg, err := unmarshal[T, M](s.value)
	if err != nil {
		var result M
		return result
	}

	return msg
}

func (s *ScanIterator[T, M]) Delete() error {
	if s.key != nil {
		log.Trace().Str("key", s.Key()).Msg("delete")
		return s.c.Delete()
	}

	return nil
}

func Scan[T any, M Message[T]](ctx context.Context, tx *bolt.Tx, path Path, keyFilter []byte) ([]M, error) {
	b, err := SetBucket(tx, path)
	if err != nil {
		return nil, errors.Wrapf(ErrPathNotFound, "path [%s]", path)
	}

	c := b.Cursor()

	var results []M

	for k, v := c.Seek(keyFilter); k != nil && bytes.HasPrefix(k, keyFilter); k, v = c.Next() {
		m, err := unmarshal[T, M](v)
		if err != nil {
			return nil, err
		}

		results = append(results, m)
	}

	return results, nil
}

func ScanWithFilter(
	ctx context.Context,
	tx *bolt.Tx,
	path Path,
	keyFilter []byte,
	valueFilter func(*dsc.RelationIdentifier) bool,
	pool graph.RelationPool,
	out *[]*dsc.RelationIdentifier,
) error {
	b, err := SetBucket(tx, path)
	if err != nil {
		return errors.Wrapf(ErrPathNotFound, "path [%s]", path)
	}

	c := b.Cursor()

	if valueFilter == nil {
		valueFilter = func(_ *dsc.RelationIdentifier) bool { return true }
	}

	results := *out

	for k, v := c.Seek(keyFilter); k != nil && bytes.HasPrefix(k, keyFilter); k, v = c.Next() {
		m := pool.Get()
		if err := unmarshalTo(v, m); err != nil {
			return err
		}

		if valueFilter(m) {
			results = append(results, m)
		}
	}

	*out = results

	return nil
}

func KeyPrefixExists[T any, M Message[T]](ctx context.Context, tx *bolt.Tx, path Path, keyFilter []byte) (bool, error) {
	b, err := SetBucket(tx, path)
	if err != nil {
		return false, errors.Wrapf(ErrPathNotFound, "path [%s]", path)
	}

	c := b.Cursor()

	k, _ := c.Seek(keyFilter)

	return (k != nil && bytes.HasPrefix(k, keyFilter)), nil
}
