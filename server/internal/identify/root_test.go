package identify_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
)

// links returns the folder links, as library/subject → work.
func (e *env) links() map[string]id.ID {
	e.t.Helper()
	rows, err := e.b.DB.Reader().Query(`SELECT library_id, subject, work_id FROM work_folder`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]id.ID{}
	for rows.Next() {
		var lib, w id.ID
		var subject string
		if err := rows.Scan(&lib, &subject, &w); err != nil {
			e.t.Fatal(err)
		}
		out[lib.String()+"/"+subject] = w
	}
	return out
}

// A renamed folder is the same root work, and its link follows it. The new name
// is identified again. A different match re-describes the root work in place,
// keeping its history.
func TestFolderRenamedRechecksItsMatch(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Movies)
	f := e.add(lib, "The Thing/The Thing.mkv")[0]
	e.run(f)
	before := e.workOf(f)[0]
	anna := e.newUser("anna")
	e.play(anna, e.versionOf(f), f, 95*time.Minute)

	e.move(lib, f, "The Thing (1982)/The Thing.mkv")
	e.run(f)
	after := e.workOf(f)[0]
	if after.ID != before.ID || !slices.Equal(after.ExternalIDs, []catalog.ExternalID{thing1982.ID}) || after.Locked {
		t.Fatalf("after the rename %+v, want %v re-described as the 1982 film", after, before.ID)
	}
	if s, ok := e.stateOf(anna, after.ID); !ok || !s.played {
		t.Fatalf("watch state after the rename: %+v, %v", s, ok)
	}
	if got := e.links(); len(got) != 1 || got[lib.ID.String()+"/The Thing (1982)"] != before.ID {
		t.Fatalf("links %v, want the renamed folder's alone", got)
	}
}

// label returns a work's edition label.
func (e *env) label(work id.ID) string {
	e.t.Helper()
	var l string
	if err := e.b.DB.Reader().QueryRow(`SELECT coalesce(edition, '') FROM work WHERE id = ?`, work).Scan(&l); err != nil {
		e.t.Fatal(err)
	}
	return l
}

// A folder renamed to carry an edition tag labels its root work, which keeps
// its versions and history.
func TestFolderEditionTagLabelsItsRoot(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Movies)
	f := e.add(lib, "Blade Runner (1982)/Blade Runner (1982).mkv")[0]
	e.run(f)
	before := e.workOf(f)[0]
	anna := e.newUser("anna")
	e.play(anna, e.versionOf(f), f, 95*time.Minute)

	e.move(lib, f, "Blade Runner (1982) {edition-Final Cut}/Blade Runner (1982).mkv")
	e.run(f)
	after := e.workOf(f)[0]
	if after.ID != before.ID || e.label(after.ID) != "Final Cut" {
		t.Fatalf("after the rename %+v labelled %q, want %v labelled Final Cut", after, e.label(after.ID), before.ID)
	}
	if s, ok := e.stateOf(anna, after.ID); !ok || !s.played {
		t.Fatalf("watch state after the rename: %+v, %v", s, ok)
	}
}

// versionLabel returns a version's edition label.
func (e *env) versionLabel(version id.ID) string {
	e.t.Helper()
	var l string
	if err := e.b.DB.Reader().QueryRow(`SELECT coalesce(edition, '') FROM media_version WHERE id = ?`, version).Scan(&l); err != nil {
		e.t.Fatal(err)
	}
	return l
}

// Files with edition tags are versions of their folder's root work, each
// labelled with its tag. A version whose label differs from its files'
// tag takes the tag's when its folder is identified again.
func TestEditionTagsLabelVersions(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Movies)
	files := e.add(lib, "Heat (1995)/Heat (1995).mkv", "Heat (1995)/Heat (1995) {edition-Extended}.mkv")
	e.run(files...)
	plain, extended := e.workOf(files[0])[0], e.workOf(files[1])[0]
	if plain.ID != extended.ID || e.label(plain.ID) != "" {
		t.Fatalf("the files are on %v and %v, the root work labelled %q; want one unlabelled root work", plain.ID, extended.ID, e.label(plain.ID))
	}
	if a, b := e.versionLabel(e.versionOf(files[0])), e.versionLabel(e.versionOf(files[1])); a != "" || b != "Extended" {
		t.Fatalf("version labels %q and %q, want none and Extended", a, b)
	}

	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.Catalog.Batch(ctx, tx, func(b *catalog.Batch) error { return b.SetVersionEdition(ctx, e.versionOf(files[1]), "Theatrical") })
	})
	e.again(files[1])
	if l := e.versionLabel(e.versionOf(files[1])); l != "Extended" {
		t.Fatalf("the version is labelled %q after identifying again, want Extended", l)
	}
}

