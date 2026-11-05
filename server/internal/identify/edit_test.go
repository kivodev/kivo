package identify_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/users"
)

// TestEditWork checks that an edit's fields and content rating are written.
// An edit that can't be made in full writes nothing. That covers a work the
// Viewer can't see, and an unknown content rating or ordering.
func TestEditWork(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Movies)
	files := e.add(lib, "Heat (1995)/Heat.mkv")
	e.run(files...)
	heat := e.workOf(files[0])[0].ID
	v := e.viewer()

	r := "R"
	edit := identify.WorkEdit{Metadata: catalog.WorkMetadata{Title: "Heat!"}, Fields: []string{catalog.FieldTitle}, ContentRating: &r}
	if err := e.id.EditWork(ctx, v, heat, edit); err != nil {
		t.Fatal(err)
	}
	if w, _ := e.b.Catalog.Work(ctx, heat); w.Title != "Heat!" {
		t.Errorf("edited title %q", w.Title)
	}

	nope, sideways := "12A", "sideways" // 12A is British
	for name, tc := range map[string]struct {
		v    users.Viewer
		edit identify.WorkEdit
		err  error
	}{
		"not visible":    {users.Viewer{}, identify.WorkEdit{Metadata: catalog.WorkMetadata{Title: "Cold"}, Fields: []string{catalog.FieldTitle}}, catalog.ErrNotFound},
		"content rating": {v, identify.WorkEdit{Metadata: catalog.WorkMetadata{Title: "Cold"}, Fields: []string{catalog.FieldTitle}, ContentRating: &nope}, identify.ErrContentRating},
		"ordering":       {v, identify.WorkEdit{Metadata: catalog.WorkMetadata{Title: "Cold"}, Fields: []string{catalog.FieldTitle}, Ordering: &sideways}, identify.ErrOrdering},
	} {
		if err := e.id.EditWork(ctx, tc.v, heat, tc.edit); !errors.Is(err, tc.err) {
			t.Errorf("%s: %v, want %v", name, err, tc.err)
		}
	}
	var title string
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT title FROM work WHERE id = ?`, heat).Scan(&title)
	})
	if title != "Heat!" {
		t.Errorf("a refused edit wrote the title %q", title)
	}
}
