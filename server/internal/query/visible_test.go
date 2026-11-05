package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/users"
)

// TestVisibleAgreesWithCanSeeWork checks that the two copies of the
// visibility rule give the same answers. One is the SQL viewer filter,
// reached through Titles and Detail. The other is users.Viewer.CanSeeWork
// over where the work is, reached through catalog.Visible, which the
// services that take a Viewer use for their checks.
//
// Every user asks about every work of the household. Extra users hide
// unrated works with no ceiling, or show them under a ceiling, and a
// library of unrated clips is added. Hiding unrated works hides video only,
// so every user that sees the audiobooks library sees its unrated books.
func TestVisibleAgreesWithCanSeeWork(t *testing.T) {
	h := newHousehold(t)
	var clipsFolder id.ID
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		l, err := h.b.Catalog.CreateLibrary(ctx, tx, catalog.LibraryInput{
			Name: "Clips", Kind: catalog.Videos, Folders: []string{"/media/Clips"},
		})
		clipsFolder = l.Folders[0].ID
		return err
	})
	h.movie(clipsFolder, "Clip", 2020, -1)
	h.movie(clipsFolder, "Rated Clip", 2020, rankG)

	viewers := map[string]users.Viewer{
		"admin": h.admin, "kid": h.kid, "friend": h.friend, "nobody": h.nobody,
		"no unrated": h.viewer("strict", users.NewUser{Parental: users.Parental{BlockUnrated: true}}),
		"teen":       h.viewer("teen", users.NewUser{Parental: users.Parental{Ceiling: ptr(rankPG13)}}),
		"zero":       {},
	}

	var works []id.ID
	books := map[id.ID]bool{}
	rows, err := h.b.DB.Reader().Query(`SELECT id, kind FROM work`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var w id.ID
		var kind string
		if err := rows.Scan(&w, &kind); err != nil {
			t.Fatal(err)
		}
		works = append(works, w)
		books[w] = kind == "book"
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()

	for name, v := range viewers {
		seen := 0
		titles, err := h.q.Titles(ctx, v, works)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range works {
			err := h.b.Catalog.Visible(ctx, v, w)
			if err != nil && !errors.Is(err, catalog.ErrNotFound) {
				t.Fatal(err)
			}
			goRule := err == nil
			_, sqlRule := titles[w]
			if books[w] && !v.ShowsUnrated() && v.AllLibraries() && !sqlRule {
				t.Errorf("%s, %s: an unrated book is hidden", name, h.names[w])
			}
			if goRule != sqlRule {
				t.Errorf("%s, %s: CanSeeWork %v, Titles %v", name, h.names[w], goRule, sqlRule)
			}
			_, err = h.q.Detail(ctx, v, w)
			if err != nil && !errors.Is(err, query.ErrNotFound) {
				t.Fatal(err)
			}
			if (err == nil) != sqlRule {
				t.Errorf("%s, %s: Detail %v, Titles %v", name, h.names[w], err, sqlRule)
			}
			if sqlRule {
				seen++
			}
		}
		t.Logf("%s sees %d of %d works", name, seen, len(works))
	}
}