// A file moved into another root work's folder becomes one of that root work's
// versions, bringing no history. A file moved to a folder of its own is a
// new root work, while the folder it left keeps its root work.
func TestMovedFileJoinsItsNewFolder(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Movies)
	files := e.add(lib, "Heat (1995)/Heat (1995).mkv", "Heat (1995)/Heat (1995) - 4K.mkv", "Ronin (1998)/Ronin (1998).mkv")
	e.run(files...)
	heat, ronin := e.workOf(files[0])[0], e.workOf(files[2])[0]
	anna := e.newUser("anna")
	e.play(anna, e.versionOf(files[1]), files[1], 95*time.Minute)

	e.move(lib, files[1], "Heat 4K/Heat (1995).mkv")
	e.run(files[1])
	moved := e.workOf(files[1])[0]
	if moved.ID == heat.ID || !slices.Equal(moved.ExternalIDs, heat.ExternalIDs) {
		t.Fatalf("a file moved to a new folder is on %+v, want a new root work described by Heat's match", moved)
	}
	if e.workOf(files[0])[0].ID != heat.ID {
		t.Fatal("the folder it left lost its root work")
	}
	if s, ok := e.stateOf(anna, heat.ID); !ok || !s.played {
		t.Fatalf("Heat's history moved: %+v, %v", s, ok)
	}
	if _, ok := e.stateOf(anna, moved.ID); ok {
		t.Fatal("the moved file brought history")
	}

	e.move(lib, files[1], "Ronin (1998)/Ronin (1998) - 4K.mkv")
	e.run(files[1])
	if w := e.workOf(files[1])[0]; w.ID != ronin.ID {
		t.Fatalf("a file moved into Ronin's folder is on %+v, want Ronin", w)
	}
}

// libraries lists the libraries a work is available in.
func (e *env) libraries(work id.ID) []id.ID {
	e.t.Helper()
	rows, err := e.b.DB.Reader().Query(`SELECT library_id FROM work_library WHERE work_id = ? ORDER BY library_id`, work)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []id.ID
	for rows.Next() {
		var l id.ID
		if err := rows.Scan(&l); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, l)
	}
	return out
}

// A folder moved to another library takes its root work along, with its
// history. A version whose file went missing earlier stays behind and is
// dropped.
func TestFolderMovedToAnotherLibraryTakesItsRoot(t *testing.T) {
	e := newEnv(t)
	movies, kids := e.library(catalog.Movies), e.library(catalog.Movies)
	files := e.add(movies, "Heat (1995)/Heat (1995).mkv", "Heat (1995)/Heat (1995) - 4K.mkv", "Heat (1995)/Heat (1995) - DVD.mkv")
	e.run(files...)
	heat := e.workOf(files[0])[0]
	anna := e.newUser("anna")
	e.play(anna, e.versionOf(files[0]), files[0], 95*time.Minute)
	e.b.Write(func(ctx context.Context, tx *store.Tx) error { return e.b.MarkGone(ctx, tx, files[2]) })

	e.move(kids, files[0], "Heat (1995)/Heat (1995).mkv")
	e.move(kids, files[1], "Heat (1995)/Heat (1995) - 4K.mkv")
	e.run(files[:2]...)
	for _, f := range files[:2] {
		if w := e.workOf(f)[0]; w.ID != heat.ID {
			t.Fatalf("a moved file is on %+v, want Heat %v", w, heat.ID)
		}
	}
	if s, ok := e.stateOf(anna, heat.ID); !ok || !s.played {
		t.Fatalf("Heat's history after the move: %+v, %v", s, ok)
	}
	if got := e.links(); len(got) != 1 || got[kids.ID.String()+"/Heat (1995)"] != heat.ID {
		t.Fatalf("links %v, want the moved folder's alone", got)
	}
	if got := e.libraries(heat.ID); !slices.Equal(got, []id.ID{kids.ID}) {
		t.Fatalf("Heat is in %v, want %v alone", got, kids.ID)
	}
	if _, err := e.b.Catalog.File(ctx, files[2]); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("the missing file left behind: %v, want it dropped", err)
	}
}

