package metadata

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
)

// fakeVideo is a separate video provider that answers from a Description.
type fakeVideo struct{ d Description }

func (f *fakeVideo) Name() string { return "fake" }
func (f *fakeVideo) Search(context.Context, catalog.WorkKind, Query, string) ([]Match, error) {
	return nil, nil
}
func (f *fakeVideo) Resolve(context.Context, catalog.ExternalID, string) (Match, error) {
	return Match{}, ErrNotFound
}
func (f *fakeVideo) Describe(_ context.Context, r Ref, _ string) (*Description, error) {
	if !slices.Contains(r.IDs, catalog.ExternalID{Provider: "fake", Kind: string(catalog.KindMovie), ID: "1"}) {
		return nil, ErrNotFound
	}
	return &f.d, nil
}
func (f *fakeVideo) Images(ctx context.Context, r Ref, lang string) (Images, error) {
	d, err := f.Describe(ctx, r, lang)
	if err != nil {
		return Images{}, err
	}
	return d.Images, nil
}
func (f *fakeVideo) Episodes(context.Context, catalog.ExternalID, Ordering, string, time.Duration) ([]Episode, error) {
	return nil, ErrNotFound
}
func (f *fakeVideo) EpisodeGroups(context.Context, catalog.ExternalID, string) ([]string, error) {
	return nil, ErrNotFound
}
func (f *fakeVideo) Owns(u string) bool         { return strings.HasPrefix(u, "https://images.example.org/") }
func (f *fakeVideo) Preview(u, _ string) string { return u }

// Any video provider that fills the package's types can describe a work. A
// refresh normalizes its text, credits, rating and collection, and ranks its
// images the same way it ranks TMDb's.
func TestRefreshFromAnotherProvider(t *testing.T) {
	e := newEnv(t)
	e.s.video = &fakeVideo{d: Description{
		Title: "  Arrival ", OriginalTitle: "Arrival", OriginalLanguage: "en", ReleaseDate: "2016-11-11",
		Overview: "A linguist is recruited.", Genres: []string{"Drama", "Drama", "Science Fiction"},
		ContentRatings: map[string]string{"US": "PG-13", "GB": "12A"},
		Cast: []Person{
			{IDs: []catalog.ExternalID{{Provider: "fake", Kind: "person", ID: "7"}}, Name: "Amy Adams", Role: roleActor, Character: "Louise Banks"},
		},
		Crew: []Person{{Name: "Denis Villeneuve", Role: roleDirector}},
		Collection: &Collection{
			Ref:    catalog.CollectionRef{Key: "fake:9", Name: "Heptapods", Order: 1},
			Images: Images{Posters: []Image{{URL: "https://images.example.org/heptapods.jpg"}}},
		},
		Images: Images{Original: "en", Posters: []Image{
			{URL: "https://images.example.org/arrival-fr.jpg", Language: "fr", Score: 9},
			{URL: "https://images.example.org/arrival-en.jpg", Language: "en", Score: 5},
		}},
	}}
	_, folder := e.b.Library(catalog.Movies)
	w := e.b.Work(catalog.WorkInput{
		Kind: catalog.KindMovie, Title: "Arrival", Year: 2016,
		ExternalIDs: []catalog.ExternalID{{Provider: "fake", Kind: string(catalog.KindMovie), ID: "1"}},
	})
	e.b.Version([]id.ID{e.b.File(folder, "Arrival (2016)/Arrival (2016).mkv")}, w)

	e.mustRefresh(w)

	for col, want := range map[string]string{"title": "Arrival", "year": "2016", "content_rating": "PG-13", "original_language": "en"} {
		if got := e.column(w, col); got != want {
			t.Errorf("%s = %q, want %q", col, got, want)
		}
	}
	if got, want := e.credits(w), []string{"actor:Amy Adams", "director:Denis Villeneuve"}; !slices.Equal(got, want) {
		t.Errorf("credits %v, want %v", got, want)
	}
	if got := e.strings(`SELECT g.name FROM work_genre wg JOIN genre g ON g.id = wg.genre_id WHERE wg.work_id = ? ORDER BY g.name`, w); !slices.Equal(got, []string{"Drama", "Science Fiction"}) {
		t.Errorf("genres %v", got)
	}
	if got := e.strings(`SELECT c.key || ' ' || c.name FROM collection c JOIN collection_work cw ON cw.collection_id = c.id WHERE cw.work_id = ?`, w); !slices.Equal(got, []string{"fake:9 Heptapods"}) {
		t.Errorf("collection %v", got)
	}
	if r := e.images(w)[artworkPoster]; r.Origin != "https://images.example.org/arrival-en.jpg" {
		t.Errorf("poster %+v, want the one in the metadata language", r)
	}
	if got := e.artwork(w, artworkPoster); len(got) != 2 || got[0].Provider != "fake" || got[1].Language != "fr" {
		t.Errorf("artwork to choose %+v", got)
	}
	if got := e.collectionArt("fake:9"); !slices.Equal(got, []string{"poster https://images.example.org/heptapods.jpg"}) {
		t.Errorf("collection artwork %v", got)
	}

	// A collection poster the provider no longer gives is removed by the next
	// refresh, and its files are deleted once that commits.
	rows, err := e.s.q.CollectionImages(ctx, "fake:9")
	if err != nil || len(rows) != 1 || !e.s.present(rows[0]) {
		t.Fatalf("collection images %+v, %v", rows, err)
	}
	e.s.video.(*fakeVideo).d.Collection.Images = Images{}
	e.mustRefresh(w)
	if got := e.collectionArt("fake:9"); len(got) != 0 {
		t.Errorf("collection artwork %v, want none", got)
	}
	if e.s.present(rows[0]) {
		t.Error("the removed poster's files are kept")
	}
}
