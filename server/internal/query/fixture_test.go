package query_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
)

// Content rating ranks the fixture uses.
const (
	rankG     = 0
	rankPG    = 7
	rankPG13  = 13
	rankR     = 17
	kidsLimit = rankPG
)

// certs are the content ratings those ranks stand for.
var certs = map[int]string{rankG: "G", rankPG: "PG", rankPG13: "PG-13", rankR: "R"}

// household is the hand-built fixture. Its users are an admin who sees
// every library, a kid with a rating ceiling who hides unrated works, and a
// friend without the Private and 4K libraries.
//
// Its media has a gone file, a double-episode file, a version labelled with
// an edition, and a provisional work. It has a 4K copy in another library
// that is a root work of its own, in a folder that can't be reached. It has a
// collection, and an audiobooks library of unrated books. One book is
// standalone and two are in a series, one of those at position 1.5. One of
// the books is in two files.
type household struct {
	t     *testing.T
	b     *catalogtest.Builder
	q     *query.Service
	state *userstate.Service
	names map[id.ID]string // for golden output

	movies, shows, private, uhd, books    id.ID // libraries
	moviesFolder, showsFolder             id.ID
	privateFolder, uhdFolder, booksFolder id.ID

	admin, kid, friend, nobody users.Viewer

	dune, bladeRunner, finalCut, finalCut4K, toyStory, toyStory2, homeMovie, secret, lost, unmatched catalogtest.Item
	expanse, bluey, unmatchedShow                                                                    id.ID // series
	expanseS1, blueyS1                                                                               id.ID // seasons
	expanse1, expanse2, expanse3, blueySpecial, bluey3, unmatchedShow1                               catalogtest.Item
	bluey1, bluey2                                                                                   id.ID // one double-episode version
	blueyDouble                                                                                      id.ID // its version
	toyStoryCollection                                                                               id.ID
	hailMary, leviathan, churn                                                                       id.ID // books
	weir, corey, porter                                                                              id.ID // their people
	villeneuve                                                                                       id.ID // Dune's director
	expanseBooks                                                                                     id.ID // their series
}

