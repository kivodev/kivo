package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"strconv"
	"strings"
)

// Version returns the applied migration version, PRAGMA user_version.
func (db *DB) Version(ctx context.Context) (int, error) {
	var v int
	if err := db.writer.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

// migrate brings the database to the latest migration. The first migration
// is the baseline, the whole schema as of its version: a new database gets
// it and then every migration after it. A database at the baseline's
// version or later gets the migrations after its own version. One below
// the baseline was made by a release too old for this one to upgrade, and
// is refused, as is one newer than the latest migration.
func (db *DB) migrate(ctx context.Context, migs fs.FS) error {
	all, err := loadMigrations(migs)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return errors.New("no migrations")
	}
	cur, err := db.Version(ctx)
	if err != nil {
		return err
	}
	base, latest := all[0].version, all[len(all)-1].version
	switch {
	case cur > latest:
		return fmt.Errorf("%w: database is at version %d, this release knows %d; run the newer release or restore a snapshot", ErrNewerDatabase, cur, latest)
	case cur > 0 && cur < base:
		return fmt.Errorf("%w: database is at version %d, and this release upgrades from version %d on; upgrade it with an earlier release first", ErrOlderDatabase, cur, base)
	}
	pending := all
	if cur > 0 {
		pending = all[cur-base+1:]
	}
	if cur > 0 && len(pending) > 0 {
		// Take a snapshot before upgrading. Rolling back means running the
		// previous binary with this file, restored with `kivo restore`.
		snap := fmt.Sprintf("%s.v%d.snapshot", db.path, cur)
		slog.InfoContext(ctx, "store: snapshot before migrating", "path", snap)
		if err := db.Snapshot(ctx, snap); err != nil {
			return fmt.Errorf("snapshot before migrating: %w", err)
		}
	}
	for _, m := range pending {
		slog.InfoContext(ctx, "store: applying migration", "name", m.name)
		if err := db.apply(ctx, m); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.name, err)
		}
	}
	return nil
}

type migration struct {
	version int
	name    string
	sql     string
}

// loadMigrations reads the migrations. They are plain SQL files named
// NNNN_description.sql, numbered on from the baseline's number with no
// gaps, and they only ever go forward.
func loadMigrations(migs fs.FS) ([]migration, error) {
	names, err := fs.Glob(migs, "*.sql")
	if err != nil {
		return nil, err
	}
	slices.Sort(names)
	out := make([]migration, 0, len(names))
	for i, name := range names {
		num, _, ok := strings.Cut(name, "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil || v < 1 {
			return nil, fmt.Errorf("migration %s: name must be NNNN_description.sql", name)
		}
		if i > 0 && v != out[0].version+i {
			return nil, fmt.Errorf("migration %s: expected version %d", name, out[0].version+i)
		}
		b, err := fs.ReadFile(migs, name)
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, name: name, sql: string(b)})
	}
	return out, nil
}

// fkOffDirective, as a migration's first line, runs the migration with
// foreign keys off. A migration that rebuilds a table needs this. The keys
// are checked with foreign_key_check before commit.
const fkOffDirective = "-- kivo:foreign_keys off"

func (db *DB) apply(ctx context.Context, m migration) (err error) {
	fkOff := strings.HasPrefix(m.sql, fkOffDirective)
	// The writer pool has one connection, so these pragmas and the
	// transaction below run on the same connection.
	if fkOff {
		if _, err := db.writer.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
			return err
		}
		defer func() {
			if _, e := db.writer.ExecContext(context.WithoutCancel(ctx), `PRAGMA foreign_keys = ON`); e != nil && err == nil {
				err = e
			}
		}()
	}

	tx, err := db.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, m.sql); err != nil {
		return err
	}
	if fkOff {
		rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
		if err != nil {
			return err
		}
		bad := rows.Next()
		if err := rows.Close(); err != nil {
			return err
		}
		if bad {
			return errors.New("foreign_key_check found violations")
		}
	}
	// PRAGMA can't take a bound parameter. The version is an int we
	// control, so formatting it in is safe.
	if _, err = tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, m.version)); err != nil {
		return err
	}
	return tx.Commit()
}
