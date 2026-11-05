package playback

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"iter"
	"log/slog"
	"math"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/previews"
	"github.com/mmcdole/kivo/internal/subtitles"
	"github.com/mmcdole/kivo/internal/tasks"
	"github.com/mmcdole/kivo/internal/transcode"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
)

var ctx = context.Background()

var (
	lan    = netip.MustParseAddr("192.168.1.20")
	remote = netip.MustParseAddr("203.0.113.5")
)

// every2s is a complete keyframe index with a keyframe every 2 seconds.
type every2s struct{}

func (every2s) Keyframes(context.Context, string) ([]media.Keyframe, bool, error) {
	var out []media.Keyframe
	for t := time.Duration(0); t < 3*time.Hour; t += 2 * time.Second {
		out = append(out, media.Keyframe{Time: t})
	}
	return out, true, nil
}

// instantMedia yields every fragment at once: one per 2 s keyframe for
// copied video, and one per grid step for encoded video. The transcode
// package's own tests cover its job mechanics. An accelerator listed in
// busy fails as its job starts. With failAfter set, every encoded job
// fails after that many fragments.
type instantMedia struct {
	mu        sync.Mutex
	busy      map[media.Accelerator]bool
	fail      error // when set, every job fails as it starts, with this kind of error
	failAfter int
	requests  []media.TranscodeRequest
	converts  []media.ConvertRequest
	hold      chan struct{} // conversions wait until it's closed
}

func (m *instantMedia) Transcode(ctx context.Context, req media.TranscodeRequest) (media.Output, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, req)
	if m.busy[req.Accelerator] {
		return nil, &media.Error{Op: "transcode", Kind: media.ErrEncoderBusy}
	}
	if m.fail != nil {
		return nil, &media.Error{Op: "transcode", Kind: m.fail}
	}
	o := &instantOutput{start: req.Start, step: 2 * time.Second}
	if req.Video.Codec != media.Copy {
		o.step, o.failAfter = req.SegmentDuration, m.failAfter
	}
	return o, nil
}

// Convert writes the source's name and the video's codec once hold is
// closed. It fails the same way a transcode would.
func (m *instantMedia) Convert(ctx context.Context, req media.ConvertRequest) error {
	m.mu.Lock()
	m.converts = append(m.converts, req)
	busy, fail, hold := m.busy[req.Accelerator], m.fail, m.hold
	m.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return &media.Error{Op: "convert", Kind: media.ErrCancelled}
		}
	}
	if busy {
		return &media.Error{Op: "convert", Kind: media.ErrEncoderBusy}
	}
	if fail != nil {
		return &media.Error{Op: "convert", Kind: fail}
	}
	req.Reached(time.Hour)
	return os.WriteFile(req.Dst, []byte(req.Path+" as "+string(req.Video.Codec)), 0o644)
}

func (m *instantMedia) last() media.TranscodeRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requests[len(m.requests)-1]
}

type instantOutput struct {
	start, step time.Duration
	failAfter   int
}

func (o *instantOutput) Init() ([]byte, error) { return []byte("init"), nil }
func (o *instantOutput) Segments() iter.Seq2[media.Segment, error] {
	return func(yield func(media.Segment, error) bool) {
		n := 0
		for t := o.start; t < 2*time.Hour; t += o.step {
			if n++; o.failAfter > 0 && n > o.failAfter {
				yield(media.Segment{}, &media.Error{Op: "transcode", Kind: media.ErrEncoderFailed})
				return
			}
			if !yield(media.Segment{Start: t, Data: []byte("x")}, nil) {
				return
			}
		}
	}
}
func (o *instantOutput) Cues(int) ([]media.Cue, error)  { return nil, nil }
func (o *instantOutput) Pause() error                   { return nil }
func (o *instantOutput) Resume() error                  { return nil }
func (o *instantOutput) Diagnostics() media.Diagnostics { return media.Diagnostics{} }
func (o *instantOutput) Close() error                   { return nil }

// qsv is an Intel GPU, as the accelerator test reports it. It encodes
// H.264 and HEVC, and tone-maps and draws subtitles on the accelerator.
var qsv = media.Caps{
	Accelerator: "qsv:/dev/dri/renderD128", Backend: media.QSV, Name: "Intel UHD Graphics 630",
	Encoders: []media.EncoderCaps{{Codec: media.H264, GOP: media.GridGOP}, {Codec: media.HEVC, GOP: media.FixedGOP}},
	Decoders: []media.VideoFormat{{Codec: media.H264, BitDepth: 8}, {Codec: media.HEVC, BitDepth: 10}},
	ToneMap:  media.OnAccelerator, BurnIn: media.OnAccelerator,
}

var cpu = media.Caps{Backend: media.Software, Name: "CPU", Encoders: []media.EncoderCaps{{Codec: media.H264, GOP: media.GridGOP}}, ToneMap: media.OnCPU, BurnIn: media.OnCPU}

type fakeAccelerators struct{ caps media.Capabilities }

func (h *fakeAccelerators) Capabilities() media.Capabilities { return h.caps }

// noMedia is the media for markers, previews and subtitles. The tasks they
// ask for are dropped, so it never reads a file.
type noMedia struct{}

func (noMedia) Sample(context.Context, media.SampleRequest) (media.Sample, error) {
	return media.Sample{}, errors.ErrUnsupported
}
func (noMedia) Chromaprint(context.Context) bool { return false }
func (noMedia) Capabilities() media.Capabilities { return media.Capabilities{} }
func (noMedia) Previews(context.Context, media.PreviewsRequest) (media.Previews, error) {
	return media.Previews{}, errors.ErrUnsupported
}
func (noMedia) Subtitles(context.Context, media.SubtitlesRequest) error { return errors.ErrUnsupported }

// extractMedia is the subtitles package's media. It writes each requested
// track as ffmpeg would, or fails with fail, and counts the reads. While
// hold is set, a read waits for it to close.
type extractMedia struct {
	mu    sync.Mutex
	fail  error
	hold  chan struct{}
	reads int
}