func newHousehold(t *testing.T) *household {
	t.Helper()
	h := &household{t: t, b: catalogtest.New(t), names: map[id.ID]string{{}: "none"}}
	h.q = query.New(h.b.DB)
	settings, err := config.Open(context.Background(), h.b.DB, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	h.state = userstate.New(h.b.DB, userstate.Deps{Catalog: h.b.Catalog, Settings: settings, Activity: activity.New(h.b.DB, events.NewBus()), Events: events.NewBus()})

	h.movies, h.moviesFolder = h.library("Movies", catalog.Movies)
	h.shows, h.showsFolder = h.library("Shows", catalog.Shows)
	h.private, h.privateFolder = h.library("Private", catalog.Movies)
	h.uhd, h.uhdFolder = h.library("Movies 4K", catalog.Movies)
	h.books, h.booksFolder = h.library("Audiobooks", catalog.Audiobooks)

	h.dune = h.movie(h.moviesFolder, "Dune", 2021, rankPG13)
	h.metadata(h.dune.Work, catalog.WorkMetadata{
		Title: "Dune", Year: 2021, ContentRating: certs[rankPG13], ContentRatingAge: ptr(rankPG13), Overview: "A desert planet.",
		Community: &catalog.CommunityRating{Rating: 8.1, Votes: 5000},
		Genres:    []string{"Science Fiction", "Adventure"}, Companies: []string{"Legendary"},
		Credits: []catalog.Credit{
			{Name: "Timothée Chalamet", Role: "actor", Character: "Paul Atreides"},
			{Name: "Zendaya", Role: "actor", Character: "Chani"},
			{Name: "Denis Villeneuve", Role: "director"},
		},
	})
	h.tracks(h.dune.File, 155*time.Minute, []catalog.Track{
		{Index: 0, Type: "video", Codec: "hevc", Default: true, Info: info(map[string]any{"width": 1920, "height": 1080, "range": "sdr"})},
		{Index: 1, Type: "audio", Codec: "eac3", Language: "eng", Default: true, Info: info(map[string]any{"channels": 6})},
		{Index: 2, Type: "subtitle", Codec: "subrip", Language: "eng", Title: "Forced", Forced: true, Info: info(map[string]any{"forced": true})},
	})

	h.bladeRunner = h.movie(h.moviesFolder, "Blade Runner", 1982, rankR)
	// Rated highest, but by few people.
	h.metadata(h.bladeRunner.Work, catalog.WorkMetadata{Year: 1982, ContentRating: certs[rankR], ContentRatingAge: ptr(rankR), Community: &catalog.CommunityRating{Rating: 9, Votes: 12}})
	h.finalCut = h.edition(h.moviesFolder, h.bladeRunner.Work, "Blade Runner", "Final Cut", 1982)
	// Its 4K copy, in the 4K library, is a root work of its own.
	w := h.b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Blade Runner: The Final Cut", Year: 1982})
	f := h.b.File(h.uhdFolder, "Blade Runner (1982)/Blade Runner (1982) {edition-Final Cut} 2160p.mkv")
	h.finalCut4K = catalogtest.Item{Work: w, Version: h.b.Version([]id.ID{f}, w), File: f}
	h.named(h.finalCut4K, "movie:Blade Runner Final Cut 4K")
	h.metadata(w, catalog.WorkMetadata{Title: "Blade Runner: The Final Cut", Year: 1982, ContentRating: certs[rankR], ContentRatingAge: ptr(rankR)})
	h.tracks(f, 117*time.Minute, []catalog.Track{
		{Index: 0, Type: "video", Codec: "hevc", Default: true, Info: info(map[string]any{"width": 3840, "height": 2160, "range": "hdr10", "dolby_vision_profile": 8})},
		{Index: 1, Type: "audio", Codec: "truehd", Language: "eng", Default: true, Info: info(map[string]any{"channels": 8})},
	})
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE library_folder SET reachable = 0 WHERE id = ?`, h.uhdFolder)
		return err
	})

	h.toyStory = h.movie(h.moviesFolder, "Toy Story", 1995, rankG)
	h.toyStory2 = h.movie(h.moviesFolder, "Toy Story 2", 1999, rankG)
	for i, m := range []catalogtest.Item{h.toyStory, h.toyStory2} {
		h.metadata(m.Work, catalog.WorkMetadata{
			Year: []int{1995, 1999}[i], ContentRating: certs[rankG], ContentRatingAge: ptr(rankG),
			Community:  &catalog.CommunityRating{Rating: []float64{8, 7.6}[i], Votes: []int{18000, 14000}[i]},
			Collection: &catalog.CollectionRef{Key: "tmdb:10194", Name: "Toy Story Collection", Order: i},
		})
	}
	h.homeMovie = h.movie(h.moviesFolder, "Home Movie", 2010, -1)
	h.secret = h.movie(h.privateFolder, "Secret", 2001, rankG)
	h.lost = h.movie(h.moviesFolder, "Lost Film", 1950, rankG)
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return h.b.MarkGone(ctx, tx, h.lost.File)
	})
	h.unmatched = h.provisional(h.moviesFolder, "some.film.2023")

	h.expanse = h.series("The Expanse", rankPG13)
	h.metadata(h.expanse, catalog.WorkMetadata{ContentRating: certs[rankPG13], ContentRatingAge: ptr(rankPG13), Community: &catalog.CommunityRating{Rating: 8.4, Votes: 3000}})
	h.expanse1 = h.episode(h.expanse, 1, 1)
	h.expanse2 = h.episode(h.expanse, 1, 2)
	h.expanse3 = h.episode(h.expanse, 1, 3)
	h.expanseS1 = h.parent(h.expanse1.Work)

	h.bluey = h.series("Bluey", rankG)
	h.blueySpecial = h.episode(h.bluey, 0, 1)
	h.bluey1 = h.b.Work(catalog.WorkInput{Kind: catalog.KindEpisode, Parent: h.season(h.bluey, 1), Title: "Magic Xylophone", ParentNumber: 1, Number: 1})
	h.bluey2 = h.b.Work(catalog.WorkInput{Kind: catalog.KindEpisode, Parent: h.season(h.bluey, 1), Title: "Hospital", ParentNumber: 1, Number: 2})
	f = h.b.File(h.showsFolder, "Bluey/Season 01/Bluey S01E01-E02.mkv")
	h.blueyDouble = h.b.Version([]id.ID{f}, h.bluey1, h.bluey2)
	h.tracks(f, 14*time.Minute, nil)
	h.names[h.bluey1], h.names[h.bluey2], h.names[h.blueyDouble], h.names[f] = "episode:Bluey 1x01", "episode:Bluey 1x02", "version:Bluey 1x01-02", "file:Bluey 1x01-02"
	h.bluey3 = h.episode(h.bluey, 1, 3)
	h.blueyS1 = h.parent(h.bluey3.Work)

	h.hailMary = h.book([]string{"Andy Weir/Project Hail Mary/Project Hail Mary.m4b"}, catalog.WorkMetadata{
		Title: "Project Hail Mary", Year: 2021, Overview: "A lone astronaut.", Genres: []string{"Science Fiction"},
		ReleaseDate: "2021-05-04", Companies: []string{"Audible Studios"},
		Book: &catalog.BookMetadata{Authors: []string{"Andy Weir"}, Narrators: []string{"Ray Porter"}, Series: []catalog.SeriesRef{}},
	})
	expanse := func(pos string) []catalog.SeriesRef {
		return []catalog.SeriesRef{{Key: catalog.LocalSeriesKey("James S. A. Corey", "The Expanse"), Name: "The Expanse", Position: pos, Primary: true}}
	}
	h.leviathan = h.book([]string{
		"James S. A. Corey/The Expanse/Book 1 - Leviathan Wakes/Part 1.mp3",
		"James S. A. Corey/The Expanse/Book 1 - Leviathan Wakes/Part 2.mp3",
	}, catalog.WorkMetadata{
		Title: "Leviathan Wakes", Year: 2011, ReleaseDate: "2011",
		Book: &catalog.BookMetadata{Authors: []string{"James S. A. Corey"}, Narrators: []string{"Jefferson Mays"}, Series: expanse("1")},
	})
	h.churn = h.book([]string{"James S. A. Corey/The Expanse/Book 1.5 - The Churn/The Churn.m4b"}, catalog.WorkMetadata{
		Title: "The Churn", Year: 2014, ReleaseDate: "2014",
		Book: &catalog.BookMetadata{Authors: []string{"James S. A. Corey"}, Narrators: []string{"Ray Porter"}, Series: expanse("1.5")},
	})
	h.weir, h.corey, h.porter = h.person("Andy Weir"), h.person("James S. A. Corey"), h.person("Ray Porter")
	h.villeneuve = h.person("Denis Villeneuve")
	if err := h.b.DB.Reader().QueryRow(`SELECT id FROM book_series`).Scan(&h.expanseBooks); err != nil {
		t.Fatal(err)
	}
	h.names[h.expanseBooks] = "bookSeries:The Expanse"

	h.unmatchedShow = h.b.Work(catalog.WorkInput{Kind: catalog.KindSeries, Title: "unknown.show", Provisional: true})
	h.names[h.unmatchedShow] = "series:unknown.show"
	h.unmatchedShow1 = h.episode(h.unmatchedShow, 1, 1)
	h.episode(h.unmatchedShow, 1, 2)

	h.admin = h.viewer("admin", users.NewUser{Admin: true})
	h.kid = h.viewer("kid", users.NewUser{Parental: users.Parental{Ceiling: ptr(kidsLimit), BlockUnrated: true}})
	h.friend = h.viewer("friend", users.NewUser{Grants: users.Grants{Libraries: []id.ID{h.movies, h.shows}}})
	// nobody has no grants, as happens when every library the user was
	// granted is deleted.
	h.nobody = h.viewer("nobody", users.NewUser{})
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE user SET all_libraries = 0 WHERE id = ?`, h.nobody.User())
		return err
	})
	h.nobody = h.mustViewer(h.nobody.User())

	h.toyStoryCollection = h.collectionOf(h.toyStory.Work)
	h.names[h.toyStoryCollection] = "collection:Toy Story"

	// Works made within a millisecond would tie on when they were added, and
	// which came first would depend on the clock. Space them a second apart
	// in creation order instead.
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		base := time.Now().Add(-24 * time.Hour).UnixMilli()
		if _, err := tx.ExecContext(ctx, `UPDATE work SET added_at = ? + rid * 1000`, base); err != nil {
			return err
		}
		return redate(ctx, tx)
	})
	return h
}

