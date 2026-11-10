package transcode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"iter"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
)

var update = flag.Bool("update", false, "rewrite golden files")

var ctx = context.Background()

// keyframesEvery returns keyframes at a fixed interval over d.
func keyframesEvery(d, every time.Duration) []media.Keyframe {
	var out []media.Keyframe
	for t := time.Duration(0); t < d; t += every {
		out = append(out, media.Keyframe{Time: t})
	}
	return out
}

type fakeKeys struct {
	keys     []media.Keyframe
	complete bool
}

func (k fakeKeys) Keyframes(context.Context, string) ([]media.Keyframe, bool, error) {
	return k.keys, k.complete, nil
}

// fakeMedia emits one fragment per keyframe from the request's start, or
// one per grid step for encoded video, at one fragment every `pace` of
// fake time. It can fail as a job starts or after a number of fragments.
// Its jobs wait while paused.
type fakeMedia struct {
	keys      []media.Keyframe
	duration  time.Duration
	pace      time.Duration
	drift     time.Duration   // added to each encoded fragment's start, once per fragment, the way a fixed GOP drifts
	failAt    int             // fail after this many fragments; 0 means never
	failWith  error           // the error failAt fails with; defaults to an encoder failure
	failStart error           // fail with this before the first fragment
	initAfter time.Duration   // how long a job takes to yield its init segment
	cues      []media.Cue     // the text track; each cue is read along with the fragment it starts in
	start     time.Duration   // the source's first timestamp, which copied cue times include
	lost      []time.Duration // keyframes a job from the start doesn't emit, because its plan is wrong there
	// early is ffmpeg's margin for B-frames: a copied job's input seek
	// (its Seek, or else its Start) lands on the last keyframe at or
	// before the seek less early.
	early time.Duration
	// swallow is how far past where a copied job's seek lands its first
	// fragment runs. ffmpeg's delay_moov, which AC-3 and E-AC-3 output
	// need, cuts the first fragment only at the second keyframe, so the
	// keyframes it spans don't start fragments of their own.
	swallow time.Duration
	missing []time.Duration // keyframes no job emits, as if the media went past them

	mu       sync.Mutex
	requests []media.TranscodeRequest
	converts []media.ConvertRequest
	pauses   int
	resumes  int
}

func (m *fakeMedia) Transcode(ctx context.Context, req media.TranscodeRequest) (media.Output, error) {
	m.mu.Lock()
	m.requests = append(m.requests, req)
	m.mu.Unlock()
	return &fakeOutput{ctx: ctx, m: m, req: req, running: make(chan struct{})}, nil
}

// Convert writes the file's name and its rendition's codec to Dst,
// reporting progress every 10 seconds. It fails the same way a job would.
func (m *fakeMedia) Convert(ctx context.Context, req media.ConvertRequest) error {
	m.mu.Lock()
	m.converts = append(m.converts, req)
	m.mu.Unlock()
	if m.failStart != nil {
		return m.failStart
	}
	for t := 10 * time.Second; t <= m.duration; t += 10 * time.Second {
		if m.failAt > 0 && t >= time.Duration(m.failAt)*10*time.Second {
			return errors.New("encoder exploded")
		}
		req.Reached(t)
	}
	return os.WriteFile(req.Dst, []byte(req.Path+" as "+string(req.Video.Codec)), 0o644)
}

func (m *fakeMedia) jobs() []media.TranscodeRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]media.TranscodeRequest(nil), m.requests...)
}

func (m *fakeMedia) pacing() (pauses, resumes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pauses, m.resumes
}

// starts lists where each fragment of a job begins.
func (m *fakeMedia) starts(req media.TranscodeRequest) []time.Duration {
	var out []time.Duration
	if req.Video.Codec == media.Copy {
		from := m.landing(req)
		for _, k := range m.keys {
			swallowed := req.Start > 0 && k.Time > from && k.Time < from+m.swallow
			if k.Time >= from && !swallowed && !slices.Contains(m.missing, k.Time) && !(req.Start == 0 && slices.Contains(m.lost, k.Time)) {
				out = append(out, k.Time)
			}
		}
		return out
	}
	for n, t := 0, req.Start; t < m.duration; n, t = n+1, t+req.SegmentDuration {
		out = append(out, t+time.Duration(n)*m.drift)
	}
	return out
}

// landing is the keyframe a copied job's input seek lands on.
func (m *fakeMedia) landing(req media.TranscodeRequest) time.Duration {
	if req.Start == 0 {
		return 0
	}
	seek := req.Start
	if req.Seek > 0 {
		seek = req.Seek
	}
	var at time.Duration
	for _, k := range m.keys {
		if k.Time <= seek-m.early {
			at = k.Time
		}
	}
	return at
}

type fakeOutput struct {
	ctx context.Context
	m   *fakeMedia
	req media.TranscodeRequest

	mu      sync.Mutex
	paused  bool
	running chan struct{} // Pause replaces it, and Resume closes it
	read    time.Duration // the end of the last fragment yielded
	cued    map[int]int   // how many cues Cues has returned, by text track
}

// Cues returns text track i's cues that start before the end of the last
// yielded fragment, from the job's first fragment onward. Every track has
// the same cues, each marked with its source track after the first.
func (o *fakeOutput) Cues(i int) ([]media.Cue, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if i >= len(o.req.Text) {
		return nil, nil
	}
	if o.cued == nil {
		o.cued = map[int]int{}
	}
	starts := o.m.starts(o.req)
	var out []media.Cue
	for ; o.cued[i] < len(o.m.cues) && o.m.cues[o.cued[i]].Start < o.read; o.cued[i]++ {
		if len(starts) > 0 && o.m.cues[o.cued[i]].Start >= starts[0] {
			c := o.m.cues[o.cued[i]]
			c.Start, c.End = c.Start+o.m.start, c.End+o.m.start
			if i > 0 {
				c.Text = fmt.Sprintf("track %d %s", o.req.Text[i].Track, c.Text)
			}
			out = append(out, c)
		}
	}
	return out, nil
}

func (o *fakeOutput) Init() ([]byte, error) {
	if o.m.initAfter > 0 {
		select {
		case <-o.ctx.Done():
			return nil, o.ctx.Err()
		case <-time.After(o.m.initAfter):
		}
	}
	if err := o.m.failStart; err != nil {
		return nil, err
	}
	return []byte("init"), nil
}

func (o *fakeOutput) Segments() iter.Seq2[media.Segment, error] {
	return func(yield func(media.Segment, error) bool) {
		for n, t := range o.m.starts(o.req) {
			o.mu.Lock()
			running, paused := o.running, o.paused
			o.mu.Unlock()
			if paused {
				select {
				case <-o.ctx.Done():
					yield(media.Segment{}, o.ctx.Err())
					return
				case <-running:
				}
			}
			select {
			case <-o.ctx.Done():
				yield(media.Segment{}, o.ctx.Err())
				return
			case <-time.After(o.m.pace):
			}
			if o.m.failAt > 0 && n >= o.m.failAt {
				err := o.m.failWith
				if err == nil {
					err = errors.New("encoder exploded")
				}
				yield(media.Segment{}, err)
				return
			}
			o.mu.Lock()
			o.read = o.m.duration
			if starts := o.m.starts(o.req); n+1 < len(starts) {
				o.read = starts[n+1]
			}
			o.mu.Unlock()
			if !yield(media.Segment{Start: t, Data: []byte(fmt.Sprintf("[%v]", t))}, nil) {
				return
			}
		}
	}
}

func (o *fakeOutput) Pause() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.m.mu.Lock()
	o.m.pauses++
	o.m.mu.Unlock()
	o.paused, o.running = true, make(chan struct{})
	return nil
}

