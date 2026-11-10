package markers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers/dbq"
	"github.com/mmcdole/kivo/internal/markers/detect"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/tasks"
)

func TestMarkerSeasonsOrder(t *testing.T) {
	now := time.Now()
	a, b, c, d := id.New(), id.New(), id.New(), id.New()
	rows := []dbqRow{
		{a, now.Add(-40 * 24 * time.Hour), now.Add(-time.Hour)}, // played long ago, added lately
		{b, now.Add(-2 * time.Hour), now.Add(-100 * time.Hour)}, // played lately
		{c, time.Time{}, now.Add(-200 * time.Hour)},             // never played, added long ago
		{d, now.Add(-time.Hour), now.Add(-300 * time.Hour)},     // played most lately
	}
	var got []id.ID
	for _, s := range markerSeasons(rowsOf(rows), now) {
		got = append(got, s.id)
	}
	if want := []id.ID{d, b, a, c}; !slices.Equal(got, want) {
		t.Errorf("order %v, want %v", got, want)
	}
}

// Episodes placed in a library that finds markers as media is added ask
// for their files' markers once the placing commits. In a nightly library
// they ask for nothing.
// The fingerprinted audio leaves out commentary, whether the track is
// flagged as commentary or only titled that way.
func TestAudioTrackSkipsCommentary(t *testing.T) {
	audio := func(index int, lang string, mt media.Track) catalog.Track {
		info, _ := json.Marshal(mt)
		return catalog.Track{Index: index, Type: string(media.Audio), Language: lang, Info: info}
	}
	f := catalog.File{Tracks: []catalog.Track{
		audio(1, "eng", media.Track{Title: "Director's Commentary"}),
		audio(2, "eng", media.Track{Commentary: true}),
		audio(3, "fre", media.Track{Title: "Commentary"}),
		audio(4, "fre", media.Track{}),
		audio(5, "eng", media.Track{Title: "Stereo"}),
	}}
	for lang, want := range map[string]int{"en": 5, "fr": 4, "": 4} {
		if got, ok := audioTrack(f, lang); !ok || got != want {
			t.Errorf("audioTrack(%q) = %d %v, want %d", lang, got, ok, want)
		}
	}
}

func TestPlaced(t *testing.T) {
	f := newShowFixture(t)
	lib := catalog.Library{ID: f.lib, Markers: catalog.NightlyAndAdded}
	placed := func(lib catalog.Library) {
		t.Helper()
		f.b.Write(func(ctx context.Context, tx *store.Tx) error {
			err := f.a.Placed(ctx, tx, lib, f.episodes)
			if got := f.tasks.take(); len(got) != 0 {
				t.Errorf("asked %+v before the commit", got)
			}
			return err
		})
	}
	placed(lib)
	if got := f.tasks.take(); len(got) != 1 || got[0].task != TaskFindMarkers || !slices.Equal(got[0].items, f.files) {
		t.Fatalf("asked %+v, want each file's markers", got)
	}
	lib.Markers = catalog.Nightly
	if placed(lib); len(f.tasks.take()) != 0 {
		t.Error("a nightly library asked as media was added")
	}
}

// find reads a file's head, and its tail with its pictures, and keeps its
// fingerprint. It leaves deciding the season to the season's last file to
// be read. When run again, it reads nothing and decides the season from
// the kept fingerprints.
func TestFind(t *testing.T) {
	f := newShowFixture(t)
	f.m.sample = func(req media.SampleRequest) (media.Sample, error) {
		return media.Sample{Points: noise(rand.New(rand.NewPCG(1, 2)), 100)}, nil
	}
	if err := f.a.find(ctx, f.files[0]); err != nil {
		t.Fatal(err)
	}
	if len(f.m.samples) != 2 || f.m.samples[1].Video == nil {
		t.Errorf("read %+v, want the head and the tail with its pictures", f.m.samples)
	}
	if fp, ok := f.a.fingerprintAt(f.files[0], 1); !ok || fp.Head == nil || fp.Tail == nil {
		t.Fatalf("kept %+v", fp)
	}
	if got := f.decidedFiles(); len(got) != 0 {
		t.Fatalf("decided %v with two files still to read", got)
	}
	f.m.samples = nil
	if err := f.a.find(ctx, f.files[0]); err != nil || len(f.m.samples) != 0 {
		t.Errorf("again: %v, read %d stretches", err, len(f.m.samples))
	}
	if got := f.decidedFiles(); !slices.Equal(got, f.files[:1]) {
		t.Errorf("again: decided %v, want the fingerprinted file", got)
	}
}