// A file moved alone to another library becomes a version of the root work of
// the folder it lands in there, or of a new root work, bringing no history.
func TestFileMovedToAnotherLibraryLeavesItsRoot(t *testing.T) {
	e := newEnv(t)
	movies, kids := e.library(catalog.Movies), e.library(catalog.Movies)
	files := e.add(movies, "Heat (1995)/Heat (1995).mkv", "Heat (1995)/Heat (1995) - 4K.mkv", "Heat (1995)/Heat (1995) - DVD.mkv")
	e.run(files...)
	theirs := e.add(kids, "Heat (1995)/Heat (1995).mkv")[0]
	e.run(theirs)
	heat, kidsHeat := e.workOf(files[0])[0], e.workOf(theirs)[0]
	anna := e.newUser("anna")
	for _, f := range files[1:] {
		e.play(anna, e.versionOf(f), f, 95*time.Minute)
	}

	e.move(kids, files[1], "Heat (1995)/Heat (1995) - 4K.mkv")
	e.run(files[1])
	if w := e.workOf(files[1])[0]; w.ID != kidsHeat.ID {
		t.Fatalf("a file moved into the other library's Heat is on %+v, want %v", w, kidsHeat.ID)
	}
	e.move(kids, files[2], "Heat DVD/Heat (1995).mkv")
	e.run(files[2])
	alone := e.workOf(files[2])[0]
	if alone.ID == heat.ID || alone.ID == kidsHeat.ID {
		t.Fatalf("a file moved to a folder of its own is on %+v, want a new root work", alone)
	}
	if e.workOf(files[0])[0].ID != heat.ID {
		t.Fatal("the folder the files left lost its root work")
	}
	if s, ok := e.stateOf(anna, heat.ID); !ok || !s.played || s.plays != 2 {
		t.Fatalf("Heat's history: %+v, %v", s, ok)
	}
	for _, w := range []id.ID{kidsHeat.ID, alone.ID} {
		if _, ok := e.stateOf(anna, w); ok {
			t.Fatalf("a moved file brought history to %v", w)
		}
	}
	for w, lib := range map[id.ID]id.ID{heat.ID: movies.ID, kidsHeat.ID: kids.ID, alone.ID: kids.ID} {
		if got := e.libraries(w); !slices.Equal(got, []id.ID{lib}) {
			t.Fatalf("%v is in %v, want %v alone", w, got, lib)
		}
	}
}

// Changing a series' match to another series re-describes it in place. Its
// episodes are found again by number and keep their works and state.
func TestFixMatchKeepsEpisodes(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	files := e.add(lib, "Firefly/Season 1/Firefly - S01E01.mkv")
	e.run(files...)
	ep := e.workOf(files[0])[0]
	anna := e.newUser("anna")
	e.play(anna, e.versionOf(files[0]), files[0], 95*time.Minute)
	expanse := catalog.ExternalID{Provider: "tmdb", Kind: "series", ID: "63639"}
	if _, err := e.id.FixMatch(ctx, e.viewer(), ep.Root, expanse); err != nil {
		t.Fatal(err)
	}
	w := e.workOf(files[0])[0]
	if w.ID != ep.ID || w.Root != ep.Root || slices.Equal(w.ExternalIDs, ep.ExternalIDs) {
		t.Fatalf("after Fix match %+v, want episode %v with the new match's ID", w, ep.ID)
	}
	if s, ok := e.stateOf(anna, ep.ID); !ok || !s.played {
		t.Fatalf("watch state after Fix match: %+v, %v", s, ok)
	}
}