func (o *fakeOutput) Resume() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.m.mu.Lock()
	o.m.resumes++
	o.m.mu.Unlock()
	if o.paused {
		o.paused = false
		close(o.running)
	}
	return nil
}

func (o *fakeOutput) Diagnostics() media.Diagnostics {
	return media.Diagnostics{Command: "ffmpeg (fake)"}
}
func (o *fakeOutput) Close() error { return nil }

// fixture is a 10-minute file with a keyframe every 2 s, which makes
// 6-second segments of three fragments each.
func fixture(t *testing.T, m *fakeMedia) (*Transcoder, Spec) {
	t.Helper()
	m.keys = keyframesEvery(10*time.Minute, 2*time.Second)
	m.duration = 10 * time.Minute
	if m.pace == 0 {
		m.pace = 100 * time.Millisecond
	}
	dir := t.TempDir()
	tr, err := New(Deps{Media: m, Keys: fakeKeys{keys: m.keys, complete: true}, Dir: filepath.Join(dir, "transcode"), Downloads: filepath.Join(dir, "downloads")}, Options{Floor: 1})
	if err != nil {
		t.Fatal(err)
	}
	return tr, Spec{Parts: []FileRef{{ID: id.New(), Path: "/m/a.mkv", Duration: 10 * time.Minute, Keyframes: m.keys}}, Video: media.VideoRendition{Codec: media.Copy}}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSegmentsAreTheirKeyframesFragments(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{}
		tr, spec := fixture(t, m)
		s, err := tr.Open(ctx, spec, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		p, err := s.Segment(ctx, "1.m4s")
		if err != nil {
			t.Fatal(err)
		}
		if got := read(t, p); got != "[6s][8s][10s]" {
			t.Fatalf("segment 1 = %q", got)
		}
		p, err = s.Segment(ctx, "init.mp4")
		if err != nil || read(t, p) != "init" {
			t.Fatalf("init %v", err)
		}
	})
}

func TestSegmentIsServedAsSoonAsItIsComplete(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{pace: time.Second}
		tr, spec := fixture(t, m)
		start := time.Now()
		s, _ := tr.Open(ctx, spec, 0)
		defer s.Close()
		if _, err := s.Segment(ctx, "0.m4s"); err != nil {
			t.Fatal(err)
		}
		// Segment 0 is three fragments, and the fourth (segment 1's first)
		// closes it. Nothing waits for segment 1 to finish.
		if got := time.Since(start); got != 4*time.Second {
			t.Fatalf("segment 0 served after %v, want 4s", got)
		}
	})
}

func TestEncodedSegmentIsServedWithItsFragment(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{pace: time.Second}
		tr, spec := fixture(t, m)
		start := time.Now()
		s, _ := tr.Open(ctx, encodedSpec(spec, ""), 0)
		defer s.Close()
		if _, err := s.Segment(ctx, "0.m4s"); err != nil {
			t.Fatal(err)
		}
		// Encoded video has one fragment per segment, so segment 0 is
		// served when its fragment is complete, not when segment 1's
		// arrives.
		if got := time.Since(start); got != time.Second {
			t.Fatalf("segment 0 served after %v, want 1s", got)
		}
	})
}

// A burned-in subtitle's fonts, wherever they're stored, are gathered into
// a folder in the stream's directory for libass, and are removed with the
// stream.
func TestBurnInFontsAreGathered(t *testing.T) {
	m := &fakeMedia{}
	tr, spec := fixture(t, m)
	store := t.TempDir()
	font := filepath.Join(store, "ab12.ttf")
	if err := os.WriteFile(font, []byte("font"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec = encodedSpec(spec, "")
	spec.BurnIn, spec.Fonts = &media.SubtitleRef{Path: "/cache/subtitles/3.ass"}, []string{font, filepath.Join(store, "gone.ttf")}
	s, err := tr.Open(ctx, spec, 0)
	if err != nil {
		t.Fatal(err)
	}
	dir := m.jobs()[0].BurnIn.Fonts
	if b, err := os.ReadFile(filepath.Join(dir, "ab12.ttf")); err != nil || string(b) != "font" {
		t.Fatalf("gathered %q %v in %q", b, err, dir)
	}
	if spec.BurnIn.Fonts != "" {
		t.Error("the caller's spec was changed")
	}
	s.Close()
	if _, err := os.Stat(dir); err == nil {
		t.Error("the fonts outlived the stream")
	}
	if _, err := os.Stat(font); err != nil {
		t.Error("the kept font went with the stream")
	}
}

func TestSeeks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{}
		tr, spec := fixture(t, m)
		s, _ := tr.Open(ctx, spec, 0)
		defer s.Close()
		for i := range 6 { // play from the start
			if _, err := s.Segment(ctx, segmentName(i)); err != nil {
				t.Fatal(err)
			}
		}

		// Seeking back into encoded segments serves them from disk, and
		// leaves the job alone.
		before := time.Now()
		if _, err := s.Segment(ctx, "2.m4s"); err != nil || time.Since(before) != 0 {
			t.Fatalf("a seek back waited %v, %v", time.Since(before), err)
		}
		if n := len(m.jobs()); n != 1 {
			t.Fatalf("%d jobs after a seek back", n)
		}

		// A little ahead of the job, the request waits for it.
		next := s.Job().Next
		if _, err := s.Segment(ctx, segmentName(next+nearSegments)); err != nil {
			t.Fatal(err)
		}
		if n := len(m.jobs()); n != 1 {
			t.Fatalf("a near seek started a job (%d)", n)
		}

		// Far ahead, the job restarts at the seek point.
		if _, err := s.Segment(ctx, "80.m4s"); err != nil {
			t.Fatal(err)
		}
		jobs := m.jobs()
		if len(jobs) != 2 || jobs[1].Start != s.plan[80].Start || s.Job().From != 80 {
			t.Fatalf("after a far seek: %d jobs, last from %v", len(jobs), jobs[len(jobs)-1].Start)
		}
		for _, r := range jobs {
			if r.Path != "/m/a.mkv" {
				t.Fatalf("job for %q", r.Path)
			}
		}

		// Behind the new job and not on disk, the job restarts there too.
		if _, err := s.Segment(ctx, "40.m4s"); err != nil || len(m.jobs()) != 3 {
			t.Fatalf("a seek back past the disk: %v, %d jobs", err, len(m.jobs()))
		}
	})
}

// bframeMargin is how far before its seek ffmpeg's input seek looks for a
// keyframe, for video with B-frames.
const bframeMargin = 3 * time.Second / 23

func TestSeekPast(t *testing.T) {
	s := func(f float64) time.Duration { return time.Duration(f * float64(time.Second)) }
	keys := []media.Keyframe{{Time: 0}, {Time: s(10.427)}, {Time: s(20.854)}, {Time: s(21.4)}, {Time: s(31.8)}}
	f := FileRef{Duration: s(32.2), Keyframes: keys}
	for _, tt := range []struct {
		name string
		k    time.Duration
		want time.Duration
	}{
		{"a normal GOP seeks maxSeekPast past its keyframe", s(10.427), s(10.927)},
		{"a GOP under a second seeks halfway through it", s(20.854), s(21.127)},
		{"the last keyframe seeks halfway to the end", s(31.8), s(32.0)},
		{"the start doesn't seek", 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := seekPast(f, tt.k); got != tt.want {
				t.Fatalf("seekPast(%v) = %v, want %v", tt.k, got, tt.want)
			}
		})
	}
}

