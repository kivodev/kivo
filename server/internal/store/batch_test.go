package store

import (
	"context"
	"errors"
	"testing"
)

// fill inserts items 1 to n.
func fill(t *testing.T, db *DB, n int) {
	t.Helper()
	err := db.Write(context.Background(), func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `WITH RECURSIVE s (i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM s WHERE i < ?)
			INSERT INTO item (id, name) SELECT i, 'item ' || i FROM s`, n)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, db *DB) int {
	t.Helper()
	var n int
	if err := db.Reader().QueryRow(`SELECT count(*) FROM item`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// deleteBatch is a Drain step that deletes at most BatchSize items.
func deleteBatch(ctx context.Context, tx *Tx) (int, error) {
	res, err := tx.ExecContext(ctx, `DELETE FROM item WHERE rowid IN (SELECT rowid FROM item LIMIT ?)`, BatchSize)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

func TestDrainRepeatsUntilABatchIsShort(t *testing.T) {
	for _, c := range []struct{ rows, steps int }{
		{0, 1},
		{BatchSize - 1, 1},
		{BatchSize, 2}, // a full batch can't tell it was the last one
		{2*BatchSize + 234, 3},
	} {
		db, _ := testDB(t, basic)
		fill(t, db, c.rows)
		steps := 0
		err := db.Drain(context.Background(), func(ctx context.Context, tx *Tx) (int, error) {
			steps++
			return deleteBatch(ctx, tx)
		})
		if err != nil {
			t.Fatal(err)
		}
		if steps != c.steps || count(t, db) != 0 {
			t.Errorf("%d rows: %d steps, %d rows left; want %d steps, none left", c.rows, steps, count(t, db), c.steps)
		}
	}
}

func TestDrainResumesAfterAnError(t *testing.T) {
	db, _ := testDB(t, basic)
	fill(t, db, 3*BatchSize+10)
	boom := errors.New("boom")
	steps := 0
	err := db.Drain(context.Background(), func(ctx context.Context, tx *Tx) (int, error) {
		if steps++; steps == 2 {
			n, _ := deleteBatch(ctx, tx) // rolled back with the error
			return n, boom
		}
		return deleteBatch(ctx, tx)
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if got := count(t, db); got != 2*BatchSize+10 {
		t.Fatalf("%d rows left; want the first batch committed and the failed one rolled back", got)
	}
	if err := db.Drain(context.Background(), deleteBatch); err != nil || count(t, db) != 0 {
		t.Fatalf("running it again: %v, %d rows left", err, count(t, db))
	}
}

func TestDrainStopsWhenCancelled(t *testing.T) {
	db, _ := testDB(t, basic)
	fill(t, db, 3*BatchSize)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	steps := 0
	err := db.Drain(ctx, func(ctx context.Context, tx *Tx) (int, error) {
		if steps++; steps == 2 {
			cancel() // the batch doesn't commit, and no other starts
		}
		return deleteBatch(ctx, tx)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if steps != 2 || count(t, db) != 2*BatchSize {
		t.Fatalf("%d steps, %d rows left; want 2 steps and only the first batch committed", steps, count(t, db))
	}
}

// insertBatch is a Batches step that adds the items not there yet.
func insertBatch(ctx context.Context, tx *Tx, batch []int) error {
	for _, i := range batch {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO item (id, name) VALUES (?, 'item ' || ?)`, i, i); err != nil {
			return err
		}
	}
	return nil
}

func items(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i + 1
	}
	return out
}

func TestBatchesCommitsOneTransactionPerBatch(t *testing.T) {
	db, _ := testDB(t, basic)
	var sizes []int
	err := Batches(context.Background(), db, items(2*BatchSize+1), func(ctx context.Context, tx *Tx, batch []int) error {
		sizes = append(sizes, len(batch))
		return insertBatch(ctx, tx, batch)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sizes) != 3 || sizes[0] != BatchSize || sizes[2] != 1 || count(t, db) != 2*BatchSize+1 {
		t.Fatalf("batches %v, %d rows", sizes, count(t, db))
	}
}

func TestBatchesResumesAfterAnError(t *testing.T) {
	db, _ := testDB(t, basic)
	all := items(3 * BatchSize)
	boom := errors.New("boom")
	calls := 0
	err := Batches(context.Background(), db, all, func(ctx context.Context, tx *Tx, batch []int) error {
		if calls++; calls == 2 {
			_ = insertBatch(ctx, tx, batch) // rolled back with the error
			return boom
		}
		return insertBatch(ctx, tx, batch)
	})
	if !errors.Is(err, boom) || calls != 2 {
		t.Fatalf("err = %v after %d calls; want boom after 2", err, calls)
	}
	if got := count(t, db); got != BatchSize {
		t.Fatalf("%d rows; want only the first batch", got)
	}
	// Each step changes only what isn't done yet, so the whole set can run
	// again.
	if err := Batches(context.Background(), db, all, insertBatch); err != nil || count(t, db) != 3*BatchSize {
		t.Fatalf("running it again: %v, %d rows", err, count(t, db))
	}
}

func TestBatchesStopsWhenCancelled(t *testing.T) {
	db, _ := testDB(t, basic)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := Batches(ctx, db, items(3*BatchSize), func(ctx context.Context, tx *Tx, batch []int) error {
		if calls++; calls == 1 {
			defer cancel() // after this batch's statements; its commit then fails
		}
		return insertBatch(ctx, tx, batch)
	})
	if !errors.Is(err, context.Canceled) || calls != 1 || count(t, db) != 0 {
		t.Fatalf("err = %v after %d calls with %d rows; want context.Canceled after 1, nothing committed", err, calls, count(t, db))
	}
}
