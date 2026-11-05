package subtitles

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/tasks"
)

var ctx = context.Background()

// fakeMedia writes what ffmpeg would, and records each request.
type fakeMedia struct {
	mu        sync.Mutex
	err       error  // returned by every call
	during    func() // run in the middle of every call
	subtitles []media.SubtitlesRequest
}

func (m *fakeMedia) Subtitles(_ context.Context, req media.SubtitlesRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subtitles = append(m.subtitles, req)
	if m.during != nil {
		m.during()
	}
	if m.err != nil {
		return m.err
	}
	for _, t := range req.Tracks {
		body := map[media.SubtitleFormat]string{
			media.SRT: "1\n00:00:01,000 --> 00:00:02,000\nHello\n",
			media.ASS: "[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\\i1}Hi{\\i0}\n",
			media.PGS: "PG",
		}[t.Format]
		if err := os.WriteFile(t.Path, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ask is one call to Ask.
type ask struct {
	task  string
	items []id.ID
}

// fakeTasks records the items asked for.
type fakeTasks struct {
	mu    sync.Mutex
	asked []ask
}

func (t *fakeTasks) Ask(task string, items ...id.ID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.asked = append(t.asked, ask{task, items})
}

func (t *fakeTasks) take() []ask {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.asked
	t.asked = nil
	return out
}

type fixture struct {
	t        *testing.T
	b        *catalogtest.Builder
	settings *config.Store
	m        *fakeMedia
	tk       *fakeTasks
	s        *Service
	cache    string
	folder   string
	lib      id.ID
	folderID id.ID
	file     id.ID
	changes  int
}

// newFixture makes the service over a library of one MKV file. The file is
// attachments.mkv from the media package's fixtures, with text, ASS, PGS
// and DVD subtitle tracks and a font.
func newFixture(t *testing.T, previews catalog.Schedule) *fixture {
	f := &fixture{t: t, b: catalogtest.New(t), m: &fakeMedia{}, tk: &fakeTasks{}, cache: t.TempDir(), folder: t.TempDir()}
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		l, err := f.b.Catalog.CreateLibrary(ctx, tx, catalog.LibraryInput{Name: "Films", Kind: catalog.Movies, Folders: []string{f.folder}, Previews: previews})
		f.lib, f.folderID = l.ID, l.Folders[0].ID
		return err
	})
	f.file = f.add("a.mkv")
	settings, err := config.Open(ctx, f.b.DB, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	f.settings = settings
	f.s = New(f.b.DB, Deps{Catalog: f.b.Catalog, Settings: f.settings, Media: f.m, Tasks: f.tk, CacheDir: f.cache, SubtitleDir: t.TempDir()}, Options{})
	f.s.room.Free = func(string) (uint64, error) { return 100 << 30, nil }
	return f
}

// add writes a copy of attachments.mkv at rel and adds it as a probed
// file.
func (f *fixture) add(rel string) id.ID {
	f.t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "media", "testdata", "attachments.mkv"))
	if err == nil {
		err = os.WriteFile(filepath.Join(f.folder, rel), src, 0o644)
	}
	if err != nil {
		f.t.Fatal(err)
	}
	var file id.ID
	var gen int64
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		st, err := f.b.Catalog.UpsertFiles(ctx, tx, f.folderID, []catalog.FileObservation{{RelPath: rel, Size: int64(len(src)), MTime: time.UnixMilli(1)}})
		if err == nil {
			file, gen = st[0].ID, st[0].Generation
		}
		return err
	})
	f.probe(file, gen)
	return file
}