// A copied job seeks just past its first keyframe, so ffmpeg's margin for
// B-frames brings the seek back to that keyframe rather than to the one
// before, and the first segment holds its own fragments.
func TestSeekLandsOnTheFirstSegmentsKeyframe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{early: bframeMargin}
		tr, spec := fixture(t, m)
		s, err := tr.Open(ctx, spec, 8*time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		for _, name := range []string{"80.m4s", "81.m4s"} {
			if _, err := s.Segment(ctx, name); err != nil {
				t.Fatalf("segment %s: %v", name, err)
			}
		}
		jobs := m.jobs()
		if len(jobs) != 1 || jobs[0].Start != 8*time.Minute || jobs[0].Seek != 8*time.Minute+maxSeekPast {
			t.Fatalf("jobs %+v, want one from 8m0s seeking to 8m0.5s", jobs)
		}
		if got := read(t, s.path(80)); got != "[8m0s][8m2s][8m4s]" {
			t.Fatalf("segment 80 = %q", got)
		}
		s.mu.Lock()
		done := s.done[79]
		s.mu.Unlock()
		if done {
			t.Fatal("segment 79 was written")
		}
	})
}

// With delay_moov, ffmpeg runs its first fragment on past the keyframe
// after the one it begins with. Seeking to the first segment's keyframe,
// that fragment holds the whole first segment, which is written from it.
// A seek to the keyframe itself would have landed on the one before, and
// the fragment would have begun in the segment before.
func TestSeekLandingEarlyInOneFragment(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{early: bframeMargin, swallow: 6 * time.Second}
		tr, spec := fixture(t, m)
		s, err := tr.Open(ctx, spec, 8*time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		c, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		if _, err := s.Segment(c, "80.m4s"); err != nil {
			t.Fatalf("segment 80: %v after %d jobs", err, len(m.jobs()))
		}
		if got := read(t, s.path(80)); got != "[8m0s]" {
			t.Fatalf("segment 80 = %q", got)
		}
		if n := len(m.jobs()); n != 1 {
			t.Fatalf("%d jobs", n)
		}
	})
}

// A copied seek that lands on the keyframe before its first segment's
// fails the job with its first fragment, and logs one warning that names
// the file and the times. Requests for the segment report the failure, so
// playback falls back, and don't start the job again.
func TestFragmentBeforeTheFirstSegmentFailsTheJob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{early: 2 * time.Second}
		tr, spec := fixture(t, m)
		s, err := tr.Open(ctx, spec, 8*time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		var log lockedBuffer
		old := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&log, nil)))
		defer slog.SetDefault(old)
		c, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		start := time.Now()
		for range 2 {
			if _, err := s.Segment(c, "80.m4s"); !errors.Is(err, ErrJobFailed) || !errors.Is(err, errBeganEarly) {
				t.Fatalf("segment 80: %v, want ErrJobFailed for errBeganEarly", err)
			}
		}
		if got := time.Since(start); got != m.pace {
			t.Fatalf("failed after %v, want %v", got, m.pace)
		}
		if n := len(m.jobs()); n != 1 {
			t.Fatalf("%d jobs", n)
		}
		warns := strings.Split(strings.TrimSpace(log.String()), "\n")
		if len(warns) != 1 || !strings.Contains(warns[0], "level=WARN") {
			t.Fatalf("log %q, want one warning", warns)
		}
		for _, want := range []string{spec.Parts[0].ID.String(), "seeking to 8m0.5s", "segment 80 at 8m0s", "began at 7m58s"} {
			if !strings.Contains(warns[0], want) {
				t.Errorf("warning %q doesn't say %q", warns[0], want)
			}
		}
	})
}

// A job that goes past a segment without writing it fails the request for
// it at once. Starting a job there again would only go past it again.
func TestPassedSegmentFailsFast(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{early: bframeMargin, missing: []time.Duration{480 * time.Second, 482 * time.Second, 484 * time.Second}}
		tr, spec := fixture(t, m)
		s, err := tr.Open(ctx, spec, 8*time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		var log lockedBuffer
		old := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&log, nil)))
		defer slog.SetDefault(old)
		c, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		start := time.Now()
		if _, err := s.Segment(c, "80.m4s"); !errors.Is(err, ErrJobFailed) {
			t.Fatalf("segment 80: %v, want ErrJobFailed", err)
		}
		// Segment 81's three fragments, and the next one, which closes it.
		if got := time.Since(start); got != 4*m.pace {
			t.Fatalf("failed after %v, want %v", got, 4*m.pace)
		}
		if _, err := s.Segment(c, "80.m4s"); !errors.Is(err, ErrJobFailed) {
			t.Fatalf("segment 80 again: %v, want ErrJobFailed", err)
		}
		if n := len(m.jobs()); n != 1 {
			t.Fatalf("%d jobs", n)
		}
		if _, err := s.Segment(c, "81.m4s"); err != nil {
			t.Fatalf("segment 81: %v", err)
		}
		if n := strings.Count(log.String(), "passed a segment"); n != 1 {
			t.Fatalf("%d warnings for two refusals, want 1", n)
		}
	})
}

// lockedBuffer collects what goroutines log.
type lockedBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// cuesEvery returns a 1.2 s cue every interval over d, each starting half
// a second into its interval. It adds one cue from 5 s to 7 s, across the
// boundary of the first segments.
func cuesEvery(d, every time.Duration) []media.Cue {
	var out []media.Cue
	for t := every / 4; t < d; t += every {
		out = append(out, media.Cue{Start: t, End: t + 1200*time.Millisecond, Text: fmt.Sprintf("at %v", t)})
		if t == 4500*time.Millisecond {
			out = append(out, media.Cue{Start: 5 * time.Second, End: 7 * time.Second, Text: "across"})
		}
	}
	return out
}

func TestTextSegmentsComeWithTheirVideo(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{cues: cuesEvery(10*time.Minute, 2*time.Second)}
		tr, spec := fixture(t, m)
		spec.Text = []TextTrack{{ID: "t2", Track: 2, Title: "English", Language: "en", Default: true}}
		s, err := tr.Open(ctx, spec, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if pl, _ := s.Playlist(PlaylistName); !strings.Contains(string(pl), `URI="text-0.m3u8"`) {
			t.Fatalf("the multivariant playlist names no subtitles:\n%s", pl)
		}
		if pl, err := s.Playlist(textPlaylist(0)); err != nil || !strings.Contains(string(pl), "#EXTINF:6.000,\nt0-1.vtt\n") {
			t.Fatalf("text playlist: %v\n%s", err, pl)
		}
		if _, err := s.Segment(ctx, "0.m4s"); err != nil {
			t.Fatal(err)
		}
		// The text segment is on disk once its video is, so there's no wait
		// and no new job.
		before := time.Now()
		p, err := s.Segment(ctx, "t0-0.vtt")
		if err != nil || time.Since(before) != 0 {
			t.Fatalf("text segment 0: %v after %v", err, time.Since(before))
		}
		want := "WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:900000,LOCAL:00:00:00.000\n\n" +
			"00:00:00.500 --> 00:00:01.700\nat 500ms\n\n00:00:02.500 --> 00:00:03.700\nat 2.5s\n\n" +
			"00:00:04.500 --> 00:00:05.700\nat 4.5s\n\n00:00:05.000 --> 00:00:06.000\nacross\n"
		if got := read(t, p); got != want {
			t.Fatalf("text segment 0:\n%s\nwant:\n%s", got, want)
		}
		// A cue across a boundary is in both segments, cut to each one.
		p, err = s.Segment(ctx, "t0-1.vtt")
		if got := read(t, p); err != nil || !strings.Contains(got, "\n00:00:06.000 --> 00:00:07.000\nacross\n") || !strings.Contains(got, "at 10.5s") || strings.Contains(got, "at 4.5s") {
			t.Fatalf("text segment 1: %v\n%s", err, got)
		}

		// A far seek requested by the text track restarts the job there
		// with the track, and the text segments come from the new job.
		p, err = s.Segment(ctx, "t0-80.vtt")
		if err != nil {
			t.Fatal(err)
		}
		jobs := m.jobs()
		if len(jobs) != 2 || jobs[1].Start != s.plan[80].Start || len(jobs[1].Text) != 1 || jobs[1].Text[0].Track != 2 ||
			filepath.Dir(jobs[1].Text[0].Path) != s.dir || jobs[1].Text[0].Path == jobs[0].Text[0].Path {
			t.Fatalf("after a far seek: %+v", jobs)
		}
		if got := read(t, p); !strings.Contains(got, "00:08:00.500 --> 00:08:01.700\nat 8m0.5s") || strings.Contains(got, "at 4.5s") {
			t.Fatalf("text segment 80:\n%s", got)
		}
		if _, err := os.Stat(s.path(80)); err != nil {
			t.Fatalf("its video: %v", err)
		}
	})
}

