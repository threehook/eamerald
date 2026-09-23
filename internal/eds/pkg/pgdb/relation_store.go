package pgdb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/aserto-dev/azm/graph"
	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	"github.com/threehook/eamerald/internal/eds/pkg/store"
)

const (
	colObjectType      = "object_type"
	colObjectID        = "object_id"
	colRelation        = "relation"
	colSubjectType     = "subject_type"
	colSubjectID       = "subject_id"
	colSubjectRelation = "subject_relation"
)

// objectOrderCols/subjectOrderCols mirror the two indexes created in migration 000001: relations_by_object and relations_by_subject.
// Ordering scans by these column lists lets keyset pagination use a plain row-value comparison ("(cols...) > (vals...)") without needing
// a separate OFFSET.
var (
	objectOrderCols  = []string{colObjectType, colObjectID, colRelation, colSubjectType, colSubjectID, colSubjectRelation}
	subjectOrderCols = []string{colSubjectType, colSubjectID, colRelation, colObjectType, colObjectID, colSubjectRelation}
)

func orderCols(dir store.Direction) []string {
	if dir == store.BySubject {
		return subjectOrderCols
	}

	return objectOrderCols
}

// relationConditions translates filter's populated fields into column/value pairs. Unlike bdb's byte-prefix scan, every populated field narrows
// the query directly, regardless of which fields are set or in what order.
func relationConditions(filter store.RelationFilter) []struct{ column, value string } {
	var conds []struct{ column, value string }

	add := func(column, value string) {
		conds = append(conds, struct{ column, value string }{column, value})
	}

	if filter.ObjectType != "" {
		add(colObjectType, filter.ObjectType)
	}

	if filter.ObjectID != "" {
		add(colObjectID, filter.ObjectID)
	}

	if filter.Relation != "" {
		add(colRelation, filter.Relation)
	}

	if filter.SubjectType != "" {
		add(colSubjectType, filter.SubjectType)
	}

	if filter.SubjectID != "" {
		add(colSubjectID, filter.SubjectID)
	}

	if filter.HasSubjectRelation {
		add(colSubjectRelation, filter.SubjectRelation)
	}

	return conds
}

func (t *tx) GetRelationExact(ctx context.Context, ident *dsc.RelationIdentifier) (*dsc.Relation, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var data []byte

	err := t.tx.QueryRow(ctx, `
		SELECT data FROM relations
		WHERE object_type = $1 AND object_id = $2 AND relation = $3 AND subject_type = $4 AND subject_id = $5 AND subject_relation = $6
	`, ident.GetObjectType(), ident.GetObjectId(), ident.GetRelation(), ident.GetSubjectType(), ident.GetSubjectId(), ident.GetSubjectRelation(),
	).Scan(&data)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("get relation: %w", err)
	}

	rel := &dsc.Relation{}
	if err := proto.Unmarshal(data, rel); err != nil {
		return nil, fmt.Errorf("unmarshal relation: %w", err)
	}

	return rel, nil
}

func (t *tx) SetRelation(ctx context.Context, rel *dsc.Relation) (*dsc.Relation, error) {
	data, err := proto.Marshal(rel)
	if err != nil {
		return nil, fmt.Errorf("marshal relation: %w", err)
	}

	createdAt := rel.GetCreatedAt().AsTime() //nolint:staticcheck // Marked as deprecated

	t.mu.Lock()
	defer t.mu.Unlock()

	_, err = t.tx.Exec(ctx, `
		INSERT INTO relations (object_type, object_id, relation, subject_type, subject_id, subject_relation, data, etag, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (object_type, object_id, relation, subject_type, subject_id, subject_relation)
		DO UPDATE SET data = EXCLUDED.data, etag = EXCLUDED.etag, updated_at = EXCLUDED.updated_at
	`, rel.GetObjectType(), rel.GetObjectId(), rel.GetRelation(), rel.GetSubjectType(), rel.GetSubjectId(), rel.GetSubjectRelation(),
		data, rel.GetEtag(), createdAt, rel.GetUpdatedAt().AsTime())
	if err != nil {
		return nil, fmt.Errorf("set relation: %w", err)
	}

	return rel, nil
}

func (t *tx) DeleteRelation(ctx context.Context, ident *dsc.RelationIdentifier) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	_, err := t.tx.Exec(ctx, `
		DELETE FROM relations
		WHERE object_type = $1 AND object_id = $2 AND relation = $3 AND subject_type = $4 AND subject_id = $5 AND subject_relation = $6
	`, ident.GetObjectType(), ident.GetObjectId(), ident.GetRelation(), ident.GetSubjectType(), ident.GetSubjectId(), ident.GetSubjectRelation())
	if err != nil {
		return fmt.Errorf("delete relation: %w", err)
	}

	return nil
}

