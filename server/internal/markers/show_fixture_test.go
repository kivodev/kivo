package markers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers/dbq"
	"github.com/mmcdole/kivo/internal/media"
)

type dbqMarker = dbq.StoredMarkersRow

// dbqRow is a nightly-task row: a season with one file, when its series
// was last played, and when the season was last added to.
type dbqRow struct {
	season        id.ID
	played, added time.Time
}

func rowsOf(rows []dbqRow) []dbq.MarkerSeasonsRow {
	var out []dbq.MarkerSeasonsRow
	ms := func(t time.Time) int64 {
		if t.IsZero() {
			return 0
		}
		return t.UnixMilli()
	}
	for _, r := range rows {
		out = append(out, dbq.MarkerSeasonsRow{SeasonID: r.season, ID: id.New(), PlayedAt: ms(r.played), AddedAt: ms(r.added)})
	}
	return out
}

// showFixture is the service over a shows library with one season of
// three probed 22-minute episodes. Each episode has a video track and an
// English audio track.
type showFixture struct {
	t        *testing.T
	b        *catalogtest.Builder
	m        *fakeMedia
	tasks    *fakeTasks
	a        *Service
	settings *config.Store
	lib      id.ID
	folder   id.ID
	series   id.ID
	season   id.ID
	episodes []id.ID
	files    []id.ID
}

func newShowFixture(t *testing.T) *showFixture {
	f := &showFixture{t: t, b: catalogtest.New(t), m: &fakeMedia{}, tasks: &fakeTasks{}}
	f.lib, f.folder = f.b.Library(catalog.Shows)
	f.series = f.b.Series("Show")
	for range 3 {
		f.addEpisode()
	}
	w, err := f.b.Catalog.Work(ctx, f.episodes[0])
	if err != nil {
		t.Fatal(err)
	}
	f.season = w.Parent
	if f.settings, err = config.Open(ctx, f.b.DB, config.Options{}); err != nil {
		t.Fatal(err)
	}
	f.a = New(f.b.DB, Deps{Catalog: f.b.Catalog, Media: f.m, Tasks: f.tasks, Settings: f.settings, CacheDir: t.TempDir()})
	f.a.room.Free = func(string) (uint64, error) { return 100 << 30, nil }
	return f
}

// addEpisode adds the season's next probed episode.
func (f *showFixture) addEpisode() {
	f.t.Helper()
	ep := f.b.Episode(f.folder, f.series, 1, len(f.episodes)+1)
	f.episodes, f.files = append(f.episodes, ep.Work), append(f.files, ep.File)
	f.probe(ep.File)
}

func (f *showFixture) probe(file id.ID) {
	f.t.Helper()
	var stored []catalog.Track
	for _, tr := range []media.Track{
		{Index: 0, Kind: media.Video, Codec: media.H264, Width: 640, Height: 360, BitDepth: 8},
		{Index: 1, Kind: media.Audio, Codec: media.AAC, Channels: 2, Language: "eng"},
	} {
		info, _ := json.Marshal(tr)
		stored = append(stored, catalog.Track{Index: tr.Index, Type: string(tr.Kind), Codec: string(tr.Codec), Language: tr.Language, Info: info})
	}
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return f.b.Catalog.SetProbe(ctx, tx, file, 1, catalog.Probe{
			Summary: catalog.ProbeSummary{Container: "matroska,webm", Duration: 22 * time.Minute},
			Tracks:  stored,
		})
	})
}

// file is the fixture's i-th file as catalog has it.
func (f *showFixture) file(i int) catalog.File {
	f.t.Helper()
	file, err := f.b.Catalog.File(ctx, f.files[i])
	if err != nil {
		f.t.Fatal(err)
	}
	return file
}