// A source whose timestamps start late, such as an MPEG-TS recording,
// gives cues on those timestamps. Each segment takes the cues in its span
// shifted by the start time, so they're spread out like the video rather
// than all landing in the last segment. The cues keep their own times.
func TestTextCuesFollowTheSourceStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{cues: cuesEvery(10*time.Minute, 2*time.Second), start: 3 * time.Hour}
		tr, spec := fixture(t, m)
		spec.Parts[0].Start = m.start
		spec.Text = []TextTrack{{Track: 2, Title: "English", Language: "en"}}
		s, err := tr.Open(ctx, spec, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		p, err := s.Segment(ctx, "t0-0.vtt")
		if err != nil {
			t.Fatal(err)
		}
		if got := read(t, p); !strings.Contains(got, "03:00:00.500 --> 03:00:01.700\nat 500ms") || strings.Contains(got, "at 10.5s") {
			t.Fatalf("text segment 0:\n%s", got)
		}
		p, err = s.Segment(ctx, "t0-1.vtt")
		if got := read(t, p); err != nil || !strings.Contains(got, "03:00:10.500 --> 03:00:11.700\nat 10.5s") || strings.Contains(got, "at 500ms") {
			t.Fatalf("text segment 1: %v\n%s", err, got)
		}
	})
}

// A subtitle file's cues land in the segments they show in, shifted onto
// the source's timestamps and cut to each segment. Overlapping rolling
// captions show as two lines, each once. A far seek finds them the same
// way.
func TestSubtitleFileSegments(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{start: 3 * time.Hour}
		tr, spec := fixture(t, m)
		spec.Parts[0].Start = m.start
		path := filepath.Join(t.TempDir(), "a.en.srt")
		srt := "263\n00:00:03,110 --> 00:00:10,729\nhis advice the lousy curve he took my\n\n" +
			"264\n00:00:06,980 --> 00:00:13,270\n{\\an8}ball dog father helped me I'll talk to\n\n" +
			"265\n00:00:10,730 --> 00:00:13,270\nhim\n\n" +
			"1\n00:07:59,000 --> 00:08:01,000\nacross a far boundary\n"
		if err := os.WriteFile(path, []byte(srt), 0o644); err != nil {
			t.Fatal(err)
		}
		spec.Text = []TextTrack{{Path: path, Format: media.SRT, Title: "English", Language: "en"}}
		s, err := tr.Open(ctx, spec, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		want := map[string]string{
			"t0-0.vtt": "\n03:00:03.110 --> 03:00:06.000\nhis advice the lousy curve he took my\n",
			"t0-1.vtt": "\n03:00:06.000 --> 03:00:10.729\nhis advice the lousy curve he took my\n" +
				"\n03:00:06.980 --> 03:00:12.000 line:0\nball dog father helped me I'll talk to\n" +
				"\n03:00:10.730 --> 03:00:12.000\nhim\n",
			"t0-2.vtt": "\n03:00:12.000 --> 03:00:13.270 line:0\nball dog father helped me I'll talk to\n" +
				"\n03:00:12.000 --> 03:00:13.270\nhim\n",
			"t0-79.vtt": "\n03:07:59.000 --> 03:08:00.000\nacross a far boundary\n",
			"t0-80.vtt": "\n03:08:00.000 --> 03:08:01.000\nacross a far boundary\n",
		}
		for _, name := range []string{"t0-0.vtt", "t0-1.vtt", "t0-2.vtt", "t0-80.vtt", "t0-79.vtt"} {
			p, err := s.Segment(ctx, name)
			if err != nil {
				t.Fatal(err)
			}
			if got := read(t, p); got != "WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:900000,LOCAL:00:00:00.000\n"+want[name] {
				t.Errorf("%s:\n%s\nwant:\n%s", name, got, want[name])
			}
		}
		for _, j := range m.jobs() {
			if len(j.Text) != 0 {
				t.Fatalf("a job reads the subtitle file: %+v", j.Text)
			}
		}
	})
}

// A stream carries every text track as a rendition: the job reads the
// embedded ones, each to its own scratch file, and the stream reads a file
// track whole. Every track's segment is written with its video, a far seek
// gives each track's segments from the new job, and only the default
// rendition is marked so.
func TestTextRenditions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{cues: cuesEvery(10*time.Minute, 2*time.Second)}
		tr, spec := fixture(t, m)
		path := filepath.Join(t.TempDir(), "a.fr.srt")
		if err := os.WriteFile(path, []byte("1\n00:00:01,000 --> 00:00:02,000\nbonjour\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		spec.Text = []TextTrack{
			{ID: "t2", Track: 2, Title: "English", Language: "en"},
			{ID: "t3", Track: 3, Title: "English", Language: "en", SDH: true, Default: true},
			{ID: "x1", Track: -1, Path: path, Format: media.SRT, Title: "French", Language: "fr", Forced: true},
		}
		s, err := tr.Open(ctx, spec, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		pl, _ := s.Playlist(PlaylistName)
		for _, want := range []string{
			`NAME="English",LANGUAGE="en",DEFAULT=NO,AUTOSELECT=YES,FORCED=NO,STABLE-RENDITION-ID="t2",URI="text-0.m3u8"`,
			`NAME="English 2",LANGUAGE="en",DEFAULT=YES,AUTOSELECT=YES,FORCED=NO,CHARACTERISTICS="` + sdhCharacteristics + `",STABLE-RENDITION-ID="t3",URI="text-1.m3u8"`,
			`NAME="French",LANGUAGE="fr",DEFAULT=NO,AUTOSELECT=YES,FORCED=YES,STABLE-RENDITION-ID="x1",URI="text-2.m3u8"`,
		} {
			if !strings.Contains(string(pl), want) {
				t.Fatalf("the multivariant playlist lacks %s:\n%s", want, pl)
			}
		}
		if _, err := s.Segment(ctx, "0.m4s"); err != nil {
			t.Fatal(err)
		}
		for name, want := range map[string]string{"t0-0.vtt": "\nat 500ms\n", "t1-0.vtt": "\ntrack 3 at 500ms\n", "t2-0.vtt": "\nbonjour\n"} {
			p, err := s.Segment(ctx, name)
			if got := read(t, p); err != nil || !strings.Contains(got, want) {
				t.Fatalf("%s: %v\n%s", name, err, got)
			}
		}
		if _, err := s.Segment(ctx, "t3-0.vtt"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("a fourth track's segment: %v", err)
		}
		if _, err := s.Segment(ctx, "t1-80.vtt"); err != nil {
			t.Fatal(err)
		}
		jobs := m.jobs()
		if len(jobs) != 2 || len(jobs[1].Text) != 2 || jobs[1].Text[0].Track != 2 || jobs[1].Text[1].Track != 3 ||
			jobs[1].Text[0].Path == jobs[1].Text[1].Path || jobs[1].Text[0].Path == jobs[0].Text[0].Path {
			t.Fatalf("after a far seek: %+v", jobs)
		}
		for _, name := range []string{"t0-80.vtt", "t2-80.vtt"} {
			if _, err := os.Stat(filepath.Join(s.dir, name)); err != nil {
				t.Fatalf("%s isn't written with its video: %v", name, err)
			}
		}
	})
}

