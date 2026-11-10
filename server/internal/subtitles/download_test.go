package subtitles

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
)

// downloads returns a service over a movies library holding one movie,
// with a fake answering for OpenSubtitles.
type downloads struct {
	t         *testing.T
	b         *catalogtest.Builder
	remote    *fakeOpenSubtitles
	s         *Service
	settings  *config.Store
	users     *users.Service
	userstate *userstate.Service
	subDir    string
	file      id.ID
}

func newDownloads(t *testing.T) *downloads {
	d := &downloads{t: t, b: catalogtest.New(t), subDir: t.TempDir(), remote: newFakeOpenSubtitles()}
	_, folder := d.b.Library(catalog.Movies)
	d.file = d.b.Movie(folder, "Dune", 2021).File
	settings, err := config.Open(ctx, d.b.DB, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	d.settings = settings
	d.setOpenSubtitles("os-key", "", "")
	d.users = users.New(d.b.DB, events.NewBus())
	d.userstate = userstate.New(d.b.DB, userstate.Deps{Catalog: d.b.Catalog, Settings: settings, Events: events.NewBus()})
	d.s = New(d.b.DB, Deps{
		Catalog: d.b.Catalog, Users: d.users, Userstate: d.userstate, Settings: settings, Media: &fakeMedia{}, Tasks: &fakeTasks{},
		CacheDir: t.TempDir(), SubtitleDir: d.subDir,
	}, Options{})
	d.s.openSubtitles.get = d.remote
	d.s.room.Free = func(string) (uint64, error) { return 100 << 30, nil }
	return d
}

// results maps the start of each fixture search result's ID, such as
// "7001.en", to the full ID.
func (d *downloads) results() map[string]string {
	d.t.Helper()
	rs, err := d.s.SearchSubtitles(ctx, d.viewer(false), d.file, SubtitleSearch{Languages: []string{"en"}})
	if err != nil {
		d.t.Fatal(err)
	}
	ids := map[string]string{}
	for _, r := range rs {
		head, _, _ := strings.Cut(r.ID, "~")
		ids[head] = r.ID
	}
	return ids
}

// viewer returns the Viewer of a new user who sees every library, or no
// library when blind is true.
func (d *downloads) viewer(blind bool) users.Viewer {
	d.t.Helper()
	svc := d.users
	grants := users.Grants{All: !blind}
	if blind {
		other, _ := d.b.Library(catalog.Movies)
		grants.Libraries = []id.ID{other}
	}
	var user users.User
	d.b.Write(func(ctx context.Context, tx *store.Tx) error {
		var err error
		user, err = svc.CreateUser(ctx, tx, users.NewUser{Username: "viewer-" + id.New().String(), Grants: grants})
		return err
	})
	v, err := svc.Viewer(ctx, user.ID)
	if err != nil {
		d.t.Fatal(err)
	}
	return v
}

func TestSearchSubtitles(t *testing.T) {
	d := newDownloads(t)
	v := d.viewer(false)
	got, err := d.s.SearchSubtitles(ctx, v, d.file, SubtitleSearch{Languages: []string{"en"}})
	if err != nil {
		t.Fatal(err)
	}
	// Results made for this file come first, then full before forced, then
	// non-SDH before SDH, then the most downloaded.
	if want := []string{"7002.en.hi", "7001.en", "7005.en.hi", "7003.en.forced"}; !slices.Equal(resultIDs(got), want) {
		t.Fatalf("results %v, want %v", resultIDs(got), want)
	}
	// A user who can't see the file can't search for it.
	if _, err := d.s.SearchSubtitles(ctx, d.viewer(true), d.file, SubtitleSearch{}); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("a file the user can't see: %v", err)
	}
	// OpenSubtitles' errors pass through.
	d.remote.set(http.StatusServiceUnavailable)
	if _, err := d.s.SearchSubtitles(ctx, v, d.file, SubtitleSearch{Languages: []string{"de"}}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("OpenSubtitles down: %v", err)
	}
}