// When find is stopped by its context, or fails for a reason that may
// pass, it keeps nothing and records nothing. A file that media finds
// broken or can't read is recorded as failed, with the reason, and keeps no
// fingerprint.
func TestFindOutcomes(t *testing.T) {
	f := newShowFixture(t)
	cctx, cancel := context.WithCancel(ctx)
	f.m.sample = func(media.SampleRequest) (media.Sample, error) {
		cancel()
		return media.Sample{}, errors.New("encoder failed: context canceled")
	}
	if err := f.a.find(cctx, f.files[0]); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
	f.m.sample = func(media.SampleRequest) (media.Sample, error) { return media.Sample{}, errors.New("i/o timeout") }
	if err := f.a.find(ctx, f.files[0]); err == nil {
		t.Error("a passing failure: nil")
	}
	if f.a.fingerprinted(f.files[0], 1) || len(f.decidedFiles()) != 0 || len(f.failures()) != 0 {
		t.Error("stopped or failing: kept or recorded")
	}

	for i, broken := range []error{media.ErrSourceCorrupt, media.ErrUnsupported} {
		f.m.sample = func(media.SampleRequest) (media.Sample, error) { return media.Sample{}, broken }
		if err := f.a.find(ctx, f.files[i+1]); err != nil {
			t.Errorf("%v: %v", broken, err)
		}
		if f.a.fingerprinted(f.files[i+1], 1) {
			t.Errorf("%v: kept a fingerprint", broken)
		}
		if gen, rules, state := f.decided(i + 1); gen != 1 || rules != detect.RulesVersion || state != "failed" {
			t.Errorf("%v: recorded %d %d %s", broken, gen, rules, state)
		}
	}
	got := f.failures()
	if len(got) != 2 || !strings.Contains(got[0].Reason+got[1].Reason, media.ErrSourceCorrupt.Error()) {
		t.Errorf("failures %+v", got)
	}
	if got := f.pending(); !slices.Equal(got, f.files[:1]) {
		t.Errorf("pending %v, want the file not failed", got)
	}
}

// The night's files decide their season once, when the season's last file
// to be read keeps its fingerprint.
func TestSeasonDecidedByItsLastFile(t *testing.T) {
	f := newShowFixture(t)
	f.sampleHeads(f.introHeads())
	for i := range 2 {
		if err := f.a.find(ctx, f.files[i]); err != nil {
			t.Fatal(err)
		}
		if got := f.decidedFiles(); len(got) != 0 {
			t.Fatalf("file %d decided %v with a file still to read", i, got)
		}
	}
	if got := f.pending(); !slices.Equal(got, f.files) {
		t.Errorf("pending %v, want every file", got)
	}
	if err := f.a.find(ctx, f.files[2]); err != nil {
		t.Fatal(err)
	}
	if got := f.decidedFiles(); !slices.Equal(got, f.files) {
		t.Errorf("the last file decided %v, want every file", got)
	}
	if rows := f.markers(1); len(rows) != 1 || rows[0].Kind != detect.Intro {
		t.Errorf("stored %+v", rows)
	}
	if got := f.pending(); len(got) != 0 {
		t.Errorf("pending %v", got)
	}
}