func TestNoTextTrackNoTextSegments(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{cues: cuesEvery(time.Minute, 2*time.Second)}
		tr, spec := fixture(t, m)
		s, _ := tr.Open(ctx, spec, 0)
		defer s.Close()
		if _, err := s.Segment(ctx, "t0-0.vtt"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("a text segment without a text track: %v", err)
		}
		if _, err := s.Playlist(textPlaylist(0)); !errors.Is(err, ErrNotFound) {
			t.Fatalf("a text playlist without a text track: %v", err)
		}
		if pl, _ := s.Playlist(PlaylistName); strings.Contains(string(pl), "SUBTITLES") {
			t.Fatalf("multivariant playlist:\n%s", pl)
		}
		synctest.Wait()
		if len(m.jobs()[0].Text) != 0 {
			t.Fatal("the job reads a text track")
		}
	})
}

func TestJobDeathWakesWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{failAt: 4}
		tr, spec := fixture(t, m)
		s, _ := tr.Open(ctx, spec, 0)
		defer s.Close()
		if _, err := s.Segment(ctx, "2.m4s"); !errors.Is(err, ErrJobFailed) {
			t.Fatalf("waiting on a job that dies: %v", err)
		}
		if _, err := s.Segment(ctx, "0.m4s"); err != nil {
			t.Fatalf("a segment the job did finish: %v", err)
		}
		if _, err := s.Segment(ctx, "50.m4s"); !errors.Is(err, ErrJobFailed) || len(m.jobs()) != 1 {
			t.Fatalf("a failed job must be reported, not restarted: %v, %d jobs", err, len(m.jobs()))
		}
	})
}

func TestSkippedSegmentWakesItsWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// The job from the start jumps from segment 1 to segment 3. A
		// request waiting on segment 2 starts a job there instead of
		// waiting forever.
		m := &fakeMedia{lost: []time.Duration{12 * time.Second, 14 * time.Second, 16 * time.Second}}
		tr, spec := fixture(t, m)
		s, _ := tr.Open(ctx, spec, 0)
		defer s.Close()
		rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		p, err := s.Segment(rctx, "2.m4s")
		if err != nil {
			t.Fatalf("waiting on a skipped segment: %v", err)
		}
		if got := read(t, p); got != "[12s][14s][16s]" || len(m.jobs()) != 2 {
			t.Fatalf("segment 2 = %q from %d jobs", got, len(m.jobs()))
		}
	})
}

// A far seek replaces the job under a request that's waiting on it. The
// request looks again, and the new job serves it.
func TestReplacedJobsWaiterIsServedByTheNewJob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{}
		tr, spec := fixture(t, m)
		s, _ := tr.Open(ctx, spec, time.Minute) // a job from segment 10
		defer s.Close()
		type result struct {
			path string
			err  error
		}
		waiter := make(chan result, 1)
		go func() {
			p, err := s.Segment(ctx, "12.m4s")
			waiter <- result{p, err}
		}()
		synctest.Wait()
		if n := len(m.jobs()); n != 1 {
			t.Fatalf("a near request started a job (%d)", n)
		}
		// Segment 9 is behind the job, so the job restarts there, which is
		// near enough to segment 12 to reach it.
		if _, err := s.Segment(ctx, "9.m4s"); err != nil {
			t.Fatal(err)
		}
		r := <-waiter
		if r.err != nil || read(t, r.path) != "[1m12s][1m14s][1m16s]" {
			t.Fatalf("the waiter on the replaced job: %v", r.err)
		}
		if jobs := m.jobs(); len(jobs) != 2 || jobs[1].Start != s.plan[9].Start {
			t.Fatalf("%d jobs, want the second from segment 9", len(jobs))
		}
	})
}

// A job that's stopped under a waiting request doesn't fail it. The request
// starts a new job.
func TestStoppedJobsWaiterStartsANewJob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{pace: idleTimeout} // the idle kill comes before the first segment
		tr, spec := fixture(t, m)
		s, _ := tr.Open(ctx, spec, 0)
		defer s.Close()
		if _, err := s.Segment(ctx, "0.m4s"); err != nil || len(m.jobs()) != 2 {
			t.Fatalf("waiting through an idle kill: %v, %d jobs", err, len(m.jobs()))
		}
	})
}

// Close fails every waiting request, for a segment or an init segment.
func TestCloseWakesWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{initAfter: time.Minute}
		tr, spec := fixture(t, m)
		s, _ := tr.Open(ctx, spec, 0)
		errs := make(chan error, 3)
		for _, name := range []string{"init.mp4", "0.m4s", "2.m4s"} {
			go func() {
				_, err := s.Segment(ctx, name)
				errs <- err
			}()
		}
		synctest.Wait()
		start := time.Now()
		s.Close()
		for range 3 {
			if err := <-errs; !errors.Is(err, ErrClosed) {
				t.Fatalf("a waiter on a closed stream: %v", err)
			}
		}
		if time.Since(start) != 0 || len(m.jobs()) != 1 {
			t.Fatalf("waiters woke after %v with %d jobs", time.Since(start), len(m.jobs()))
		}
	})
}

// A failed job fails the init requests waiting on it and those that come
// later, rather than starting again.
func TestJobFailureFailsInitRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{initAfter: time.Second, failStart: &media.Error{Op: "transcode", Kind: media.ErrSourceCorrupt}}
		tr, spec := fixture(t, m)
		s, _ := tr.Open(ctx, spec, 0)
		defer s.Close()
		start := time.Now()
		if _, err := s.Segment(ctx, "init.mp4"); !errors.Is(err, ErrJobFailed) || time.Since(start) != time.Second {
			t.Fatalf("waiting on a job that fails: %v after %v", err, time.Since(start))
		}
		if _, err := s.Segment(ctx, "init.mp4"); !errors.Is(err, ErrJobFailed) || len(m.jobs()) != 1 {
			t.Fatalf("after the failure: %v, %d jobs", err, len(m.jobs()))
		}
	})
}

func TestCancelledInitRequestStopsWaiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{initAfter: time.Second}
		tr, spec := fixture(t, m)
		s, _ := tr.Open(ctx, spec, 0)
		defer s.Close()
		rctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		if _, err := s.Segment(rctx, "init.mp4"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err %v", err)
		}
		if p, err := s.Segment(ctx, "init.mp4"); err != nil || read(t, p) != "init" || len(m.jobs()) != 1 {
			t.Fatalf("the init segment once it's written: %v, %d jobs", err, len(m.jobs()))
		}
	})
}

// A job that fails after its first segment has still started.
func TestFailureAfterTheFirstSegmentHasStarted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{failAt: 4}
		tr, spec := fixture(t, m)
		s, _ := tr.Open(ctx, spec, 0)
		defer s.Close()
		s.mu.Lock()
		j := s.job
		s.mu.Unlock()
		time.Sleep(time.Minute)
		if s.Job().State != JobFailed {
			t.Fatalf("job %+v", s.Job())
		}
		if err := s.started(ctx, j); err != nil {
			t.Fatalf("started: %v", err)
		}
	})
}

func TestCancelledRequestStopsWaiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{pace: time.Second}
		tr, spec := fixture(t, m)
		s, _ := tr.Open(ctx, spec, 0)
		defer s.Close()
		rctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		if _, err := s.Segment(rctx, "1.m4s"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err %v", err)
		}
	})
}

