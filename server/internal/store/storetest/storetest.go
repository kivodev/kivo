package storetest

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mmcdole/kivo/internal/store"
)

var (
	templateOnce sync.Once
	template     []byte
	templateErr  error
)

// New returns a migrated database in t's temp directory, closed when the
// test ends. Migrations run once per test binary, and each test gets a
// copy.
func New(t testing.TB) *store.DB {
	t.Helper()
	templateOnce.Do(buildTemplate)
	if templateErr != nil {
		t.Fatalf("storetest: template: %v", templateErr)
	}
	path := filepath.Join(t.TempDir(), "kivo.db")
	if err := os.WriteFile(path, template, 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// buildTemplate migrates a database and keeps the file's bytes in memory,
// so nothing is left on disk when the test binary exits.
func buildTemplate() {
	dir, err := os.MkdirTemp("", "kivo-storetest-")
	if err != nil {
		templateErr = err
		return
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "template.db")
	db, err := store.Open(context.Background(), path)
	if err != nil {
		templateErr = err
		return
	}
	// Closing the last connection folds the WAL into the file.
	if templateErr = db.Close(); templateErr != nil {
		return
	}
	template, templateErr = os.ReadFile(path)
}
