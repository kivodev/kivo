package storetest

import (
	"context"
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
