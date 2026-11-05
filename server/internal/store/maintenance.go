package store

import (
	"context"
	"fmt"
)

const checkpointSQL = `PRAGMA wal_checkpoint(TRUNCATE)`

// Checkpoint copies the WAL into the database and truncates it.
func (db *DB) Checkpoint(ctx context.Context) error {
	_, err := db.writer.ExecContext(ctx, checkpointSQL)
	return err
}

// Optimize refreshes the query planner's statistics where they've drifted
// (SQLite's PRAGMA optimize) and checkpoints the WAL. The nightly
// "Optimize database" task calls it.
func (db *DB) Optimize(ctx context.Context) error {
	for _, q := range []string{`PRAGMA optimize`, checkpointSQL} {
		if _, err := db.writer.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("run %s: %w", q, err)
		}
	}
	return nil
}
