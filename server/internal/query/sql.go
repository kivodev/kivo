package query

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// statement is one of the package's SQL statements. Its @name parameters
// are bound with sql.Named. Every read runs statements, and stmt records
// each one in statements. The tests read that list to check each
// statement's query plan, that it carries the Viewer predicate, and that it
// only reads.
type statement struct{ name, sql string }

var statements []statement

func stmt(name, sql string) statement {
	s := statement{name, sql}
	statements = append(statements, s)
	return s
}

// viewer is a Viewer in the form the statements' parameters take.
type viewer struct {
	user    id.ID
	visible []any // the arguments of catalog.VisibleWork
}

func viewerOf(v users.Viewer) viewer {
	return viewer{user: v.User(), visible: catalog.VisibleArgs(v)}
}

// args returns a statement's arguments: more, followed by the Viewer's.
// Every statement takes the Viewer's arguments: the user, for the user's
// state, and the values the Viewer predicate reads. A statement ignores
// the arguments it doesn't name.
func (vw viewer) args(more ...any) []any {
	return append(append(more, sql.Named("user", vw.user)), vw.visible...)
}

// set returns values as a set parameter, which a statement reads with
// json_each. It is a JSON array, or NULL when values is empty. A filter
// takes NULL as not asked for.
func set[T any](values []T) any {
	if len(values) == 0 {
		return nil
	}
	b, _ := json.Marshal(values)
	return string(b)
}

// visible is catalog.VisibleWork, the Viewer predicate on w, which every
// statement about works carries. A test looks for its "-- viewer-filter"
// marker in every statement.
var visible = catalog.VisibleWork

// libraryVisible is the Viewer predicate on a library l. The statements
// that list libraries or their files carry it in place of visible.
var libraryVisible = `-- viewer-filter: libraries
  ` + catalog.Granted("l.id")

// querier runs a statement. It is a read transaction or the reader pool.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// scanner is a row to read: *sql.Rows positioned at a row, or *sql.Row.
type scanner interface{ Scan(dest ...any) error }

// observe, when a test sets it, is told each statement as it runs.
var observe func(statement)

// each runs st and calls fn for each of its rows.
func each(ctx context.Context, q querier, st statement, args []any, fn func(scanner) error) error {
	if observe != nil {
		observe(st)
	}
	rows, err := q.QueryContext(ctx, st.sql, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// rowsOf runs st and reads each of its rows into a T with scan. It returns
// nil when there are no rows.
func rowsOf[T any](ctx context.Context, q querier, st statement, args []any, scan func(scanner, *T) error) ([]T, error) {
	var out []T
	err := each(ctx, q, st, args, func(r scanner) error {
		var v T
		if err := scan(r, &v); err != nil {
			return err
		}
		out = append(out, v)
		return nil
	})
	return out, err
}

// queryRow runs st for its one row. If there is no row, Scan returns
// sql.ErrNoRows.
func queryRow(ctx context.Context, q querier, st statement, args []any) *sql.Row {
	if observe != nil {
		observe(st)
	}
	return q.QueryRowContext(ctx, st.sql, args...)
}

// rowOf runs st and reads its one row into dest.
func rowOf(ctx context.Context, q querier, st statement, args []any, dest ...any) error {
	return queryRow(ctx, q, st, args).Scan(dest...)
}

// photoColumns are a person's photo or a trailer's thumbnail, read from the
// image i. They are all NULL when there is none.
const photoColumns = "i.id, i.blurhash, i.width, i.height"

// photo reads the photo columns.
type photo struct {
	id            id.ID
	blurhash      sql.NullString
	width, height sql.NullInt64
}

func (p *photo) dest() []any { return []any{&p.id, &p.blurhash, &p.width, &p.height} }

// image returns the photo, or nil when there is none.
func (p *photo) image() *Image {
	if p.id.IsZero() {
		return nil
	}
	return &Image{ID: p.id, Blurhash: p.blurhash.String, Width: int(p.width.Int64), Height: int(p.height.Int64)}
}
