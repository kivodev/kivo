package query_test

import (
	"context"
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

// similarTo returns the titles of a work's More like this row, as v sees
// them.
func (h *household) similarTo(v users.Viewer, work id.ID) []string {
	h.t.Helper()
	d, err := h.q.Detail(ctx, v, work)
	if err != nil {
		h.t.Fatal(err)
	}
	if d.Similar == nil {
		h.t.Fatal("Similar is nil")
	}
	var out []string
	for _, s := range d.Similar {
		out = append(out, s.Title)
	}
	return out
}

// tmdbID gives a work its TMDb ID.
func (h *household) tmdbID(work id.ID, kind, ext string) {
	h.t.Helper()
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO work_external_id (work_id, provider, kind, ext_id) VALUES (?, 'tmdb', ?, ?)`, work, kind, ext)
		return err
	})
}

// similarMovie adds a movie to the Movies library with a rating rank, a
// community rating, and metadata.
func (h *household) similarMovie(folder id.ID, title string, rank int, rating float64, m catalog.WorkMetadata) id.ID {
	h.t.Helper()
	w := h.movie(folder, title, 2000, rank).Work
	m.Title, m.Year = title, 2000
	if rank >= 0 {
		m.ContentRating, m.ContentRatingAge = certs[rank], ptr(rank)
	}
	m.Community = &catalog.CommunityRating{Rating: rating, Votes: 1000}
	h.metadata(w, m)
	return w
}

func actors(names ...string) []catalog.Credit {
	var out []catalog.Credit
	for _, n := range names {
		out = append(out, catalog.Credit{Name: n, Role: "actor"})
	}
	return out
}

// TestSimilarScores checks each signal's score, the threshold of 20, and
// the order: by score, then community rating, then title. TMDb's
// recommendations count by their place, and only for titles of the work's
// kind.
func TestSimilarScores(t *testing.T) {
	h := newHousehold(t)
	seed := h.similarMovie(h.moviesFolder, "Arrival", rankPG13, 7.9, catalog.WorkMetadata{
		Genres: []string{"Science Fiction", "Drama"}, Tags: []string{"aliens"}, Companies: []string{"FilmNation"},
		Credits: append(actors("Amy Adams", "Jeremy Renner", "Forest Whitaker", "Michael Stuhlbarg", "Tzi Ma", "Mark O'Brien"),
			catalog.Credit{Name: "Denis Villeneuve", Role: "director"}),
		Recommendations: []string{"900", "901", "902"},
	})
	first := h.similarMovie(h.moviesFolder, "Interstellar", rankPG13, 8.4, catalog.WorkMetadata{})
	h.tmdbID(first, "movie", "900")
	third := h.similarMovie(h.moviesFolder, "Contact", rankPG, 7.4, catalog.WorkMetadata{})
	h.tmdbID(third, "movie", "902")
	// A series with the ID TMDb gives a recommended movie isn't that movie.
	h.tmdbID(h.expanse, "series", "901")
	// The same director, and one genre: 60. Dune has them already.
	// A top-billed actor of both, and a genre: 25.
	h.similarMovie(h.moviesFolder, "Nocturnal Animals", rankR, 7.3, catalog.WorkMetadata{
		Genres: []string{"Drama"}, Credits: actors("Amy Adams"),
	})
	// An actor billed sixth in either is no signal: only the genre counts.
	h.similarMovie(h.moviesFolder, "Sixth Billed", rankPG13, 9, catalog.WorkMetadata{
		Genres:  []string{"Drama"},
		Credits: actors("A", "B", "C", "D", "E", "Jeremy Renner"),
	})
	h.similarMovie(h.moviesFolder, "Bit Part", rankPG13, 9, catalog.WorkMetadata{
		Genres: []string{"Drama"}, Credits: actors("Mark O'Brien"),
	})
	// Two genres: 20, just enough. A tag, a company and a genre: 20 too,
	// with a lower rating. Two more at 20 with the same rating go by title.
	h.similarMovie(h.moviesFolder, "Two Genres", rankPG13, 8, catalog.WorkMetadata{Genres: []string{"Drama", "Science Fiction"}})
	h.similarMovie(h.moviesFolder, "Tag and Company", rankPG13, 6, catalog.WorkMetadata{
		Genres: []string{"Drama"}, Tags: []string{"aliens"}, Companies: []string{"FilmNation"},
	})
	h.similarMovie(h.moviesFolder, "Beta Tie", rankPG13, 7, catalog.WorkMetadata{Genres: []string{"Drama", "Science Fiction"}})
	h.similarMovie(h.moviesFolder, "Alpha Tie", rankPG13, 7, catalog.WorkMetadata{Genres: []string{"Drama", "Science Fiction"}})
	h.similarMovie(h.moviesFolder, "One Genre", rankPG13, 9, catalog.WorkMetadata{Genres: []string{"Drama"}})

	want := []string{"Interstellar", "Contact", "Dune", "Nocturnal Animals", "Two Genres", "Alpha Tie", "Beta Tie", "Tag and Company"}
	for range 3 {
		if got := h.similarTo(h.admin, seed); !slices.Equal(got, want) {
			t.Fatalf("like Arrival: %q, want %q", got, want)
		}
	}
	// The TMDb list is the work's own: Interstellar recommends nothing.
	if got := h.similarTo(h.admin, first); len(got) != 0 {
		t.Errorf("like Interstellar: %q, want none", got)
	}
}

// TestSimilarLeavesOut checks that the row leaves out the work itself,
// what the user played, the other members of the work's collection or book
// series, what the Viewer may not see, and titles of other kinds, and that
// seasons and episodes have no row.
func TestSimilarLeavesOut(t *testing.T) {
	h := newHousehold(t)
	family := []string{"Animation", "Family"}
	for _, w := range []id.ID{h.toyStory.Work, h.toyStory2.Work, h.bluey, h.expanse} {
		h.metadata(w, catalog.WorkMetadata{Genres: family, ContentRating: certs[rankG], ContentRatingAge: ptr(rankG)})
	}
	played := h.similarMovie(h.moviesFolder, "Cars", rankG, 7, catalog.WorkMetadata{Genres: family})
	h.similarMovie(h.moviesFolder, "Up", rankG, 8, catalog.WorkMetadata{Genres: family})
	h.similarMovie(h.privateFolder, "Private Cartoon", rankG, 8, catalog.WorkMetadata{Genres: family})
	h.similarMovie(h.moviesFolder, "Shrek", rankPG13, 8, catalog.WorkMetadata{Genres: family})
	h.similarMovie(h.moviesFolder, "Unrated Cartoon", -1, 8, catalog.WorkMetadata{Genres: family})
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO user_state (user_id, work_id, played, play_count, updated_at) VALUES (?, ?, 1, 1, 1)`, h.admin.User(), played)
		return err
	})

	// Toy Story 2 is in Toy Story's collection, and Bluey is a series. The
	// friend doesn't have the Private library, and the kid sees neither
	// PG-13 nor unrated titles.
	for _, tt := range []struct {
		name string
		v    users.Viewer
		want []string
	}{
		{"admin", h.admin, []string{"Private Cartoon", "Shrek", "Unrated Cartoon", "Up"}},
		{"friend", h.friend, []string{"Shrek", "Unrated Cartoon", "Up", "Cars"}},
		{"kid", h.kid, []string{"Private Cartoon", "Up", "Cars"}},
	} {
		if got := h.similarTo(tt.v, h.toyStory.Work); !slices.Equal(got, tt.want) {
			t.Errorf("%s: like Toy Story: %q, want %q", tt.name, got, tt.want)
		}
	}

	// A series is played when every episode is.
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO user_state (user_id, work_id, played, play_count, updated_at)
			SELECT ?, id, 1, 1, 1 FROM work WHERE root_id = ? AND kind = 'episode'`, h.admin.User(), h.expanse)
		return err
	})
	if got, want := h.similarTo(h.admin, h.bluey), []string(nil); !slices.Equal(got, want) {
		t.Errorf("admin: like Bluey: %q, want %q", got, want)
	}
	if got, want := h.similarTo(h.friend, h.bluey), []string{"The Expanse"}; !slices.Equal(got, want) {
		t.Errorf("friend: like Bluey: %q, want %q", got, want)
	}

	// Books: the same author, but not the same series.
	h.book([]string{"Andy Weir/Artemis/Artemis.m4b"}, catalog.WorkMetadata{
		Title: "Artemis", Year: 2017, ReleaseDate: "2017",
		Book: &catalog.BookMetadata{Authors: []string{"Andy Weir"}, Narrators: []string{"Rosario Dawson"}, Series: []catalog.SeriesRef{}},
	})
	if got, want := h.similarTo(h.admin, h.hailMary), []string{"Artemis"}; !slices.Equal(got, want) {
		t.Errorf("like Project Hail Mary: %q, want %q", got, want)
	}
	if got := h.similarTo(h.admin, h.leviathan); len(got) != 0 {
		t.Errorf("like Leviathan Wakes: %q, want none", got)
	}

	for name, w := range map[string]id.ID{"a season": h.blueyS1, "an episode": h.bluey3.Work} {
		if got := h.similarTo(h.admin, w); len(got) != 0 {
			t.Errorf("like %s: %q, want none", name, got)
		}
	}
}