func (m *extractMedia) Subtitles(_ context.Context, req media.SubtitlesRequest) error {
	m.mu.Lock()
	m.reads++
	fail, hold := m.fail, m.hold
	m.mu.Unlock()
	if hold != nil {
		<-hold
	}
	if fail != nil {
		return fail
	}
	for _, t := range req.Tracks {
		body := map[media.SubtitleFormat]string{
			media.SRT: "1\n00:00:01,000 --> 00:00:02,000\nHello\n",
			media.ASS: "[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Hi\n",
		}[t.Format]
		if err := os.WriteFile(t.Path, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (m *extractMedia) set(fail error, hold chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fail, m.hold = fail, hold
}

func (m *extractMedia) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reads
}

type fixture struct {
	t        *testing.T
	media    *instantMedia
	accel    *fakeAccelerators
	b        *catalogtest.Builder
	users    *users.Service
	activity *activity.Service
	auth     *auth.Service
	state    *userstate.Service
	cfg      *config.Store
	tc       *transcode.Transcoder
	// markers gives plays their chapters and markers, and drops the tasks
	// a play asks for.
	markers *markers.Service
	// subtitles and previews give plays what they prepared and made. That's
	// nothing, unless a test puts it in cache with extract or makePreviews,
	// or a download has the subtitles package extract it through
	// extractions.
	subtitles   *subtitles.Service
	extractions *extractMedia
	previews    *previews.Service
	requested   *requests     // the files the subtitles service was asked to prepare
	runner      *tasks.Runner // the runner runTasks uses
	cache       string        // the cache/ directory of subtitles and previews
	tcDir       string        // where the transcoder keeps each play's segments
	subDir      string        // where downloaded subtitles are kept
	svc         *Service
	bus         *events.Bus // the service's event bus
	sub         *events.Sub // receives every event on bus
	p           auth.Caller
	v           users.Viewer
	lib         id.ID
	folder      id.ID
}

// deps returns the fixture's services, with tasks to prepare downloads.
func (f *fixture) deps(t Tasks) Deps {
	return Deps{
		Catalog: f.b.Catalog, Users: f.users, Auth: f.auth, Userstate: f.state, Transcoder: f.tc, Settings: f.cfg, Markers: f.markers, Tasks: t,
		Activity: f.activity, Subtitles: f.subtitles, Previews: f.previews, Accelerators: f.accel, Events: f.bus,
	}
}

// published returns the events of the given kind published since the
// last call, and drops the others.
func (f *fixture) published(kind events.Kind) []events.Event {
	var out []events.Event
	for len(f.sub.C()) > 0 {
		if e := <-f.sub.C(); e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func newFixture(t *testing.T, floor uint64) *fixture {
	f := &fixture{t: t, b: catalogtest.New(t), media: &instantMedia{busy: map[media.Accelerator]bool{}}, accel: &fakeAccelerators{}, requested: &requests{}, bus: events.NewBus()}
	f.sub = f.bus.Subscribe(func(events.Scope) bool { return true })
	t.Cleanup(f.sub.Close)
	f.users = users.New(f.b.DB)
	var err error
	if f.cfg, err = config.Open(ctx, f.b.DB, config.Options{}); err != nil {
		t.Fatal(err)
	}
	f.activity = activity.New(f.b.DB, events.NewBus())
	f.auth = auth.New(f.b.DB, auth.Deps{Users: f.users, Settings: f.cfg, Activity: f.activity})
	iss, err := f.auth.Setup(ctx, auth.SignInRequest{Username: "admin", Password: "correct horse battery", RemoteIP: lan, Kind: auth.SessionBrowser})
	if err != nil {
		t.Fatal(err)
	}
	f.p = iss.Session.Caller()
	f.v = f.viewer()
	f.state = userstate.New(f.b.DB, userstate.Deps{Catalog: f.b.Catalog, Settings: f.cfg, Activity: f.activity, Events: events.NewBus()})
	f.tcDir = filepath.Join(t.TempDir(), "tc")
	if f.tc, err = transcode.New(transcode.Deps{Media: f.media, Keys: every2s{}, Dir: f.tcDir, Downloads: filepath.Join(t.TempDir(), "downloads")}, transcode.Options{Floor: floor}); err != nil {
		t.Fatal(err)
	}
	f.markers = markers.New(f.b.DB, markers.Deps{Catalog: f.b.Catalog, Media: noMedia{}, Tasks: discard{}, Settings: f.cfg, CacheDir: t.TempDir()})
	f.cache, f.subDir, f.extractions = t.TempDir(), t.TempDir(), &extractMedia{}
	f.subtitles = subtitles.New(f.b.DB, subtitles.Deps{
		Catalog: f.b.Catalog, Settings: f.cfg, Media: f.extractions, Tasks: f.requested, CacheDir: f.cache, SubtitleDir: f.subDir,
	}, subtitles.Options{})
	f.previews = previews.New(f.b.DB, previews.Deps{Catalog: f.b.Catalog, Media: noMedia{}, Tasks: discard{}, CacheDir: f.cache})
	f.svc = New(f.b.DB, f.deps(discard{}))
	t.Cleanup(func() { f.svc.endAll(ctx) }) // end every play, as shutdown does
	f.lib, f.folder = f.b.Library(catalog.Movies)
	return f
}

func (f *fixture) viewer() users.Viewer {
	v, err := f.users.Viewer(ctx, f.p.User)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

// movie adds a two-hour movie in container ("mkv" or "mp4") with H.264 and
// AAC, at the given bitrate.
func (f *fixture) movie(title, container string, bitrate int) catalogtest.Item {
	f.t.Helper()
	m := f.b.Movie(f.folder, title, 2000)
	f.probe(m.File, container, 2*time.Hour, bitrate)
	return m
}

func (f *fixture) probe(file id.ID, container string, dur time.Duration, bitrate int) {
	f.t.Helper()
	names := map[string]string{"mkv": "matroska,webm", "mp4": "mov,mp4,m4a,3gp,3g2,mj2"}
	track := func(t media.Track) catalog.Track {
		info, _ := json.Marshal(t)
		return catalog.Track{Index: t.Index, Type: string(t.Kind), Codec: string(t.Codec), Default: t.Default, Info: info}
	}
	cur, err := f.b.Catalog.File(ctx, file)
	if err != nil {
		f.t.Fatal(err)
	}
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return f.b.Catalog.SetProbe(ctx, tx, file, cur.Generation, catalog.Probe{Summary: catalog.ProbeSummary{Container: names[container], Duration: dur, Bitrate: bitrate}, Tracks: []catalog.Track{
			track(media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8}),
			track(media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Default: true}),
		}})
	})
}

func (f *fixture) start(work id.ID, ip netip.Addr) (Started, error) {
	return f.svc.Start(ctx, f.p, f.v, StartRequest{Work: work, Caps: browser, ClientIP: ip})
}

func (f *fixture) mustStart(work id.ID) Started {
	f.t.Helper()
	s, err := f.start(work, lan)
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

// audioOf is the audio track a play selected, or the zero ID.
func audioOf(s Started) id.ID {
	t, _ := s.Decision.Selected(media.Audio)
	return t.Track
}

// subtitleOf is the subtitle track a play selected, or the zero ID for
// off.
func subtitleOf(s Started) id.ID {
	t, _ := s.Decision.Selected(media.Subtitle)
	return t.Track
}

// choiceOf is how a play shows one of its choices: a track, or the zero ID
// for subtitles off.
func choiceOf(s Started, track id.ID) TrackDecision {
	i := slices.IndexFunc(s.Decision.Tracks, func(t TrackDecision) bool { return t.Track == track && (!track.IsZero() || t.Type == media.Subtitle) })
	if i < 0 {
		return TrackDecision{}
	}
	return s.Decision.Tracks[i]
}

// externalOf is a play's external subtitle file for a track, if it has
// one.
func externalOf(s Started, track id.ID) (ExternalSubtitle, bool) {
	i := slices.IndexFunc(s.ExternalSubtitles, func(d ExternalSubtitle) bool { return d.Track == track })
	if i < 0 {
		return ExternalSubtitle{}, false
	}
	return s.ExternalSubtitles[i], true
}

func (f *fixture) exec(q string, args ...any) {
	f.t.Helper()
	f.b.Write(func(ctx context.Context, tx *store.Tx) error { _, err := tx.ExecContext(ctx, q, args...); return err })
}

func TestDirectPlayAndRemux(t *testing.T) {
	f := newFixture(t, 0)
	mp4 := f.movie("Heat", "mp4", 8_000_000)
	s := f.mustStart(mp4.Work)
	if s.Decision.Delivery != DeliverFile || s.Decision.Summary() != DirectPlay || s.Duration != 2*time.Hour {
		t.Fatalf("mp4: %+v", s)
	}
	if m, err := f.svc.File(ctx, s.ID, 0); err != nil || filepath.Base(m.Path) == "" || m.Type != "video/mp4" {
		t.Fatalf("file: %+v %v", m, err)
	}

	mkv := f.movie("Ronin", "mkv", 8_000_000)
	s = f.mustStart(mkv.Work)
	if s.Decision.Delivery != DeliverStream || s.Decision.Summary() != DirectStream {
		t.Fatalf("mkv: %+v", s.Decision)
	}
	pl, err := f.svc.HLS(ctx, s.ID, "main.m3u8")
	if err != nil || len(pl.Data) == 0 || pl.Type != "application/vnd.apple.mpegurl" {
		t.Fatalf("playlist: %v", err)
	}
	if _, err := f.svc.HLS(ctx, s.ID, "3.m4s"); err != nil {
		t.Fatalf("segment: %v", err)
	}
	if _, err := f.svc.HLS(ctx, s.ID, "../etc/passwd.m3u8"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad name: %v", err)
	}
}

func TestStartResumesAndStopSaves(t *testing.T) {
	f := newFixture(t, 0)
	m := f.movie("Heat", "mp4", 8_000_000)
	s := f.mustStart(m.Work)
	if err := f.svc.Heartbeat(ctx, f.p, s.ID, Heartbeat{Position: 30 * time.Minute}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Stop(ctx, f.p, f.v, s.ID, StopRequest{Position: ptr(41 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if s2 := f.mustStart(m.Work); s2.Start != 41*time.Minute {
		t.Fatalf("resumed at %v", s2.Start)
	}
}

func ptr[T any](v T) *T { return &v }

func TestAnAdminEndsAPlayWhereItWas(t *testing.T) {
	f := newFixture(t, 0)
	m := f.movie("Heat", "mp4", 8_000_000)
	s := f.mustStart(m.Work)
	if err := f.svc.Heartbeat(ctx, f.p, s.ID, Heartbeat{Position: 30 * time.Minute}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.endAny(ctx, s.ID, "Server maintenance"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.endAny(ctx, s.ID, ""); !errors.Is(err, ErrNoPlay) {
		t.Fatalf("ending it again: %v", err)
	}
	if ended := f.published(events.PlayEnded); len(ended) != 1 || ended[0].Scope.User != f.v.User() ||
		ended[0].Data != (Ended{Play: s.ID, Message: "Server maintenance"}) {
		t.Fatalf("ended %+v", ended)
	}
	// The owner's position stands, and a stop without a position keeps it
	// too.
	s = f.mustStart(m.Work)
	if s.Start != 30*time.Minute {
		t.Fatalf("resumed at %v", s.Start)
	}
	if err := f.svc.Stop(ctx, f.p, f.v, s.ID, StopRequest{}); err != nil || f.mustStart(m.Work).Start != 30*time.Minute {
		t.Fatalf("a stop without a position: %v", err)
	}
}

func TestAHeartbeatThatFailsToWriteKeepsThePlay(t *testing.T) {
	f := newFixture(t, 0)
	m := f.movie("Heat", "mp4", 8_000_000)
	s := f.mustStart(m.Work)
	failed, cancel := context.WithCancel(ctx)
	cancel()
	if err := f.svc.Heartbeat(failed, f.p, s.ID, Heartbeat{Position: 10 * time.Minute}); err == nil {
		t.Fatal("a heartbeat wrote with a cancelled context")
	}
	if err := f.svc.Heartbeat(ctx, f.p, s.ID, Heartbeat{Position: 10*time.Minute + 10*time.Second}); err != nil {
		t.Fatalf("the next heartbeat: %v", err)
	}
	if pos, _ := f.state.Resume(ctx, f.p.User, m.Work, m.Version); pos != 10*time.Minute+10*time.Second {
		t.Fatalf("position %v after the next heartbeat", pos)
	}
}

func TestAHeartbeatRacingAStopIsRefused(t *testing.T) {
	f := newFixture(t, 0)
	m := f.movie("Heat", "mp4", 8_000_000)
	s := f.mustStart(m.Work)
	// A heartbeat finds the play, and then the stop ends it before the
	// heartbeat reports.
	pl, err := f.svc.owned(f.p, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Stop(ctx, f.p, f.v, s.ID, StopRequest{Position: ptr(115 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.heartbeat(ctx, pl, Heartbeat{Position: 116 * time.Minute}); !errors.Is(err, ErrNoPlay) {
		t.Fatalf("late heartbeat: %v", err)
	}
	var count int
	if err := f.b.DB.Reader().QueryRow(`SELECT play_count FROM user_state WHERE user_id = ? AND work_id = ?`, f.p.User, m.Work).Scan(&count); err != nil || count != 1 {
		t.Fatalf("play count %d, %v; want 1", count, err)
	}
}

// The plays page is told about a start, each heartbeat, and the end.
func TestChangedHearsWhatNowPlayingShows(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, 0)
		told := func(what string) {
			t.Helper()
			time.Sleep(time.Second) // the interval between playsChanged events
			synctest.Wait()
			if got := f.published(events.PlaysChanged); len(got) != 1 || !got[0].Scope.AdminOnly {
				t.Fatalf("%s: told %+v; want playsChanged once", what, got)
			}
		}
		s := f.mustStart(f.movie("Heat", "mp4", 1).Work)
		told("a start")
		if err := f.svc.Heartbeat(ctx, f.p, s.ID, Heartbeat{Position: time.Minute, State: StatePaused}); err != nil {
			t.Fatal(err)
		}
		told("a heartbeat")
		if err := f.svc.Stop(ctx, f.p, f.v, s.ID, StopRequest{}); err != nil {
			t.Fatal(err)
		}
		told("the end")
	})
}

func TestOnlyTheOwnerControlsAPlay(t *testing.T) {
	f := newFixture(t, 0)
	s := f.mustStart(f.movie("Heat", "mp4", 1).Work)
	other := auth.Caller{User: id.New(), Session: id.New()}
	if err := f.svc.Heartbeat(ctx, other, s.ID, Heartbeat{}); !errors.Is(err, ErrNoPlay) {
		t.Fatalf("heartbeat: %v", err)
	}
	if err := f.svc.Stop(ctx, other, users.Viewer{}, s.ID, StopRequest{}); !errors.Is(err, ErrNoPlay) {
		t.Fatalf("stop: %v", err)
	}
	if _, err := f.svc.Fallback(ctx, other, users.Viewer{}, s.ID, FailReport{}); !errors.Is(err, ErrNoPlay) {
		t.Fatalf("fallback: %v", err)
	}
	if err := f.svc.Heartbeat(ctx, f.p, s.ID, Heartbeat{}); err != nil {
		t.Fatalf("the owner: %v", err)
	}
}

func TestUnknownAndExpiredAreTheSame(t *testing.T) {
	f := newFixture(t, 0)
	s := f.mustStart(f.movie("Heat", "mp4", 1).Work)
	f.svc.mu.Lock()
	f.svc.plays[s.ID].seen = time.Now().Add(-playTimeout - time.Second)
	f.svc.mu.Unlock()
	_, expired := f.svc.File(ctx, s.ID, 0)
	_, unknown := f.svc.File(ctx, id.New(), 0)
	if !errors.Is(expired, ErrNoPlay) || expired.Error() != unknown.Error() {
		t.Fatalf("expired %v, unknown %v", expired, unknown)
	}
	f.svc.endExpired(ctx)
	if len(f.svc.Plays()) != 0 {
		t.Fatal("endExpired kept an expired play")
	}
}

func TestRevocationAppliesAtTheNextRequest(t *testing.T) {
	for name, revoke := range map[string]func(f *fixture){
		"session signed out": func(f *fixture) {
			if err := f.auth.SignOut(ctx, f.p); err != nil {
				f.t.Fatal(err)
			}
		},
		"owner disabled": func(f *fixture) {
			f.b.Write(func(ctx context.Context, tx *store.Tx) error { // a second admin, so the first can be disabled
				_, err := f.users.CreateUser(ctx, tx, users.NewUser{Username: "other", Admin: true, Grants: users.Grants{All: true}})
				return err
			})
			if err := f.users.Update(ctx, f.p.User, users.Change{Disabled: new(true)}); err != nil {
				f.t.Fatal(err)
			}
		},
		"grant removed": func(f *fixture) {
			other, _ := f.b.Library(catalog.Movies)
			if err := f.users.Update(ctx, f.p.User, users.Change{Grants: &users.Grants{Libraries: []id.ID{other}}}); err != nil {
				f.t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, 0)
			s := f.mustStart(f.movie("Heat", "mkv", 1).Work)
			if _, err := f.svc.HLS(ctx, s.ID, "0.m4s"); err != nil {
				t.Fatal(err)
			}
			revoke(f)
			if _, err := f.svc.HLS(ctx, s.ID, "1.m4s"); !errors.Is(err, ErrNoPlay) {
				t.Fatalf("next segment: %v", err)
			}
		})
	}
}

func TestARestartEndsPlays(t *testing.T) {
	f := newFixture(t, 0)
	m := f.movie("Heat", "mp4", 1)
	s := f.mustStart(m.Work)
	if err := f.svc.Heartbeat(ctx, f.p, s.ID, Heartbeat{Position: 20 * time.Minute, State: StatePaused}); err != nil {
		t.Fatal(err)
	}
	// The old process is gone without ending its plays, so the position is
	// the one the heartbeat wrote.
	restarted := New(f.b.DB, f.deps(discard{}))
	if _, err := restarted.File(ctx, s.ID, 0); !errors.Is(err, ErrNoPlay) {
		t.Fatalf("an old play after a restart: %v", err)
	}
	f.svc = restarted
	if s2 := f.mustStart(m.Work); s2.Start != 20*time.Minute {
		t.Fatalf("a new play starts at %v, want the saved 20m", s2.Start)
	}
}

func TestStreamLimitCountsPausedPlays(t *testing.T) {
	f := newFixture(t, 0)
	f.exec(`UPDATE user SET stream_limit = 1 WHERE id = ?`, f.p.User)
	first := f.mustStart(f.movie("Heat", "mp4", 1).Work)
	if err := f.svc.Heartbeat(ctx, f.p, first.ID, Heartbeat{State: StatePaused}); err != nil {
		t.Fatal(err)
	}
	_, err := f.start(f.movie("Ronin", "mp4", 1).Work, lan)
	var limit *StreamLimitError
	if !errors.As(err, &limit) || len(limit.Plays) != 1 || limit.Plays[0].ID != first.ID || limit.Plays[0].State != StatePaused {
		t.Fatalf("over the limit: %v", err)
	}
}

func TestMIMEType(t *testing.T) {
	for _, c := range []struct{ container, name, want string }{
		{"matroska,webm", "Heat.mkv", "video/x-matroska"},
		{"matroska,webm", "Heat.webm", "video/webm"},
		{"mov,mp4,m4a,3gp,3g2,mj2", "Heat.mp4", "video/mp4"},
		{"mpegts", "Heat.ts", "video/mp2t"},
		{"avi", "Heat.avi", "video/x-msvideo"},
		{"mov,mp4,m4a,3gp,3g2,mj2", "Dune.m4b", "audio/mp4"},
		{"mp3", "Dune 1 of 3.mp3", "audio/mpeg"},
		{"flac", "Dune.flac", "audio/flac"},
		{"flv", "Heat.flv", "application/octet-stream"},
	} {
		if got := mimeType(c.container, c.name); got != c.want {
			t.Errorf("%s %s: %s, want %s", c.container, c.name, got, c.want)
		}
	}
}

// A play still starting holds its stream, so two simultaneous starts
// can't both pass a limit of one. A start that fails gives the stream
// back.
func TestStartingPlaysHoldAStream(t *testing.T) {
	f := newFixture(t, 0)
	f.exec(`UPDATE user SET stream_limit = 1 WHERE id = ?`, f.p.User)
	if err := f.svc.reserveStream(f.p.User, 1); err != nil {
		t.Fatal(err)
	}
	var limit *StreamLimitError
	if _, err := f.start(f.movie("Heat", "mp4", 1).Work, lan); !errors.As(err, &limit) {
		t.Fatalf("while another play starts: %v", err)
	}
	f.svc.mu.Lock()
	f.svc.releaseLocked(reservation{user: f.p.User})
	f.svc.mu.Unlock()

	f.media.fail = media.ErrSourceUnreadable
	if _, err := f.start(f.hdrMovie("Dune").Work, lan); err == nil {
		t.Fatal("an unreadable source started")
	}
	f.media.fail = nil
	s := f.mustStart(f.movie("Ronin", "mp4", 1).Work)

	// A fallback ends the failed play before its new one takes the stream.
	if _, err := f.svc.Fallback(ctx, f.p, f.v, s.ID, FailReport{Position: time.Minute}); err != nil {
		t.Fatalf("a fallback at the limit: %v", err)
	}
}

// TestServerRemoteCap checks that the server's remote streaming limit caps
// a remote play of a user without a cap of their own, and that a user's
// own cap wins, as Jellyfin's user and server limits do.
func TestServerRemoteCap(t *testing.T) {
	f := newFixture(t, 0)
	f.setting(func(s *config.Settings) { s.RemoteQualityCap = 8_000_000 })
	over := f.movie("Ronin", "mp4", 30_000_000)
	if s, err := f.start(over.Work, remote); err != nil || s.Cap != (Limit{MaxBitrate: 8_000_000, Reason: ReasonRemoteQualityLimit}) {
		t.Fatalf("the server's cap: %+v %v", s, err)
	}
	if s, err := f.start(over.Work, lan); err != nil || s.Cap != (Limit{}) {
		t.Fatalf("on the LAN: %+v %v", s, err)
	}
	f.exec(`UPDATE user SET remote_quality_cap = 20000000 WHERE id = ?`, f.p.User)
	if s, err := f.start(over.Work, remote); err != nil || s.Cap.MaxBitrate != 20_000_000 {
		t.Fatalf("the user's own cap: %+v %v", s, err)
	}
}

func TestRemoteCap(t *testing.T) {
	f := newFixture(t, 0)
	f.exec(`UPDATE user SET remote_quality_cap = 10000000 WHERE id = ?`, f.p.User)
	under := f.movie("Heat", "mp4", 8_000_000)
	s, err := f.start(under.Work, remote)
	if err != nil || s.Local || s.MaxBitrate != 10_000_000 || s.Decision.Summary() != DirectPlay {
		t.Fatalf("under the cap: %+v %v", s, err)
	}
	over := f.movie("Ronin", "mp4", 30_000_000)
	s, err = f.start(over.Work, remote)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Decision.Selected("video"); v.Method() != Transcode || v.Reason != ReasonRemoteQualityLimit || v.Video.Bitrate > 10_000_000 {
		t.Fatalf("over the cap: %+v", v)
	}
	if s.Cap != (Limit{MaxBitrate: 10_000_000, Reason: ReasonRemoteQualityLimit}) {
		t.Errorf("the server's cap: %+v", s.Cap)
	}
	if s, err := f.start(over.Work, lan); err != nil || s.MaxBitrate != 0 || s.Cap != (Limit{}) {
		t.Fatalf("on the LAN the remote cap doesn't apply: %+v %v", s, err)
	}
	// A lower quality the client picks caps the play, but isn't the
	// server's cap.
	s, err = f.svc.Start(ctx, f.p, f.v, StartRequest{Work: over.Work, Caps: browser, ClientIP: remote, Quality: Quality{MaxBitrate: 2_000_000}})
	if err != nil || s.MaxBitrate != 2_000_000 || s.Cap.MaxBitrate != 10_000_000 {
		t.Fatalf("a picked quality under the cap: %+v %v", s, err)
	}
}

func TestBelowTheFloorRemuxIsRefusedDirectPlayWorks(t *testing.T) {
	f := newFixture(t, math.MaxUint64)
	_, err := f.start(f.movie("Ronin", "mkv", 1).Work, lan)
	var refused *RefusedError
	if !errors.As(err, &refused) || !errors.Is(err, ErrTranscodeBusy) || refused.Reason != RefuseStorageFull {
		t.Fatalf("remux below the floor: %v", err)
	}
	if _, err := f.start(f.movie("Heat", "mp4", 1).Work, lan); err != nil {
		t.Fatalf("direct play below the floor: %v", err)
	}
}

func TestAReplacedFileIsDecidedAnew(t *testing.T) {
	f := newFixture(t, 0)
	m := f.movie("Heat", "mp4", 1)
	playing := f.mustStart(m.Work)
	cur, _ := f.b.Catalog.File(ctx, m.File)
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := f.b.Catalog.UpsertFiles(ctx, tx, f.folder, []catalog.FileObservation{{RelPath: cur.RelPath, Size: cur.Size + 1, MTime: cur.MTime}})
		return err
	})
	f.probe(m.File, "mkv", 90*time.Minute, 1)
	if _, err := f.svc.File(ctx, playing.ID, 0); err != nil {
		t.Fatalf("the playing play: %v", err)
	}
	next := f.mustStart(m.Work)
	if next.Duration != 90*time.Minute || next.Decision.Delivery != DeliverStream {
		t.Fatalf("the next play: %+v", next)
	}
}

func TestFallbackRetriesAsATranscode(t *testing.T) {
	f := newFixture(t, 0)
	m := f.movie("Heat", "mp4", 1)
	s := f.mustStart(m.Work)
	s2, err := f.svc.Fallback(ctx, f.p, f.v, s.ID, FailReport{Position: 10 * time.Minute, Code: "mediaDecodeError"})
	if err != nil {
		t.Fatal(err)
	}
	if s2.ID == s.ID || s2.Version != s.Version || s2.Decision.Delivery != DeliverStream || s2.Start != 10*time.Minute {
		t.Fatalf("after a direct-play failure: %+v", s2)
	}
	for _, typ := range []media.TrackKind{media.Video, media.Audio} {
		if tr, _ := s2.Decision.Selected(typ); tr.Method() != Transcode || tr.Reason != ReasonFallback {
			t.Fatalf("the fallback's %s: %+v", typ, tr)
		}
	}
	if _, err := f.svc.File(ctx, s.ID, 0); !errors.Is(err, ErrNoPlay) {
		t.Fatalf("the failed play: %v", err)
	}
	if ended := f.published(events.PlayEnded); len(ended) != 0 {
		t.Fatalf("the failed play's player was told it ended elsewhere: %+v", ended)
	}
	if _, err := f.svc.HLS(ctx, s2.ID, "300.m4s"); err != nil {
		t.Fatalf("the fallback plays: %v", err)
	}
	// A play that transcodes every track on the CPU has nothing else to
	// try.
	if _, err := f.svc.Fallback(ctx, f.p, f.v, s2.ID, FailReport{Position: 20 * time.Minute}); !errors.Is(err, ErrNoFallback) {
		t.Fatalf("a fallback's fallback: %v", err)
	}
	if _, err := f.svc.HLS(ctx, s2.ID, "600.m4s"); !errors.Is(err, ErrNoPlay) {
		t.Fatalf("the failed fallback ends: %v", err)
	}
	if n := f.videoTranscodes(); n != 0 {
		t.Fatalf("%d video transcodes held", n)
	}
}

// A play whose fallback failed too is stopped with the failure. The server
// logs it with the player's detail, ends the play as failed without telling
// its player it ended elsewhere, and keeps the failure's position.
func TestStopWithAFailureLogsIt(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	f := newFixture(t, 0)
	m := f.movie("Heat", "mp4", 1)
	s := f.mustStart(m.Work)
	fail := &FailReport{Position: 12 * time.Minute, Code: "mediaDecodeError", Detail: "PIPELINE_ERROR_DECODE"}
	// An admin may stop someone else's play, but not report its failure.
	other := auth.Caller{User: id.New(), Session: id.New()}
	if err := f.svc.Stop(ctx, other, f.v, s.ID, StopRequest{Position: ptr(time.Minute), Failure: fail}); !errors.Is(err, ErrNoPlay) {
		t.Fatalf("an admin's failure for someone else's play: %v", err)
	}
	if err := f.svc.Stop(ctx, f.p, f.v, s.ID, StopRequest{Position: ptr(12 * time.Minute), Failure: fail}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.File(ctx, s.ID, 0); !errors.Is(err, ErrNoPlay) {
		t.Fatalf("the failed play: %v", err)
	}
	if ended := f.published(events.PlayEnded); len(ended) != 0 {
		t.Fatalf("the failed play's player was told it ended elsewhere: %+v", ended)
	}
	out := logs.String()
	for _, want := range []string{"level=WARN", "nothing left to try", "code=mediaDecodeError", "detail=PIPELINE_ERROR_DECODE", `reason="playback failed"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q:\n%s", want, out)
		}
	}
	if s2 := f.mustStart(m.Work); s2.Start != 12*time.Minute {
		t.Fatalf("resumed at %v", s2.Start)
	}
}

// videoTranscodes returns the server's count of video transcodes held.
func (f *fixture) videoTranscodes() int {
	f.svc.mu.Lock()
	defer f.svc.mu.Unlock()
	return f.svc.videoTranscodesLocked()
}

func TestFallbackKeepsTheTracks(t *testing.T) {
	f := newFixture(t, 0)
	m := f.b.Movie(f.folder, "Heat", 1995)
	f.probeWith(m.File, "mkv", 2*time.Hour, 8_000_000,
		media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8},
		media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Default: true, Language: "eng"},
		media.Track{Index: 2, Kind: media.Audio, Codec: "aac", Channels: 2, Language: "fra"},
		media.Track{Index: 3, Kind: media.Subtitle, Codec: "subrip", Language: "fra"},
	)
	cur, err := f.b.Catalog.File(ctx, m.File)
	if err != nil {
		t.Fatal(err)
	}
	french, sub := trackID(cur, "audio", 2), trackID(cur, "subtitle", 3)
	s, err := f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, AudioTrack: &french, SubtitleTrack: &sub, Caps: browser, ClientIP: lan})
	if err != nil {
		t.Fatal(err)
	}
	s2, err := f.svc.Fallback(ctx, f.p, f.v, s.ID, FailReport{Position: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if audioOf(s2) != french || subtitleOf(s2) != sub {
		t.Fatalf("the fallback's tracks: audio %v, subtitle %v", audioOf(s2), subtitleOf(s2))
	}
}

// TestFallbackKeepsSwitchedTracks checks that a fallback starts with the
// tracks a heartbeat switched to, not the ones the play started with, and
// that a heartbeat naming a track the version doesn't offer changes
// nothing.
func TestFallbackKeepsSwitchedTracks(t *testing.T) {
	f := newFixture(t, 0)
	m := f.b.Movie(f.folder, "Heat", 1995)
	f.probeWith(m.File, "mkv", 2*time.Hour, 8_000_000,
		media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8},
		media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Default: true, Language: "eng"},
		media.Track{Index: 2, Kind: media.Audio, Codec: "aac", Channels: 2, Language: "fra"},
		media.Track{Index: 3, Kind: media.Subtitle, Codec: "subrip", Language: "fra"},
	)
	cur, err := f.b.Catalog.File(ctx, m.File)
	if err != nil {
		t.Fatal(err)
	}
	english, french, sub := trackID(cur, "audio", 1), trackID(cur, "audio", 2), trackID(cur, "subtitle", 3)
	off := id.ID{}
	s, err := f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, AudioTrack: &english, SubtitleTrack: &off, Caps: browser, ClientIP: lan})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Heartbeat(ctx, f.p, s.ID, Heartbeat{Position: time.Minute, Audio: &french, Subtitle: &sub}); err != nil {
		t.Fatal(err)
	}
	bogus := id.New()
	if err := f.svc.Heartbeat(ctx, f.p, s.ID, Heartbeat{Position: time.Minute, Audio: &bogus, Subtitle: &bogus}); err != nil {
		t.Fatal(err)
	}
	s2, err := f.svc.Fallback(ctx, f.p, f.v, s.ID, FailReport{Position: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if audioOf(s2) != french || subtitleOf(s2) != sub {
		t.Fatalf("the fallback's tracks: audio %v, subtitle %v", audioOf(s2), subtitleOf(s2))
	}
}

// TestDialogueBoostEncodesTheAudio checks that a play with a dialogue
// boost streams the video copied and the audio encoded with the boost,
// and that its fallback keeps the boost.
func TestDialogueBoostEncodesTheAudio(t *testing.T) {
	f := newFixture(t, 0)
	m := f.movie("Heat", "mp4", 8_000_000)
	s, err := f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: browser, ClientIP: lan, DialogueBoost: true})
	if err != nil {
		t.Fatal(err)
	}
	if s.Decision.Delivery != DeliverStream || s.Decision.Summary() != DirectStream || s.Decision.DialogueBoost == nil || !s.Decision.DialogueBoost.Enabled {
		t.Fatalf("a boosted play: %+v", s.Decision)
	}
	if _, err := f.svc.HLS(ctx, s.ID, "3.m4s"); err != nil {
		t.Fatal(err)
	}
	r := f.media.last()
	if r.Video.Codec != media.Copy || len(r.Audio) != 1 || r.Audio[0].Out.Codec != media.AAC || !r.Audio[0].Out.DialogueBoost {
		t.Fatalf("its transcode: video %+v, audio %+v", r.Video, r.Audio)
	}
	s2, err := f.svc.Fallback(ctx, f.p, f.v, s.ID, FailReport{Position: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.HLS(ctx, s2.ID, "33.m4s"); err != nil {
		t.Fatal(err)
	}
	if b := s2.Decision.DialogueBoost; b == nil || !b.Enabled || !f.media.last().Audio[0].Out.DialogueBoost {
		t.Fatalf("the fallback's boost: %+v", b)
	}
}

func TestInvisibleWorksAreNotFound(t *testing.T) {
	f := newFixture(t, 0)
	m := f.movie("Heat", "mp4", 1)
	other, _ := f.b.Library(catalog.Movies)
	if err := f.users.Update(ctx, f.p.User, users.Change{Grants: &users.Grants{Libraries: []id.ID{other}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Start(ctx, f.p, f.viewer(), StartRequest{Work: m.Work, Caps: browser, ClientIP: lan}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a work in an ungranted library: %v", err)
	}
	if _, err := f.svc.Start(ctx, f.p, f.viewer(), StartRequest{Work: id.New(), Caps: browser, ClientIP: lan}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no such work: %v", err)
	}
}

// TestAnotherWorksVersionIsNotFound checks that plays, downloads and track
// lists only use the work's own versions. Asking by ID for a version of
// another work returns not found.
func TestAnotherWorksVersionIsNotFound(t *testing.T) {
	f := newFixture(t, 0)
	heat := f.movie("Heat", "mp4", 8_000_000)
	ronin := f.movie("Ronin", "mp4", 8_000_000)
	v := f.viewer()
	if _, err := f.svc.Start(ctx, f.p, v, StartRequest{Work: heat.Work, Version: ronin.Version, Caps: browser, ClientIP: lan}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("playing another work's version: %v", err)
	}
	if _, err := f.svc.Download(ctx, f.p, v, DownloadRequest{Work: heat.Work, Version: ronin.Version, Caps: iPhone}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("downloading another work's version: %v", err)
	}
	p, err := f.svc.Preferred(ctx, v, heat.Work)
	if _, ok := p.Versions[heat.Version]; err != nil || !ok || len(p.Versions) != 1 {
		t.Fatalf("track lists: %v (%v), want only %s", p.Versions, err, heat.Version)
	}
}

func TestMultiPartVersionsPlayOverHLS(t *testing.T) {
	f := newFixture(t, 0)
	w := f.b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Stacked", Year: 1999})
	cd1, cd2 := f.b.File(f.folder, "Stacked (1999)/Stacked cd1.mkv"), f.b.File(f.folder, "Stacked (1999)/Stacked cd2.mkv")
	f.b.Version([]id.ID{cd1, cd2}, w)
	f.probe(cd1, "mkv", time.Hour, 8_000_000)
	f.probe(cd2, "mkv", time.Hour, 8_000_000)
	s := f.mustStart(w)
	if s.Decision.Delivery != DeliverStream || s.Duration != 2*time.Hour || len(s.Parts) != 2 {
		t.Fatalf("a two-part mkv in a browser: %+v", s)
	}
	pl, err := f.svc.HLS(ctx, s.ID, "main.m3u8")
	if err != nil || !strings.Contains(string(pl.Data), "#EXT-X-DISCONTINUITY") {
		t.Fatalf("playlist: %v\n%s", err, pl.Data)
	}
	if _, err := f.svc.HLS(ctx, s.ID, "init-1.mp4"); err != nil {
		t.Fatalf("the second part's init: %v", err)
	}
}

// hdrMovie adds a two-hour 4K HEVC Main 10 HDR10 movie in mkv, with AAC
// and the given subtitle tracks after it.
func (f *fixture) hdrMovie(title string, subs ...media.Track) catalogtest.Item {
	f.t.Helper()
	m := f.b.Movie(f.folder, title, 2000)
	tracks := append([]media.Track{
		{Index: 0, Kind: media.Video, Codec: "hevc", Profile: "Main 10", Width: 3840, Height: 2160, BitDepth: 10, Range: media.HDR10, FrameRate: 23.976},
		{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Default: true},
	}, subs...)
	f.probeWith(m.File, "mkv", 2*time.Hour, 40_000_000, tracks...)
	return m
}

func (f *fixture) probeWith(file id.ID, container string, dur time.Duration, bitrate int, tracks ...media.Track) {
	f.t.Helper()
	names := map[string]string{"mkv": "matroska,webm", "mp4": "mov,mp4,m4a,3gp,3g2,mj2", "flac": "flac"}
	var stored []catalog.Track
	for _, t := range tracks {
		info, _ := json.Marshal(t)
		stored = append(stored, catalog.Track{Index: t.Index, Type: string(t.Kind), Codec: string(t.Codec), Language: t.Language, Default: t.Default, Forced: t.Forced, HearingImpaired: t.HearingImpaired, Info: info})
	}
	cur, err := f.b.Catalog.File(ctx, file)
	if err != nil {
		f.t.Fatal(err)
	}
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return f.b.Catalog.SetProbe(ctx, tx, file, cur.Generation, catalog.Probe{Summary: catalog.ProbeSummary{Container: names[container], Duration: dur, Bitrate: bitrate}, Tracks: stored})
	})
}

func (f *fixture) setting(fn func(*config.Settings)) {
	f.t.Helper()
	if err := f.cfg.Update(ctx, func(s *config.Settings) error { fn(s); return nil }); err != nil {
		f.t.Fatal(err)
	}
}

func hasProblem(m *Service, code string) bool {
	for _, p := range m.Health(ctx) {
		if p.Code == code {
			return true
		}
	}
	return false
}

func TestVideoTranscodeOnAnAccelerator(t *testing.T) {
	f := newFixture(t, 0)
	f.accel.caps = media.Capabilities{Tested: true, Accelerators: []media.Caps{qsv}, CPU: cpu}
	m := f.hdrMovie("Dune")
	s := f.mustStart(m.Work)
	v, _ := s.Decision.Selected("video")
	want := media.VideoRendition{Codec: media.H264, Profile: "high", BitDepth: 8, Width: 1920, Height: 1080, Bitrate: 20_000_000, HDR: media.ToneMap}
	if v.Method() != Transcode || v.Reason != ReasonVideoCodecUnsupported || *v.Video != want || v.Accelerator != qsv.Accelerator || v.SoftwareReason != "" {
		t.Fatalf("4K HDR HEVC in a browser: %+v %+v", v, v.Video)
	}
	if r := f.media.last(); r.Accelerator != qsv.Accelerator || r.Source.Codec != "hevc" || r.Video != want || r.SegmentDuration != 2*time.Second {
		t.Fatalf("the job: %+v", r)
	}
	if _, err := f.svc.HLS(ctx, s.ID, "500.m4s"); err != nil {
		t.Fatalf("a far segment: %v", err)
	}

	// A client that plays 8-bit HEVC gets HEVC from the GPU.
	hevc8 := browser
	hevc8.Video = append(slices.Clone(browser.Video), VideoEntry{Codec: "hevc", Delivery: []Delivery{DeliverFile, DeliverStream}, Profiles: []string{"Main"}})
	s, err := f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: hevc8, ClientIP: lan})
	if v, _ := s.Decision.Selected("video"); err != nil || v.Video.Codec != media.HEVC || v.Video.Profile != "main" {
		t.Fatalf("HEVC out: %+v %v", v, err)
	}

	// A remote play capped at 4 Mbps gets 720p within the cap, the audio
	// included.
	s, err = f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: browser, ClientIP: remote,
		Quality: Quality{MaxBitrate: 20_000_000}, RemoteQuality: &Quality{MaxBitrate: 4_000_000}})
	if v, _ := s.Decision.Selected("video"); err != nil || v.Video.Height != 720 || v.Video.Bitrate != 4_000_000-128_000 || v.Reason != ReasonVideoCodecUnsupported || s.MaxBitrate != 4_000_000 {
		t.Fatalf("remote at 4 Mbps: %+v %v", v.Video, err)
	}
}

func TestNoAcceleratorUsesTheCPU(t *testing.T) {
	f := newFixture(t, 0)
	f.accel.caps = media.Capabilities{Tested: true, Accelerators: []media.Caps{qsv}, CPU: cpu}
	f.setting(func(s *config.Settings) { s.Accelerator = config.AcceleratorNone })
	s := f.mustStart(f.hdrMovie("Dune").Work)
	if v, _ := s.Decision.Selected("video"); v.Accelerator != "" || v.SoftwareReason != ReasonNoAccelerator || f.media.last().Accelerator != "" {
		t.Fatalf("no accelerator: %+v", v)
	}
	if !hasProblem(f.svc, "noAccelerator") {
		t.Fatalf("health: %+v", f.svc.Health(ctx))
	}

	// With the setting naming an accelerator that isn't there, the play
	// uses the CPU and a problem says so.
	f.setting(func(s *config.Settings) { s.Accelerator = "nvenc:0" })
	s = f.mustStart(f.hdrMovie("Arrival").Work)
	if v, _ := s.Decision.Selected("video"); v.Accelerator != "" || v.SoftwareReason != ReasonAcceleratorUnsupported || !hasProblem(f.svc, "acceleratorMissing") {
		t.Fatalf("set to a missing accelerator: %+v", v)
	}
}

func TestABusyAcceleratorFallsBackToTheCPU(t *testing.T) {
	f := newFixture(t, 0)
	f.accel.caps = media.Capabilities{Tested: true, Accelerators: []media.Caps{qsv}, CPU: cpu}
	f.media.busy[qsv.Accelerator] = true
	s := f.mustStart(f.hdrMovie("Dune").Work)
	if v, _ := s.Decision.Selected("video"); v.Accelerator != "" || v.SoftwareReason != ReasonAcceleratorFailed || v.Video.Codec != media.H264 {
		t.Fatalf("busy GPU: %+v", v)
	}
	if f.media.last().Accelerator != "" || !hasProblem(f.svc, "gpuFailed") {
		t.Fatalf("job on %q; health %+v", f.media.last().Accelerator, f.svc.Health(ctx))
	}
}

func TestUnreadableSourceIsRefused(t *testing.T) {
	f := newFixture(t, 0)
	f.media.fail = media.ErrSourceUnreadable
	_, err := f.start(f.hdrMovie("Dune").Work, lan)
	var refused *RefusedError
	if !errors.As(err, &refused) || !errors.Is(err, ErrNotPlayable) || refused.Reason != RefuseTranscodeFailed {
		t.Fatalf("an unreadable source: %v", err)
	}
}

func TestTranscodeLimit(t *testing.T) {
	f := newFixture(t, 0)
	f.setting(func(s *config.Settings) { s.VideoTranscodeLimit = 1 })
	first := f.mustStart(f.hdrMovie("Dune").Work)
	if err := f.svc.Heartbeat(ctx, f.p, first.ID, Heartbeat{State: StatePaused}); err != nil {
		t.Fatal(err)
	}
	_, err := f.start(f.hdrMovie("Arrival").Work, lan)
	var refused *RefusedError
	if !errors.As(err, &refused) || !errors.Is(err, ErrTranscodeBusy) || refused.Reason != RefuseTranscodeLimit {
		t.Fatalf("over the limit, paused or not: %v", err)
	}
	if _, err := f.start(f.movie("Heat", "mp4", 1).Work, lan); err != nil {
		t.Fatalf("direct play over the transcode limit: %v", err)
	}
	if _, err := f.start(f.movie("Ronin", "mkv", 1).Work, lan); err != nil {
		t.Fatalf("a remux over the transcode limit: %v", err)
	}
	if err := f.svc.Stop(ctx, f.p, f.v, first.ID, StopRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.start(f.hdrMovie("Sicario").Work, lan); err != nil {
		t.Fatalf("after the first stopped: %v", err)
	}
}

func TestMidStreamFailureFallsBackToTheCPU(t *testing.T) {
	f := newFixture(t, 0)
	f.accel.caps = media.Capabilities{Tested: true, Accelerators: []media.Caps{qsv}, CPU: cpu}
	f.media.failAfter = 5
	s := f.mustStart(f.hdrMovie("Dune").Work)
	if v, _ := s.Decision.Selected("video"); v.Accelerator == "" {
		t.Fatalf("not on the GPU: %+v", v)
	}
	// The player asks for each segment in turn, so the request after the
	// fifth waits on the job and gets its failure. A request further ahead
	// would be a far seek, which starts a new job at that point.
	playUntilFailed := func(sid id.ID, from int) error {
		for i := from; i < from+5; i++ {
			if _, err := f.svc.HLS(ctx, sid, strconv.Itoa(i)+".m4s"); err != nil {
				t.Fatalf("segment %d: %v", i, err)
			}
		}
		_, err := f.svc.HLS(ctx, sid, strconv.Itoa(from+5)+".m4s")
		return err
	}
	if err := playUntilFailed(s.ID, 0); !errors.Is(err, ErrJobFailed) {
		t.Fatalf("a segment after the GPU failed: %v", err)
	}
	s2, err := f.svc.Fallback(ctx, f.p, f.v, s.ID, FailReport{Position: 40 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := s2.Decision.Selected("video"); v.Method() != Transcode || v.Accelerator != "" || v.SoftwareReason != ReasonAcceleratorFailed || s2.Start != 40*time.Second {
		t.Fatalf("after a GPU failure mid-stream: %+v at %v", v, s2.Start)
	}
	if a, _ := s2.Decision.Selected("audio"); a.Method() != Transcode || a.Reason != ReasonFallback {
		t.Fatalf("the fallback's audio: %+v", a)
	}
	if !hasProblem(f.svc, "gpuFailed") {
		t.Fatal("no problem reported")
	}
	// The CPU fails too, so nothing else is left to try. Its job starts at
	// 40 s, which is segment 20.
	if err := playUntilFailed(s2.ID, 20); !errors.Is(err, ErrJobFailed) {
		t.Fatalf("on the CPU: %v", err)
	}
	if _, err := f.svc.Fallback(ctx, f.p, f.v, s2.ID, FailReport{Position: time.Minute}); !errors.Is(err, ErrNoFallback) {
		t.Fatalf("after the CPU failed: %v", err)
	}
	if n := f.videoTranscodes(); n != 0 {
		t.Fatalf("%d video transcodes held", n)
	}
}

// TestFallbackRacesAnEnd races a fallback against a stop, an admin's stop
// and an expiry, under -race. Whichever ends the play first, no transcode
// is left without a play, and every video transcode is given back.
func TestFallbackRacesAnEnd(t *testing.T) {
	f := newFixture(t, 0)
	work := f.movie("Ronin", "mkv", 8_000_000).Work // a remux, so the fallback is a video transcode
	for i := range 30 {
		s := f.mustStart(work)
		var wg sync.WaitGroup
		wg.Go(func() {
			if _, err := f.svc.Fallback(ctx, f.p, f.v, s.ID, FailReport{Position: time.Minute}); err != nil && !errors.Is(err, ErrNoPlay) && !errors.Is(err, ErrPlayStopped) {
				t.Errorf("fallback: %v", err)
			}
		})
		wg.Go(func() {
			switch i % 3 {
			case 0:
				_ = f.svc.Stop(ctx, f.p, f.v, s.ID, StopRequest{})
			case 1:
				_ = f.svc.endAny(ctx, s.ID, "")
			case 2:
				f.svc.mu.Lock()
				if live := f.svc.plays[s.ID]; live != nil {
					live.seen = time.Now().Add(-time.Hour)
				}
				f.svc.mu.Unlock()
				f.svc.endExpired(ctx)
			}
		})
		wg.Wait()
		// At most the fallback play is left, with its transcode.
		f.svc.mu.Lock()
		for _, live := range f.svc.plays {
			if live.tc == nil || !live.transcodesVideo() {
				t.Errorf("a play left: %+v", live.decision)
			}
		}
		f.svc.mu.Unlock()
		f.svc.endAll(ctx)
		if n := f.videoTranscodes(); n != 0 {
			t.Fatalf("%d video transcodes held", n)
		}
		if entries, err := os.ReadDir(f.tcDir); err != nil || len(entries) != 0 {
			t.Fatalf("segment folders left: %v %v", entries, err)
		}
	}
}

func TestSubtitles(t *testing.T) {
	f := newFixture(t, 0)
	subDir := f.subDir
	m := f.b.Movie(f.folder, "Heat", 1995)
	f.probeWith(m.File, "mp4", 2*time.Hour, 8_000_000,
		media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8},
		media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Default: true},
		media.Track{Index: 2, Kind: media.Subtitle, Codec: "subrip", Language: "eng"},
		media.Track{Index: 3, Kind: media.Subtitle, Codec: "hdmv_pgs_subtitle", Language: "eng"},
	)
	if err := os.MkdirAll(filepath.Join(subDir, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "x", "heat.fr.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\nBonjour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		if err := f.b.Catalog.SetExternalSubtitles(ctx, tx, m.File, []catalog.SubtitleFile{{RelPath: "Heat (1995)/Heat.en.forced.srt", Format: "srt", Language: "en", Forced: true}}); err != nil {
			return err
		}
		_, err := f.b.Catalog.AddDownloadedSubtitle(ctx, tx, m.File, catalog.SubtitleFile{RelPath: "x/heat.fr.srt", Format: "srt", Language: "fr"})
		return err
	})
	srt, pgs := f.stream(m.File, 2), f.stream(m.File, 3)
	forced, fr := f.external(m.File, "Heat (1995)/Heat.en.forced.srt"), f.external(m.File, "x/heat.fr.srt")

	// Direct play delivers the files external. The embedded SRT isn't
	// extracted and this browser can't draw PGS, so choosing either starts
	// a new play.
	s := f.mustStart(m.Work)
	if s.Decision.Summary() != DirectPlay {
		t.Fatalf("decision %+v", s.Decision)
	}
	for track, want := range map[id.ID]Delivery{srt: "", pgs: "", forced: DeliverExternal, fr: DeliverExternal, {}: DeliverNone} {
		if got := choiceOf(s, track); got.Delivery != want {
			t.Errorf("%v: %+v, want %q", track, got, want)
		}
	}
	if c := choiceOf(s, srt); c.Reason != ReasonSubtitlePreparing {
		t.Errorf("the SRT's reason: %q", c.Reason)
	}
	if d, ok := externalOf(s, fr); len(s.ExternalSubtitles) != 2 || !ok || d.Name != fr.String()+".vtt" || !d.Ready || d.Format != media.VTT {
		t.Fatalf("external subtitles %+v", s.ExternalSubtitles)
	}
	sf, err := f.svc.Subtitle(ctx, s.ID, fr.String()+".vtt")
	if err != nil || string(sf.Data) != "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nBonjour\n" {
		t.Fatalf("the downloaded subtitle: %q %v", sf.Data, err)
	}
	if _, err := f.svc.Subtitle(ctx, s.ID, srt.String()+".vtt"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("an embedded track not extracted: %v", err)
	}
	for _, bad := range []string{id.New().String() + ".vtt", fr.String() + ".sup", "2.vtt", "../" + fr.String() + ".vtt", "x.ttf"} {
		if _, err := f.svc.Subtitle(ctx, s.ID, bad); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: %v", bad, err)
		}
	}

	// PGS selected in a browser is burned in from the source's track.
	s, err = f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: browser, ClientIP: lan, SubtitleTrack: &pgs})
	if err != nil {
		t.Fatal(err)
	}
	if sub, _ := s.Decision.Selected("subtitle"); sub.Delivery != DeliverBurn || f.media.last().BurnIn == nil || f.media.last().BurnIn.Track != 3 {
		t.Fatalf("PGS burned in: %+v %+v", sub, f.media.last().BurnIn)
	}
	// The external SRT, burned in on request, is drawn from its file.
	_, err = f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: browser, ClientIP: lan, SubtitleTrack: &forced, Settings: Settings{BurnSubtitles: true}})
	if b := f.media.last().BurnIn; err != nil || b == nil || b.Path != catalogtest.Abs("/media/movies1/Heat (1995)/Heat.en.forced.srt") {
		t.Fatalf("an external SRT burned in: %+v %v", b, err)
	}
	// The embedded SRT, burned in on request before it's extracted, goes
	// plain in the stream of a remux, with the reason. The direct play
	// becomes that remux. The track is never burned in from the source.
	s, err = f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: browser, ClientIP: lan, SubtitleTrack: &srt, Settings: Settings{BurnSubtitles: true}})
	if err != nil {
		t.Fatal(err)
	}
	// A remux's job starts in the background. Fetching its first segment
	// waits until it has.
	if _, err := f.svc.HLS(ctx, s.ID, "0.m4s"); err != nil {
		t.Fatal(err)
	}
	sub, _ := s.Decision.Selected("subtitle")
	video, _ := s.Decision.Selected("video")
	if sub.Delivery != DeliverStream || sub.Reason != ReasonSubtitlePreparing || video.Method() != DirectStream || video.Reason != ReasonSubtitlePreparing ||
		f.media.last().BurnIn != nil || len(f.media.last().Text) != 1 || f.media.last().Text[0].Track != 2 {
		t.Fatalf("burning in a track not extracted yet: %+v %+v %+v", sub, video, f.media.last())
	}
	// While it's in the stream to be burned in, the play carries only it.
	// Choosing another track starts a play that burns that one in, and
	// choosing off needs none.
	for track, want := range map[id.ID]Delivery{pgs: "", forced: "", fr: "", {}: DeliverNone} {
		if c := choiceOf(s, track); c.Delivery != want {
			t.Errorf("%v while burning in: %+v, want %q", track, c, want)
		}
	}

	// On an app that draws no subtitle files, the downloaded subtitle goes
	// in the stream of a remux, read from its file rather than by the job.
	// Every other text track is a rendition too, the embedded one read by
	// the job, so switching needs no new play. The PGS track needs one.
	app := noExternal(safari)
	s, err = f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: app, ClientIP: lan, SubtitleTrack: &fr})
	if err != nil {
		t.Fatal(err)
	}
	seg, err := f.svc.HLS(ctx, s.ID, "t2-0.vtt")
	if err != nil {
		t.Fatal(err)
	}
	sub, _ = s.Decision.Selected("subtitle")
	video, _ = s.Decision.Selected("video")
	if sub.Delivery != DeliverStream || sub.Reason != "" || video.Method() != DirectStream || video.Reason != ReasonSubtitleCodecUnsupported ||
		len(f.media.last().Text) != 1 || f.media.last().Text[0].Track != 2 {
		t.Fatalf("a downloaded subtitle on the app: %+v %+v %+v", sub, video, f.media.last())
	}
	if b, err := os.ReadFile(seg.Path); err != nil || !strings.Contains(string(b), "00:00:01.000 --> 00:00:02.000\nBonjour") {
		t.Fatalf("its first text segment: %q %v", b, err)
	}
	for track, want := range map[id.ID]Delivery{srt: DeliverStream, pgs: "", forced: DeliverStream, fr: DeliverStream, {}: DeliverNone} {
		if got := choiceOf(s, track); got.Delivery != want {
			t.Errorf("%v on the app: %+v, want %q", track, got, want)
		}
	}
	if len(s.ExternalSubtitles) != 0 {
		t.Fatalf("external subtitles %+v", s.ExternalSubtitles)
	}
	pl, err := f.svc.HLS(ctx, s.ID, "index.m3u8")
	if err != nil || strings.Count(string(pl.Data), "TYPE=SUBTITLES") != 3 || strings.Count(string(pl.Data), "DEFAULT=YES") != 1 ||
		!strings.Contains(string(pl.Data), `DEFAULT=YES,AUTOSELECT=YES,FORCED=NO,STABLE-RENDITION-ID="`+fr.String()+`"`) {
		t.Fatalf("the multivariant playlist: %v\n%s", err, pl.Data)
	}
}

// requests records the files the subtitles service was asked to extract
// first.
type requests struct {
	mu    sync.Mutex
	files []id.ID
}

func (r *requests) Ask(_ string, files ...id.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.files = append(r.files, files...)
}

// take returns the files asked for since the last call.
func (r *requests) take() []id.ID {
	r.mu.Lock()
	defer r.mu.Unlock()
	files := r.files
	r.files = nil
	return files
}

// extract puts a file's subtitles where the subtitles package keeps them
// once extracted. It writes each embedded subtitle track in the formats
// it's served in. Given a font, it also writes a list naming that font as
// the one the ASS track uses, and puts the font in the font store if
// stored is true. It returns the folder holding the subtitles.
func (f *fixture) extract(file id.ID, font string, stored bool) string {
	f.t.Helper()
	cf, err := f.b.Catalog.File(ctx, file)
	if err != nil {
		f.t.Fatal(err)
	}
	dir := config.CachePath(filepath.Join(f.cache, "subtitles"), file, file.String(), config.CacheLeaf(cf.Generation, 0))
	f.put(dir, "", "")
	for _, t := range cf.Tracks {
		format, ok := media.Codec(t.Codec).SubtitleFormat()
		if !ok || media.TrackKind(t.Type) != media.Subtitle {
			continue
		}
		if format.Text() {
			f.put(dir, strconv.Itoa(t.Index)+"."+media.VTT.Ext(), "")
		}
		if format == media.ASS || format == media.PGS {
			f.put(dir, strconv.Itoa(t.Index)+"."+format.Ext(), "")
		}
	}
	if font != "" {
		f.put(dir, "fonts.json", `["`+font+`"]`)
		if stored {
			f.put(filepath.Join(f.cache, "fonts", font[:2]), font, "")
		}
	}
	return dir
}

// makePreviews puts a file's scrub previews where the previews package
// keeps them once made, and returns the folder holding their sheets.
func (f *fixture) makePreviews(file id.ID) string {
	f.t.Helper()
	cf, err := f.b.Catalog.File(ctx, file)
	if err != nil {
		f.t.Fatal(err)
	}
	dir := config.CachePath(filepath.Join(f.cache, "previews"), file, file.String(), config.CacheLeaf(cf.Generation, 0))
	b, err := json.Marshal(media.Previews{Interval: 10 * time.Second, Width: 320, Height: 180, Columns: 10, Rows: 10, Sheets: 3})
	if err != nil {
		f.t.Fatal(err)
	}
	f.put(dir, "manifest.json", string(b))
	return dir
}

// put writes a file named name in dir, creating dir first. An empty name
// only creates dir.
func (f *fixture) put(dir, name, data string) {
	f.t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	if name == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func TestTextTracksStreamInHLS(t *testing.T) {
	f := newFixture(t, 0)
	font := strings.Repeat("ab", 32) + ".ttf"
	m := f.b.Movie(f.folder, "Heat", 1995)
	f.probeWith(m.File, "mkv", 2*time.Hour, 8_000_000,
		media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8},
		media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Default: true},
		media.Track{Index: 2, Kind: media.Subtitle, Codec: "subrip", Language: "eng"},
		media.Track{Index: 3, Kind: media.Subtitle, Codec: "ass", Language: "jpn"},
	)
	start := func(index int) Started {
		t.Helper()
		track := f.stream(m.File, index)
		s, err := f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: browser, ClientIP: lan, SubtitleTrack: &track})
		if err != nil {
			t.Fatal(err)
		}
		// A remux's job starts in the background. Fetching its first
		// segment waits until it has.
		if _, err := f.svc.HLS(ctx, s.ID, "0.m4s"); err != nil {
			t.Fatal(err)
		}
		return s
	}
	ass := f.stream(m.File, 3)

	// An embedded SubRip track in a remux that isn't extracted is in the
	// stream from the first segment, read by the job. So is the unprepared
	// ASS track, plain, so switching to it needs no new play.
	srt := f.stream(m.File, 2)
	s := start(2)
	sub, _ := s.Decision.Selected("subtitle")
	text := f.media.last().Text
	if sub.Delivery != DeliverStream || sub.Reason != "" || len(text) != 2 || text[0].Track != 2 || text[1].Track != 3 {
		t.Fatalf("SubRip in a remux: %+v, job's text %+v", sub, text)
	}
	if c := choiceOf(s, ass); c.Delivery != DeliverStream || c.Codec != webVTT || len(s.ExternalSubtitles) != 0 {
		t.Fatalf("the ASS track %+v, external subtitles %+v", c, s.ExternalSubtitles)
	}
	pl, err := f.svc.HLS(ctx, s.ID, transcode.PlaylistName)
	if err != nil || !strings.Contains(string(pl.Data), `LANGUAGE="en",DEFAULT=YES`) || !strings.Contains(string(pl.Data), `LANGUAGE="ja",DEFAULT=NO`) {
		t.Fatalf("playlist %v:\n%s", err, pl.Data)
	}
	if m, err := f.svc.HLS(ctx, s.ID, "t0-0.vtt"); err != nil || filepath.Ext(m.Path) != ".vtt" || m.Type != "text/vtt; charset=utf-8" {
		t.Fatalf("text segment: %+v %v", m, err)
	}

	// An unprepared ASS track goes plain in the stream for the play, with
	// the reason, and the play asks for nothing. Once the tracks are
	// extracted, the stream reads them from their extractions instead of
	// the job.
	for _, extracted := range []bool{false, true} {
		if extracted {
			f.extract(m.File, font, false)
		}
		s = start(3)
		text := f.media.last().Text
		if sub, _ := s.Decision.Selected("subtitle"); sub.Delivery != DeliverStream || sub.Reason != ReasonSubtitlePreparing ||
			len(text) != map[bool]int{false: 2, true: 0}[extracted] {
			t.Fatalf("ASS not prepared (extracted %v): %+v, job's text %+v", extracted, sub, text)
		}
		requested := f.requested.take()
		if _, ok := externalOf(s, ass); ok || choiceOf(s, srt).Delivery != DeliverStream || len(requested) != 0 || f.extractions.count() != 0 {
			t.Fatalf("external subtitles %+v, requested %v", s.ExternalSubtitles, requested)
		}
	}

	// Once prepared, it's drawn styled, with its fonts named by their
	// hashes, and only the SubRip track is in the stream.
	f.extract(m.File, font, true)
	s = start(3)
	if sub, _ := s.Decision.Selected("subtitle"); sub.Delivery != DeliverExternal || sub.Codec != "ass" || len(f.media.last().Text) != 0 || len(f.requested.take()) != 0 {
		t.Fatalf("ASS once prepared: %+v, job's text %+v", sub, f.media.last().Text)
	}
	if d, ok := externalOf(s, ass); !ok || !d.Ready || d.Name != ass.String()+".ass" || !slices.Equal(d.Fonts, []string{strings.Repeat("ab", 32)}) {
		t.Fatalf("external subtitles %+v", s.ExternalSubtitles)
	}
	if pl, err := f.svc.HLS(ctx, s.ID, transcode.PlaylistName); err != nil || strings.Count(string(pl.Data), "TYPE=SUBTITLES") != 1 ||
		strings.Contains(string(pl.Data), "DEFAULT=YES") {
		t.Fatalf("playlist with the styled track drawn: %v\n%s", err, pl.Data)
	}
}

// A play decides styled subtitles once, as it starts. Prepared ASS is
// drawn by a client that draws ASS. For a client that doesn't, it's burned
// in from the prepared file and its fonts. Unprepared ASS goes plain in
// the stream, and a direct play becomes a remux to carry it.
func TestStyledSubtitles(t *testing.T) {
	f := newFixture(t, 0)
	m := f.b.Movie(f.folder, "Heat", 1995)
	f.probeWith(m.File, "mkv", 2*time.Hour, 8_000_000,
		media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8},
		media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Default: true},
		media.Track{Index: 2, Kind: media.Subtitle, Codec: "subrip", Language: "eng"},
		media.Track{Index: 3, Kind: media.Subtitle, Codec: "ass", Language: "jpn"},
	)
	srt, ass := f.stream(m.File, 2), f.stream(m.File, 3)
	tv := alike([]string{"mkv"}, true, browser.Video, browser.Audio, vttFile, assFile)
	phone := tv
	phone.Subtitles = []SubtitleEntry{vttStream, vttFile}
	start := func(caps Capabilities, track id.ID) Started {
		t.Helper()
		s, err := f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: caps, ClientIP: lan, SubtitleTrack: &track})
		if err != nil {
			t.Fatal(err)
		}
		// An HLS play's job starts in the background. Fetching its first
		// segment waits until it has.
		if s.Decision.Delivery == DeliverStream {
			if _, err := f.svc.HLS(ctx, s.ID, "0.m4s"); err != nil {
				t.Fatal(err)
			}
		}
		return s
	}

	// When the track isn't prepared, a client that would play the file
	// directly gets a remux with the track plain in its stream.
	s := start(tv, ass)
	video, _ := s.Decision.Selected("video")
	sub, _ := s.Decision.Selected("subtitle")
	if s.Decision.Delivery != DeliverStream || video.Method() != DirectStream || video.Reason != ReasonSubtitlePreparing || sub.Delivery != DeliverStream {
		t.Fatalf("not prepared: %+v", s.Decision)
	}
	// The same happens on a client that doesn't draw ASS. With SubRip
	// chosen, the ASS track is plain in the stream too, so switching to it
	// needs no new play.
	s = start(phone, ass)
	if sub, _ := s.Decision.Selected("subtitle"); sub.Delivery != DeliverStream || sub.Reason != ReasonSubtitlePreparing || f.media.last().BurnIn != nil {
		t.Fatalf("not prepared, on a phone: %+v", s.Decision)
	}
	s = start(phone, srt)
	if c := choiceOf(s, ass); c.Delivery != DeliverStream {
		t.Fatalf("the ASS track %+v", c)
	}

	// Once prepared, it's a direct play with the styled file and its fonts.
	dir := f.extract(m.File, strings.Repeat("cd", 32)+".otf", true)
	s = start(tv, ass)
	if sub, _ := s.Decision.Selected("subtitle"); s.Decision.Summary() != DirectPlay || sub.Delivery != DeliverExternal || sub.Codec != "ass" {
		t.Fatalf("prepared: %+v", s.Decision)
	}
	if d, ok := externalOf(s, ass); !ok || !d.Ready || !slices.Equal(d.Fonts, []string{strings.Repeat("cd", 32)}) {
		t.Fatalf("external subtitles %+v", s.ExternalSubtitles)
	}
	// Once prepared, on a client that doesn't draw ASS, it's burned in from
	// the prepared file, with its fonts gathered for the job.
	s = start(phone, ass)
	b := f.media.last().BurnIn
	if sub, _ := s.Decision.Selected("subtitle"); sub.Delivery != DeliverBurn || b == nil || b.Path != filepath.Join(dir, "3.ass") || b.Fonts == "" {
		t.Fatalf("prepared, on a phone: %+v, burn-in %+v", sub, b)
	}
	// On that client, with SubRip chosen, the prepared ASS track isn't
	// offered as a file. Choosing it starts a play that burns it in.
	s = start(phone, srt)
	if c := choiceOf(s, ass); c.Delivery != "" || c.Reason != ReasonSubtitleCodecUnsupported {
		t.Fatalf("the prepared ASS track %+v", c)
	}
}

// TestTextAsListed checks that an external text subtitle comes as WebVTT,
// from any text source.
func TestTextAsListed(t *testing.T) {
	f := newFixture(t, 0)
	m := f.b.Movie(f.folder, "Heat", 1995)
	f.probeWith(m.File, "mp4", 2*time.Hour, 8_000_000,
		media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8},
		media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Default: true},
		media.Track{Index: 2, Kind: media.Subtitle, Codec: "subrip", Language: "eng"},
		media.Track{Index: 3, Kind: media.Subtitle, Codec: "mov_text", Language: "eng"},
		media.Track{Index: 4, Kind: media.Subtitle, Codec: "webvtt", Language: "eng"},
	)
	cue := "\n\n00:00:01.000 --> 00:00:02.000\nHello\n"
	dir := f.extract(m.File, "", false)
	for _, n := range []string{"2.vtt", "3.vtt", "4.vtt"} {
		f.put(dir, n, "WEBVTT"+cue)
	}
	files := map[string]string{
		"x/heat.fr.srt": "1\r\n00:00:01,000 --> 00:00:02,000\r\nHello\r\n",
		"x/heat.de.vtt": "WEBVTT" + cue,
		"x/heat.es.ass": "[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
			"Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\\b1}Hello\n",
	}
	for rel, data := range files {
		f.put(filepath.Join(f.subDir, "x"), filepath.Base(rel), data)
	}
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		for rel := range files {
			lang, format, _ := strings.Cut(strings.TrimPrefix(filepath.Base(rel), "heat."), ".")
			if _, err := f.b.Catalog.AddDownloadedSubtitle(ctx, tx, m.File, catalog.SubtitleFile{RelPath: rel, Format: format, Language: lang}); err != nil {
				return err
			}
		}
		return nil
	})
	tracks := map[string]id.ID{"subrip": f.stream(m.File, 2), "mov_text": f.stream(m.File, 3), "webvtt": f.stream(m.File, 4),
		"SubRip file": f.external(m.File, "x/heat.fr.srt"), "WebVTT file": f.external(m.File, "x/heat.de.vtt"), "ASS file": f.external(m.File, "x/heat.es.ass")}

	for _, c := range []struct {
		codec  media.Codec
		format media.SubtitleFormat
		typ    string
		body   string
	}{
		{webVTT, media.VTT, "text/vtt; charset=utf-8", "WEBVTT" + cue},
	} {
		caps := browser
		caps.Subtitles = []SubtitleEntry{vttStream, {Codec: c.codec, Delivery: []Delivery{DeliverExternal}}}
		for name, track := range tracks {
			s, err := f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: caps, ClientIP: lan, SubtitleTrack: &track})
			if err != nil {
				t.Fatal(err)
			}
			sub, _ := s.Decision.Selected(media.Subtitle)
			x, ok := externalOf(s, track)
			if sub.Delivery != DeliverExternal || sub.Codec != c.codec || !ok || x.Format != c.format || x.Name != subtitleName(track, c.format) || !x.Ready {
				t.Errorf("%s as %s: %+v, %+v", name, c.codec, sub, x)
				continue
			}
			got, err := f.svc.Subtitle(ctx, s.ID, x.Name)
			if got.Path != "" {
				got.Data, _ = os.ReadFile(got.Path) // an extraction served as it is
			}
			if err != nil || got.Type != c.typ || string(got.Data) != c.body {
				t.Errorf("%s as %s: %q %q %v", name, c.codec, got.Type, got.Data, err)
			}
		}
	}
}

func TestSubtitlesOnDemand(t *testing.T) {
	f := newFixture(t, 0)
	m := f.b.Movie(f.folder, "Heat", 1995)
	f.probeWith(m.File, "mp4", 2*time.Hour, 8_000_000,
		media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8},
		media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Default: true},
		media.Track{Index: 2, Kind: media.Subtitle, Codec: "subrip", Language: "eng"},
		media.Track{Index: 3, Kind: media.Subtitle, Codec: "hdmv_pgs_subtitle", Language: "eng"},
	)

	// A text track that isn't extracted yet goes in the stream. The file
	// that would play directly is remuxed for this play, and the play asks
	// for nothing. Choosing the PGS track, which isn't extracted either,
	// starts a new play. No previews are made yet.
	srt := f.stream(m.File, 2)
	caps := browser
	caps.Subtitles = []SubtitleEntry{vttStream, vttFile, pgsFile}
	s, err := f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: caps, ClientIP: lan, SubtitleTrack: &srt})
	if err != nil {
		t.Fatal(err)
	}
	sub, _ := s.Decision.Selected("subtitle")
	video, _ := s.Decision.Selected("video")
	if s.Decision.Summary() != DirectStream || sub.Delivery != DeliverStream || video.Reason != ReasonSubtitlePreparing {
		t.Fatalf("decision %+v", s.Decision)
	}
	if c := choiceOf(s, f.stream(m.File, 3)); c.Delivery != "" || c.Reason != ReasonSubtitlePreparing || len(s.ExternalSubtitles) != 0 || s.Previews {
		t.Fatalf("the PGS track %+v, external subtitles %+v, previews %v", c, s.ExternalSubtitles, s.Previews)
	}
	if _, err := f.svc.Subtitle(ctx, s.ID, srt.String()+".vtt"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("before extraction: %v", err)
	}
	if _, _, err := f.svc.Preview(ctx, s.ID, PreviewManifest); !errors.Is(err, ErrNotFound) {
		t.Errorf("previews before they're made: %v", err)
	}

	// A PGS track that isn't extracted yet is burned in, and the play asks
	// for nothing.
	pgs := f.stream(m.File, 3)
	s, err = f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: caps, ClientIP: lan, SubtitleTrack: &pgs})
	if err != nil {
		t.Fatal(err)
	}
	if sub, _ := s.Decision.Selected("subtitle"); sub.Delivery != DeliverBurn || sub.Reason != ReasonSubtitlePreparing {
		t.Fatalf("PGS not extracted: %+v", s.Decision)
	}
	if requested := f.requested.take(); len(requested) != 0 || f.extractions.count() != 0 {
		t.Fatalf("plays asked for %v, extracted %d times", requested, f.extractions.count())
	}

	// Once extracted, the next play plays the file and draws the track
	// from its extraction.
	dir := f.extract(m.File, "", false)
	s, err = f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: caps, ClientIP: lan, SubtitleTrack: &srt})
	if err != nil {
		t.Fatal(err)
	}
	if s.Decision.Summary() != DirectPlay {
		t.Errorf("extracted: %+v", s.Decision)
	}
	if sf, err := f.svc.Subtitle(ctx, s.ID, srt.String()+".vtt"); err != nil || sf.Path != filepath.Join(dir, "2.vtt") {
		t.Errorf("the SubRip track: %+v %v", sf, err)
	}

	// Previews, once made, are in a new play's response.
	previewDir := f.makePreviews(m.File)
	s = f.mustStart(m.Work)
	manifest, found, err := f.svc.Preview(ctx, s.ID, PreviewManifest)
	if !s.Previews || err != nil || !found || !strings.Contains(string(manifest.Data), `"sheets":["sheet-0.webp","sheet-1.webp","sheet-2.webp"]`) {
		t.Errorf("previews %v: %s %v", s.Previews, manifest.Data, err)
	}
	if sheet, found, err := f.svc.Preview(ctx, s.ID, "sheet-2.webp"); err != nil || !found || sheet.Path != filepath.Join(previewDir, "sheet-2.webp") {
		t.Errorf("a sheet: %+v %v %v", sheet, found, err)
	}
	if _, found, err := f.svc.Preview(ctx, s.ID, "sheet-3.webp"); err != nil || found {
		t.Errorf("no such sheet: %v %v", found, err)
	}
}

// stream returns the ID of a file's embedded track at a stream index. A
// negative index returns the zero ID, which means off.
func (f *fixture) stream(file id.ID, index int) id.ID {
	f.t.Helper()
	if index < 0 {
		return id.ID{}
	}
	cf, err := f.b.Catalog.File(ctx, file)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, tr := range cf.Tracks {
		if tr.Index == index {
			return tr.ID
		}
	}
	f.t.Fatalf("no track %d in %+v", index, cf.Tracks)
	return id.ID{}
}

// external returns the ID of a file's subtitle file at a path.
func (f *fixture) external(file id.ID, rel string) id.ID {
	f.t.Helper()
	cf, err := f.b.Catalog.File(ctx, file)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, x := range cf.SubtitleFiles {
		if x.RelPath == rel {
			return x.ID
		}
	}
	f.t.Fatalf("no subtitle file %s in %+v", rel, cf.SubtitleFiles)
	return id.ID{}
}

// anime is a file with English and Japanese audio, and full, forced and
// SDH English subtitles.
func (f *fixture) anime(file id.ID) {
	f.t.Helper()
	f.probeWith(file, "mkv", 25*time.Minute, 4_000_000,
		media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8},
		media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Language: "eng", Default: true},
		media.Track{Index: 2, Kind: media.Audio, Codec: "aac", Channels: 2, Language: "jpn"},
		media.Track{Index: 3, Kind: media.Subtitle, Codec: "subrip", Language: "eng", Default: true},
		media.Track{Index: 4, Kind: media.Subtitle, Codec: "subrip", Language: "eng", Forced: true},
		media.Track{Index: 5, Kind: media.Subtitle, Codec: "subrip", Language: "eng", HearingImpaired: true},
	)
}

func TestDefaultTracks(t *testing.T) {
	f := newFixture(t, 0)
	m := f.b.Movie(f.folder, "Akira", 1988)
	f.anime(m.File)

	// With no preferences, the play gets the default audio and the forced
	// subtitles in its language. The default flag on the full track isn't
	// trusted.
	s := f.mustStart(m.Work)
	if audioOf(s) != f.stream(m.File, 1) || subtitleOf(s) != f.stream(m.File, 4) {
		t.Fatalf("defaults: audio %v, subtitles %v", audioOf(s), subtitleOf(s))
	}

	for _, c := range []struct {
		name                   string
		audio, subtitles, mode string
		metadataLanguage       string // the server's metadata language; en when empty
		wantAudio, wantSub     int    // stream indexes; -1 means off
	}{
		{name: "Japanese audio, foreign audio only", audio: "ja", subtitles: "en", mode: users.SubtitlesSmart, wantAudio: 2, wantSub: 3},
		{name: "original audio, foreign audio only", subtitles: "en", mode: users.SubtitlesSmart, wantAudio: 1, wantSub: 4},
		{name: "foreign audio only in the server's language", audio: "ja", mode: users.SubtitlesSmart, wantAudio: 2, wantSub: 3},
		{name: "foreign audio only, no track in the language", audio: "ja", subtitles: "fr", mode: users.SubtitlesSmart, wantAudio: 2, wantSub: -1},
		{name: "foreign audio only, the server's language has none", audio: "ja", mode: users.SubtitlesSmart, metadataLanguage: "fr", wantAudio: 2, wantSub: -1},
		{name: "always", subtitles: "en", mode: users.SubtitlesAlways, wantAudio: 1, wantSub: 3},
		{name: "always, no track in the language", subtitles: "de", mode: users.SubtitlesAlways, wantAudio: 1, wantSub: 4},
		{name: "only forced, Japanese audio", audio: "ja", subtitles: "en", mode: users.SubtitlesForced, wantAudio: 2, wantSub: -1},
		{name: "off", subtitles: "en", mode: users.SubtitlesNone, wantAudio: 1, wantSub: -1},
		{name: "a language the file doesn't have", audio: "fr", mode: users.SubtitlesForced, wantAudio: 1, wantSub: 4},
	} {
		lang := cmp.Or(c.metadataLanguage, "en")
		f.setting(func(s *config.Settings) { s.MetadataLanguage = lang })
		change := users.PlaybackChange{AudioLanguage: &c.audio, SubtitleLanguage: &c.subtitles, SubtitleMode: &c.mode}
		if err := f.users.EditPreferences(ctx, f.v, users.PreferencesChange{Playback: &change}); err != nil {
			t.Fatal(err)
		}
		if s := f.mustStart(m.Work); audioOf(s) != f.stream(m.File, c.wantAudio) || subtitleOf(s) != f.stream(m.File, c.wantSub) {
			t.Errorf("%s: audio %v, subtitles %v; want streams %d, %d", c.name, audioOf(s), subtitleOf(s), c.wantAudio, c.wantSub)
		}
	}
}

func TestASeriesRemembersItsTracks(t *testing.T) {
	f := newFixture(t, 0)
	series := f.b.Series("Cowboy Bebop")
	e1, e2 := f.b.Episode(f.folder, series, 1, 1), f.b.Episode(f.folder, series, 1, 2)
	f.anime(e1.File)
	f.anime(e2.File)
	// is reports whether a play plays a file's tracks at these stream
	// indexes (-1 for off).
	is := func(s Started, file id.ID, audio, sub int) bool {
		return audioOf(s) == f.stream(file, audio) && subtitleOf(s) == f.stream(file, sub)
	}
	audio, sdh := f.stream(e1.File, 2), f.stream(e1.File, 5)
	if _, err := f.svc.Start(ctx, f.p, f.v, StartRequest{Work: e1.Work, Caps: browser, ClientIP: lan, AudioTrack: &audio, SubtitleTrack: &sdh}); err != nil {
		t.Fatal(err)
	}
	s := f.mustStart(e2.Work)
	if !is(s, e2.File, 2, 5) {
		t.Fatalf("the next episode: audio %v, subtitles %v", audioOf(s), subtitleOf(s))
	}
	var off id.ID
	if _, err := f.svc.Start(ctx, f.p, f.v, StartRequest{Work: e2.Work, Caps: browser, ClientIP: lan, SubtitleTrack: &off}); err != nil {
		t.Fatal(err)
	}
	if s = f.mustStart(e1.Work); !is(s, e1.File, 2, -1) {
		t.Fatalf("subtitles turned off: audio %v, subtitles %v", audioOf(s), subtitleOf(s))
	}

	// When the user doesn't remember choices, each episode starts from the
	// user's defaults, and choices aren't saved.
	if err := f.users.EditPreferences(ctx, f.v, users.PreferencesChange{Playback: &users.PlaybackChange{RememberTracks: new(false)}}); err != nil {
		t.Fatal(err)
	}
	if s = f.mustStart(e2.Work); !is(s, e2.File, 1, 4) {
		t.Fatalf("not remembering: audio %v, subtitles %v", audioOf(s), subtitleOf(s))
	}
	full := f.stream(e2.File, 3)
	if _, err := f.svc.Start(ctx, f.p, f.v, StartRequest{Work: e2.Work, Caps: browser, ClientIP: lan, SubtitleTrack: &full}); err != nil {
		t.Fatal(err)
	}
	if err := f.users.EditPreferences(ctx, f.v, users.PreferencesChange{Playback: &users.PlaybackChange{RememberTracks: new(true)}}); err != nil {
		t.Fatal(err)
	}
	if s = f.mustStart(e1.Work); !is(s, e1.File, 2, -1) {
		t.Fatalf("remembering again: audio %v, subtitles %v", audioOf(s), subtitleOf(s))
	}
}

func TestPreferredIsWhatAPlayStartsWith(t *testing.T) {
	f := newFixture(t, 0)
	series := f.b.Series("Cowboy Bebop")
	e1, e2 := f.b.Episode(f.folder, series, 1, 1), f.b.Episode(f.folder, series, 1, 2)
	f.anime(e1.File)
	f.anime(e2.File)
	// matches reports whether an episode's preferred tracks are those its
	// play starts with.
	matches := func(e catalogtest.Item) bool {
		t.Helper()
		p, err := f.svc.Preferred(ctx, f.v, e.Work)
		if err != nil {
			t.Fatal(err)
		}
		s := f.mustStart(e.Work)
		got := p.Versions[e.Version]
		return p.Remembers && got.Audio == audioOf(s) && got.Subtitle == subtitleOf(s)
	}
	if !matches(e1) {
		t.Fatal("defaults: preferred tracks differ from the play's")
	}

	// A subtitle switched without a new play becomes the series' choice.
	s := f.mustStart(e1.Work)
	full := f.stream(e1.File, 3)
	if err := f.svc.Heartbeat(ctx, f.p, s.ID, Heartbeat{Position: time.Minute, Subtitle: &full}); err != nil {
		t.Fatal(err)
	}
	if !matches(e2) || subtitleOf(f.mustStart(e2.Work)) != f.stream(e2.File, 3) {
		t.Fatal("after a switch: the next episode doesn't start with full subtitles")
	}
	var off id.ID
	if err := f.svc.Heartbeat(ctx, f.p, s.ID, Heartbeat{Position: time.Minute, Subtitle: &off}); err != nil {
		t.Fatal(err)
	}
	if !matches(e2) || !subtitleOf(f.mustStart(e2.Work)).IsZero() {
		t.Fatal("after switching off: the next episode starts with subtitles")
	}

	// A movie's choices aren't remembered.
	m := f.b.Movie(f.folder, "Akira", 1988)
	f.anime(m.File)
	if p, err := f.svc.Preferred(ctx, f.v, m.Work); err != nil || p.Remembers {
		t.Fatalf("a movie: %+v, %v", p, err)
	}
}

func TestAChosenTrackNamesItsVersion(t *testing.T) {
	f := newFixture(t, 0)
	m := f.b.Movie(f.folder, "Akira", 1988)
	f.anime(m.File)
	small := f.b.File(f.folder, "Akira (1988)/Akira 576p.mkv")
	smallVersion := f.b.Version([]id.ID{small}, m.Work)
	f.probeWith(small, "mkv", 25*time.Minute, 1_500_000,
		media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 720, Height: 576, BitDepth: 8},
		media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Language: "eng", Default: true},
		media.Track{Index: 2, Kind: media.Audio, Codec: "aac", Channels: 2, Language: "jpn"},
		media.Track{Index: 3, Kind: media.Subtitle, Codec: "subrip", Language: "eng"},
	)
	if s := f.mustStart(m.Work); s.Version != m.Version {
		t.Fatalf("no choice: version %v, want the first, %v", s.Version, m.Version)
	}

	// Choosing the smaller version's tracks without naming a version plays
	// that version.
	audio, sub := f.stream(small, 2), f.stream(small, 3)
	s, err := f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: browser, ClientIP: lan, AudioTrack: &audio, SubtitleTrack: &sub})
	if err != nil {
		t.Fatal(err)
	}
	if s.Version != smallVersion || audioOf(s) != audio || subtitleOf(s) != sub {
		t.Fatalf("tracks without a version: version %v, audio %v, subtitles %v", s.Version, audioOf(s), subtitleOf(s))
	}
	var off id.ID
	if s, err = f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: browser, ClientIP: lan, AudioTrack: &audio, SubtitleTrack: &off}); err != nil || s.Version != smallVersion {
		t.Fatalf("audio without a version, subtitles off: version %v, err %v", s.Version, err)
	}

	// A track from a version other than the chosen one isn't found.
	if _, err := f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Version: m.Version, Caps: browser, ClientIP: lan, AudioTrack: &audio}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another version's track: %v", err)
	}
}

func TestUnusableGPUIsAProblemOnlyWithoutAWorkingOne(t *testing.T) {
	f := newFixture(t, 0)
	nvidia := media.AcceleratorProblem{Accelerator: "nvenc:0", Backend: media.NVENC, Name: "NVIDIA GPU", Reason: media.NoDriver}
	f.accel.caps = media.Capabilities{Tested: true, Accelerators: []media.Caps{qsv}, CPU: cpu, Problems: []media.AcceleratorProblem{nvidia}}
	if hasProblem(f.svc, "gpuUnusable") {
		t.Error("a second, unusable GPU is a problem while the Intel one works")
	}
	f.accel.caps = media.Capabilities{Tested: true, CPU: cpu, Problems: []media.AcceleratorProblem{nvidia}}
	if !hasProblem(f.svc, "gpuUnusable") {
		t.Error("the only GPU can't be used, and no problem says so")
	}
}

// A play with no version or track chosen plays the version the user last
// played, while the work still has it, else the first in order.
func TestDefaultVersion(t *testing.T) {
	first, second := id.New(), id.New()
	versions := []catalog.Version{{ID: first}, {ID: second, Edition: "Final Cut"}}
	for _, c := range []struct {
		name     string
		versions []catalog.Version
		last     id.ID
		want     []catalog.Version
	}{
		{"the last version", versions, second, versions[1:]},
		{"none played", versions, id.ID{}, versions[:1]},
		{"a last version that is gone", versions, id.New(), versions[:1]},
		{"no versions", nil, second, nil},
	} {
		got := defaultVersion(c.versions, c.last)
		if len(got) != len(c.want) || len(got) == 1 && got[0].ID != c.want[0].ID {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAPlayWithNoChoicePlaysTheDefaultVersion(t *testing.T) {
	f := newFixture(t, 0)
	stop := func(s Started) {
		t.Helper()
		if err := f.svc.Stop(ctx, f.p, f.v, s.ID, StopRequest{Position: ptr(10 * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	m := f.movie("Heat", "mp4", 8_000_000)
	small := f.b.File(f.folder, "Heat (2000)/Heat 576p.mkv")
	smallVersion := f.b.Version([]id.ID{small}, m.Work)
	f.probeWith(small, "mkv", 2*time.Hour, 1_500_000,
		media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 720, Height: 576, BitDepth: 8},
		media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Default: true},
	)
	s := f.mustStart(m.Work)
	if s.Version != m.Version {
		t.Fatalf("none played: version %v, want the first, %v", s.Version, m.Version)
	}
	stop(s)
	s, err := f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Version: smallVersion, Caps: browser, ClientIP: lan})
	if err != nil {
		t.Fatal(err)
	}
	stop(s)
	if s = f.mustStart(m.Work); s.Version != smallVersion {
		t.Fatalf("after playing the 576p version: version %v, want it again", s.Version)
	}
	stop(s)
	var off id.ID
	if s, err = f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Caps: browser, ClientIP: lan, SubtitleTrack: &off}); err != nil || s.Version != smallVersion {
		t.Fatalf("subtitles off and no version: version %v, %v; want the last played", s.Version, err)
	}
	stop(s)

	// A version whose file is gone is passed over until the file returns.
	f.b.Write(func(ctx context.Context, tx *store.Tx) error { return f.b.MarkGone(ctx, tx, small) })
	if s = f.mustStart(m.Work); s.Version != m.Version {
		t.Fatalf("the last played version gone: version %v, want the first, %v", s.Version, m.Version)
	}
	stop(s)
	if _, err := f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Version: smallVersion, Caps: browser, ClientIP: lan}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the gone version named: %v, want not found", err)
	}
	f.b.File(f.folder, "Heat (2000)/Heat 576p.mkv")
	if s, err = f.svc.Start(ctx, f.p, f.v, StartRequest{Work: m.Work, Version: smallVersion, Caps: browser, ClientIP: lan}); err != nil || s.Version != smallVersion {
		t.Fatalf("the gone version's file back: version %v, %v; want it", s.Version, err)
	}
}

// trackID returns the ID of a file's embedded track of the given kind at
// the given stream index, or the zero ID.
func trackID(f catalog.File, kind media.TrackKind, index int) id.ID {
	if i := slices.IndexFunc(f.Tracks, func(t catalog.Track) bool { return t.Type == string(kind) && t.Index == index }); i >= 0 {
		return f.Tracks[i].ID
	}
	return id.ID{}
}