// TestSearchSubtitlesForTheUser checks what a search takes from its user.
// With no language asked, it searches the user's subtitle language. SDH
// ranks first for an episode whose series the user last watched with SDH,
// when the user remembers track choices.
func TestSearchSubtitlesForTheUser(t *testing.T) {
	d := newDownloads(t)
	err := d.settings.Update(ctx, func(s *config.Settings) error {
		s.MetadataLanguage = "de" // the fake has no German subtitles
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	v := d.viewer(false)
	if got, err := d.s.SearchSubtitles(ctx, v, d.file, SubtitleSearch{}); err != nil || len(got) != 0 {
		t.Fatalf("no language, none preferred: %v %v", resultIDs(got), err)
	}
	en := "en"
	if err := d.users.EditPreferences(ctx, v, users.PreferencesChange{Playback: &users.PlaybackChange{SubtitleLanguage: &en}}); err != nil {
		t.Fatal(err)
	}
	if got, err := d.s.SearchSubtitles(ctx, v, d.file, SubtitleSearch{}); err != nil || len(got) != 4 {
		t.Fatalf("no language, English preferred: %v %v", resultIDs(got), err)
	}
	if got, err := d.s.SearchSubtitles(ctx, v, d.file, SubtitleSearch{Languages: []string{"de"}}); err != nil || len(got) != 0 {
		t.Fatalf("German asked, English preferred: %v %v", resultIDs(got), err)
	}

	_, folder := d.b.Library(catalog.Shows)
	series := d.b.Series("Dune: Prophecy")
	episode := d.b.Episode(folder, series, 1, 1).File
	notSDH := []string{"7002.en.hi", "7001.en", "7005.en.hi", "7003.en.forced"}
	sdh := []string{"7002.en.hi", "7005.en.hi", "7001.en", "7003.en.forced"}
	search := func(name string, file id.ID, want []string) {
		t.Helper()
		got, err := d.s.SearchSubtitles(ctx, v, file, SubtitleSearch{Languages: []string{"en"}})
		if err != nil || !slices.Equal(resultIDs(got), want) {
			t.Errorf("%s: %v %v, want %v", name, resultIDs(got), err, want)
		}
	}
	search("an episode with no choice", episode, notSDH)
	if err := d.userstate.SetTrackChoice(ctx, v.User(), series, userstate.TrackChoice{SubtitleLanguage: "en", SubtitleKind: "sdh"}); err != nil {
		t.Fatal(err)
	}
	search("an episode of a series last watched with SDH", episode, sdh)
	search("a movie", d.file, notSDH)
	off := false
	if err := d.users.EditPreferences(ctx, v, users.PreferencesChange{Playback: &users.PlaybackChange{RememberTracks: &off}}); err != nil {
		t.Fatal(err)
	}
	search("a user who doesn't remember choices", episode, notSDH)
}

// A download is titled with its result's release. It replaces the file's
// downloaded subtitle with the same language and forced flag, including
// its file. A forced subtitle and a full one are kept side by side.
func TestDownloadSubtitleReplacesTheSameLanguage(t *testing.T) {
	d := newDownloads(t)
	v := d.viewer(false)
	if _, err := d.s.DownloadSubtitle(ctx, d.viewer(true), d.file, "7002.en.hi"); !errors.Is(err, catalog.ErrNotFound) || d.remote.count("/api/v1/download") != 0 {
		t.Fatalf("a file the user can't see: %v", err)
	}
	ids := d.results()
	download := func(result string) DownloadedSubtitle {
		t.Helper()
		got, err := d.s.DownloadSubtitle(ctx, v, d.file, result)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	first := download(ids["7001.en"])
	b, err := os.ReadFile(filepath.Join(d.subDir, filepath.FromSlash(first.Track.RelPath)))
	if err != nil || !strings.Contains(string(b), "The spice must flow.") || first.Track.Format != "srt" || !first.Track.Downloaded {
		t.Fatalf("stored %+v: %q %v", first.Track, b, err)
	}
	forced := download(ids["7003.en.forced"])
	if first.Track.Title != "Dune.Part.One.2021.1080p.WEB-DL" || len(first.Replaced) != 0 || len(forced.Replaced) != 0 {
		t.Fatalf("first %+v, forced %+v", first, forced)
	}
	second := download(ids["7002.en.hi"])
	if !slices.Equal(second.Replaced, []id.ID{first.Track.ID}) || second.Track.Title != "Dune.2021.2160p.UHD.BluRay.REMUX" {
		t.Fatalf("second %+v", second)
	}
	f, _ := d.b.Catalog.File(ctx, d.file)
	var titles []string
	for _, x := range f.SubtitleFiles {
		titles = append(titles, x.Title)
	}
	if !slices.Equal(titles, []string{"Dune.2021.2160p.UHD.BluRay.REMUX", "Dune.2021.Forced"}) {
		t.Fatalf("kept %+v", f.SubtitleFiles)
	}
	if _, err := os.Stat(filepath.Join(d.subDir, filepath.FromSlash(first.Track.RelPath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the replaced file: %v", err)
	}
	// Downloading the same result again replaces only itself.
	if again := download(ids["7002.en.hi"]); again.Track.ID != second.Track.ID || len(again.Replaced) != 0 {
		t.Fatalf("again %+v", again)
	}
}

// Replaced is told about a replacement inside its transaction, and an
// error from it undoes the replacement.
func TestReplacedIsToldInTheReplacement(t *testing.T) {
	d := newDownloads(t)
	var told [][]id.ID
	fail := errors.New("downloads not moved")
	d.s.replaced = func(_ context.Context, _ *store.Tx, replaced []id.ID, by id.ID) error {
		told = append(told, append(slices.Clone(replaced), by))
		return fail
	}
	v := d.viewer(false)
	first, err := d.s.DownloadSubtitle(ctx, v, d.file, "7001.en")
	if err != nil || len(told) != 0 {
		t.Fatalf("a first download: %v, told %v", err, told)
	}
	if _, err := d.s.DownloadSubtitle(ctx, v, d.file, "7002.en.hi"); !errors.Is(err, fail) || len(told) != 1 || told[0][0] != first.Track.ID {
		t.Fatalf("a replacement: %v, told %v", err, told)
	}
	f, _ := d.b.Catalog.File(ctx, d.file)
	if len(f.SubtitleFiles) != 1 || f.SubtitleFiles[0].ID != first.Track.ID {
		t.Fatalf("after a failed replacement %+v", f.SubtitleFiles)
	}
	if _, err := os.Stat(filepath.Join(d.subDir, filepath.FromSlash(first.Track.RelPath))); err != nil {
		t.Fatalf("the kept subtitle's file: %v", err)
	}
}