func TestIdleKillsTheJobNotTheStream(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{pace: 2 * time.Second} // real time, so the job doesn't reach the end first
		tr, spec := fixture(t, m)
		s, _ := tr.Open(ctx, spec, 0)
		defer s.Close()
		for range 3 {
			time.Sleep(time.Minute)
			s.Touch() // heartbeats keep it alive
		}
		synctest.Wait()
		if s.Job().State == "" { // the job should be running, or paused well ahead of the player
			t.Fatal("killed while heartbeats came in")
		}
		time.Sleep(idleTimeout + time.Second)
		synctest.Wait()
		if s.Job().State != "" {
			t.Fatal("an idle stream's job is still running")
		}
		if _, err := s.Segment(ctx, "90.m4s"); err != nil || len(m.jobs()) != 2 {
			t.Fatalf("the next request after an idle kill: %v, %d jobs", err, len(m.jobs()))
		}
	})
}

func TestCloseRemovesTheStreamAndFloorRefuses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{}
		tr, spec := fixture(t, m)
		s, _ := tr.Open(ctx, spec, 0)
		if _, err := s.Segment(ctx, "1.m4s"); err != nil {
			t.Fatal(err)
		}
		dir := s.dir
		s.Close()
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stream directory left behind: %v", err)
		}
		if _, err := s.Segment(ctx, "1.m4s"); !errors.Is(err, ErrClosed) {
			t.Fatalf("after close: %v", err)
		}

		tr.st.floor = math.MaxUint64
		if _, err := tr.Open(ctx, spec, 0); !errors.Is(err, ErrStorageFull) {
			t.Fatalf("below the floor: %v", err)
		}
	})
}

