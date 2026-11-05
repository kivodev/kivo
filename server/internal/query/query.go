package query

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/store"
)

var (
	// ErrNotFound means there is no such work, library or collection, or the
	// Viewer may not see it. The two are deliberately the same.
	ErrNotFound = fault.New(fault.NotFound, "not found")
	// ErrInvalid means a request can't be answered as asked: an unknown sort
	// or filter value, or a cursor from another listing.
	ErrInvalid = fault.New(fault.Invalid, "invalid request")
)

// Service answers the UI's reads. It holds no state and caches nothing, so a
// change is visible to the next read.
type Service struct {
	db *store.DB
}

// New returns the Service over db. It reads through the read-only pool.
func New(db *store.DB) *Service { return &Service{db: db} }

// invalid returns ErrInvalid about the request parameter param, with its
// field code and the reason.
func invalid(param, code, why string) error {
	return fault.About(fmt.Errorf("%w: %s", ErrInvalid, why), fault.Field{Name: param, Code: code})
}

// notFound maps a query's error. A missing row becomes ErrNotFound, and
// any other error is returned unchanged.
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