// A file that can't be read leaves the pending list and doesn't hold up
// its season. The season's last file decides the season without it. Once
// retried, the file is pending again.
func TestFailedFile(t *testing.T) {
	f := newShowFixture(t)
	broken := f.file(0).Path()
	f.m.sample = func(req media.SampleRequest) (media.Sample, error) {
		if req.Path == broken {
			return media.Sample{}, media.ErrSourceCorrupt
		}
		return media.Sample{Points: noise(rand.New(rand.NewPCG(1, 2)), 100)}, nil
	}
	for i := range f.files {
		if err := f.a.find(ctx, f.files[i]); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.decidedFiles(); !slices.Equal(got, f.files[1:]) {
		t.Errorf("decided %v, want the files read", got)
	}
	if got := f.pending(); len(got) != 0 {
		t.Errorf("pending %v", got)
	}
	if left, err := f.a.pendingCount(ctx); err != nil || left != 0 {
		t.Errorf("%d left, %v", left, err)
	}
	if got := f.failures(); len(got) != 1 || got[0].Item != f.files[0] || !strings.Contains(got[0].Reason, media.ErrSourceCorrupt.Error()) {
		t.Fatalf("failures %+v", got)
	}
	if got, err := f.a.retry(ctx, id.New()); err != nil || len(got) != 0 {
		t.Errorf("retry another file: %v, %v", got, err)
	}
	if got, err := f.a.retry(ctx, f.files[0]); err != nil || !slices.Equal(got, f.files[:1]) {
		t.Fatalf("retry: %v, %v", got, err)
	}
	if got := f.pending(); !slices.Equal(got, f.files[:1]) || len(f.failures()) != 0 {
		t.Errorf("retried: pending %v, failures %v", got, f.failures())
	}
}

// The pending list holds each episode file that is neither decided by the
// current rules nor failed, whether fingerprinted or not. Without
// Chromaprint, nothing is pending.
func TestPending(t *testing.T) {
	f := newShowFixture(t)
	if got := f.pending(); !slices.Equal(got, f.files) {
		t.Fatalf("pending %v, want every file", got)
	}
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return dbq.New(tx).SetFailed(ctx, dbq.SetFailedParams{
			FileID: f.files[2], Generation: 1, Rules: detect.RulesVersion, Reason: sql.NullString{String: "broken", Valid: true}, UpdatedAt: 1,
		})
	})
	if got := f.pending(); !slices.Equal(got, f.files[:2]) {
		t.Errorf("one failed: pending %v, want the others", got)
	}
	for i := range f.episodes {
		if err := f.a.keepFingerprint(ctx, f.file(i), &detect.Fingerprint{}); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.pending(); !slices.Equal(got, f.files[:2]) {
		t.Errorf("fingerprinted, never decided: pending %v, want those not failed", got)
	}
	f.setRules(detect.RulesVersion)
	if got := f.pending(); len(got) != 0 {
		t.Errorf("decided: pending %v", got)
	}
	f.setRules(detect.RulesVersion - 1)
	f.m.noChromaprint = true
	if got := f.pending(); len(got) != 0 {
		t.Errorf("no Chromaprint: pending %v", got)
	}
}

// left matches the length of the pending list, over files decided by the
// current rules, failed, decided by older rules or at an older generation,
// and never decided.
func TestCountPendingIsPending(t *testing.T) {
	f := newShowFixture(t)
	f.addEpisode()
	f.addEpisode()
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		for i, d := range []dbq.SetDecidedParams{
			{Generation: 1, Rules: detect.RulesVersion},
			{Generation: 1, Rules: detect.RulesVersion - 1},
			{Generation: 0, Rules: detect.RulesVersion},
		} {
			d.FileID, d.State, d.UpdatedAt = f.files[i], decidedNone, 1
			if err := q.SetDecided(ctx, d); err != nil {
				return err
			}
		}
		return q.SetFailed(ctx, dbq.SetFailedParams{
			FileID: f.files[3], Generation: 1, Rules: detect.RulesVersion - 1, Reason: sql.NullString{String: "broken", Valid: true}, UpdatedAt: 1,
		})
	})
	pending := f.pending()
	if !slices.Equal(pending, []id.ID{f.files[1], f.files[2], f.files[4]}) {
		t.Errorf("pending %v", pending)
	}
	if left, err := f.a.pendingCount(ctx); err != nil || left != len(pending) {
		t.Errorf("%d left, %v; pending %d", left, err, len(pending))
	}
}

// A season's decision stores its markers without reading any media.
func TestDecideStores(t *testing.T) {
	f := newShowFixture(t)
	f.sharedIntros()
	if err := f.a.decide(ctx, f.season); err != nil {
		t.Fatal(err)
	}
	rows := f.markers(1)
	if len(rows) != 1 || rows[0].Kind != detect.Intro || rows[0].Source != fromFingerprint || !near1s(time.Duration(rows[0].StartMs)*time.Millisecond, pts(400)) {
		t.Fatalf("stored %+v", rows)
	}
	if gen, rules, state := f.decided(1); gen != 1 || rules != detect.RulesVersion || state != decidedDone {
		t.Errorf("decided %d %d %s", gen, rules, state)
	}
	if len(f.m.samples) != 0 {
		t.Errorf("read %d stretches", len(f.m.samples))
	}
}