func TestStartupClearsStreamsAndKeepsDownloads(t *testing.T) {
	dir, downloads := filepath.Join(t.TempDir(), "transcode"), filepath.Join(t.TempDir(), "downloads")
	for _, d := range []string{dir, downloads} {
		if err := os.MkdirAll(filepath.Join(d, "left-by-a-crash"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tr, err := New(Deps{Media: &fakeMedia{}, Keys: fakeKeys{}, Dir: dir, Downloads: downloads}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("a stream's segments outlived a restart: %v", entries)
	}
	if names, err := tr.Downloads(); err != nil || !slices.Equal(names, []string{"left-by-a-crash"}) {
		t.Fatalf("downloads %v %v", names, err)
	}
}

func TestRemuxNeedsCloseKeyframes(t *testing.T) {
	m := &fakeMedia{}
	tr, spec := fixture(t, m)
	if keys := tr.RemuxKeyframes(ctx, spec.Parts[0]); len(keys) != len(m.keys) {
		t.Fatalf("2-second GOPs: %d keyframes", len(keys))
	}
	tr.deps.Keys = fakeKeys{keys: keyframesEvery(10*time.Minute, 30*time.Second), complete: true}
	if keys := tr.RemuxKeyframes(ctx, spec.Parts[0]); keys != nil {
		t.Fatalf("30-second GOPs: %d keyframes", len(keys))
	}
	tr.deps.Keys = fakeKeys{keys: m.keys, complete: false}
	if keys := tr.RemuxKeyframes(ctx, spec.Parts[0]); keys != nil {
		t.Fatalf("an incomplete index: %d keyframes", len(keys))
	}
	spec.Parts[0].Keyframes = nil
	if _, err := tr.Open(ctx, spec, 0); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("a remux without keyframes: %v", err)
	}
}

func TestPlaylistGolden(t *testing.T) {
	// Irregular keyframes, like a real remux has: a short GOP, a long one,
	// and a tail shorter than a segment.
	keys := []media.Keyframe{{Time: 0}, {Time: 2 * time.Second}, {Time: 4 * time.Second}, {Time: 7 * time.Second}, {Time: 9500 * time.Millisecond},
		{Time: 12 * time.Second}, {Time: 21 * time.Second}, {Time: 23 * time.Second}, {Time: 29 * time.Second}}
	plan := planKeyframes(31*time.Second+500*time.Millisecond, keys)
	spec := Spec{
		Parts:  []FileRef{{Path: "/m/a.mkv", Duration: 31*time.Second + 500*time.Millisecond, Bitrate: 9_800_000}},
		Source: media.Track{Kind: media.Video, Codec: media.H264, Profile: "High", Level: 41},
		Video:  media.VideoRendition{Codec: media.Copy},
		Audio:  []media.AudioOut{{Track: 1, Source: "eac3", Out: media.AudioRendition{Codec: media.Copy}}},
		Text: []TextTrack{
			{ID: "0190b0a8-0000-7000-8000-000000000003", Track: 3, Title: `English "SDH"`, Language: "en", SDH: true, Default: true},
			{ID: "0190b0a8-0000-7000-8000-000000000004", Track: 4, Title: "Français", Language: "fr", Forced: true},
		},
	}
	playlists := renderPlaylists(spec, plan)
	var got []byte
	for _, name := range []string{PlaylistName, videoPlaylist, textPlaylist(0), textPlaylist(1)} {
		got = append(append(got, "# "+name+"\n"...), playlists[name]...)
	}
	golden := filepath.Join("testdata", "playlist.golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("playlist:\n%s\nwant:\n%s", got, want)
	}
}

func TestCodecs(t *testing.T) {
	copied := func(src media.Track, audio ...media.Codec) Spec {
		s := Spec{Source: src, Video: media.VideoRendition{Codec: media.Copy}}
		for _, c := range audio {
			s.Audio = append(s.Audio, media.AudioOut{Source: c, Out: media.AudioRendition{Codec: media.Copy}})
		}
		return s
	}
	hd := media.Track{Kind: media.Video, Codec: media.HEVC, Width: 3840, Height: 2160, FrameRate: 23.976}
	for _, c := range []struct {
		name string
		spec Spec
		want string
	}{
		{"copied H.264 High 4.1, AAC", copied(media.Track{Codec: media.H264, Profile: "High", Level: 41}, media.AAC), "avc1.640029,mp4a.40.2"},
		{"copied H.264 Constrained Baseline 3.0", copied(media.Track{Codec: media.H264, Profile: "Constrained Baseline", Level: 30}), "avc1.42E01E"},
		{"copied HEVC Main 10 5.1, E-AC-3", copied(media.Track{Codec: media.HEVC, Profile: "Main 10", Level: 153}, "eac3"), "hvc1.2.4.L153.B0,ec-3"},
		{"copied HEVC Main 4.0, AC-3", copied(media.Track{Codec: media.HEVC, Profile: "Main", Level: 120}, "ac3"), "hvc1.1.6.L120.B0,ac-3"},
		{"audio twice", copied(media.Track{Codec: media.H264, Profile: "Main", Level: 40}, media.AAC, media.AAC), "avc1.4D4028,mp4a.40.2"},
		{"no level", copied(media.Track{Codec: media.H264, Profile: "High"}), ""},
		{"unknown profile", copied(media.Track{Codec: media.HEVC, Profile: "Rext", Level: 153}), ""},
		{"unknown audio", copied(media.Track{Codec: media.H264, Profile: "High", Level: 41}, "dts"), ""},
		{"Dolby Vision 5 kept", copied(media.Track{Codec: media.HEVC, Profile: "Main 10", Level: 153, DolbyVisionProfile: 5}), ""},
		{"encoded 1080p H.264 from 4K, AAC", Spec{Source: hd, Video: media.VideoRendition{Codec: media.H264, Width: 1920, Height: 1080},
			Audio: []media.AudioOut{{Source: "truehd", Out: media.AudioRendition{Codec: media.AAC}}}}, "avc1.640029,mp4a.40.2"},
		{"encoded 4K HEVC", Spec{Source: hd, Video: media.VideoRendition{Codec: media.HEVC}}, "hvc1.1.6.L150.B0"},
		{"encoded 720p main H.264", Spec{Source: hd, Video: media.VideoRendition{Codec: media.H264, Profile: "main", Height: 720}}, "avc1.4D401F"},
	} {
		if got := c.spec.codecs(); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSegmentNames(t *testing.T) {
	for name, want := range map[string]struct{ i, text int }{
		"0.m4s": {0, -1}, "12.m4s": {12, -1}, "t0-12.vtt": {12, 0}, "t3-0.vtt": {0, 3},
	} {
		if i, text, ok := parseSegment(name); !ok || i != want.i || text != want.text {
			t.Errorf("%s: %d %v %v", name, i, text, ok)
		}
	}
	for _, bad := range []string{"-1.m4s", "01.m4s", "a.m4s", "1.ts", "../1.m4s", "1.m4s/x", "01.vtt", "1.m4s.vtt",
		"12.vtt", "t-1.vtt", "t01-1.vtt", "t1-01.vtt", "t1.vtt", "x1-1.vtt", "t1-1-1.vtt", "tt1-1.vtt"} {
		if _, _, ok := parseSegment(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestStoppedIsNotFailed(t *testing.T) {
	for _, err := range []error{context.Canceled, fmt.Errorf("transcode x: %w", media.ErrCancelled)} {
		if !stopped(err) {
			t.Errorf("%v counts as a failure", err)
		}
	}
	if stopped(errors.New("encoder exploded")) {
		t.Error("a real failure counts as stopped")
	}
}

func TestRemuxableAcceptsCommonGOPs(t *testing.T) {
	gop := 250 * time.Second * 1001 / 24000 // x264's default at 23.976 fps, which is 10.43 s
	keys := keyframesEvery(2*time.Hour, gop)
	if !remuxable(2*time.Hour, keys, true) {
		t.Fatal("250-frame GOPs at 23.976 fps should remux")
	}
	for _, s := range planKeyframes(2*time.Hour, keys) {
		if s.Duration > gop+time.Millisecond {
			t.Fatalf("segment of %v from %v GOPs", s.Duration, gop)
		}
	}
}

// encodedSpec is the fixture's spec, encoding video to H.264 on device.
func encodedSpec(spec Spec, accelerator media.Accelerator) Spec {
	spec.Source = media.Track{Kind: media.Video, Codec: "hevc", Width: 3840, Height: 2160, BitDepth: 10}
	spec.Video = media.VideoRendition{Codec: media.H264, Height: 1080, Bitrate: 8_000_000}
	spec.Accelerator = accelerator
	return spec
}

func TestVideoTranscodeIsOnTheGrid(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{}
		tr, spec := fixture(t, m)
		// Encoded video never uses the source's keyframes.
		m.keys, spec.Parts[0].Keyframes = nil, nil
		m.drift = 2 * time.Second // far worse than a fixed GOP's drift, to show that drift doesn't matter
		s, err := tr.Open(ctx, encodedSpec(spec, ""), 61*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		minute := int(time.Minute / gridSegment)
		if len(s.plan) != int(10*time.Minute/gridSegment) || s.plan[minute].Start != time.Minute {
			t.Fatalf("plan: %d segments, the one at a minute at %v", len(s.plan), s.plan[minute].Start)
		}
		jobs := m.jobs()
		if len(jobs) != 1 || jobs[0].Start != time.Minute || jobs[0].SegmentDuration != gridSegment || jobs[0].Source.Codec != "hevc" {
			t.Fatalf("job request %+v", jobs)
		}
		s.mu.Lock()
		first := s.done[minute]
		s.mu.Unlock()
		if !first {
			t.Fatal("Open returned before the first segment")
		}
		p, err := s.Segment(ctx, segmentName(minute+3))
		if err != nil {
			t.Fatal(err)
		}
		// Three segments on the grid past one minute, plus three fragments'
		// worth of drift.
		if got, want := read(t, p), fmt.Sprintf("[%v]", time.Minute+3*gridSegment+3*m.drift); got != want {
			t.Fatalf("segment %d = %q, want %q", minute+3, got, want)
		}
	})
}

func TestPacingPausesAheadAndResumes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		at := func(d time.Duration) int { return int(d / gridSegment) } // the segment at d
		m := &fakeMedia{}
		tr, spec := fixture(t, m)
		s, err := tr.Open(ctx, encodedSpec(spec, ""), 0)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if _, err := s.Segment(ctx, "0.m4s"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Minute) // far longer than encoding 90 seconds takes
		synctest.Wait()
		j := s.Job()
		if j.State != JobPaused || j.Next != at(90*time.Second) {
			t.Fatalf("with the player at 0 s: %s at segment %d, want paused at %d (90 s)", j.State, j.Next, at(90*time.Second))
		}
		// A 2 s segment every 100 ms is 20× realtime, and the pause doesn't
		// count.
		if j.Encoded != 90*time.Second || j.Speed != 20 {
			t.Fatalf("encoded to %v at %.2f×; want 90 s at 20×", j.Encoded, j.Speed)
		}

		// The player reads on, but stays more than 60 s behind, so the job
		// stays paused.
		if _, err := s.Segment(ctx, segmentName(at(24*time.Second))); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if s.Job().State != JobPaused {
			t.Fatal("resumed while the player was 66 s behind")
		}
		// Within 60 s the job resumes, then pauses again 90 s ahead of the
		// player.
		if _, err := s.Segment(ctx, segmentName(at(36*time.Second))); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if j := s.Job(); j.State != JobPaused || j.Next != at(126*time.Second) {
			t.Fatalf("after catching up: %s at segment %d, want paused at %d", j.State, j.Next, at(126*time.Second))
		}
		if pauses, resumes := m.pacing(); pauses != 2 || resumes != 1 {
			t.Fatalf("%d pauses, %d resumes", pauses, resumes)
		}

		// A far seek's new job paces from where the player is now.
		far := at(6 * time.Minute)
		if _, err := s.Segment(ctx, segmentName(far)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if j := s.Job(); j.State != JobPaused || j.From != far || j.Next != far+at(90*time.Second) {
			t.Fatalf("after a far seek: %+v", j)
		}
	})
}

func TestAcceleratorFailureBeforeTheFirstSegment(t *testing.T) {
	const qsv = media.Accelerator("qsv:/dev/dri/renderD128")
	for _, c := range []struct {
		name        string
		accelerator media.Accelerator
		failStart   error
		want        error
	}{
		{"busy accelerator", qsv, &media.Error{Op: "transcode", Kind: media.ErrEncoderBusy}, ErrAcceleratorFailed},
		{"failed accelerator", qsv, &media.Error{Op: "transcode", Kind: media.ErrEncoderFailed}, ErrAcceleratorFailed},
		{"a failed CPU encoder", "", &media.Error{Op: "transcode", Kind: media.ErrEncoderFailed}, ErrJobFailed},
		{"a corrupt source on an accelerator", qsv, &media.Error{Op: "transcode", Kind: media.ErrSourceCorrupt}, ErrJobFailed},
	} {
		t.Run(c.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := &fakeMedia{failStart: c.failStart}
				tr, spec := fixture(t, m)
				_, err := tr.Open(ctx, encodedSpec(spec, c.accelerator), 0)
				if !errors.Is(err, c.want) || !errors.Is(err, c.failStart) {
					t.Fatalf("Open: %v, want %v wrapping %v", err, c.want, c.failStart)
				}
				if entries, _ := os.ReadDir(tr.st.dir); len(entries) != 0 {
					t.Fatalf("a refused stream left %v", entries)
				}
			})
		})
	}
}

func TestAcceleratorFailureAfterTheFirstSegmentFailsTheJob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{failAt: 3, failWith: &media.Error{Op: "transcode", Kind: media.ErrEncoderFailed}}
		tr, spec := fixture(t, m)
		s, err := tr.Open(ctx, encodedSpec(spec, "nvenc:0"), 0)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		_, err = s.Segment(ctx, "3.m4s")
		if !errors.Is(err, ErrJobFailed) || errors.Is(err, ErrAcceleratorFailed) {
			t.Fatalf("mid-stream: %v", err)
		}
		if j := s.Job(); j.State != JobFailed || !errors.Is(j.Err, media.ErrEncoderFailed) {
			t.Fatalf("job %+v", j)
		}

		// A later job that's busy at its start is an accelerator failure
		// again, because the stream can't continue on this device.
		m.failStart = &media.Error{Op: "transcode", Kind: media.ErrEncoderBusy}
		s.mu.Lock()
		s.startLocked(50)
		s.mu.Unlock()
		if _, err := s.Segment(ctx, "50.m4s"); !errors.Is(err, ErrAcceleratorFailed) {
			t.Fatalf("a later job busy at its start: %v", err)
		}
	})
}

