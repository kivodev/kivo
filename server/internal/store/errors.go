package store

import (
	"errors"

	"github.com/ncruces/go-sqlite3"

	"github.com/mmcdole/kivo/internal/fault"
)

var (
	// ErrNewerDatabase means the database was migrated by a newer release.
	ErrNewerDatabase = fault.New(fault.Internal, "database is newer than this release")
	// ErrOlderDatabase means the database is older than this release's
	// baseline migration, so this release can't upgrade it.
	ErrOlderDatabase = fault.New(fault.Internal, "database is too old for this release to upgrade")
	// ErrNestedWrite means Write was called inside another Write, which would
	// deadlock on the single writer connection.
	ErrNestedWrite = fault.New(fault.Internal, "write inside a write")
	// ErrLowSpace means there isn't enough free space for a snapshot.
	ErrLowSpace = fault.New(fault.Internal, "not enough free space")
	// ErrInUse means another connection has the database open, as the
	// server does while it runs.
	ErrInUse = fault.New(fault.Internal, "database in use; stop the server first")
)

// busy reports whether err comes from a lock that another connection
// holds.
func busy(err error) bool {
	return errors.Is(err, sqlite3.BUSY) || errors.Is(err, sqlite3.LOCKED)
}

// IsUnique reports whether err is a unique or primary-key constraint failure.
func IsUnique(err error) bool {
	var e *sqlite3.Error
	if !errors.As(err, &e) {
		return false
	}
	x := e.ExtendedCode()
	return x == sqlite3.CONSTRAINT_UNIQUE || x == sqlite3.CONSTRAINT_PRIMARYKEY
}

// IsForeignKey reports whether err is a foreign-key constraint failure.
func IsForeignKey(err error) bool {
	var e *sqlite3.Error
	return errors.As(err, &e) && e.ExtendedCode() == sqlite3.CONSTRAINT_FOREIGNKEY
}
