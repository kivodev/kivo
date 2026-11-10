package storetest

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/mmcdole/kivo/internal/store"
)

func TestNewGivesEachTestItsOwnDatabase(t *testing.T) {
	a, b := New(t), New(t)
	if a.Path() == b.Path() {
		t.Fatal("two calls shared a database file")
	}
	err := a.Write(context.Background(), func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TABLE only_in_a (x INTEGER)`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := b.Reader().QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name = 'only_in_a'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("b sees a's table (n=%d, err=%v)", n, err)
	}
}

func TestNewLeavesNothingInTheTempDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	templateOnce = sync.Once{} // build the template again, under tmp
	New(t)
	left, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("left behind in the temp dir: %v", left)
	}
}
