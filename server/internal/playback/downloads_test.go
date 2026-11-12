package playback

import (
	"context"
	"errors"
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
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/subtitles"
	"github.com/mmcdole/kivo/internal/tasks"
	"github.com/mmcdole/kivo/internal/transcode"
	"github.com/mmcdole/kivo/internal/users"
)

// iPhone is what an iPhone plays offline: MP4, H.264 and HEVC with HDR,
// AAC and Dolby Digital. It draws WebVTT and ASS itself.
var iPhone = alike([]string{"mp4"}, true, []VideoEntry{safari.Video[0], safari.Video[1]},
	[]AudioEntry{{Codec: "aac", MaxChannels: 2}, {Codec: "ac3", MaxChannels: 2}, {Codec: "eac3", MaxChannels: 2}},
	vttFile, assFile)

// downloads gives the fixture's service a task runner that prepares
// downloads and runs until the test ends.
func (f *fixture) downloads() {
	f.t.Helper()
	stop := f.runTasks()
	f.t.Cleanup(stop)
}

// runTasks gives the fixture a new service and a task runner with its
// tasks. It returns a function that stops the runner and waits for it,
// the way a stopping server does.
func (f *fixture) runTasks() (stop func()) {
	f.t.Helper()
	r := tasks.New(f.b.DB, f.cfg, events.NewBus(), tasks.Options{})
	f.svc, f.runner = New(f.b.DB, f.deps(r)), r
	f.dl = NewDownloads(f.svc)
	for _, t := range f.dl.Tasks() {
		r.Add(t)
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { _ = r.Run(runCtx); close(done) }()
	return func() { cancel(); <-done }
}

// heardDownloads reports whether user's clients were told that their
// downloads changed. It waits out changed's interval.
func (f *fixture) heardDownloads(user id.ID) bool {
	deadline := time.After(3 * time.Second)
	for {
		select {
		case e := <-f.sub.C():
			if e.Kind == events.DownloadsChanged && e.Scope.User == user {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

func (f *fixture) download(work id.ID, quality string) (DownloadStatus, error) {
	return f.dl.Request(ctx, f.p, f.v, DownloadRequest{Work: work, Caps: iPhone, Quality: quality})
}

func (f *fixture) mustDownload(work id.ID, quality string) DownloadStatus {
	f.t.Helper()
	d, err := f.download(work, quality)
	if err != nil {
		f.t.Fatal(err)
	}
	return d
}

// await waits for a download to reach a state.
func (f *fixture) await(download id.ID, want DownloadState) DownloadStatus {
	f.t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		d, err := f.dl.Status(ctx, f.p, download)
		if err != nil {
			f.t.Fatal(err)
		}
		if d.State == want {
			return d
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("download %+v, want %s", d, want)
		}
	}
}

// onDisk writes a file's content where the catalog says it is, after
// moving its library's folder to a temporary one.
func (f *fixture) onDisk(file id.ID, content string) {
	f.t.Helper()
	f.exec("UPDATE library_folder SET path = ? WHERE id = (SELECT folder_id FROM media_file WHERE id = ?)", f.t.TempDir(), file)
	cf, err := f.b.Catalog.File(ctx, file)
	if err != nil {
		f.t.Fatal(err)
	}
	p := cf.Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// sized sets a file's size, the way a scan records it.
func (f *fixture) sized(file id.ID, size int64) {
	f.exec("UPDATE media_file SET size = ? WHERE id = ?", size, file)
}

func TestADownloadIsTheOriginalOrAConversion(t *testing.T) {
	f := newFixture(t, 0)
	f.downloads()
	// 3 Mbps over two hours is under 1080p's estimate.
	small := f.movie("Heat", "mp4", 3_000_000)
	f.onDisk(small.File, "the movie")
	d := f.mustDownload(small.Work, "high")
	if d.State != DownloadReady || d.Decision.Delivery != DeliverFile || d.Quality != "high" {
		t.Fatalf("an mp4 smaller than the estimate: %+v", d)
	}
	// Its size and ETag come from the file on disk.
	if p := d.Parts[0]; p.Size != int64(len("the movie")) || p.ETag == "" || p.SHA256 != "" || d.Size != p.Size || !d.SizeExact {
		t.Fatalf("the original's part: %+v", d.Parts)
	}
	file, err := f.dl.File(ctx, f.v, d.ID, 0)
	if err != nil || file.Type != "video/mp4" || file.ETag != d.Parts[0].ETag {
		t.Fatalf("the original: %+v %v", file, err)
	}

	// Asking again, as an app does when the answer was lost, returns the
	// same download.
	if again := f.mustDownload(small.Work, "high"); again.ID != d.ID {
		t.Fatalf("asked again: %s, then %s", d.ID, again.ID)
	}

	mkv := f.movie("Ronin", "mkv", 3_000_000)
	d = f.mustDownload(mkv.Work, "high")
	if d.Decision.Delivery != DeliverConversion || d.Decision.Summary() != DirectStream {
		t.Fatalf("an mkv for AVPlayer: %+v", d.Decision)
	}
	if _, err := f.dl.File(ctx, f.v, d.ID, 0); err != nil && !errors.Is(err, ErrNotReady) {
		t.Fatalf("before it's ready: %v", err)
	}
	d = f.await(d.ID, DownloadReady)
	p := d.Parts[0]
	if p.Size == 0 || len(p.SHA256) != 64 || p.ETag != `"`+p.SHA256+`"` || d.Size != p.Size || !d.SizeExact || d.Expires.Before(time.Now().Add(6*24*time.Hour)) {
		t.Fatalf("ready: %+v", d)
	}
	file, err = f.dl.File(ctx, f.v, d.ID, 0)
	if err != nil || file.Type != "video/mp4" || file.ETag != p.ETag {
		t.Fatalf("file: %+v %v", file, err)
	}
	if b, _ := os.ReadFile(file.Path); !strings.HasSuffix(string(b), " as copy") {
		t.Fatalf("converted %q", b)
	}
	if entries, _ := os.ReadDir(f.tcDir); len(entries) != 0 {
		t.Fatalf("a conversion touched the plays' segments: %v", entries)
	}
	if _, err := f.dl.File(ctx, f.v, d.ID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no second part: %v", err)
	}
}

func TestDownloadsArePreparedOneAtATimeInLine(t *testing.T) {
	f := newFixture(t, 0)
	f.media.hold = make(chan struct{})
	f.downloads()
	a := f.mustDownload(f.movie("Heat", "mkv", 1).Work, "high")
	b := f.mustDownload(f.movie("Ronin", "mkv", 1).Work, "high")
	c := f.mustDownload(f.movie("Thief", "mkv", 1).Work, "high")
	f.await(a.ID, DownloadPreparing)
	if got := f.await(b.ID, DownloadQueued); got.Progress != 0 || got.Remaining != 0 {
		t.Fatalf("second in line: %+v", got)
	}
	f.await(c.ID, DownloadQueued)
	if !f.heardDownloads(f.p.User) {
		t.Fatal("the user didn't hear its downloads move")
	}
	if _, err := f.start(f.hdrMovie("Arrival").Work, lan); err != nil {
		t.Fatalf("a stream beside a converting download: %v", err)
	}

	// Removing the first download stops it, and the next one takes its
	// place.
	if err := f.dl.Delete(ctx, f.p, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.dl.Status(ctx, f.p, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed: %v", err)
	}
	f.await(b.ID, DownloadPreparing)
	f.await(c.ID, DownloadQueued)
	close(f.media.hold)
	f.await(b.ID, DownloadReady)
	f.await(c.ID, DownloadReady)
	var converted []string
	for _, r := range f.media.converts {
		converted = append(converted, filepath.Base(filepath.Dir(r.Path)))
	}
	if !slices.Equal(converted, []string{"Heat (2000)", "Ronin (2000)", "Thief (2000)"}) {
		t.Fatalf("converted in the order %v", converted)
	}
	if names, _ := f.tc.Downloads(); len(names) != 2 {
		t.Fatalf("cache/downloads holds %v", names)
	}
}

// A download that settles tells its own user, and the user whose download
// starts next, but not another user whose download is still in line.
func TestASettledDownloadTellsOnlyItsUser(t *testing.T) {
	f := newFixture(t, 0)
	f.media.hold = make(chan struct{})
	defer close(f.media.hold)
	f.downloads()
	m, sub := f.subtitled()
	hold := make(chan struct{})
	f.extractions.set(nil, hold)
	a, err := f.dl.Request(ctx, f.p, f.v, DownloadRequest{Work: m.Work, SubtitleTrack: &sub, Caps: iPhone})
	if err != nil {
		t.Fatal(err)
	}
	b := f.mustDownload(f.movie("Ronin", "mkv", 1).Work, "high")
	f.await(a.ID, DownloadPreparing)
	f.await(b.ID, DownloadQueued)

	// Another user's download waits behind both.
	var friend id.ID
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		u, err := f.users.CreateUser(ctx, tx, users.NewUser{Username: "friend", Grants: users.Grants{All: true}})
		friend = u.ID
		return err
	})
	f.exec(`INSERT INTO download (id, user_id, session_id, work_id, version_id, request, delivered, state, created_at, queued_at)
		SELECT ?, ?, session_id, work_id, version_id, request, delivered, 'queued', created_at, queued_at + 3600000 FROM download WHERE id = ?`,
		id.New(), friend, b.ID)
	f.published(events.DownloadsChanged)

	close(hold)
	f.await(a.ID, DownloadReady)
	f.await(b.ID, DownloadPreparing)
	if !f.heardDownloads(f.p.User) {
		t.Fatal("the user didn't hear its downloads move")
	}
	if f.heardDownloads(friend) {
		t.Fatal("another user with a download in line heard of it")
	}
}

func TestAPreparingDownloadSaysHowLongItHasLeft(t *testing.T) {
	p := &preparation{total: time.Hour}
	if done, left := p.progressLocked(time.Now()); done != 0 || left != 0 {
		t.Fatalf("not started: %v %v", done, left)
	}
	now := time.Now()
	p.started, p.reached = now.Add(-5*time.Second), 6*time.Minute
	if done, left := p.progressLocked(now); done != 0.1 || left != 0 {
		t.Fatalf("too soon to tell: %v %v", done, left)
	}
	p.started = now.Add(-2 * time.Minute)
	if done, left := p.progressLocked(now); done != 0.1 || left != 18*time.Minute {
		t.Fatalf("a tenth in two minutes: %v %v", done, left)
	}
}

func TestAReadyConversionIsKeptUntilFetchedOrExpired(t *testing.T) {
	f := newFixture(t, 0)
	f.downloads()
	a := f.await(f.mustDownload(f.movie("Heat", "mkv", 1).Work, "high").ID, DownloadReady)
	b := f.await(f.mustDownload(f.movie("Ronin", "mkv", 1).Work, "high").ID, DownloadReady)
	fa, _ := f.dl.File(ctx, f.v, a.ID, 0)
	fb, _ := f.dl.File(ctx, f.v, b.ID, 0)

	// A play's expiry doesn't affect the download.
	f.svc.endExpired(ctx)
	if err := f.dl.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fa.Path); err != nil {
		t.Fatalf("a ready conversion went: %v", err)
	}

	// The device has every file, so the server deletes its copy.
	if err := f.dl.Delete(ctx, f.p, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(fa.Path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fetched, its conversion is still there: %v", err)
	}

	// Seven days after the download is ready, the prune deletes it, along
	// with any conversion that has no download.
	f.exec("UPDATE download SET settled_at = ?", time.Now().Add(-keepDownload-time.Hour).UnixMilli())
	stray := filepath.Join(filepath.Dir(filepath.Dir(fb.Path)), id.New().String())
	if err := os.Mkdir(stray, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := f.dl.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.dl.Status(ctx, f.p, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired: %v", err)
	}
	if names, _ := f.tc.Downloads(); len(names) != 0 {
		t.Fatalf("cache/downloads holds %v", names)
	}
}

func TestADownloadSurvivesARestart(t *testing.T) {
	f := newFixture(t, 0)
	f.media.hold = make(chan struct{})
	stop := f.runTasks()
	d := f.mustDownload(f.movie("Heat", "mkv", 1).Work, "high")
	f.await(d.ID, DownloadPreparing)

	// The server stops partway through.
	stop()
	if got, _ := f.dl.Status(ctx, f.p, d.ID); got.State != DownloadQueued {
		t.Fatalf("stopped part way: %+v", got)
	}
	if names, _ := f.tc.Downloads(); len(names) != 0 {
		t.Fatalf("a conversion cut short left %v", names)
	}

	// After the restart, its preparation starts again on its own.
	f.media.hold = nil
	f.downloads()
	got := f.await(d.ID, DownloadReady)
	if got.Parts[0].SHA256 == "" || len(f.media.converts) != 2 {
		t.Fatalf("after a restart: %+v, %d conversions", got, len(f.media.converts))
	}
}

func TestAFailedDownloadSaysWhyAndRetries(t *testing.T) {
	f := newFixture(t, 0)
	f.downloads()
	f.media.fail = media.ErrEncoderFailed
	d := f.mustDownload(f.movie("Heat", "mkv", 1).Work, "high")
	if got := f.await(d.ID, DownloadFailed); got.Reason != RefuseTranscodeFailed {
		t.Fatalf("failed: %+v", got)
	}
	if _, err := f.dl.File(ctx, f.v, d.ID, 0); !errors.Is(err, ErrNotReady) {
		t.Fatalf("a failed conversion's file: %v", err)
	}
	f.media.mu.Lock()
	f.media.fail = nil
	f.media.mu.Unlock()
	again, err := f.dl.Retry(ctx, f.p, f.v, d.ID)
	if err != nil || again.ID != d.ID || again.State == DownloadFailed || again.Reason != "" {
		t.Fatalf("retried: %+v %v", again, err)
	}
	f.await(d.ID, DownloadReady)

	// A full disk is reported as the reason.
	f.tc = mustTranscoder(t, f.media, 1<<20)
	f.downloads()
	d = f.mustDownload(f.movie("Ronin", "mkv", 1).Work, "high")
	if got := f.await(d.ID, DownloadFailed); got.Reason != RefuseStorageFull {
		t.Fatalf("a full disk: %+v", got)
	}
}

// An app asking again after an update, when it reports what it plays
// differently, gets the download it has. One asked for another way, at
// another quality or with other tracks, is a download of its own.
func TestAskingAgainWithOtherCapabilitiesReturnsTheSameDownload(t *testing.T) {
	f := newFixture(t, 0)
	f.media.hold = make(chan struct{})
	f.downloads()
	defer close(f.media.hold)
	m, sub := f.subtitled()
	d := f.mustDownload(m.Work, "standard")
	updated := iPhone
	updated.Audio = append(slices.Clone(iPhone.Audio), AudioEntry{Codec: "flac", MaxChannels: 2})
	again, err := f.dl.Request(ctx, f.p, f.v, DownloadRequest{Work: m.Work, Caps: updated, Quality: "standard"})
	if err != nil || again.ID != d.ID {
		t.Fatalf("asked again after an update: %s, then %s %v", d.ID, again.ID, err)
	}
	for name, r := range map[string]DownloadRequest{
		"quality":  {Work: m.Work, Caps: iPhone, Quality: "high"},
		"subtitle": {Work: m.Work, Caps: iPhone, Quality: "standard", SubtitleTrack: &sub},
		"settings": {Work: m.Work, Caps: iPhone, Quality: "standard", Settings: Settings{BurnSubtitles: true}},
	} {
		if other, err := f.dl.Request(ctx, f.p, f.v, r); err != nil || other.ID == d.ID {
			t.Errorf("asked with another %s: %s, then %s %v", name, d.ID, other.ID, err)
		}
	}
}

// A download in line is prepared with the audio and subtitle it chose when
// it was asked for, though its user prefers others by then.
func TestAQueuedDownloadKeepsItsTracksWhenPreferencesChange(t *testing.T) {
	f := newFixture(t, 0)
	f.svc = New(f.b.DB, f.deps(discard{})) // the test prepares it by hand below
	f.dl = NewDownloads(f.svc)
	a := f.dualAudioEpisode()
	f.extract(a.file, "", false)
	d, err := f.dl.Request(ctx, f.p, f.v, DownloadRequest{Work: a.work, Caps: tv, Quality: "standard"})
	if err != nil || d.State != DownloadQueued || selectedOf(d.Decision, media.Audio) != a.eng || selectedOf(d.Decision, media.Subtitle) != a.signs {
		t.Fatalf("asked with no preference: %+v %v", d, err)
	}
	audio, subtitles, mode := "ja", "fr", users.SubtitlesAlways
	change := users.PlaybackChange{AudioLanguage: &audio, SubtitleLanguage: &subtitles, SubtitleMode: &mode}
	if err := f.users.EditPreferences(ctx, f.v, users.PreferencesChange{Playback: &change}); err != nil {
		t.Fatal(err)
	}
	pref, err := f.svc.Preferred(ctx, f.viewer(), a.work)
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range pref.Versions {
		if tr.Audio != a.jpn || tr.Subtitle != a.fre {
			t.Fatalf("a play now starts with %+v, want Japanese with French subtitles", tr)
		}
	}
	if err := f.dl.prepare(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	got := f.await(d.ID, DownloadReady)
	if selectedOf(got.Decision, media.Audio) != a.eng || selectedOf(got.Decision, media.Subtitle) != a.signs {
		t.Fatalf("prepared with audio %v and subtitle %v, want the English %v and its signs %v",
			selectedOf(got.Decision, media.Audio), selectedOf(got.Decision, media.Subtitle), a.eng, a.signs)
	}
	if c := f.converts(); len(c) != 1 || len(c[0].Audio) != 1 || c[0].Audio[0].Language != "eng" {
		t.Fatalf("converted %+v", c)
	}

	// Subtitles chosen off stay off.
	off := id.ID{}
	d, err = f.dl.Request(ctx, f.p, f.v, DownloadRequest{Work: a.work, SubtitleTrack: &off, Caps: tv, Quality: "standard"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.dl.prepare(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.await(d.ID, DownloadReady); selectedOf(got.Decision, media.Subtitle) != off {
		t.Fatalf("subtitles off, prepared with %v", selectedOf(got.Decision, media.Subtitle))
	}
}

// A download whose version is gone when its turn comes fails, saying its
// file couldn't be read.
func TestADownloadWhoseVersionIsGoneFails(t *testing.T) {
	f := newFixture(t, 0)
	f.svc = New(f.b.DB, f.deps(discard{}))
	f.dl = NewDownloads(f.svc)
	d := f.mustDownload(f.movie("Heat", "mkv", 1).Work, "high")
	f.exec("UPDATE download SET version_id = ?", id.New()) // as if the version was replaced by another
	if err := f.dl.prepare(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.await(d.ID, DownloadFailed); got.Reason != RefuseSourceUnreadable {
		t.Fatalf("its version gone: %+v", got)
	}
}

func TestADownloadIsAskedForOnceItsRowCommits(t *testing.T) {
	f := newFixture(t, 0)
	asked := &asks{f: f}
	f.svc = New(f.b.DB, f.deps(asked))
	f.dl = NewDownloads(f.svc)
	d := f.mustDownload(f.movie("Heat", "mkv", 1).Work, "high")
	if got := asked.list(); len(got) != 1 || got[0] != TaskPrepareDownloads+" "+d.ID.String()+" seen" {
		t.Fatalf("asked %v", got)
	}

	// The task's passes also see it as an item left to do.
	if open, err := f.dl.q.OpenDownloads(ctx); err != nil || !slices.Equal(open, []id.ID{d.ID}) {
		t.Fatalf("open downloads: %v %v", open, err)
	}
}

func TestAnUnreadableFileIsTriedAgainBeforeItFails(t *testing.T) {
	f := newFixture(t, 0)
	f.svc = New(f.b.DB, f.deps(discard{})) // the test runs each pass by hand below
	f.dl = NewDownloads(f.svc)
	f.media.fail = media.ErrSourceUnreadable
	d := f.mustDownload(f.movie("Heat", "mkv", 1).Work, "high")
	for try := 1; try < downloadAttempts; try++ {
		if err := f.dl.prepare(ctx, d.ID); !errors.Is(err, media.ErrSourceUnreadable) {
			t.Fatalf("try %d: %v", try, err)
		}
		if got, _ := f.dl.Status(ctx, f.p, d.ID); got.State != DownloadQueued {
			t.Fatalf("waiting for a later pass after try %d: %+v", try, got)
		}
		if open, _ := f.dl.q.OpenDownloads(ctx); !slices.Contains(open, d.ID) {
			t.Fatalf("not pending after try %d: %v", try, open)
		}
	}
	if err := f.dl.prepare(ctx, d.ID); err != nil {
		t.Fatalf("the last try: %v", err)
	}
	if got, _ := f.dl.Status(ctx, f.p, d.ID); got.State != DownloadFailed || got.Reason != RefuseSourceUnreadable {
		t.Fatalf("after the last try: %+v", got)
	}
	if open, _ := f.dl.q.OpenDownloads(ctx); slices.Contains(open, d.ID) {
		t.Fatalf("a failed download is still pending: %v", open)
	}
}

// discard drops every request, so a test can run a task's items by hand.
type discard struct{}

func (discard) Enqueue(string, ...id.ID) {}

// asks records each request, with whether the download was already
// readable outside the transaction that created it.
type asks struct {
	f  *fixture
	mu sync.Mutex
	l  []string
}

func (a *asks) Enqueue(task string, items ...id.ID) {
	for _, i := range items {
		seen := "unseen"
		if _, err := a.f.dl.q.GetDownload(ctx, i); err == nil {
			seen = "seen"
		}
		a.mu.Lock()
		a.l = append(a.l, task+" "+i.String()+" "+seen)
		a.mu.Unlock()
	}
}

func (a *asks) list() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.l)
}

// mustTranscoder returns a transcoder for media on a disk with free bytes
// free.
func mustTranscoder(t *testing.T, m transcode.Media, free uint64) *transcode.Transcoder {
	t.Helper()
	tc, err := transcode.New(transcode.Deps{Media: m, Keys: every2s{}, Dir: t.TempDir(), Downloads: t.TempDir()}, freeSpace(free))
	if err != nil {
		t.Fatal(err)
	}
	return tc
}

// subtitled adds an MP4 movie with an embedded SubRip track, and returns
// the movie and the track. An iPhone downloads the movie as the original,
// with the track as a WebVTT file.
func (f *fixture) subtitled() (catalogtest.Item, id.ID) {
	f.t.Helper()
	m := f.b.Movie(f.folder, "Heat", 1995)
	f.probeWith(m.File, "mp4", 2*time.Hour, 1_000_000,
		media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8},
		media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Default: true},
		media.Track{Index: 2, Kind: media.Subtitle, Codec: "subrip", Language: "eng"},
	)
	file, _ := f.b.Catalog.File(ctx, m.File)
	return m, file.Tracks[2].ID
}

// A download whose subtitle isn't extracted extracts it during
// preparation, showing the step, and asks for nothing.
func TestADownloadExtractsItsSubtitle(t *testing.T) {
	f := newFixture(t, 0)
	f.downloads()
	m, sub := f.subtitled()
	hold := make(chan struct{})
	f.extractions.set(nil, hold)
	d, err := f.dl.Request(ctx, f.p, f.v, DownloadRequest{Work: m.Work, SubtitleTrack: &sub, Caps: iPhone})
	if err != nil {
		t.Fatal(err)
	}
	if d.Decision.Delivery != DeliverFile || d.State == DownloadReady || selectedOf(d.Decision, media.Subtitle) != sub {
		t.Fatalf("the original, its subtitle to extract: %+v", d)
	}
	f.await(d.ID, DownloadPreparing)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		// The download reports its step. The runner counts the downloads.
		if items := f.runner.Items(TaskPrepareDownloads); len(items) == 1 && items[0].Step == StepExtractSubtitles {
			if items[0].Done != 0 || items[0].Total != 0 {
				t.Fatalf("its count: %+v", items[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("items %+v, want the download extracting its subtitles", f.runner.Items(TaskPrepareDownloads))
		}
	}
	close(hold)
	d = f.await(d.ID, DownloadReady)
	if i := slices.IndexFunc(d.ExternalSubtitles, func(s ExternalSubtitle) bool { return s.Track == sub }); i < 0 || !d.ExternalSubtitles[i].Ready || d.ExternalSubtitles[i].Format != "vtt" {
		t.Fatalf("its subtitle: %+v", d.ExternalSubtitles)
	}
	if _, err := f.dl.Subtitle(ctx, f.v, d.ID, d.ExternalSubtitles[0].Name); err != nil {
		t.Fatal(err)
	}
	if n, asked := f.extractions.count(), f.requested.take(); n != 1 || len(asked) != 0 {
		t.Fatalf("extracted %d times, asked for %v", n, asked)
	}
}

// When a file's subtitle extraction failed for good, a download fails at
// once if it needs its own subtitle. If the failed track is another track
// it delivers as a file, the download goes without that track and doesn't
// read the file again.
func TestADownloadOfUnextractableSubtitlesDoesntWait(t *testing.T) {
	f := newFixture(t, 0)
	f.downloads()
	m, sub := f.subtitled()
	f.extractions.set(&media.Error{Op: "x", Kind: media.ErrUnsupported}, nil)
	if err := f.subtitles.Prepare(ctx, m.File); !errors.Is(err, subtitles.ErrUnextractable) {
		t.Fatalf("prepare: %v", err)
	}
	f.extractions.set(nil, nil)

	start := time.Now()
	d, err := f.dl.Request(ctx, f.p, f.v, DownloadRequest{Work: m.Work, SubtitleTrack: &sub, Caps: iPhone})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.await(d.ID, DownloadFailed); got.Reason != RefuseSubtitlesUnavailable {
		t.Fatalf("its own subtitle unextractable: %+v", got)
	}

	off := id.ID{}
	d, err = f.dl.Request(ctx, f.p, f.v, DownloadRequest{Work: m.Work, SubtitleTrack: &off, Caps: iPhone})
	if err != nil {
		t.Fatal(err)
	}
	got := f.await(d.ID, DownloadReady)
	if i := slices.IndexFunc(got.ExternalSubtitles, func(s ExternalSubtitle) bool { return s.Track == sub }); i < 0 || got.ExternalSubtitles[i].Ready {
		t.Fatalf("another track unextractable: %+v", got.ExternalSubtitles)
	}
	if n := f.extractions.count(); n != 1 || time.Since(start) > 3*time.Second {
		t.Fatalf("read %d times, took %s", n, time.Since(start))
	}
}

// A download that uses a downloaded subtitle moves to the new subtitle
// when another one replaces it.
func TestADownloadMovesToTheSubtitleThatReplacedItsOwn(t *testing.T) {
	f := newFixture(t, 0)
	f.downloads()
	m := f.b.Movie(f.folder, "Heat", 1995)
	f.probeWith(m.File, "mp4", 2*time.Hour, 1_000_000,
		media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8},
		media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Default: true},
	)
	add := func(rel string) (got id.ID) {
		f.b.Write(func(ctx context.Context, tx *store.Tx) error {
			var err error
			if got, err = f.b.Catalog.AddDownloadedSubtitle(ctx, tx, m.File, catalog.SubtitleFile{RelPath: rel, Format: "srt", Language: "en"}); err != nil {
				return err
			}
			_, err = f.b.Catalog.DropReplacedSubtitles(ctx, tx, m.File, got)
			return err
		})
		return got
	}
	old := add("x/en.1.srt")
	d, err := f.dl.Request(ctx, f.p, f.v, DownloadRequest{Work: m.Work, SubtitleTrack: &old, Caps: iPhone})
	if err != nil || selectedOf(d.Decision, media.Subtitle) != old {
		t.Fatalf("the download %+v %v", d, err)
	}
	replacement := add("x/en.2.srt")
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return f.dl.SubtitlesReplaced(ctx, tx, []id.ID{old}, replacement)
	})
	d, err = f.dl.Status(ctx, f.p, d.ID)
	if err != nil || selectedOf(d.Decision, media.Subtitle) != replacement || !slices.ContainsFunc(d.ExternalSubtitles, func(s ExternalSubtitle) bool { return s.Track == replacement }) {
		t.Fatalf("after the replacement %+v %v", d, err)
	}
}

// A subtitle replaced while its download converts is the one the download
// names once it's ready.
func TestADownloadConvertingMovesToTheSubtitleThatReplacedItsOwn(t *testing.T) {
	f := newFixture(t, 0)
	f.media.hold = make(chan struct{})
	f.downloads()
	m := f.movie("Heat", "mkv", 1)
	add := func(rel string) (got id.ID) {
		f.b.Write(func(ctx context.Context, tx *store.Tx) error {
			var err error
			if got, err = f.b.Catalog.AddDownloadedSubtitle(ctx, tx, m.File, catalog.SubtitleFile{RelPath: rel, Format: "srt", Language: "en"}); err != nil {
				return err
			}
			_, err = f.b.Catalog.DropReplacedSubtitles(ctx, tx, m.File, got)
			return err
		})
		return got
	}
	old := add("x/en.1.srt")
	d, err := f.dl.Request(ctx, f.p, f.v, DownloadRequest{Work: m.Work, SubtitleTrack: &old, Caps: iPhone})
	if err != nil || d.Decision.Delivery != DeliverConversion || selectedOf(d.Decision, media.Subtitle) != old {
		t.Fatalf("the download %+v %v", d, err)
	}
	for len(f.converts()) == 0 {
		time.Sleep(time.Millisecond)
	}
	replacement := add("x/en.2.srt")
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return f.dl.SubtitlesReplaced(ctx, tx, []id.ID{old}, replacement)
	})
	close(f.media.hold)
	d = f.await(d.ID, DownloadReady)
	if selectedOf(d.Decision, media.Subtitle) != replacement || !slices.ContainsFunc(d.ExternalSubtitles, func(s ExternalSubtitle) bool { return s.Track == replacement }) {
		t.Fatalf("ready after the replacement: subtitle %v, want %v; files %+v", selectedOf(d.Decision, media.Subtitle), replacement, d.ExternalSubtitles)
	}
}

func TestAskingForTheSameDownloadAtOnceKeepsOne(t *testing.T) {
	f := newFixture(t, 0)
	f.downloads()
	book := f.b.Work(catalog.WorkInput{Kind: catalog.KindBook, Title: "Dune"})
	file := f.b.File(f.folderOf(catalog.Audiobooks), "Dune/Dune.m4b")
	f.b.Version([]id.ID{file}, book)
	f.probeWith(file, "mp4", 20*time.Hour, 64_000, media.Track{Index: 0, Kind: media.Audio, Codec: "aac", Channels: 2, Default: true})
	f.onDisk(file, "the book")

	// Taps that reach the server together, before any has its answer: the
	// database is busy writing while they arrive, so each looks for the
	// download before any has stored it.
	const taps = 3
	var mu sync.Mutex
	ids := map[id.ID]bool{}
	var wg sync.WaitGroup
	if err := f.b.DB.Write(ctx, func(context.Context, *store.Tx) error {
		for range taps {
			wg.Go(func() {
				d, err := f.download(book, "")
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				ids[d.ID] = true
				mu.Unlock()
			})
		}
		time.Sleep(200 * time.Millisecond)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if len(ids) != 1 {
		t.Fatalf("%d taps made %d downloads", taps, len(ids))
	}
}

func TestDownloadRefusals(t *testing.T) {
	f := newFixture(t, 0)
	f.downloads()
	book := f.b.Work(catalog.WorkInput{Kind: catalog.KindBook, Title: "Dune"})
	file := f.b.File(f.folderOf(catalog.Audiobooks), "Dune/Dune.flac")
	f.b.Version([]id.ID{file}, book)
	f.probeWith(file, "flac", 20*time.Hour, 900_000, media.Track{Index: 0, Kind: media.Audio, Codec: "flac", Channels: 2, Default: true})
	_, err := f.download(book, "")
	var refused *RefusedError
	if !errors.As(err, &refused) || !errors.Is(err, ErrNotPlayable) || refused.Reason != RefuseFileUnplayable ||
		refused.Unmet == nil || *refused.Unmet != (Unmet{Field: "codec", Limit: "aac,ac3,eac3", Actual: "flac"}) {
		t.Fatalf("a book AVPlayer can't play: %v %+v", err, refused.Unmet)
	}
	d, err := f.dl.Request(ctx, f.p, f.v, DownloadRequest{Work: book, Caps: alike([]string{"flac"}, false, nil, []AudioEntry{{Codec: "flac", MaxChannels: 2}}), Quality: "standard"})
	if err != nil || d.Decision.Delivery != DeliverFile || d.State != DownloadReady || d.Quality != "original" {
		t.Fatalf("a book the device plays is its original: %+v %v", d, err)
	}

	f.exec("UPDATE user SET downloads_allowed = 0")
	_, err = f.download(f.movie("Heat", "mp4", 1).Work, "")
	if !errors.As(err, &refused) || refused.Reason != RefuseDownloadsNotAllowed {
		t.Fatalf("a user without downloads: %v", err)
	}
	if _, err := f.dl.File(ctx, f.viewer(), d.ID, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a download's file once downloads are off: %v", err)
	}
}

func TestEstimates(t *testing.T) {
	f := newFixture(t, 0)
	f.downloads()
	// 2 hours at 5 Mbps is 4.5 GB.
	heat := f.movie("Heat", "mp4", 5_000_000)
	f.sized(heat.File, 4_500_000_000)
	book := f.b.Work(catalog.WorkInput{Kind: catalog.KindBook, Title: "Dune"})
	bookFile := f.b.File(f.folderOf(catalog.Audiobooks), "Dune/Dune.m4b")
	f.b.Version([]id.ID{bookFile}, book)
	f.probeWith(bookFile, "mp4", 20*time.Hour, 64_000, media.Track{Index: 0, Kind: media.Audio, Codec: "aac", Channels: 2, Default: true})
	f.sized(bookFile, 576_000_000)
	gone := id.New()

	got, err := f.dl.Estimate(ctx, f.p, f.v, iPhone, Settings{}, []EstimateRequest{{Work: heat.Work}, {Work: book}, {Work: gone}})
	if err != nil {
		t.Fatal(err)
	}
	stereo := int64(2*time.Hour/time.Second) * 128_000 / 8
	want := []EstimateOption{
		{Quality: "original", Size: 4_500_000_000, AsIs: true, Delivery: DeliverFile},
		{Quality: "high", Size: 4_500_000_000, AsIs: true, Delivery: DeliverFile},
		{Quality: "standard", Size: int64(2*time.Hour/time.Second)*3_000_000/8 + stereo, Delivery: DeliverConversion},
	}
	if e := got[0]; !e.Playable || e.Version != heat.Version || !slices.Equal(e.Options, want) {
		t.Fatalf("a movie:\n got %+v\nwant %+v", e, want)
	}
	if e := got[1]; !e.Playable || !slices.Equal(e.Options, []EstimateOption{{Quality: "original", Size: 576_000_000, AsIs: true, Delivery: DeliverFile}}) {
		t.Fatalf("a book: %+v", e)
	}
	if e := got[2]; e.Playable || len(e.Options) != 0 {
		t.Fatalf("a work that isn't there: %+v", e)
	}

	// The Standard estimate is what a Standard download says before it's
	// ready: the most it can come to.
	d := f.mustDownload(heat.Work, "standard")
	if d.Decision.Delivery != DeliverConversion || d.Size != want[2].Size || d.SizeExact {
		t.Fatalf("a Standard download: %+v", d)
	}
}

func TestAConversionOnABusyAcceleratorRunsOnTheCPU(t *testing.T) {
	f := newFixture(t, 0)
	f.accel.caps = media.Capabilities{Tested: true, Accelerators: []media.Caps{qsv}, CPU: cpu}
	f.media.busy[qsv.Accelerator] = true
	f.setting(func(s *config.Settings) { s.VideoTranscodeLimit = 1 })
	f.downloads()
	d := f.mustDownload(f.hdrMovie("Dune").Work, "high")
	if v, _ := d.Decision.Selected("video"); v.Method() != Transcode || v.Video.Codec != media.HEVC || v.Video.Bitrate != 8_000_000 {
		t.Fatalf("decided %+v", v)
	}
	d = f.await(d.ID, DownloadReady)
	f.media.mu.Lock()
	first, retry := f.media.converts[0], f.media.converts[len(f.media.converts)-1]
	f.media.mu.Unlock()
	if first.Accelerator != qsv.Accelerator || retry.Accelerator != "" || retry.Video.Codec != media.H264 {
		t.Fatalf("on the GPU %+v, then %+v", first, retry)
	}
	// Once ready, it reports the codec and encoder its file was made with.
	if v, _ := d.Decision.Selected("video"); v.Codec != media.H264 || v.Video.Codec != media.H264 || v.Encoder != "CPU" {
		t.Fatalf("ready, it reports %s by %q, want the H.264 the CPU encoded", v.Codec, v.Encoder)
	}
	// A conversion doesn't count as one of the server's video transcodes.
	if _, err := f.start(f.hdrMovie("Arrival").Work, lan); err != nil {
		t.Fatalf("a stream beside a converted download: %v", err)
	}
}

func TestAvailability(t *testing.T) {
	f := newFixture(t, 0)
	heat, ronin := f.movie("Heat", "mp4", 1), f.movie("Ronin", "mp4", 1)
	check := func(v users.Viewer, want ...bool) {
		t.Helper()
		got, err := f.dl.Available(ctx, v, []KeptVersion{
			{heat.Work, heat.Version},
			{heat.Work, ronin.Version}, // a version of another work
			{ronin.Work, id.New()},     // a deleted version
		})
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("available %v %v, want %v", got, err, want)
		}
	}
	check(f.v, true, false, false)

	other, _ := f.b.Library(catalog.Movies)
	if err := f.users.Update(ctx, id.ID{}, f.p.User, users.Change{Grants: &users.Grants{Libraries: []id.ID{other}}}); err != nil {
		t.Fatal(err)
	}
	check(f.viewer(), false, false, false)
	if err := f.users.Update(ctx, id.ID{}, f.p.User, users.Change{Grants: &users.Grants{All: true}}); err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE user SET downloads_allowed = 0")
	check(f.viewer(), false, false, false)
}

// selectedOf is the track of a kind a decision selected, or the zero ID.
func selectedOf(d Decision, kind media.TrackKind) id.ID {
	t, _ := d.Selected(kind)
	return t.Track
}

func TestDownloadsPrepareTwoAtOnceOnAnAccelerator(t *testing.T) {
	f := newFixture(t, 0)
	if n := f.dl.preparingAtOnce(config.Settings{}); n != 1 {
		t.Fatalf("on the CPU: %d", n)
	}
	f.accel.caps = media.Capabilities{Tested: true, Accelerators: []media.Caps{qsv}, CPU: cpu}
	if n := f.dl.preparingAtOnce(config.Settings{}); n != 2 {
		t.Fatalf("on an accelerator: %d", n)
	}
	f.setting(func(s *config.Settings) { s.Accelerator = config.AcceleratorNone })
	if n := f.dl.preparingAtOnce(config.Settings{}); n != 1 {
		t.Fatalf("with acceleration off: %d", n)
	}
}

// The Tasks page counts the downloads prepared since none were waiting,
// out of those and the ones asked for, and shows a single download's step
// without a count. After a restart, the downloads still waiting count once
// they start.
func TestTheTasksPageCountsDownloadsPrepared(t *testing.T) {
	f := newFixture(t, 0)
	f.media.hold = make(chan struct{})
	r := tasks.New(f.b.DB, f.cfg, events.NewBus(), tasks.Options{})
	f.svc, f.runner = New(f.b.DB, f.deps(r)), r
	f.dl = NewDownloads(f.svc)
	for _, task := range f.dl.Tasks() {
		r.Add(task)
	}
	// All three are asked for before the runner starts, so they're in
	// line when the first is prepared.
	var last DownloadStatus
	for _, title := range []string{"Heat", "Ronin", "Thief"} {
		last = f.mustDownload(f.movie(title, "mkv", 1).Work, "high")
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { _ = r.Run(runCtx); close(done) }()
	stop := func() { cancel(); <-done }

	type report struct {
		Running     bool
		Step        string
		Done, Total int
	}
	status := func() tasks.Status {
		all, err := f.runner.Tasks(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range all {
			if s.Name == TaskPrepareDownloads {
				return s
			}
		}
		t.Fatal("no prepare task")
		return tasks.Status{}
	}
	// at waits until the nth conversion has begun, or until the task is
	// idle when n is 0, then checks what the task reports.
	at := func(n int, want report) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
			f.media.mu.Lock()
			converts := len(f.media.converts)
			f.media.mu.Unlock()
			s := status()
			if n == 0 && !s.Running || n > 0 && converts == n && s.Step == StepConvert {
				got := report{s.Running, s.Step, s.Done, s.Total}
				t.Logf("conversion %d: %+v, %d pending", n, got, s.PendingCount)
				if got != want {
					t.Errorf("conversion %d: %+v, want %+v", n, got, want)
				}
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("waiting for conversion %d: %+v", n, s)
			}
		}
	}
	// next lets the conversion in flight finish and holds the next one.
	next := func() {
		f.media.mu.Lock()
		was := f.media.hold
		f.media.hold = make(chan struct{})
		f.media.mu.Unlock()
		close(was)
	}

	at(1, report{true, StepConvert, 0, 3})
	next()
	at(2, report{true, StepConvert, 1, 3})
	next()
	at(3, report{true, StepConvert, 2, 3})
	next()
	f.await(last.ID, DownloadReady)
	at(0, report{})

	f.mustDownload(f.movie("Sicario", "mkv", 1).Work, "high")
	at(4, report{true, StepConvert, 0, 0})
	next()
	at(0, report{})

	// The server stops while one download converts and another waits.
	first := f.mustDownload(f.movie("Collateral", "mkv", 1).Work, "high")
	f.mustDownload(f.movie("Insider", "mkv", 1).Work, "high")
	f.await(first.ID, DownloadPreparing)
	at(5, report{true, StepConvert, 0, 2})
	stop()
	f.downloads()
	at(6, report{true, StepConvert, 0, 0})
	next()
	at(7, report{true, StepConvert, 1, 2})
	next()
	at(0, report{})
}
