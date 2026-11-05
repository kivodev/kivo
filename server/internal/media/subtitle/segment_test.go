package subtitle

import (
	"cmp"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

const segmentHeader = "WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:900000,LOCAL:00:00:00.000\n"

func TestSegment(t *testing.T) {
	s := time.Second
	cue := func(start, end time.Duration, text string) media.Cue {
		return media.Cue{Start: start, End: end, Text: text}
	}
	for _, tc := range []struct {
		name     string
		cues     []media.Cue
		from, to time.Duration
		want     string
	}{
		{"no cues", nil, 0, 6 * s, ""},
		{"a cue inside", []media.Cue{cue(61500*time.Millisecond, 63*s, "Hi")}, 60 * s, 66 * s,
			"\n00:01:01.500 --> 00:01:03.000\nHi\n"},
		{"a cue ending as the segment starts", []media.Cue{cue(2*s, 6*s, "Gone")}, 6 * s, 12 * s, ""},
		{"a cue starting as the segment ends", []media.Cue{cue(12*s, 13*s, "Next")}, 6 * s, 12 * s, ""},
		{"a cue starting as the segment starts", []media.Cue{cue(6*s, 7*s, "Here")}, 6 * s, 12 * s,
			"\n00:00:06.000 --> 00:00:07.000\nHere\n"},
		{"a cue across both boundaries", []media.Cue{cue(0, time.Hour, "Long")}, 6 * s, 12 * s,
			"\n00:00:06.000 --> 00:00:12.000\nLong\n"},
		{"no time, or backwards", []media.Cue{cue(7*s, 7*s, "Empty"), cue(8*s, 7*s, "Backwards")}, 6 * s, 12 * s, ""},
		{"less than a millisecond once cut", []media.Cue{cue(5*s, 6*s+time.Millisecond/2, "Sliver")}, 6 * s, 12 * s, ""},
		{"before zero", []media.Cue{cue(-time.Second, time.Second, "Early"), cue(-2*s, -s, "Gone")}, -21 * time.Millisecond, 6 * s,
			"\n00:00:00.000 --> 00:00:01.000\nEarly\n"},
		{"out of order", []media.Cue{cue(9*s, 10*s, "Second"), cue(7*s, 8*s, "First")}, 6 * s, 12 * s,
			"\n00:00:07.000 --> 00:00:08.000\nFirst\n\n00:00:09.000 --> 00:00:10.000\nSecond\n"},
		{"repeated in the source", []media.Cue{cue(7*s, 8*s, "Twice"), cue(7*s, 8*s, "Twice"), cue(7*s, 8*s, "Other")}, 6 * s, 12 * s,
			"\n00:00:07.000 --> 00:00:08.000\nTwice\n\n00:00:07.000 --> 00:00:08.000\nOther\n"},
		{"rolling captions overlapping", []media.Cue{cue(3*s, 8*s, "one"), cue(5*s, 14*s, "two")}, 6 * s, 12 * s,
			"\n00:00:06.000 --> 00:00:08.000\none\n\n00:00:06.000 --> 00:00:12.000\ntwo\n"},
		{"settings", []media.Cue{{Start: 7 * s, End: 8 * s, Text: "Top", Settings: "line:0"}}, 6 * s, 12 * s,
			"\n00:00:07.000 --> 00:00:08.000 line:0\nTop\n"},
		{"the last segment of a part", []media.Cue{cue(5*s, 99*s, "Credits")}, 6 * s, math.MaxInt64,
			"\n00:00:06.000 --> 00:01:39.000\nCredits\n"},
	} {
		if got := string(Segment(tc.cues, tc.from, tc.to)); got != segmentHeader+tc.want {
			t.Errorf("%s: got\n%s\nwant\n%s", tc.name, got, segmentHeader+tc.want)
		}
	}
}

// Whatever the cues and boundaries, a player that joins every segment's
// cues shows each distinct source cue exactly once at every moment it
// should show. The cue's pieces run end to end, from its start to its end.
func TestSegmentsShowEachCueOnce(t *testing.T) {
	for seed := range uint64(200) {
		r := rand.New(rand.NewPCG(seed, 1))
		ms := func(n int) time.Duration { return time.Duration(r.IntN(n)) * time.Millisecond }
		var src []media.Cue
		for i := range 1 + r.IntN(60) {
			c := media.Cue{Start: ms(60_000), Text: fmt.Sprintf("cue %d", i)}
			c.End = c.Start + ms(20_000) // up to 20 s, across several segments
			if r.IntN(5) == 0 {
				c.Settings = "line:0"
			}
			src = append(src, c)
			if r.IntN(10) == 0 {
				src = append(src, c) // repeated in the source
			}
		}
		r.Shuffle(len(src), func(i, j int) { src[i], src[j] = src[j], src[i] })
		bounds := []time.Duration{0}
		for bounds[len(bounds)-1] < 70*time.Second {
			bounds = append(bounds, bounds[len(bounds)-1]+time.Millisecond+ms(10_000))
		}
		bounds[len(bounds)-1] = math.MaxInt64 // the last segment takes the rest

		pieces := map[string][]media.Cue{}
		for i := range len(bounds) - 1 {
			cues, err := Cues(Segment(src, bounds[i], bounds[i+1]), media.VTT)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.IsSortedFunc(cues, func(a, b media.Cue) int { return cmp.Compare(a.Start, b.Start) }) {
				t.Fatalf("seed %d: segment %d out of order: %+v", seed, i, cues)
			}
			for _, c := range cues {
				pieces[c.Text] = append(pieces[c.Text], c)
			}
		}
		for _, c := range src {
			p := pieces[c.Text]
			if c.End == c.Start {
				if len(p) != 0 {
					t.Fatalf("seed %d: %q shows for no time, but is %+v", seed, c.Text, p)
				}
				continue
			}
			ok := len(p) > 0 && p[0].Start == c.Start && p[len(p)-1].End == c.End
			for k := range p {
				ok = ok && p[k].Settings == c.Settings && (k == 0 || p[k].Start == p[k-1].End)
			}
			if !ok {
				t.Fatalf("seed %d: %+v shows as %+v", seed, c, p)
			}
		}
	}
}

func TestSegmentEscapesNothingTwice(t *testing.T) {
	cues, err := Cues([]byte("1\n00:00:01,000 --> 00:00:02,000\nTom &amp; Jerry & <v Bob>hi</v> <3\n"), media.SRT)
	if err != nil {
		t.Fatal(err)
	}
	got := string(Segment(cues, 0, 6*time.Second))
	if !strings.HasSuffix(got, "\nTom &amp; Jerry &amp; <v Bob>hi</v> &lt;3\n") {
		t.Fatalf("got\n%s", got)
	}
}