// probe records a file as probed at gen, with the tracks and font of
// attachments.mkv.
func (f *fixture) probe(file id.ID, gen int64) {
	f.t.Helper()
	tracks := []media.Track{
		{Index: 0, Kind: media.Video, Codec: media.H264, Width: 64, Height: 64, BitDepth: 8},
		{Index: 1, Kind: media.Audio, Codec: media.AAC, Channels: 1},
		{Index: 2, Kind: media.Subtitle, Codec: "subrip", Language: "eng"},
		{Index: 3, Kind: media.Subtitle, Codec: "ass", Language: "jpn"},
		{Index: 4, Kind: media.Subtitle, Codec: "hdmv_pgs_subtitle", Language: "eng"},
		{Index: 5, Kind: media.Subtitle, Codec: "dvd_subtitle", Language: "fre"},
	}
	var stored []catalog.Track
	for _, t := range tracks {
		info, _ := json.Marshal(t)
		stored = append(stored, catalog.Track{Index: t.Index, Type: string(t.Kind), Codec: string(t.Codec), Info: info})
	}
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return f.b.Catalog.SetProbe(ctx, tx, file, gen, catalog.Probe{
			Summary:     catalog.ProbeSummary{Container: "matroska,webm", Duration: 8 * time.Second},
			Tracks:      stored,
			Attachments: []catalog.Attachment{{Kind: catalog.AttachmentFont, Filename: "font.ttf", MimeType: "application/x-truetype-font", Size: 36, SHA256: fontSHA}},
		})
	})
}

// fontSHA is the SHA-256 of the first font attachments.mkv carries.
const fontSHA = "e50562b91d93c17c2e795e02747574de6e63b103ccdd2c852f8f7246800ed6d0"

// change makes the scan see that a's file changed, which bumps its
// generation, and probes it again.
func (f *fixture) change() {
	f.t.Helper()
	f.changes++
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := f.b.Catalog.UpsertFiles(ctx, tx, f.folderID, []catalog.FileObservation{{RelPath: "a.mkv", Size: 1, MTime: time.UnixMilli(int64(1 + f.changes))}})
		return err
	})
	f.probe(f.file, f.gen())
}

func (f *fixture) gen() int64 {
	f.t.Helper()
	cur, err := f.s.current(ctx, f.file)
	if err != nil {
		f.t.Fatal(err)
	}
	return cur
}

func (f *fixture) run() error {
	return f.s.do(ctx, f.file)
}