// redate copies works' added_at into work_library after a test sets them,
// the way catalog keeps it. Each row gets its own work's added_at, and the
// newest added_at among the works whose versions make it available there.
func redate(ctx context.Context, tx *store.Tx) error {
	_, err := tx.ExecContext(ctx, `UPDATE work_library SET
		added_at = (SELECT w.added_at FROM work w WHERE w.id = work_library.work_id),
		last_added_at = (SELECT max(d.added_at) FROM work d
			JOIN work_library dl ON dl.library_id = work_library.library_id AND dl.work_id = d.id
			WHERE (d.id = work_library.work_id
				OR (d.kind <> 'extra' AND (d.root_id = work_library.work_id OR d.parent_id = work_library.work_id)))
			AND EXISTS (SELECT 1 FROM media_version_work vw WHERE vw.work_id = d.id))`)
	return err
}

// play plays a version to pos, as one play.
func (h *household) play(v users.Viewer, version id.ID, pos time.Duration) {
	h.t.Helper()
	err := h.state.Report(context.Background(), userstate.Progress{
		Play: id.New(), User: v.User(), Version: version, Position: pos, At: time.Now(), Stopped: true,
	})
	if err != nil {
		h.t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond) // so last played times are distinct
}

// watch gives the admin and the friend some history. Dune and an Expanse
// episode are in progress, Bluey's double episode is finished, and Toy
// Story is rated and on My List.
func (h *household) watch() {
	h.t.Helper()
	for _, v := range []users.Viewer{h.admin, h.friend} {
		h.play(v, h.dune.Version, 40*time.Minute)
		h.play(v, h.expanse1.Version, 43*time.Minute)
		h.play(v, h.expanse2.Version, 10*time.Minute)
		h.play(v, h.blueyDouble, 14*time.Minute)
		ctx := context.Background()
		lists, err := h.q.Lists(ctx, v)
		if err != nil {
			h.t.Fatal(err)
		}
		if err := h.state.AddToList(ctx, v, lists[0].ID, h.toyStory.Work); err != nil {
			h.t.Fatal(err)
		}
		if err := h.state.SetRating(ctx, v.User(), h.toyStory.Work, ptr(9)); err != nil {
			h.t.Fatal(err)
		}
	}
}