// markers returns the markers stored for the fixture's i-th file.
func (f *showFixture) markers(i int) []dbqMarker {
	rows, err := f.a.q.StoredMarkers(ctx, id.Set(f.files[i]))
	if err != nil {
		f.t.Fatal(err)
	}
	return rows
}

// introHeads returns the points of the heads of the fixture's files, which
// share an intro.
func (f *showFixture) introHeads() [][]uint32 {
	r := rand.New(rand.NewPCG(3, 4))
	intro := noise(r, 600)
	var out [][]uint32
	for i := range f.episodes {
		out = append(out, episode(r, 2600, segment{100 + 300*i, intro}))
	}
	return out
}

// sharedIntros keeps a fingerprint for each of the fixture's files, whose
// heads share an intro.
func (f *showFixture) sharedIntros() {
	f.t.Helper()
	for i, head := range f.introHeads() {
		if err := f.a.keepFingerprint(ctx, f.file(i), &detect.Fingerprint{Head: &detect.Window{Points: detect.PackPoints(head)}}); err != nil {
			f.t.Fatal(err)
		}
	}
}

// sampleHeads makes media read the fixture's i-th file's head as heads[i],
// and every tail as nothing.
func (f *showFixture) sampleHeads(heads [][]uint32) {
	paths := map[string][]uint32{}
	for i, head := range heads {
		paths[f.file(i).Path()] = head
	}
	f.m.sample = func(req media.SampleRequest) (media.Sample, error) {
		if req.Start > 0 {
			return media.Sample{}, nil
		}
		return media.Sample{Points: paths[req.Path]}, nil
	}
}

