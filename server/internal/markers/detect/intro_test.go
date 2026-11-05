package detect

import (
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
)

func TestIntroSnaps(t *testing.T) {
	f := File{ID: id.New(), Duration: 30 * time.Minute, Part: 1, Parts: 1, Fingerprint: &Fingerprint{Head: &Window{
		Silences: []media.Span{{Start: sec(95.5), End: sec(96.2)}},
	}}}
	f.Chapters = []time.Duration{0, sec(31), sec(300)}
	for _, tc := range []struct {
		name       string
		found      span
		start, end time.Duration
	}{
		{"start near the start of the file", span{sec(0.6), sec(58.7)}, 0, sec(60)},
		{"start near a chapter, end near a silence", span{sec(29.5), sec(94.2)}, sec(31), sec(95.5)},
		{"nothing near", span{sec(10), sec(70)}, sec(10.85), sec(71.3)},
	} {
		got := f.intro(map[id.ID]found{f.ID: {span: tc.found, partners: 3}})
		if got == nil || got.start != tc.start || got.end != tc.end {
			t.Errorf("%s: got %+v, want %v-%v", tc.name, got, tc.start, tc.end)
		}
	}
	if got := f.intro(map[id.ID]found{f.ID: {span: span{sec(10), sec(70)}, partners: 1}}); got != nil {
		t.Errorf("one partner: %+v, want none", got)
	}
}