func (h *household) library(name string, kind catalog.LibraryKind) (lib, folder id.ID) {
	h.t.Helper()
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		l, err := h.b.Catalog.CreateLibrary(ctx, tx, catalog.LibraryInput{Name: name, Kind: kind, Folders: []string{catalogtest.Abs("/media/" + name)}})
		lib, folder = l.ID, l.Folders[0].ID
		return err
	})
	h.names[lib] = "library:" + name
	return lib, folder
}

// movie adds a matched movie with a rating rank (-1 for unrated) and a two
// hour file.
func (h *household) movie(folder id.ID, title string, year, rank int) catalogtest.Item {
	h.t.Helper()
	m := h.b.Movie(folder, title, year)
	h.named(m, "movie:"+title)
	m2 := catalog.WorkMetadata{Title: title, Year: year}
	if rank >= 0 {
		m2.ContentRating, m2.ContentRatingAge = certs[rank], ptr(rank)
	}
	h.metadata(m.Work, m2)
	h.tracks(m.File, 2*time.Hour, nil)
	return m
}

// edition adds a version of root labelled with its file's edition tag.
func (h *household) edition(folder, root id.ID, name, edition string, year int) catalogtest.Item {
	h.t.Helper()
	f := h.b.File(folder, fmt.Sprintf("%s (%d)/%s (%d) {edition-%s}.mkv", name, year, name, year, edition))
	m := catalogtest.Item{Work: root, Version: h.b.Version([]id.ID{f}, root), File: f}
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return h.b.Catalog.Batch(ctx, tx, func(b *catalog.Batch) error { return b.SetVersionEdition(ctx, m.Version, edition) })
	})
	h.names[m.Version] = "version:" + name + " " + edition
	h.names[m.File] = "file:" + name + " " + edition
	h.tracks(f, 117*time.Minute, nil)
	return m
}

func (h *household) provisional(folder id.ID, name string) catalogtest.Item {
	h.t.Helper()
	w := h.b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: name, Provisional: true})
	f := h.b.File(folder, name+".mkv")
	m := catalogtest.Item{Work: w, Version: h.b.Version([]id.ID{f}, w), File: f}
	h.named(m, "movie:"+name)
	h.tracks(f, 2*time.Hour, nil)
	return m
}

func (h *household) series(title string, rank int) id.ID {
	h.t.Helper()
	s := h.b.Series(title)
	h.names[s] = "series:" + title
	h.metadata(s, catalog.WorkMetadata{Title: title, ContentRating: certs[rank], ContentRatingAge: ptr(rank)})
	return s
}

func (h *household) season(series id.ID, n int) id.ID {
	return h.b.Work(catalog.WorkInput{Kind: catalog.KindSeason, Parent: series, Title: fmt.Sprintf("Season %d", n), Number: n})
}

func (h *household) episode(series id.ID, season, episode int) catalogtest.Item {
	h.t.Helper()
	e := h.b.Episode(h.showsFolder, series, season, episode)
	h.named(e, fmt.Sprintf("episode:%s %dx%02d", h.names[series][len("series:"):], season, episode))
	h.names[h.parent(e.Work)] = fmt.Sprintf("season:%s %d", h.names[series][len("series:"):], season)
	h.tracks(e.File, 25*time.Minute, nil)
	return e
}

