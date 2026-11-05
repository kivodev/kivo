package storetest

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mmcdole/kivo/internal/store"
)

var (
	templateOnce sync.Once
	templatePath string
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
	if err := copyFile(templatePath, path); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func buildTemplate() {
	dir, err := os.MkdirTemp("", "kivo-storetest-")
	if err != nil {
		templateErr = err
		return
	}
	templatePath = filepath.Join(dir, "template.db")
	db, err := store.Open(context.Background(), templatePath)
	if err != nil {
		templateErr = err
		return
	}
	templateErr = db.Close() // closing the last connection folds the WAL into the file
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