func TestUnreadableSourceIsRetriedByTheNextRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{failAt: 2, failWith: &media.Error{Op: "transcode", Kind: media.ErrSourceUnreadable}}
		tr, spec := fixture(t, m)
		s, _ := tr.Open(ctx, spec, 0)
		defer s.Close()
		if _, err := s.Segment(ctx, "3.m4s"); !errors.Is(err, media.ErrSourceUnreadable) || errors.Is(err, ErrJobFailed) {
			t.Fatalf("an unreadable source: %v", err)
		}
		m.mu.Lock()
		m.failAt = 0
		m.mu.Unlock()
		if _, err := s.Segment(ctx, "3.m4s"); err != nil || len(m.jobs()) != 2 {
			t.Fatalf("the next request: %v, %d jobs", err, len(m.jobs()))
		}
	})
}

func TestDownloadWritesOneFilePerPart(t *testing.T) {
	m := &fakeMedia{}
	tr, spec := fixture(t, m)
	spec.Parts = append(spec.Parts, FileRef{ID: id.New(), Path: "/m/b.mkv", Duration: 5 * time.Minute})
	var reached []time.Duration
	files, err := tr.Download(ctx, "a", spec, func(d time.Duration) { reached = append(reached, d) })
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || read(t, files[0].Path) != "/m/a.mkv as copy" || read(t, files[1].Path) != "/m/b.mkv as copy" {
		t.Fatalf("files %v", files)
	}
	sum := sha256.Sum256([]byte("/m/a.mkv as copy"))
	if files[0].Size != int64(len("/m/a.mkv as copy")) || files[0].SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("size and hash %+v", files[0])
	}
	if entries, _ := os.ReadDir(filepath.Join(tr.dl.dir, "a")); len(entries) != 2 {
		t.Fatalf("the conversion's folder holds %v", entries)
	}
	if entries, _ := os.ReadDir(tr.st.dir); len(entries) != 0 {
		t.Fatalf("a download touched the streams' directory: %v", entries)
	}
	// The second part's progress follows the first's, on the joined timeline.
	if last := reached[len(reached)-1]; last != 15*time.Minute || !slices.IsSorted(reached) {
		t.Fatalf("progress %v", reached)
	}
	// Converting again replaces whatever an earlier attempt left.
	if err := os.WriteFile(filepath.Join(tr.dl.dir, "a", "0.mp4.tmp"), []byte("cut short"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Download(ctx, "a", spec, func(time.Duration) {}); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(filepath.Join(tr.dl.dir, "a")); len(entries) != 2 {
		t.Fatalf("converted again, the folder holds %v", entries)
	}
	if err := tr.DeleteDownload("a"); err != nil {
		t.Fatal(err)
	}
	if names, _ := tr.Downloads(); len(names) != 0 {
		t.Fatalf("removed, it left %v", names)
	}
	if err := tr.DeleteDownload(".."); err == nil {
		t.Fatal("a name outside cache/downloads")
	}

	hw := spec
	hw.Video, hw.Source, hw.Accelerator = media.VideoRendition{Codec: media.HEVC}, media.Track{Kind: media.Video}, "nvenc:0"
	m.failStart = &media.Error{Op: "convert", Kind: media.ErrEncoderBusy}
	if _, err := tr.Download(ctx, "b", hw, func(time.Duration) {}); !errors.Is(err, ErrAcceleratorFailed) {
		t.Fatalf("busy accelerator: %v", err)
	}
	m.failStart = nil
	m.failAt = 3 // after 30 seconds of output
	if _, err := tr.Download(ctx, "b", hw, func(time.Duration) {}); !errors.Is(err, ErrJobFailed) {
		t.Fatalf("a failure after output: %v", err)
	}
	if names, _ := tr.Downloads(); len(names) != 0 {
		t.Fatalf("failed conversions left %v", names)
	}
	tr.dl.floor = math.MaxUint64
	if _, err := tr.Download(ctx, "c", spec, func(time.Duration) {}); !errors.Is(err, ErrStorageFull) {
		t.Fatalf("below the floor: %v", err)
	}
}

func TestGridPlan(t *testing.T) {
	plan := planGrid(9*gridSegment + gridSegment/2)
	last := 9 * gridSegment
	if len(plan) != 10 || plan[9] != (span{Start: last, Duration: gridSegment / 2, At: last}) {
		t.Fatalf("plan %v", plan)
	}
}

func TestPartsPlayAsOneTimeline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &fakeMedia{}
		tr, spec := fixture(t, m)
		spec.Parts = append(spec.Parts, FileRef{ID: id.New(), Path: "/m/a-cd2.mkv", Duration: 10 * time.Minute, Keyframes: m.keys})
		s, err := tr.Open(ctx, spec, 12*time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if n := len(s.plan); n != 200 {
			t.Fatalf("%d segments, want 100 a part", n)
		}
		pl, _ := s.Playlist(videoPlaylist)
		if !strings.Contains(string(pl), "99.m4s\n#EXT-X-DISCONTINUITY\n#EXT-X-MAP:URI=\"init-1.mp4\"\n#EXTINF:6.000,\n100.m4s") {
			t.Fatalf("playlist across parts:\n%s", pl)
		}
		synctest.Wait()
		if j := m.jobs()[0]; j.Path != "/m/a-cd2.mkv" || j.Start != 2*time.Minute {
			t.Fatalf("job at 12 min: %s from %v", j.Path, j.Start)
		}
		p, err := s.Segment(ctx, "121.m4s")
		if err != nil || read(t, p) != "[2m6s][2m8s][2m10s]" {
			t.Fatalf("a segment of the second part: %v", err)
		}
		if p, err := s.Segment(ctx, "init-1.mp4"); err != nil || read(t, p) != "init" {
			t.Fatalf("the second part's init: %v", err)
		}
		// The first part's init starts a job in the first part.
		if _, err := s.Segment(ctx, "init.mp4"); err != nil {
			t.Fatal(err)
		}
		if j := m.jobs(); len(j) != 2 || j[1].Path != "/m/a.mkv" || j[1].Start != 0 {
			t.Fatalf("jobs %+v", j)
		}
		// A job ends with its part, and the next part's first segment starts
		// a job there.
		if _, err := s.Segment(ctx, "99.m4s"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Segment(ctx, "100.m4s"); err != nil {
			t.Fatal(err)
		}
		if j := m.jobs(); j[len(j)-1].Path != "/m/a-cd2.mkv" || j[len(j)-1].Start != 0 {
			t.Fatalf("the job for the second part's start: %+v", j[len(j)-1])
		}
	})
}

func TestInitNames(t *testing.T) {
	for p := range 12 {
		if got, ok := parseInit(initName(p)); !ok || got != p {
			t.Errorf("part %d: %q parses as %d %v", p, initName(p), got, ok)
		}
	}
	for _, bad := range []string{"init-0.mp4", "init-01.mp4", "init-x.mp4", "init.m4s", "init-1.mp4/x"} {
		if _, ok := parseInit(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
}