func (t *tx) ScanRelations(
	ctx context.Context, dir store.Direction, filter store.RelationFilter, pageToken string,
) (store.Iterator[*dsc.Relation], error) {
	cols := orderCols(dir)
	conds := relationConditions(filter)

	where := make([]string, 0, len(conds)+1)
	args := make([]any, 0, len(conds)+len(cols))

	for _, c := range conds {
		args = append(args, c.value)
		where = append(where, fmt.Sprintf("%s = $%d", c.column, len(args)))
	}

	if pageToken != "" {
		vals, err := decodeToken(pageToken, len(cols))
		if err != nil {
			return nil, err
		}

		placeholders := make([]string, len(cols))

		for i, v := range vals {
			args = append(args, v)
			placeholders[i] = fmt.Sprintf("$%d", len(args))
		}

		// >=, not >: the token is the peeked-but-not-yet-returned row's own key (mirroring bbolt's Seek, which is inclusive of the exact key),
		// so it must be included in the resumed page.
		where = append(where, fmt.Sprintf("(%s) >= (%s)", strings.Join(cols, ", "), strings.Join(placeholders, ", ")))
	}

	query := "SELECT object_type, object_id, relation, subject_type, subject_id, subject_relation, data FROM relations" +
		whereClause(where) + " ORDER BY " + strings.Join(cols, ", ")

	t.mu.Lock()
	defer t.mu.Unlock()

	rows, err := t.tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("scan relations: %w", err)
	}

	return &relationIterator{rows: rows, cols: cols, mu: t.mu}, nil
}

// ScanRelationsFiltered is the check/graph hot path: it decodes matches directly into pool-provided *dsc.RelationIdentifier instances (reusing them
// across calls, per graph.RelationPool's contract) rather than allocating one per row, and applies valueFilter before appending to out.
// There is no bdb-style byte-prefix scan to replicate here: every populated filter field becomes its own WHERE clause, so Postgres does the narrowing
// that bdb's ObjFilter/SubFilter + client-side valueFilter used to split across two steps.
func (t *tx) ScanRelationsFiltered(
	ctx context.Context, dir store.Direction, filter store.RelationFilter,
	valueFilter func(*dsc.RelationIdentifier) bool, pool graph.RelationPool, out *[]*dsc.RelationIdentifier,
) error {
	_ = dir // the index chosen by dir doesn't affect which rows an unordered, fully-filtered scan returns.

	conds := relationConditions(filter)
	where := make([]string, 0, len(conds))
	args := make([]any, 0, len(conds))

	for _, c := range conds {
		args = append(args, c.value)
		where = append(where, fmt.Sprintf("%s = $%d", c.column, len(args)))
	}

	query := "SELECT object_type, object_id, relation, subject_type, subject_id, subject_relation FROM relations" + whereClause(where)

	t.mu.Lock()
	defer t.mu.Unlock()

	rows, err := t.tx.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("scan relations filtered: %w", err)
	}
	defer rows.Close()

	results := *out

	for rows.Next() {
		m := pool.Get()

		if err := rows.Scan(&m.ObjectType, &m.ObjectId, &m.Relation, &m.SubjectType, &m.SubjectId, &m.SubjectRelation); err != nil {
			return fmt.Errorf("scan relation identifier: %w", err)
		}

		if valueFilter(m) {
			results = append(results, m)
		} else {
			pool.Put(m)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("scan relations filtered: %w", err)
	}

	*out = results

	return nil
}

func (t *tx) RelationsExistForObject(ctx context.Context, dir store.Direction, objectType, objectID string) (bool, error) {
	typeCol, idCol := colObjectType, colObjectID
	if dir == store.BySubject {
		typeCol, idCol = colSubjectType, colSubjectID
	}

	var exists bool

	query := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM relations WHERE %s = $1 AND %s = $2)`, typeCol, idCol)

	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.tx.QueryRow(ctx, query, objectType, objectID).Scan(&exists); err != nil {
		return false, fmt.Errorf("relations exist for object: %w", err)
	}

	return exists, nil
}

type relationIterator struct {
	rows  pgx.Rows
	mu    *sync.Mutex
	cols  []string
	key   [6]string
	value *dsc.Relation
}

var _ store.Iterator[*dsc.Relation] = (*relationIterator)(nil)

func (r *relationIterator) Next() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.rows.Next() {
		return false
	}

	var data []byte
	if err := r.rows.Scan(&r.key[0], &r.key[1], &r.key[2], &r.key[3], &r.key[4], &r.key[5], &data); err != nil {
		return false
	}

	rel := &dsc.Relation{}
	if err := proto.Unmarshal(data, rel); err == nil {
		r.value = rel
	} else {
		r.value = &dsc.Relation{}
	}

	return true
}

func (r *relationIterator) Value() *dsc.Relation { return r.value }

// Token encodes the current row's values in the same order as cols (object- or subject-indexed order), so it can be plugged directly into a later
// ScanRelations pageToken's row-value comparison.
func (r *relationIterator) Token() string {
	ordered := map[string]string{
		colObjectType: r.key[0], colObjectID: r.key[1], colRelation: r.key[2],
		colSubjectType: r.key[3], colSubjectID: r.key[4], colSubjectRelation: r.key[5],
	}

	vals := make([]string, len(r.cols))
	for i, c := range r.cols {
		vals[i] = ordered[c]
	}

	return encodeToken(vals...)
}

// Close releases the underlying pgx.Rows/connection; see objectIterator.Close for why this must always be called.
func (r *relationIterator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rows.Close()

	return r.rows.Err()
}
