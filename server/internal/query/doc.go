// Package query answers every read the UI needs, filtered by a users.Viewer.
// It writes nothing, and reads any table through SQL rather than through
// other packages' Go APIs, so each answer is one query (or a few, in one
// read transaction) over indexes.
//
// Every query carries the same Viewer predicate, so what a user can't see
// is indistinguishable from what doesn't exist: both are ErrNotFound. A work
// has one WorkSummary everywhere: in grids, rows, children, search and its
// WorkDetail.
//
// A library's grid, a smart list, a smart list's row on Home and a query a
// client sends are each a WorkQuery, and compile turns every one of them
// into its statements.
package query