// pending lists what the extract task has left.
func (f *fixture) pending() []id.ID {
	f.t.Helper()
	files, err := f.s.pending(ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return files
}

// failed lists the files whose extraction failed.
func (f *fixture) failed() []tasks.Failure {
	f.t.Helper()
	out, err := f.s.failed(ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

// When a file changes while it's being read, the result is dropped,
// nothing is recorded, and the file's current generation is pending.
func TestStaleResultsAreDropped(t *testing.T) {
	f := newFixture(t, catalog.Nightly)
	f.m.during = f.change
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	if f.s.has(f.file, f.gen()) || f.s.has(f.file, f.gen()-1) {
		t.Fatal("a stale result was kept")
	}
	if p := f.pending(); !slices.Equal(p, []id.ID{f.file}) {
		t.Fatalf("pending %v", p)
	}
}

func TestSubtitles(t *testing.T) {
	f := newFixture(t, catalog.Nightly)

	// The file isn't extracted yet, and looking doesn't ask for anything.
	if _, ok, err := f.s.Subtitle(ctx, f.file, 2, "vtt"); ok || err != nil {
		t.Fatalf("before extraction: %v %v", ok, err)
	}
	if _, ok, err := f.s.Styled(ctx, f.file); ok || err != nil {
		t.Fatalf("styled before extraction: %v %v", ok, err)
	}
	if a := f.tk.take(); len(a) != 0 {
		t.Fatalf("a look asked for %+v", a)
	}

	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	if len(f.m.subtitles) != 1 || len(f.m.subtitles[0].Tracks) != 3 {
		t.Fatalf("asked to extract %+v, want tracks 2, 3 and 4 in one run", f.m.subtitles)
	}
	for _, c := range []struct {
		track  int
		format media.SubtitleFormat
		has    string
	}{
		{2, "vtt", "00:00:01.000 --> 00:00:02.000\nHello"},
		{3, "vtt", "00:00:01.000 --> 00:00:02.000\nHi"},
		{3, "ass", `{\i1}Hi{\i0}`},
		{4, media.PGS, "PG"},
	} {
		p, ok, err := f.s.Subtitle(ctx, f.file, c.track, c.format)
		b, _ := os.ReadFile(p)
		if !ok || err != nil || !strings.Contains(string(b), c.has) {
			t.Errorf("track %d as %s: %q %v %v", c.track, c.format, b, ok, err)
		}
	}
	for _, c := range []struct {
		track  int
		format media.SubtitleFormat
	}{{2, "srt"}, {5, media.PGS}, {4, "vtt"}} {
		if _, ok, _ := f.s.Subtitle(ctx, f.file, c.track, c.format); ok {
			t.Errorf("track %d as %s is there", c.track, c.format)
		}
	}
	fonts, ok, err := f.s.Styled(ctx, f.file)
	if !ok || err != nil || len(fonts) != 1 || filepath.Base(fonts[0]) != fontSHA+".ttf" {
		t.Fatalf("styled %v %q %v", ok, fonts, err)
	}
	if p, ok := f.s.fontFile(fontSHA); !ok || p != fonts[0] {
		t.Fatalf("the font by its hash: %q %v", p, ok)
	}
	for _, bad := range []string{strings.Repeat("0", 64), "../" + fontSHA[3:], "*", fontSHA[:63]} {
		if _, ok := f.s.fontFile(bad); ok {
			t.Errorf("font %q found", bad)
		}
	}

	if p := f.pending(); len(p) != 0 {
		t.Fatalf("pending %v after extraction", p)
	}

	// Once prepared, with every font stored and recorded, extracting again
	// reads nothing and asks for nothing.
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	if len(f.m.subtitles) != 1 || len(f.tk.take()) != 0 {
		t.Fatalf("a prepared file was read again (%d) or asked for something", len(f.m.subtitles))
	}
}

// A file whose fonts aren't all in the store isn't prepared, and is
// extracted again.
func TestStyledNeedsEveryFont(t *testing.T) {
	f := newFixture(t, catalog.Nightly)
	if err := f.run(); err != nil {
		t.Fatal(err)
	}

	// With cache/fonts/ deleted, the file isn't prepared. Extracting reads
	// the file again, which stores the font again.
	if err := os.RemoveAll(filepath.Join(f.cache, fontStore)); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := f.s.Styled(ctx, f.file); ok || err != nil {
		t.Fatalf("styled without its font: %v %v", ok, err)
	}
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	if fonts, ok, _ := f.s.Styled(ctx, f.file); len(f.m.subtitles) != 2 || !ok || len(fonts) != 1 {
		t.Fatalf("extracted %d times; styled %v %q", len(f.m.subtitles), ok, fonts)
	}
	if _, err := os.Stat(f.s.fontPath(fontSHA + ".ttf")); err != nil {
		t.Fatal(err)
	}
}

// A font whose bytes don't match what the probe recorded, for example
// because the file changed since, isn't stored under the recorded hash.
func TestFontsAreTheRecordedOnes(t *testing.T) {
	f := newFixture(t, catalog.Nightly)
	cf, err := f.b.Catalog.File(ctx, f.file)
	if err != nil {
		t.Fatal(err)
	}
	other := strings.Repeat("ab", 32)
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return f.b.Catalog.SetProbe(ctx, tx, f.file, cf.Generation, catalog.Probe{
			Summary:     catalog.ProbeSummary{Container: "matroska,webm", Duration: 8 * time.Second},
			Tracks:      cf.Tracks,
			Attachments: []catalog.Attachment{{Kind: catalog.AttachmentFont, Filename: "font.ttf", Size: 36, SHA256: other}},
		})
	})
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	if fonts, ok, err := f.s.Styled(ctx, f.file); !ok || err != nil || len(fonts) != 0 {
		t.Fatalf("styled %v %q %v, want no fonts", ok, fonts, err)
	}
	if _, ok := f.s.fontFile(other); ok {
		t.Fatal("stored under a hash its bytes don't have")
	}
}

// A font is stored once, by its content, for every file that has it. The
// prune deletes it once no file's subtitles list it.
func TestFontsAreShared(t *testing.T) {
	f := newFixture(t, catalog.Nightly)
	first := f.file
	second := f.add("b.mkv")
	for _, file := range []id.ID{first, second} {
		f.file = file
		if err := f.run(); err != nil {
			t.Fatal(err)
		}
	}
	var want string
	for _, file := range []id.ID{first, second} {
		fonts, ok, err := f.s.Styled(ctx, file)
		if err != nil || !ok || len(fonts) != 1 || (want != "" && fonts[0] != want) {
			t.Fatalf("fonts %q %v %v, want one shared", fonts, ok, err)
		}
		want = fonts[0]
		if b, err := os.ReadFile(want); err != nil || fmt.Sprintf("%x", sha256.Sum256(b)) != fontSHA {
			t.Fatalf("font %q %v", b, err)
		}
	}
	if !fontName(filepath.Base(want)) || filepath.Ext(want) != ".ttf" {
		t.Fatalf("stored as %s", want)
	}
	if stored, _ := filepath.Glob(filepath.Join(f.cache, "fonts", "*", "*")); !slices.Equal(stored, []string{want}) {
		t.Fatalf("stored %v, want only %s", stored, want)
	}
	if entries, _ := filepath.Glob(filepath.Join(f.s.result(first, f.gen()), "*")); slices.ContainsFunc(entries, func(p string) bool { return filepath.Base(p) == fontsDir }) {
		t.Fatal("fonts copied into a file's result")
	}

	// The prune keeps a font while any file lists it.
	unlisted := filepath.Join(f.cache, "fonts", "00", strings.Repeat("0", 64)+".otf")
	if err := os.MkdirAll(filepath.Dir(unlisted), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unlisted, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f.deleteFile(first)
	if err := f.s.prune(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("a font a file lists was removed: %v", err)
	}
	if _, err := os.Stat(unlisted); err == nil {
		t.Fatal("a font no file lists was kept")
	}
	f.deleteFile(second)
	if err := f.s.prune(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(want); err == nil {
		t.Fatal("a font no file lists was kept")
	}
}

// deleteFile deletes a file's row, the same as pruning a missing file
// does.
func (f *fixture) deleteFile(file id.ID) {
	f.t.Helper()
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM media_file WHERE id = ?`, file)
		return err
	})
}

func TestPrune(t *testing.T) {
	f := newFixture(t, catalog.Nightly)
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	current := f.s.result(f.file, f.gen())
	old := f.s.result(f.file, f.gen()+7)
	deleted := f.s.result(id.New(), 1)
	for _, d := range []string{old, deleted} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.prune(ctx); err != nil {
		t.Fatal(err)
	}
	for d, keep := range map[string]bool{current: true, old: false, deleted: false, filepath.Dir(deleted): false} {
		if _, err := os.Stat(d); (err == nil) != keep {
			t.Errorf("%s: kept %v, want %v", d, err == nil, keep)
		}
	}
	if p := f.pending(); len(p) != 0 {
		t.Fatalf("pending %v: the prune forgot subtitles still there", p)
	}
}

// A corrupt file is recorded as failed, with nothing prepared. It leaves
// the pending list until it changes or its failure is retried. The prune
// keeps its failure.
func TestCorruptFilesFail(t *testing.T) {
	f := newFixture(t, catalog.Nightly)
	f.m.err = &media.Error{Op: "x", Kind: media.ErrSourceCorrupt}
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	if p := f.pending(); len(p) != 0 {
		t.Fatalf("pending %v after it failed", p)
	}
	if fl := f.failed(); len(fl) != 1 || fl[0].Item != f.file || fl[0].Reason == "" || fl[0].At.IsZero() {
		t.Fatalf("failed %+v", fl)
	}
	if _, ok, _ := f.s.Subtitle(ctx, f.file, 2, "vtt"); ok {
		t.Error("a subtitle of a corrupt file")
	}
	if _, ok, err := f.s.Styled(ctx, f.file); ok || err != nil {
		t.Errorf("styled %v %v, want not prepared", ok, err)
	}

	// The prune keeps its failure, even though it has no result.
	if err := f.s.prune(ctx); err != nil {
		t.Fatal(err)
	}
	if p := f.pending(); len(p) != 0 || len(f.failed()) != 1 {
		t.Fatalf("pending %v, failed %+v after a prune", p, f.failed())
	}

	// After a retry, the failure is forgotten and the file is pending
	// again.
	again, err := f.s.retry(ctx, f.file)
	if err != nil || !slices.Equal(again, []id.ID{f.file}) {
		t.Fatalf("retry %v %v", again, err)
	}
	if p := f.pending(); !slices.Equal(p, []id.ID{f.file}) || len(f.failed()) != 0 {
		t.Fatalf("pending %v, failed %+v after a retry", p, f.failed())
	}
}

// A file the media package can't extract is recorded as failed, with no
// result. It leaves the pending list until it changes.
func TestUnsupportedFilesFail(t *testing.T) {
	f := newFixture(t, catalog.Nightly)
	f.m.err = &media.Error{Op: "x", Kind: media.ErrUnsupported}
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	if p, fl := f.pending(), f.failed(); len(p) != 0 || len(fl) != 1 || fl[0].Item != f.file {
		t.Fatalf("pending %v, failed %+v", p, fl)
	}
	if f.s.has(f.file, f.gen()) {
		t.Fatal("a result for a file media can't extract")
	}

	// A retry with no items forgets every failure. When the file fails
	// again, it stays failed.
	if again, err := f.s.retry(ctx); err != nil || !slices.Equal(again, []id.ID{f.file}) {
		t.Fatalf("retry of every failure: %v %v", again, err)
	}
	if err := f.run(); err != nil || len(f.failed()) != 1 {
		t.Fatalf("failed again: %v %+v", err, f.failed())
	}

	// Once the file changes, its current generation is pending, and no
	// failure is listed.
	f.change()
	if p := f.pending(); !slices.Equal(p, []id.ID{f.file}) || len(f.failed()) != 0 {
		t.Fatalf("changed: pending %v, failed %+v", p, f.failed())
	}
}

// keepTracks probes a file again, keeping its video and audio but only the
// subtitle tracks with the given codecs.
func (f *fixture) keepTracks(file id.ID, codecs ...string) {
	f.t.Helper()
	cf, err := f.b.Catalog.File(ctx, file)
	if err != nil {
		f.t.Fatal(err)
	}
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return f.b.Catalog.SetProbe(ctx, tx, file, cf.Generation, catalog.Probe{
			Summary: catalog.ProbeSummary{Container: "matroska,webm", Duration: 8 * time.Second},
			Tracks: slices.DeleteFunc(cf.Tracks, func(t catalog.Track) bool {
				return media.TrackKind(t.Type) == media.Subtitle && !slices.Contains(codecs, t.Codec)
			}),
		})
	})
}

// Pending lists present files with an unextracted embedded ASS, SSA or PGS
// track, in libraries that extract subtitles. A file with only text tracks
// isn't pending.
func TestPending(t *testing.T) {
	f := newFixture(t, catalog.Nightly)
	styled := f.add("b.mkv")
	pgs := f.add("c.mkv")
	text := f.add("d.mkv")
	f.keepTracks(pgs, "hdmv_pgs_subtitle")
	f.keepTracks(text, "subrip", "dvd_subtitle")
	if p := f.pending(); !slices.Equal(p, []id.ID{f.file, styled, pgs}) {
		t.Fatalf("pending %v, want %v", p, []id.ID{f.file, styled, pgs})
	}
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	if p := f.pending(); !slices.Equal(p, []id.ID{styled, pgs}) {
		t.Fatalf("pending %v after one was extracted", p)
	}
	never := catalog.Never
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, _, err := f.b.Catalog.UpdateLibrary(ctx, tx, f.lib, catalog.LibraryChange{Subtitles: &never})
		return err
	})
	if p := f.pending(); len(p) != 0 {
		t.Fatalf("pending %v in a library that never extracts them", p)
	}
	if ok, err := f.s.wanted(ctx); err != nil || ok {
		t.Fatalf("wanted %v, %v with no library extracting them", ok, err)
	}
}

// A file whose only extracted-ahead track is PGS is pending. Its track is
// extracted unchanged as a .sup file, which the prune removes along with
// the file's earlier generation.
func TestPGSIsExtractedAhead(t *testing.T) {
	f := newFixture(t, catalog.Nightly)
	f.keepTracks(f.file, "subrip", "hdmv_pgs_subtitle")
	if p := f.pending(); !slices.Equal(p, []id.ID{f.file}) {
		t.Fatalf("pending %v, want the PGS file", p)
	}
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	old, ok, err := f.s.Subtitle(ctx, f.file, 4, media.PGS)
	if b, _ := os.ReadFile(old); !ok || err != nil || filepath.Ext(old) != ".sup" || string(b) != "PG" {
		t.Fatalf("PGS track: %q %q %v %v", old, b, ok, err)
	}
	if p := f.pending(); len(p) != 0 {
		t.Fatalf("pending %v after extraction", p)
	}

	f.change()
	f.keepTracks(f.file, "hdmv_pgs_subtitle")
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	if err := f.s.prune(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); err == nil {
		t.Fatal("the earlier generation's .sup was kept")
	}
	if p, ok, _ := f.s.Subtitle(ctx, f.file, 4, media.PGS); !ok || filepath.Ext(p) != ".sup" {
		t.Fatalf("the current generation's .sup: %q %v", p, ok)
	}
}

// A probed file with an ASS, SSA or PGS track, in a library that extracts
// subtitles as media is added, is asked for once the probe's transaction
// commits. A file without such a track, or in a library that extracts
// nightly, isn't.
func TestProbed(t *testing.T) {
	f := newFixture(t, catalog.Nightly)
	cf, err := f.b.Catalog.File(ctx, f.file)
	if err != nil {
		t.Fatal(err)
	}
	lib := catalog.Library{ID: f.lib, Subtitles: catalog.NightlyAndAdded}
	probed := func(tracks []catalog.Track, fail error) error {
		return f.b.DB.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
			f.s.Probed(tx, lib, f.file, tracks)
			if a := f.tk.take(); len(a) != 0 {
				t.Errorf("asked for %+v before commit", a)
			}
			return fail
		})
	}
	ssa := slices.Clone(cf.Tracks)
	ssa[3].Codec = "ssa"
	pgs := slices.DeleteFunc(slices.Clone(cf.Tracks), func(t catalog.Track) bool { return t.Codec == "ass" })
	text := slices.DeleteFunc(slices.Clone(pgs), func(t catalog.Track) bool { return t.Codec == "hdmv_pgs_subtitle" })
	if err := probed(text, nil); err != nil {
		t.Fatal(err)
	}
	if a := f.tk.take(); len(a) != 0 {
		t.Fatalf("without an ASS or PGS track: asked for %+v", a)
	}
	if err := probed(cf.Tracks, errors.New("rolled back")); err == nil {
		t.Fatal("no error")
	}
	if a := f.tk.take(); len(a) != 0 {
		t.Fatalf("rolled back: asked for %+v", a)
	}
	for _, tracks := range [][]catalog.Track{cf.Tracks, ssa, pgs} {
		if err := probed(tracks, nil); err != nil {
			t.Fatal(err)
		}
		if a := f.tk.take(); len(a) != 1 || a[0].task != TaskExtractSubtitles || !slices.Equal(a[0].items, []id.ID{f.file}) {
			t.Fatalf("asked for %+v", a)
		}
	}
	lib.Subtitles = catalog.Nightly
	if err := probed(cf.Tracks, nil); err != nil {
		t.Fatal(err)
	}
	if a := f.tk.take(); len(a) != 0 {
		t.Fatalf("in a library that extracts them nightly: asked for %+v", a)
	}
}

// Deleting cache/ loses nothing for good. The prune forgets subtitles
// recorded as extracted whose result or fonts are gone, so the nightly
// task finds them missing.
func TestDeletedCacheIsRebuilt(t *testing.T) {
	for name, lose := range map[string]func(f *fixture) string{
		"result": func(f *fixture) string { return f.s.result(f.file, f.gen()) },
		"font":   func(f *fixture) string { return f.s.fontPath(fontSHA + ".ttf") },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, catalog.Nightly)
			if err := f.run(); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(lose(f)); err != nil {
				t.Fatal(err)
			}
			if err := f.s.prune(ctx); err != nil {
				t.Fatal(err)
			}
			if p := f.pending(); !slices.Equal(p, []id.ID{f.file}) {
				t.Fatalf("pending %v", p)
			}
			if err := f.run(); err != nil {
				t.Fatal(err)
			}
			if len(f.m.subtitles) != 2 {
				t.Fatalf("extracted %d times", len(f.m.subtitles))
			}
			if p := f.pending(); len(p) != 0 {
				t.Fatalf("pending %v", p)
			}
			if _, ok, _ := f.s.Subtitle(ctx, f.file, 2, "vtt"); !ok {
				t.Error("subtitles not rebuilt")
			}
			if fonts, ok, _ := f.s.Styled(ctx, f.file); !ok || len(fonts) != 1 {
				t.Error("fonts not rebuilt")
			} else if _, err := os.Stat(fonts[0]); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestFullCacheDisk(t *testing.T) {
	f := newFixture(t, catalog.Nightly)
	f.s.room.Free = func(string) (uint64, error) { return 10 << 20, nil }
	err := f.run()
	if !errors.Is(err, health.ErrDiskFull) || len(f.m.subtitles) != 0 || len(f.s.Health(ctx)) != 1 {
		t.Fatalf("on a full disk: %v, %d runs, problems %+v", err, len(f.m.subtitles), f.s.Health(ctx))
	}
	if p := f.pending(); !slices.Equal(p, []id.ID{f.file}) {
		t.Fatalf("pending %v on a full disk", p)
	}
	f.s.room.Free = func(string) (uint64, error) { return 10 << 30, nil }
	if err := f.run(); err != nil || len(f.s.Health(ctx)) != 0 {
		t.Fatalf("with room again: %v %+v", err, f.s.Health(ctx))
	}
}

// Prepare returns at once, reading nothing, for a file already prepared at
// its generation, whatever its library's schedule. A deleted file has
// nothing to prepare.
func TestPrepareReturnsAtOnceWhenPrepared(t *testing.T) {
	f := newFixture(t, catalog.Never)
	if err := f.s.Prepare(ctx, f.file); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Prepare(ctx, f.file); err != nil || len(f.m.subtitles) != 1 {
		t.Fatalf("prepared again: %v, read %d times", err, len(f.m.subtitles))
	}
	if _, ok, _ := f.s.Styled(ctx, f.file); !ok {
		t.Fatal("not prepared in a library that never extracts them ahead")
	}
	if err := f.s.Prepare(ctx, id.New()); err != nil {
		t.Fatalf("a deleted file: %v", err)
	}
}

// Prepare returns a permanent failure at once, with its reason, until the
// file changes.
func TestPrepareReturnsAFailureAtOnce(t *testing.T) {
	f := newFixture(t, catalog.Nightly)
	f.m.err = &media.Error{Op: "x", Kind: media.ErrSourceCorrupt}
	if err := f.s.Prepare(ctx, f.file); !errors.Is(err, ErrUnextractable) || !errors.Is(err, media.ErrSourceCorrupt) {
		t.Fatalf("failed: %v", err)
	}
	f.m.err = nil
	err := f.s.Prepare(ctx, f.file)
	if !errors.Is(err, ErrUnextractable) || !strings.Contains(err.Error(), "source corrupt") || len(f.m.subtitles) != 1 {
		t.Fatalf("failed for good: %v, read %d times", err, len(f.m.subtitles))
	}
	f.change()
	if err := f.s.Prepare(ctx, f.file); err != nil || len(f.m.subtitles) != 2 {
		t.Fatalf("changed: %v, read %d times", err, len(f.m.subtitles))
	}
}

// Two callers preparing the same file at once extract it only once. The
// second waits for the first and finds the file prepared.
func TestConcurrentPreparesExtractOnce(t *testing.T) {
	f := newFixture(t, catalog.Nightly)
	entered, release := make(chan struct{}), make(chan struct{})
	f.m.during = func() { close(entered); <-release }
	errs := make(chan error, 2)
	go func() { errs <- f.s.Prepare(ctx, f.file) }()
	<-entered
	go func() { errs <- f.s.Prepare(ctx, f.file) }()
	time.Sleep(20 * time.Millisecond) // the second waits on the first
	close(release)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if len(f.m.subtitles) != 1 {
		t.Fatalf("extracted %d times", len(f.m.subtitles))
	}
}
