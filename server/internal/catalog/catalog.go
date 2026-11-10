package catalog

import (
	"database/sql"
	"errors"

	"github.com/mmcdole/kivo/internal/catalog/dbq"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/store"
)

var (
	// ErrNotFound means no such library, folder, file, version, work, person
	// or series.
	ErrNotFound = fault.New(fault.NotFound, "not found")
	// ErrStale means a write was conditioned on a file generation that has
	// since moved on. The caller discards its result.
	ErrStale = fault.New(fault.Conflict, "stale")
	// ErrInvalid means the input breaks an integrity rule.
	ErrInvalid = fault.New(fault.Invalid, "invalid")
	// ErrNotABook means a batch edit of books holds a work that isn't one.
	ErrNotABook = fault.New(fault.Invalid, "only books are edited together")
	// ErrFolderInUse means a library folder already belongs to another
	// library, or to this one while it's being removed.
	ErrFolderInUse = fault.New(fault.Conflict, "folder is already in a library")
	// ErrPresent means a file that was to be deleted as missing is on disk.
	ErrPresent = fault.Coded(fault.Conflict, "fileStillPresent", "only a missing file can be deleted")
	// ErrUnreachable means a library folder can't be read, as when its disk
	// or share is offline. The work that hit it waits and tries again.
	ErrUnreachable = errors.New("library folder unreachable")
)

// Catalog holds what exists. A write takes the caller's transaction, so a
// change that spans packages is one atomic write. For example, identify
// reassigns a version while userstate moves its watch state. A write that
// changes which works are available is a method on Batch, which rebuilds
// each touched work's work_library row once, when the batch ends. Only an
// admin's edits and deletions open their own transaction.
type Catalog struct {
	db  *store.DB
	q   *dbq.Queries
	bus *events.Bus
}

// New returns the catalog over db. Once each write commits, it publishes
// libraryChanged on bus for the libraries the write changed. A library
// changes when it is created, edited or starts to be deleted. Editing
// covers renaming it and changing its folders or settings. A library also
// changes when its root works change, that is when a movie, series, book
// or video becomes available in it or stops being available. That happens
// as a root work is added, matched, unmatched, moved, gone or deleted.
func New(db *store.DB, bus *events.Bus) *Catalog {
	return &Catalog{db: db, q: dbq.New(db.Reader()), bus: bus}
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func nullInt(n int) sql.NullInt64 { return sql.NullInt64{Int64: int64(n), Valid: n != 0} }

func nullIntPtr(n *int) sql.NullInt64 {
	if n == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*n), Valid: true}
}

// notFound maps a query's sql.ErrNoRows to ErrNotFound and returns any
// other err unchanged.
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
