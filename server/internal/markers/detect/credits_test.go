package detect

import (
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
)

func sec(n float64) time.Duration { return time.Duration(n * float64(time.Second)) }

// frame returns a keyframe's statistics from its share of luma below 32,
// its luma's 10th percentile, its luma's maximum, and its saturation's
// 10th percentile.
func frame(at float64, below32 uint8, low, maxi, sat float32) media.KeyframeStats {
	return media.KeyframeStats{At: sec(at), Below: [3]uint8{below32, below32, below32}, YMin: low, YLow: low, YHigh: low + 2, YMax: maxi, SatLow: sat, SatAvg: sat}
}

// lit is a keyframe of a lit scene.
var lit = media.KeyframeStats{Below: [3]uint8{4, 6, 10}, YMin: 16, YLow: 40, YHigh: 180, YMax: 235, SatLow: 20, SatAvg: 60}

func TestLooks(t *testing.T) {
	const level = 16
	for _, tc := range []struct {
		name string
		f    media.KeyframeStats
		want look
	}{
		{"white text on black", frame(0, 95, 16, 235, 2), lettered},
		{"black", frame(0, 99, 16, 30, 1), black},
		{"dense names on black", frame(0, 75, 16, 235, 2), mostly},
		{"a dark blue night scene", frame(0, 95, 16, 200, 40), story},
		{"a lit scene", lit, story},
		{"black text on a white card", media.KeyframeStats{Below: [3]uint8{3, 3, 3}, YMin: 16, YLow: 200, YHigh: 204, YMax: 210, SatAvg: 5}, card},
		{"a dim scene", frame(0, 95, 30, 200, 2), story},
	} {
		if got := lookOf(tc.f, level); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

// tailOf returns a 30-minute episode's tail, with a keyframe every 2s from
// 23m on. The keyframes show story until at, then the looks given, one per
// keyframe. The last look repeats to the end.
func tailOf(at float64, looks ...media.KeyframeStats) []keyframe {
	var frames []media.KeyframeStats
	for t := 23 * 60.0; t < 30*60; t += 2 {
		f := lit
		f.At = sec(t)
		if t >= at {
			f = looks[min(len(looks)-1, int((t-at)/2))]
			f.At = sec(t)
		}
		frames = append(frames, f)
	}
	return classify(frames)
}

func TestCredits(t *testing.T) {
	text, dark := frame(0, 95, 16, 235, 2), frame(0, 99, 16, 30, 1)
	end := 30 * time.Minute
	base := tail{duration: end, window: 23 * time.Minute}
	for _, tc := range []struct {
		name  string
		tl    tail
		want  span
		ok    bool
		final bool
	}{
		{
			name: "audio with text: from the text",
			tl:   with(base, &found{span: span{sec(28*60 + 30), sec(30*60 - 3)}, partners: 3}, tailOf(28*60+40, text)),
			want: span{sec(28*60 + 40), end}, ok: true, final: true,
		},
		{
			name: "pictures alone ending near the end",
			tl:   with(base, nil, tailOf(28*60, dark, dark, text)),
			want: span{sec(28 * 60), end}, ok: true, final: true,
		},
		{
			name: "audio over the story's own scene",
			tl:   with(base, &found{span: span{sec(28 * 60), end}, partners: 4}, tailOf(31*60)),
		},
		{
			name: "audio with nothing to show it, stopping early",
			tl:   with(base, &found{span: span{sec(27 * 60), sec(28 * 60)}, partners: 4}, nil),
		},
		{
			name: "audio alone to the end, nothing to see",
			tl:   with(base, &found{span: span{sec(28 * 60), sec(30*60 - 4)}, partners: 4}, nil),
			want: span{sec(28 * 60), end}, ok: true, final: true,
		},
		{
			name: "an ending theme over a title sequence's pictures",
			tl:   with(base, &found{span: span{sec(28*60 + 30), sec(30*60 - 1)}, partners: 8}, repeatedFrom(tailOf(31*60), 28*60+30)),
			want: span{sec(28*60 + 30), end}, ok: true, final: true,
		},
		{
			name: "an ending theme starting over the story's last scene",
			tl:   with(base, &found{span: span{sec(28*60 + 30), sec(30*60 - 1)}, partners: 8}, repeatedFrom(tailOf(31*60), 28*60+40)),
			want: span{sec(28*60 + 40), end}, ok: true, final: true,
		},
		{
			name: "an ending theme before a next episode's preview ends at the silence between them",
			tl: func() tail {
				tl := with(base, &found{span: span{sec(27 * 60), sec(29*60 + 33)}, partners: 8}, repeatedFrom(tailOf(31*60), 27*60))
				tl.silences = []media.Span{{Start: sec(29*60 + 30), End: sec(29*60 + 31)}}
				return tl
			}(),
			want: span{sec(27 * 60), sec(29*60 + 30)}, ok: true, final: true,
		},
		{
			name: "more after the theme than a preview",
			tl:   with(base, &found{span: span{sec(27 * 60), sec(29*60 - 30)}, partners: 8}, repeatedFrom(tailOf(31*60), 27*60)),
		},
		{
			name: "one partner's audio is not enough",
			tl:   with(base, &found{span: span{sec(28 * 60), end}, partners: 1}, nil),
		},
	} {
		got, ok := tc.tl.credits()
		if ok != tc.ok || ok && (got.span != tc.want || tc.tl.final(got) != tc.final) {
			t.Errorf("%s: got %v %v final %v, want %v %v final %v", tc.name, got.span, ok, ok && tc.tl.final(got), tc.want, tc.ok, tc.final)
		}
	}
}

// repeatedFrom marks the keyframes from at onward as repeated.
func repeatedFrom(ks []keyframe, at float64) []keyframe {
	for i := range ks {
		ks[i].repeated = ks[i].at >= sec(at)
	}
	return ks
}

func TestRepeats(t *testing.T) {
	// Four episodes share a theme at different points of their tails. The
	// pictures under the theme match at the same points of it. The first
	// episode's story before the theme doesn't match, and neither does the
	// last episode's odd picture.
	shot := func(at float64, low float32) media.KeyframeStats {
		return media.KeyframeStats{At: sec(at), YLow: low, YHigh: low + 100, SatAvg: 30}
	}
	starts := []float64{100, 102, 98, 101}
	var files []File
	matches := map[id.ID]found{}
	for ep, at := range starts {
		f := File{ID: id.New(), Duration: 30 * time.Minute, Episode: ep, Fingerprint: &Fingerprint{Tail: &Window{Start: sec(10)}}}
		for i, low := range []float32{20, 60, 40} {
			if ep == 3 && i == 1 {
				low = 90
			}
			f.Fingerprint.Tail.Keyframes = append(f.Fingerprint.Tail.Keyframes, shot(10+at+float64(5*i), low))
		}
		if ep == 0 {
			f.Fingerprint.Tail.Keyframes = append([]media.KeyframeStats{shot(10+at-5, 60)}, f.Fingerprint.Tail.Keyframes...)
		}
		files = append(files, f)
		matches[f.ID] = found{span: span{sec(at), sec(at + 60)}, partners: 3}
	}
	got := repeats(files, matches)
	if want := []bool{false, true, true, true}; !slices.Equal(got[files[0].ID], want) {
		t.Errorf("first episode: %v, want %v", got[files[0].ID], want)
	}
	if want := []bool{true, false, true}; !slices.Equal(got[files[3].ID], want) {
		t.Errorf("last episode: %v, want %v", got[files[3].ID], want)
	}
	// One other episode alike is not enough.
	got = repeats(files[:2], matches)
	if slices.Contains(got[files[0].ID], true) {
		t.Errorf("two episodes: %v, want none repeated", got[files[0].ID])
	}
}

func with(t tail, audio *found, frames []keyframe) tail {
	t.audio, t.frames = audio, frames
	return t
}
