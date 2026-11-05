package store

import "strings"

// HandWritten returns the statement named name in src. src is a file of
// hand-written queries that sqlc can't parse, laid out as sqlc expects,
// with "-- name: Name :kind" before each statement. The statement's
// @names are bound with sql.Named, which SQLite accepts as they are. A
// name missing from src is a programming error, so HandWritten panics.
func HandWritten(src, name string) string {
	_, rest, ok := strings.Cut(src, "-- name: "+name+" ")
	if !ok {
		panic("store: no hand-written query " + name)
	}
	_, stmt, _ := strings.Cut(rest, "\n")
	stmt, _, _ = strings.Cut(stmt, ";")
	return strings.TrimSpace(stmt)
}
