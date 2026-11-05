package detect

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
)

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

func TestPairFindsSharedRun(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	intro := noise(r, 600) // 74s
	a := episode(r, 4000, segment{100, intro})
	b := episode(r, 4000, segment{900, intro})
	sa, sb, ok := pair(a, b, introLimits)
	if !ok {
		t.Fatal("no match")
	}
	if !near1s(sa.start, pts(100)) || !near1s(sa.end, pts(700)) || !near1s(sb.start, pts(900)) || !near1s(sb.end, pts(1500)) {
		t.Errorf("got %v and %v, want 12.4s-86.7s and 111.5s-185.8s", sa, sb)
	}

	// A gap of under maxGap inside the shared run, such as a line of
	// dialogue over the theme, doesn't split it. A longer gap does.
	gapped := episode(r, 4000, segment{100, intro[:300]}, segment{420, intro[320:]})
	if sa, _, ok := pair(gapped, b, introLimits); !ok || !near1s(sa.start, pts(100)) || !near1s(sa.end, pts(700)) {
		t.Errorf("a short gap: %v %v", sa, ok)
	}
	split := episode(r, 4000, segment{100, intro[:200]}, segment{340, intro[240:]})
	if sa, _, ok := pair(split, b, introLimits); !ok || !near1s(sa.start, pts(340)) {
		t.Errorf("a long gap: %v %v, want the longer piece", sa, ok)
	}

	// A run too short to be an intro, or nothing shared, gives no match.
	short := episode(r, 4000, segment{100, intro[:80]})
	if _, _, ok := pair(short, b, introLimits); ok {
		t.Error("a 10s run matched")
	}
	if _, _, ok := pair(noise(r, 4000), b, introLimits); ok {
		t.Error("unrelated audio matched")
	}
}

func TestPairToleratesDrift(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	intro := noise(r, 600)
	// The same intro with a point dropped every 150, as a slightly faster
	// release plays it.
	var drifted []uint32
	for i, p := range intro {
		if i%150 != 149 {
			drifted = append(drifted, p)
		}
	}
	a := episode(r, 3000, segment{50, intro})
	b := episode(r, 3000, segment{50, drifted})
	if sa, _, ok := pair(a, b, introLimits); !ok || sa.length() < pts(550) {
		t.Errorf("got %v %v, want most of the intro", sa, ok)
	}
}

// seasonOf returns prints of the episodes, each episode its own file.
func seasonOf(eps ...[]uint32) ([]print, []id.ID) {
	var out []print
	var files []id.ID
	for i, p := range eps {
		f := id.New()
		files = append(files, f)
		out = append(out, print{file: f, episode: i, points: p})
	}
	return out, files
}

func TestSeasonConsensus(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	intro := noise(r, 500)
	var eps [][]uint32
	for i := range 6 {
		eps = append(eps, episode(r, 4000, segment{200 + 37*i, intro}))
	}
	prints, files := seasonOf(eps...)
	got := season(prints, introLimits)
	for i, f := range files {
		m, ok := got[f]
		if !ok || m.partners < 5 || !near1s(m.start, pts(200+37*i)) || !near1s(m.end, pts(700+37*i)) {
			t.Errorf("episode %d: %+v %v", i, m, ok)
		}
	}

	// Two versions of one episode are one vote between them.
	prints = append(prints, print{file: id.New(), episode: 0, points: noisy(r, eps[0], 2)})
	if m := season(prints, introLimits)[files[1]]; m.partners != 5 {
		t.Errorf("with a second version of episode 0: %d partners, want 5", m.partners)
	}
}

func TestSeasonMidSeasonChange(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	first, second := noise(r, 500), noise(r, 700)
	var eps [][]uint32
	for i := range 12 {
		theme := first
		if i >= 6 {
			theme = second
		}
		eps = append(eps, episode(r, 4000, segment{300, theme}))
	}
	prints, files := seasonOf(eps...)
	got := season(prints, introLimits)
	for i, f := range files {
		want := pts(500)
		if i >= 6 {
			want = pts(700)
		}
		if m := got[f]; !near1s(m.length(), want) || m.partners < 2 {
			t.Errorf("episode %d: %+v, want %v long", i, m, want)
		}
	}
}

func TestSeasonFallsBackToFartherEpisodes(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 10))
	special := noise(r, 400)
	eps := make([][]uint32, 14)
	for i := range eps {
		eps[i] = noise(r, 3000)
	}
	// Episodes 0, 10 and 13 share an opening no neighbor of episode 0's has.
	for _, i := range []int{0, 10, 13} {
		copy(eps[i][100:], noisy(r, special, 2))
	}
	prints, files := seasonOf(eps...)
	if m, ok := season(prints, introLimits)[files[0]]; !ok || m.partners != 2 {
		t.Errorf("episode 0: %+v %v, want two partners found past its neighbors", m, ok)
	}
}

func TestSeasonRecurringCueNeedsTwoPartners(t *testing.T) {
	r := rand.New(rand.NewPCG(11, 12))
	cue := noise(r, 130) // 16s of music two episodes share
	a := episode(r, 4000, segment{1500, cue})
	b := episode(r, 4000, segment{2600, cue})
	prints, files := seasonOf(a, b, noise(r, 4000), noise(r, 4000))
	got := season(prints, introLimits)
	f := File{ID: files[0], Duration: time.Hour, Part: 1, Parts: 1, Fingerprint: &Fingerprint{Head: &Window{}}}
	if g := f.intro(got); g != nil {
		t.Errorf("a cue one other episode shares became an intro: %+v", g)
	}
}

func TestConsensusMedian(t *testing.T) {
	s := func(a, b int) span { return span{time.Duration(a) * time.Second, time.Duration(b) * time.Second} }
	got, ok := consensus(map[int][]span{
		1: {s(10, 70)},
		2: {s(11, 71), s(300, 320)}, // its closest to the anchor counts
		3: {s(12, 69)},
		4: {s(500, 530)}, // agrees with nothing
	})
	if !ok || got.partners != 3 || got.span != s(11, 70) {
		t.Errorf("got %+v %v, want 11s-70s from 3 partners", got, ok)
	}
}