// book adds a book of one or more files, the first with two chapters.
func (h *household) book(files []string, m catalog.WorkMetadata) id.ID {
	h.t.Helper()
	w := h.b.Work(catalog.WorkInput{Kind: catalog.KindBook, Title: m.Title})
	var parts []id.ID
	for i, rel := range files {
		f := h.b.File(h.booksFolder, rel)
		parts = append(parts, f)
		var chapters []catalog.Chapter
		if i == 0 {
			chapters = []catalog.Chapter{{Index: 0, End: 10 * time.Minute, Title: "Opening Credits"}, {Index: 1, Start: 10 * time.Minute, End: time.Hour, Title: "Chapter 1"}}
		}
		h.b.Write(func(ctx context.Context, tx *store.Tx) error {
			return h.b.Catalog.SetProbe(ctx, tx, f, 1, catalog.Probe{
				Summary:  catalog.ProbeSummary{Container: "mov,mp4,m4a,3gp,3g2,mj2", Duration: time.Hour, Bitrate: 64_000},
				Tracks:   []catalog.Track{{Index: 0, Type: "audio", Codec: "aac", Default: true, Info: info(map[string]any{"channels": 2})}},
				Chapters: chapters,
			})
		})
		h.names[f] = "file:" + rel
	}
	v := h.b.Version(parts, w)
	h.names[w], h.names[v] = "book:"+m.Title, "version:"+m.Title
	h.metadata(w, m)
	return w
}

func (h *household) person(name string) id.ID {
	h.t.Helper()
	var p id.ID
	if err := h.b.DB.Reader().QueryRow(`SELECT id FROM person WHERE name = ?`, name).Scan(&p); err != nil {
		h.t.Fatal(err)
	}
	h.names[p] = "person:" + name
	return p
}

func (h *household) parent(work id.ID) id.ID {
	h.t.Helper()
	w, err := h.b.Catalog.Work(context.Background(), work)
	if err != nil {
		h.t.Fatal(err)
	}
	return w.Parent
}

func (h *household) named(it catalogtest.Item, name string) {
	h.names[it.Work] = name
	h.names[it.Version] = "version:" + name
	h.names[it.File] = "file:" + name
}

func (h *household) metadata(work id.ID, m catalog.WorkMetadata) {
	h.t.Helper()
	w, err := h.b.Catalog.Work(context.Background(), work)
	if err != nil {
		h.t.Fatal(err)
	}
	if m.Title == "" {
		m.Title = w.Title
	}
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return h.b.Catalog.UpdateMetadata(ctx, tx, work, m)
	})
}

func (h *household) tracks(file id.ID, d time.Duration, tracks []catalog.Track) {
	h.t.Helper()
	f, err := h.b.Catalog.File(context.Background(), file)
	if err != nil {
		h.t.Fatal(err)
	}
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return h.b.Catalog.SetProbe(ctx, tx, file, f.Generation, catalog.Probe{Summary: catalog.ProbeSummary{Container: "matroska", Duration: d, Bitrate: 8_000_000}, Tracks: tracks})
	})
}

func (h *household) collectionOf(work id.ID) id.ID {
	h.t.Helper()
	var c id.ID
	err := h.b.DB.Reader().QueryRow(`SELECT collection_id FROM collection_work WHERE work_id = ?`, work).Scan(&c)
	if err != nil {
		h.t.Fatal(err)
	}
	return c
}

// viewer makes a user and returns its Viewer. No grants give it every
// library.
func (h *household) viewer(name string, u users.NewUser) users.Viewer {
	h.t.Helper()
	svc := users.New(h.b.DB, events.NewBus())
	u.Username = name
	if len(u.Grants.Libraries) == 0 {
		u.Grants.All = true
	}
	var user id.ID
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		u, err := svc.CreateUser(ctx, tx, u)
		user = u.ID
		return err
	})
	h.names[user] = "user:" + name
	return h.mustViewer(user)
}

func (h *household) mustViewer(user id.ID) users.Viewer {
	h.t.Helper()
	v, err := users.New(h.b.DB, events.NewBus()).Viewer(context.Background(), user)
	if err != nil {
		h.t.Fatal(err)
	}
	return v
}

func info(m map[string]any) json.RawMessage {
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}

func ptr[T any](v T) *T { return &v }
