package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/health"
)

//go:embed migrations/*.sql
var migrations embed.FS

// DB is the server's single SQLite database. It has a writer pool of one
// connection, which serializes every write, and a read-only pool for
// everything else.
type DB struct {
	path    string
	writer  *sql.DB
	reader  *sql.DB
	writeMu sync.Mutex // held for the whole of each Write

	problems []health.Problem
}

// Open opens the database at path, creating it if needed, and applies
// pending migrations. It warns if path is on a network or FUSE filesystem.
func Open(ctx context.Context, path string) (*DB, error) {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	db, err := open(ctx, path, sub)
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	return db, nil
}

// open is Open with the given migrations.
func open(ctx context.Context, path string, migs fs.FS) (_ *DB, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	path = abs
	db := &DB{path: path}
	defer func() {
		if err != nil {
			db.Close()
		}
	}()

	if db.writer, err = openWriter(path); err != nil {
		return nil, err
	}
	db.writer.SetMaxOpenConns(1)
	db.writer.SetConnMaxIdleTime(0)
	if db.reader, err = openReader(path); err != nil {
		return nil, err
	}
	db.reader.SetMaxOpenConns(max(4, runtime.NumCPU()))

	// journal_mode is stored in the file, so setting it once is enough.
	if _, err = db.writer.ExecContext(ctx, `PRAGMA journal_mode = WAL; PRAGMA journal_size_limit = 67108864`); err != nil {
		return nil, err
	}
	if err = db.migrate(ctx, migs); err != nil {
		return nil, err
	}

	if kind := networkFS(filepath.Dir(path)); kind != "" {
		p := health.Problem{
			Component: "store",
			Code:      "databaseNotLocal",
			Message:   fmt.Sprintf("The database is on a %s filesystem (%s). Network and FUSE filesystems can corrupt SQLite databases.", kind, path),
			RaisedAt:  time.Now(),
			Severity:  health.Warning,
		}
		db.problems = append(db.problems, p)
		slog.WarnContext(ctx, "store: database not on a local disk", "path", path, "filesystem", kind)
	}
	return db, nil
}

// Close closes both pools. The last connection to close checkpoints the WAL.
func (db *DB) Close() error {
	var errs []error
	if db.reader != nil {
		errs = append(errs, db.reader.Close())
	}
	if db.writer != nil {
		errs = append(errs, db.writer.Close())
	}
	return errors.Join(errs...)
}

// Path is the database file's absolute path.
func (db *DB) Path() string { return db.path }

// Reader returns the read-only pool, for single-statement reads. It
// satisfies sqlc's DBTX.
func (db *DB) Reader() *sql.DB { return db.reader }

// Health implements health.Reporter.
func (db *DB) Health(context.Context) []health.Problem { return db.problems }
