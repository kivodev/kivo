package scan_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/scan"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/media/ffmpeg"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/tasks"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
)

var ctx = context.Background()

// TestAudiobookPipeline runs the audiobook pipeline end to end over real
// files made with ffmpeg, laid out as a real library is. It covers folders
// from Bookarr and in Audiobookshelf's layout, a single chaptered m4b with
// a cover, a book of per-chapter MP3s with a cover image, a book with CD
// folders, and positions in folder names and tags. KIVO_FFMPEG names the
// ffmpeg to use, as in the media package's tests.
func TestAudiobookPipeline(t *testing.T) {
	bin := os.Getenv("KIVO_FFMPEG")
	if bin == "" {
		t.Skip("KIVO_FFMPEG not set")
	}
	probe := os.Getenv("KIVO_FFPROBE")
	if probe == "" {
		probe = filepath.Join(filepath.Dir(bin), "ffprobe")
	}
	dir := t.TempDir()
	tmp := t.TempDir()
	chapters := filepath.Join(tmp, "chapters.txt")
	cover := filepath.Join(tmp, "cover.jpg")
	write(t, chapters, ";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=1000\ntitle=Opening Credits\n"+
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=1000\nEND=2000\ntitle=Chapter 1\n")
	ffrun(t, bin, "-f", "lavfi", "-i", "testsrc2=size=64x64", "-frames:v", "1", cover)
	tone := []string{"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=22050:duration=2"}
	m4b := func(rel string, tags ...string) {
		args := append(append([]string{}, tone...), "-i", chapters, "-map", "0:a", "-map_chapters", "1", "-c:a", "aac", "-b:a", "32k")
		ffrun(t, bin, append(append(args, tags...), mkdir(t, dir, rel))...)
	}
	mp3 := func(rel string, tags ...string) {
		args := append(append([]string{}, tone...), "-c:a", "libmp3lame", "-b:a", "32k")
		ffrun(t, bin, append(append(args, tags...), mkdir(t, dir, rel))...)
	}
	meta := func(kv ...string) []string {
		var out []string
		for i := 0; i < len(kv); i += 2 {
			out = append(out, "-metadata", kv[i]+"="+kv[i+1])
		}
		return out
	}

	// A single m4b with chapters and its cover inside, as inAudible makes
	// them, in Bookarr's Author/Title layout.
	ffrun(t, bin, append(append(append([]string{}, tone...), "-i", cover, "-i", chapters, "-map", "0:a", "-map", "1:v", "-map_chapters", "2",
		"-c:a", "aac", "-b:a", "32k", "-c:v", "copy", "-disposition:v:0", "attached_pic"),
		append(meta("album", "Project Hail Mary", "artist", "Andy Weir", "composer", "Ray Porter", "date", "2021",
			"description", "A lone astronaut.", "genre", "Audiobook"), mkdir(t, dir, "Andy Weir/Project Hail Mary/Project Hail Mary.m4b"))...)...)
	// There is no series folder. The series is in the tags, and the ASIN
	// is in the name.
	m4b("Peter F. Hamilton/Exodus/Exodus - The Archimedes Engine [B0CTKSG1VB].m4b", append(meta(
		"album", "Exodus: The Archimedes Engine (Unabridged)", "artist", "Peter F. Hamilton", "composer", "John Lee",
		"SERIES", "Exodus", "PART", "1", "PUBLISHER", "Random House Audio", "date", "2024"), "-movflags", "use_metadata_tags")...)
	// Series folders with positions. One position is a decimal, and one
	// folder names its narrator.
	m4b("James S. A. Corey/The Expanse/Book 1 - Leviathan Wakes {Jefferson Mays}/Leviathan Wakes.m4b",
		meta("artist", "James S.A. Corey (The Expanse #1)", "composer", "Somebody Else")...)
	m4b("James S. A. Corey/The Expanse/Book 1.5 - The Churn/The Churn.m4b", meta("composer", "Ray Porter")...)
	// Per-chapter files numbered as tracks, with a cover image beside them,
	// and the CUE, NFO and Bookarr files that are not part of the book.
	for i, name := range []string{"01 - Opening Credits", "02 - Chapter 1", "10 - Chapter 9"} {
		mp3("Stephen King/11.22.63/"+name+".mp3", meta("track", []string{"1/3", "2/3", "3/3"}[i], "composer", "Craig Wasson", "album", "11.22.63")...)
	}
	copyFile(t, cover, filepath.Join(dir, "Stephen King/11.22.63/cover.jpg"))
	write(t, filepath.Join(dir, "Stephen King/11.22.63/11.22.63.cue"), "FILE \"x.mp3\" MP3\n")
	write(t, filepath.Join(dir, "Stephen King/11.22.63/11.22.63.nfo"), "Read By: Craig Wasson\n")
	write(t, filepath.Join(dir, "Stephen King/11.22.63/.bookarr-3f2a.recycle"), "x")
	// CD folders make one book.
	mp3("Michael J. Sullivan/Riyria Revelations/Heir of Novron/CD2/01.mp3", meta("composer", "Tim Gerard Reynolds")...)
	mp3("Michael J. Sullivan/Riyria Revelations/Heir of Novron/CD1/01.mp3", meta("composer", "Tim Gerard Reynolds")...)
	// A title starting with a number is not a position.
	mp3("Dodie Smith/Dalmatians/101 Dalmatians/101 Dalmatians.mp3")
	settle(t, dir)

	p := newPipeline(t, bin, probe)
	lib := p.library(dir)
	p.runAll()

	page, err := p.q.Browse(ctx, p.admin, lib, query.BrowseOptions{Sort: query.ByAuthor})
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	byTitle := map[string]query.WorkSummary{}
	for _, c := range page.Items {
		titles = append(titles, c.Title)
		byTitle[c.Title] = c
	}
	if want := []string{"Project Hail Mary", "101 Dalmatians", "The Churn", "Leviathan Wakes", "Heir of Novron", "Exodus", "11.22.63"}; !reflect.DeepEqual(titles, want) {
		t.Fatalf("books by author %q\nwant %q", titles, want)
	}

	detail := func(title string) query.WorkDetail {
		t.Helper()
		d, err := p.q.Detail(ctx, p.admin, byTitle[title].ID)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	names := func(ps []query.PersonRef) []string {
		var out []string
		for _, x := range ps {
			out = append(out, x.Name)
		}
		return out
	}

	hail := detail("Project Hail Mary")
	if b := hail.Summary.Book; !reflect.DeepEqual(names(b.Authors), []string{"Andy Weir"}) || !reflect.DeepEqual(names(b.Narrators), []string{"Ray Porter"}) ||
		len(b.Series) != 0 || hail.Summary.Year != 2021 || hail.Overview != "A lone astronaut." || hail.Versions[0].Chapters != 2 {
		t.Errorf("Project Hail Mary: %+v %+v %+v", hail.Summary, *hail.Summary.Book, hail.Versions)
	}
	if hail.Summary.Images.Poster == nil || origin(t, p, hail.Summary.ID) != filepath.Join(dir, "Andy Weir/Project Hail Mary/Project Hail Mary.m4b") {
		t.Error("Project Hail Mary's cover isn't its embedded picture")
	}

	exodus := detail("Exodus")
	if s := exodus.Summary.Book.Series; len(s) != 1 || s[0].Name != "Exodus" || s[0].Position != "1" || exodus.Book.Publisher != "Random House Audio" ||
		!reflect.DeepEqual(names(exodus.Summary.Book.Narrators), []string{"John Lee"}) {
		t.Errorf("Exodus: %+v %+v", *exodus.Summary.Book, *exodus.Book)
	}
	if w, _ := p.cat.Work(ctx, exodus.Summary.ID); !slices.Contains(w.ExternalIDs, catalog.ExternalID{Provider: "audible", Kind: "book", ID: "B0CTKSG1VB"}) {
		t.Errorf("Exodus' ASIN: %+v", w.ExternalIDs)
	}

	// The folders' author, series and narrator win over the tags'.
	lev := detail("Leviathan Wakes")
	if b := lev.Summary.Book; !reflect.DeepEqual(names(b.Authors), []string{"James S. A. Corey"}) || !reflect.DeepEqual(names(b.Narrators), []string{"Jefferson Mays"}) ||
		len(b.Series) != 1 || b.Series[0].Position != "1" {
		t.Errorf("Leviathan Wakes: %+v", *b)
	}
	series, err := p.q.Series(ctx, p.admin, lev.Summary.Book.Series[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(series.Books) != 2 || series.Books[0].Title != "Leviathan Wakes" || series.Books[1].Title != "The Churn" ||
		series.Books[1].Book.Series[0].Position != "1.5" {
		t.Errorf("The Expanse: %+v", series)
	}

	king := detail("11.22.63")
	var parts []string
	for _, part := range king.Versions[0].Parts {
		parts = append(parts, part.Name)
	}
	if !reflect.DeepEqual(parts, []string{"01 - Opening Credits.mp3", "02 - Chapter 1.mp3", "10 - Chapter 9.mp3"}) || king.Versions[0].Chapters != 3 {
		t.Errorf("11.22.63: parts %v, %d chapters", parts, king.Versions[0].Chapters)
	}
	if origin(t, p, king.Summary.ID) != filepath.Join(dir, "Stephen King/11.22.63/cover.jpg") {
		t.Error("11.22.63's cover isn't cover.jpg")
	}

	heir := detail("Heir of Novron")
	if len(heir.Versions) != 1 || len(heir.Versions[0].Parts) != 2 || !strings.Contains(heir.Versions[0].Parts[0].Path, "/CD1/") {
		t.Errorf("Heir of Novron: %+v", heir.Versions)
	}
	if d := detail("101 Dalmatians"); len(d.Summary.Book.Series) != 1 || d.Summary.Book.Series[0].Position != "" {
		t.Errorf("101 Dalmatians: %+v", *d.Summary.Book)
	}
	var stray int
	if err := p.b.DB.Reader().QueryRow(`SELECT count(*) FROM media_file WHERE rel_path LIKE '%.cue' OR rel_path LIKE '%.nfo' OR rel_path LIKE '%.bookarr-%'`).Scan(&stray); err != nil || stray != 0 {
		t.Errorf("%d stray files catalogued, %v", stray, err)
	}

	// An edit survives a rescan and a refresh.
	p.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return p.cat.EditMetadata(ctx, tx, hail.Summary.ID, catalog.WorkMetadata{Title: "Hail Mary", Book: &catalog.BookMetadata{Narrators: []string{"R. Porter"}}},
			catalog.FieldTitle, catalog.FieldNarrators)
	})
	if err := p.id.Refresh(ctx, p.admin, hail.Summary.ID); err != nil {
		t.Fatal(err)
	}
	p.runAll()
	hail = detail("Project Hail Mary")
	if hail.Summary.Title != "Hail Mary" || !reflect.DeepEqual(names(hail.Summary.Book.Narrators), []string{"R. Porter"}) ||
		!reflect.DeepEqual(names(hail.Summary.Book.Authors), []string{"Andy Weir"}) {
		t.Errorf("after a refresh: %+v", hail.Summary)
	}
}

// pipeline runs scan, identify and metadata over a real ffmpeg. Their
// tasks' items are done by hand, and the work they ask of other packages
// is dropped.
type pipeline struct {
	t     *testing.T
	b     *catalogtest.Builder
	cat   *catalog.Catalog
	s     *scan.Scanner
	id    *identify.Identifier
	meta  *metadata.Service
	q     *query.Service
	admin users.Viewer
}

func newPipeline(t *testing.T, ffmpegBin, ffprobeBin string) *pipeline {
	b := catalogtest.New(t)
	cfg, err := config.Open(ctx, b.DB, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	p := &pipeline{t: t, b: b, cat: b.Catalog, q: query.New(b.DB)}
	runner := ffmpeg.New(ffmpegBin, ffprobeBin)
	p.meta = metadata.New(b.DB, metadata.Deps{Catalog: b.Catalog, Imager: runner, Settings: cfg, Events: events.NewBus(), Tasks: discard{}, CacheDir: t.TempDir(), ImageDir: t.TempDir()}, metadata.Options{})
	act := activity.New(b.DB, events.NewBus())
	state := userstate.New(b.DB, userstate.Deps{Catalog: b.Catalog, Settings: cfg, Activity: act, Events: events.NewBus()})
	p.id = identify.New(b.DB, identify.Deps{
		Catalog: b.Catalog, Metadata: p.meta, Userstate: state, Activity: act, Tasks: discard{}, Markers: discard{},
	}, identify.Options{})
	p.s = scan.New(b.DB, scan.Deps{
		Catalog: b.Catalog, Tasks: discard{}, Prober: runner, Settings: cfg, Identifier: p.id, Previews: noPreviews{}, Subtitles: noSubtitles{},
	}, scan.Options{})
	svc := users.New(b.DB, events.NewBus())
	var user users.User
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		user, err = svc.CreateUser(ctx, tx, users.NewUser{Username: "admin", Admin: true, Grants: users.Grants{All: true}})
		return err
	})
	if p.admin, err = svc.Viewer(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	return p
}

func (p *pipeline) library(dir string) id.ID {
	p.t.Helper()
	lib, err := p.s.AddLibrary(ctx, catalog.LibraryInput{Name: "Audiobooks", Kind: catalog.Audiobooks, Folders: []string{dir}, UseLocalMetadata: true})
	if err != nil {
		p.t.Fatal(err)
	}
	return lib
}

// runAll scans every library and then refreshes the works due, until no
// file is behind and no work is due.
func (p *pipeline) runAll() {
	p.t.Helper()
	var scanLibraries, refresh tasks.Task
	for _, t := range append(p.s.Tasks(), p.meta.Tasks()...) {
		switch t.Name {
		case scan.TaskScanLibraries:
			scanLibraries = t
		case metadata.TaskRefreshMetadata:
			refresh = t
		}
	}
	for range 20 {
		libs, err := p.cat.Libraries(ctx)
		if err != nil {
			p.t.Fatal(err)
		}
		for _, l := range libs {
			if err := scanLibraries.Do(ctx, l.ID); err != nil {
				p.t.Fatalf("scan: %v", err)
			}
		}
		due, err := refresh.Pending(ctx)
		if err != nil {
			p.t.Fatal(err)
		}
		for _, w := range due {
			if err := refresh.Do(ctx, w); err != nil {
				p.t.Fatalf("refresh: %v", err)
			}
		}
		behind, err := p.s.Behind(ctx)
		if err != nil {
			p.t.Fatal(err)
		}
		if len(due) == 0 && len(behind) == 0 {
			return
		}
	}
	p.t.Fatal("work kept coming")
}

// origin returns where a work's poster was made from.
func origin(t *testing.T, p *pipeline, work id.ID) string {
	t.Helper()
	var o string
	if err := p.b.DB.Reader().QueryRow(`SELECT origin FROM image WHERE work_id = ? AND kind = 'poster'`, work).Scan(&o); err != nil {
		t.Fatal(err)
	}
	return o
}

func ffrun(t *testing.T, bin string, args ...string) {
	t.Helper()
	out, err := exec.Command(bin, append([]string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg %v: %v\n%s", args, err, out)
	}
}

func mkdir(t *testing.T, dir, rel string) string {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	write(t, to, string(b))
}

// settle dates everything a while back, because a scan wants files that
// aren't still being written.
func settle(t *testing.T, dir string) {
	t.Helper()
	at := time.Now().Add(-time.Hour)
	err := filepath.Walk(dir, func(p string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(p, at, at)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// discard drops what the pipeline asks of tasks and markers. runAll does
// the scans and refreshes it needs by hand.
type discard struct{}

func (discard) Ask(string, ...id.ID)                                              {}
func (discard) Placed(context.Context, *store.Tx, catalog.Library, []id.ID) error { return nil }

// noPreviews and noSubtitles drop what a probe asks of previews and
// subtitles.
type (
	noPreviews  struct{}
	noSubtitles struct{}
)

func (noPreviews) Probed(*store.Tx, catalog.Library, id.ID)                   {}
func (noSubtitles) Probed(*store.Tx, catalog.Library, id.ID, []catalog.Track) {}
