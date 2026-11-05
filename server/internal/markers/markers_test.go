package markers

import (
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers/dbq"
)

func TestOverlayStoredMarkers(t *testing.T) {
	s := time.Second
	a, b := id.New(), id.New()
	parts := []catalog.File{
		{ID: a, Probe: catalog.ProbeSummary{Duration: 600 * s}},
		{ID: b, Probe: catalog.ProbeSummary{Duration: 600 * s}},
	}
	chapterIntro := Marker{Kind: IntroMarker, Start: 30 * s, End: 90 * s, Source: ChapterSource}
	row := func(file id.ID, kind, source string, start, end int64, final bool) dbq.StoredMarkersRow {
		r := dbq.StoredMarkersRow{FileID: file, Kind: kind, Source: source, StartMs: start * 1000, Final: final}
		if end >= 0 {
			r.EndMs = sql.NullInt64{Int64: end * 1000, Valid: true}
		}
		return r
	}
	for _, tc := range []struct {
		name string
		ms   []Marker
		rows []dbq.StoredMarkersRow
		want []Marker
	}{
		{
			name: "a found marker where the chapters give none, moved by the parts before it",
			ms:   []Marker{chapterIntro},
			rows: []dbq.StoredMarkersRow{row(b, "credits", "fingerprint", 540, 600, true)},
			want: []Marker{chapterIntro, {Kind: CreditsMarker, Start: 1140 * s, End: 1200 * s, Source: FingerprintSource, Final: true}},
		},
		{
			name: "a found marker replaces the chapters' of its kind",
			ms:   []Marker{chapterIntro},
			rows: []dbq.StoredMarkersRow{row(a, "intro", "fingerprint", 10, 70, false)},
			want: []Marker{{Kind: IntroMarker, Start: 10 * s, End: 70 * s, Source: FingerprintSource}},
		},
		{
			name: "an admin's wins over the chapters'",
			ms:   []Marker{chapterIntro},
			rows: []dbq.StoredMarkersRow{row(a, "intro", "manual", 40, 95, false), row(a, "intro", "fingerprint", 41, 95, false)},
			want: []Marker{{Kind: IntroMarker, Start: 40 * s, End: 95 * s, Source: ManualSource}},
		},
		{
			name: "an admin's none clears the kind",
			ms:   []Marker{chapterIntro},
			rows: []dbq.StoredMarkersRow{row(a, "intro", "manual", 0, -1, false), row(b, "recap", "manual", 0, 30, false)},
			want: []Marker{{Kind: RecapMarker, Start: 600 * s, End: 630 * s, Source: ManualSource}},
		},
		{
			name: "final credits not in the last part aren't final",
			rows: []dbq.StoredMarkersRow{row(a, "credits", "fingerprint", 550, 600, true)},
			want: []Marker{{Kind: CreditsMarker, Start: 550 * s, End: 600 * s, Source: FingerprintSource}},
		},
	} {
		if got := overlay(tc.ms, tc.rows, parts); !slices.Equal(got, tc.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", tc.name, got, tc.want)
		}
	}
}

// A plain "Intro" is a cold open only beside an opening in the same file.
func TestPlainIntroBesideAnOpening(t *testing.T) {
	s := time.Second
	file := func(cs ...catalog.Chapter) []catalog.File {
		return []catalog.File{{RelPath: "a.mkv", Probe: catalog.ProbeSummary{Duration: 1300 * s}, Chapters: cs}}
	}
	alone := file(catalog.Chapter{Start: 0, End: 90 * s, Title: "Intro"}, catalog.Chapter{Start: 90 * s, End: 1300 * s, Title: "Part A"})
	if _, ms := chapters(catalog.KindEpisode, alone); len(ms) != 1 || ms[0].End != 90*s {
		t.Errorf("alone: %+v, want the intro", ms)
	}
	paired := file(catalog.Chapter{Start: 0, End: 90 * s, Title: "Intro"}, catalog.Chapter{Start: 90 * s, End: 180 * s, Title: "Opening"},
		catalog.Chapter{Start: 180 * s, End: 1300 * s, Title: "Part A"})
	if _, ms := chapters(catalog.KindEpisode, paired); len(ms) != 1 || ms[0].Start != 90*s {
		t.Errorf("beside an opening: %+v, want the opening", ms)
	}
}
