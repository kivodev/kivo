package store

import (
	"context"
	"slices"
)

// BatchSize is how many rows one write transaction of a large write
// changes. A large write is a sequence of such transactions. Each one is
// short, consistent and safe to repeat. This way the single writer is
// never held for long, and an interrupted write resumes where it stopped.
const BatchSize = 500

// Drain works through the rows that a condition in the database selects.
// Each step runs in its own write transaction, until a step reports fewer
// than BatchSize rows. Drain returns step's first error, or ctx's error
// when ctx is done between steps. Batches already committed stay
// committed.
//
// A step takes at most BatchSize rows that still match the condition, and
// changes them so they no longer match. That makes a step safe to repeat,
// and an interrupted Drain resumes when it's run again. A step selects its
// rows with `WHERE rowid IN (SELECT rowid ... LIMIT ?)`, or with an id IN
// subquery for a WITHOUT ROWID table.
func (db *DB) Drain(ctx context.Context, step func(ctx context.Context, tx *Tx) (n int, err error)) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var n int
		err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
			var err error
			n, err = step(ctx, tx)
			return err
		})
		if err != nil {
			return err
		}
		if n < BatchSize {
			return nil
		}
	}
}

// Batches works through a set that exists only in memory, such as the
// files a scan found missing. It runs one write transaction per BatchSize
// items, in order. It returns step's first error, or ctx's error when ctx
// is done between batches. Batches already committed stay committed. step
// must change only rows still in the state it targets, so a batch run
// again after an interruption does no harm.
func Batches[T any](ctx context.Context, db *DB, items []T, step func(ctx context.Context, tx *Tx, batch []T) error) error {
	for batch := range slices.Chunk(items, BatchSize) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := db.Write(ctx, func(ctx context.Context, tx *Tx) error { return step(ctx, tx, batch) }); err != nil {
			return err
		}
	}
	return nil
}