// setRules records the rules the fixture's files were decided by, at
// their generation.
func (f *showFixture) setRules(rules int64) {
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		for _, file := range f.files {
			err := dbq.New(tx).SetDecided(ctx, dbq.SetDecidedParams{FileID: file, Generation: 1, Rules: rules, State: decidedNone, UpdatedAt: 1})
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// decided returns what detection decided for the fixture's i-th file: the
// generation, the rules and the state.
func (f *showFixture) decided(i int) (gen, rules int64, state string) {
	f.t.Helper()
	err := f.b.DB.Reader().QueryRowContext(ctx, `SELECT generation, rules, state FROM marker_file WHERE file_id = ?`, f.files[i]).Scan(&gen, &rules, &state)
	if err != nil {
		f.t.Fatal(err)
	}
	return gen, rules, state
}

// decidedFiles returns the fixture's files whose markers are decided, as
// done or none, in the fixture's order.
func (f *showFixture) decidedFiles() []id.ID {
	f.t.Helper()
	var out []id.ID
	for _, file := range f.files {
		var n int
		err := f.b.DB.Reader().QueryRowContext(ctx, `SELECT count(*) FROM marker_file WHERE file_id = ? AND state <> 'failed'`, file).Scan(&n)
		if err != nil {
			f.t.Fatal(err)
		}
		if n > 0 {
			out = append(out, file)
		}
	}
	return out
}

// pending returns what the nightly task has left.
func (f *showFixture) pending() []id.ID {
	f.t.Helper()
	got, err := f.a.pending(ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return got
}

// failures returns the files whose fingerprint failed for good.
func (f *showFixture) failures() []tasks.Failure {
	f.t.Helper()
	got, err := f.a.failed(ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return got
}

func TestOlderRulesDecideAgain(t *testing.T) {
	for _, tc := range []struct {
		name    string
		markers bool
	}{{"with markers", true}, {"without markers", false}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newShowFixture(t)
			if tc.markers {
				f.sharedIntros()
			} else {
				for i := range f.episodes {
					if err := f.a.keepFingerprint(ctx, f.file(i), &detect.Fingerprint{}); err != nil {
						t.Fatal(err)
					}
				}
			}
			manual := dbq.InsertMarkerParams{
				FileID: f.files[0], Generation: f.file(0).Generation, Kind: detect.Credits, Source: "manual",
				StartMs: 1000, EndMs: sql.NullInt64{Int64: 2000, Valid: true}, Reason: "set by hand",
			}
			f.b.Write(func(ctx context.Context, tx *store.Tx) error { return dbq.New(tx).InsertMarker(ctx, manual) })
			if err := f.a.decide(ctx, f.season); err != nil {
				t.Fatal(err)
			}
			want := f.markers(1)
			if (len(want) > 0) != tc.markers {
				t.Fatalf("stored %+v", want)
			}
			if got := f.pending(); len(got) != 0 {
				t.Fatalf("decided by these rules: pending %v", got)
			}
			// Older rules make every file pending. The first file found
			// decides the season again from the kept fingerprints, reading
			// no media and keeping the admin's marker.
			f.setRules(detect.RulesVersion - 1)
			if got := f.pending(); !slices.Equal(got, f.files) {
				t.Fatalf("older rules: pending %v, want every file", got)
			}
			f.b.Write(func(ctx context.Context, tx *store.Tx) error {
				return dbq.New(tx).DeleteFoundMarkers(ctx, f.files[1])
			})
			if err := f.a.find(ctx, f.files[0]); err != nil {
				t.Fatal(err)
			}
			if len(f.m.samples) != 0 {
				t.Errorf("read %d stretches", len(f.m.samples))
			}
			if got := f.markers(1); !slices.Equal(got, want) {
				t.Errorf("decided again: %+v, want %+v", got, want)
			}
			if got := f.markers(0); !slices.ContainsFunc(got, func(m dbqMarker) bool {
				return m.Source == "manual" && m.Kind == detect.Credits && m.StartMs == 1000 && m.EndMs.Int64 == 2000
			}) {
				t.Errorf("the admin's marker: %+v", got)
			}
			if got := f.pending(); len(got) != 0 {
				t.Errorf("decided again: pending %v", got)
			}
		})
	}
}

func TestOlderFingerprintsAreMissing(t *testing.T) {
	f := newShowFixture(t)
	older := func(file catalog.File) string {
		return config.CachePath(f.a.resultDir(), file.ID, file.ID.String(), config.CacheLeaf(file.Generation, detect.FingerprintVersion-1))
	}
	for i := range f.episodes {
		if err := os.MkdirAll(older(f.file(i)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(older(f.file(i)), fingerprintFile), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f.setRules(detect.RulesVersion - 1)
	if f.a.fingerprinted(f.files[0], f.file(0).Generation) {
		t.Fatal("an older fingerprint counts as made")
	}
	if got := f.pending(); !slices.Equal(got, f.files) {
		t.Fatalf("pending %v, want every file", got)
	}
	if err := f.a.find(ctx, f.files[0]); err != nil {
		t.Fatal(err)
	}
	if len(f.m.samples) == 0 || !f.a.fingerprinted(f.files[0], f.file(0).Generation) {
		t.Fatalf("read %d stretches", len(f.m.samples))
	}
	if err := f.a.prune(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(older(f.file(0))); err == nil {
		t.Error("the older fingerprint kept")
	}
	if !f.a.fingerprinted(f.files[0], f.file(0).Generation) {
		t.Error("the fingerprint pruned")
	}
}

// noise returns n random fingerprint points.
func noise(r *rand.Rand, n int) []uint32 {
	out := make([]uint32, n)
	for i := range out {
		out[i] = r.Uint32()
	}
	return out
}

// noisy returns points with up to flips bits flipped in each, as a second
// encoding of the same audio gives.
func noisy(r *rand.Rand, points []uint32, flips int) []uint32 {
	out := make([]uint32, len(points))
	for i, p := range points {
		for range r.IntN(flips + 1) {
			p ^= 1 << r.IntN(32)
		}
		out[i] = p
	}
	return out
}

// episode returns a fingerprint of n points with shared segments laid in.
// Each segment is a pair of an offset and the points to place there.
func episode(r *rand.Rand, n int, segments ...segment) []uint32 {
	out := noise(r, n)
	for _, s := range segments {
		copy(out[s.at:], noisy(r, s.points, 3))
	}
	return out
}

type segment struct {
	at     int
	points []uint32
}

// pts returns the duration of n points.
func pts(n int) time.Duration { return time.Duration(n) * media.PointStep }

func near1s(a, b time.Duration) bool { return (a - b).Abs() <= time.Second }
