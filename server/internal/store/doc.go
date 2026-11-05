// Package store opens the database, runs forward-only migrations, and takes
// snapshots and backups. It owns the DDL and the full-text search rules
// that the SQL calls into (word folding and match_rank). It holds no other
// domain logic. No other package imports the SQLite driver. driver.go is
// the only file that names it, and errors.go maps the error codes the
// server checks.
package store
