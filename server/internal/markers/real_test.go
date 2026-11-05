package markers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers/detect"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/media/ffmpeg"
)

// TestRealMarkers finds the intro that four generated episodes share. Each
// episode is 400 seconds of its own chords, with the same 40 seconds of
// sweeping tones laid in a little later in each. Generated tones
// fingerprint less distinctly than recorded sound, so the intro's placement
// is checked within a few seconds. The test runs the ffmpeg that
// KIVO_FFMPEG names, or else the one on the PATH. It skips when that ffmpeg
// can't write Chromaprint fingerprints.
func TestRealMarkers(t *testing.T) {
	bin := os.Getenv("KIVO_FFMPEG")
	if bin == "" {
		bin, _ = exec.LookPath("ffmpeg")
	}
	if bin == "" || !ffmpeg.New(bin, "").Chromaprint(t.Context()) {
		t.Skip("no ffmpeg that writes Chromaprint fingerprints")
	}
	dir := t.TempDir()
	b := catalogtest.New(t)
	var lib catalog.Library
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		var err error
		lib, err = b.Catalog.CreateLibrary(ctx, tx, catalog.LibraryInput{Name: "TV", Kind: catalog.Shows, Folders: []string{dir}})
		return err
	})
	series := b.Series("Tones")
	season := b.Work(catalog.WorkInput{Kind: catalog.KindSeason, Parent: series, Title: "Season 1", Number: 1})
	const length = 400
	var files []id.ID
	for i := range 4 {
		at := 5 + 5*i
		rel := fmt.Sprintf("Tones/Season 1/S01E%02d.mka", i+1)
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(bin, "-v", "error",
			"-f", "lavfi", "-i", tune(at, 10+i),
			"-f", "lavfi", "-i", "aevalsrc=0.4*sin(2*PI*(300+150*sin(0.7*t)+80*floor(t))*t)+0.2*sin(2*PI*(500+60*floor(2*t))*t):d=40:s=22050",
			"-f", "lavfi", "-i", tune(length-40-at, 50+i),
			"-filter_complex", "[0:a][1:a][2:a]concat=n=3:v=0:a=1[a]", "-map", "[a]", "-c:a", "flac", path).CombinedOutput()
		if err != nil {
			t.Fatal(err, string(out))
		}
		ep := b.Work(catalog.WorkInput{Kind: catalog.KindEpisode, Parent: season, Title: fmt.Sprintf("Episode %d", i+1), ParentNumber: 1, Number: i + 1})
		var file id.ID
		b.Write(func(ctx context.Context, tx *store.Tx) error {
			st, err := b.Catalog.UpsertFiles(ctx, tx, lib.Folders[0].ID, []catalog.FileObservation{{RelPath: rel, Size: 1, MTime: time.UnixMilli(1)}})
			if err != nil {
				return err
			}
			file = st[0].ID
			info, _ := json.Marshal(media.Track{Index: 0, Kind: media.Audio, Codec: "flac", Channels: 1})
			return b.Catalog.SetProbe(ctx, tx, file, st[0].Generation, catalog.Probe{
				Summary: catalog.ProbeSummary{Container: "matroska,webm", Duration: length * time.Second},
				Tracks:  []catalog.Track{{Index: 0, Type: "audio", Codec: "flac", Info: info}},
			})
		})
		b.Version([]id.ID{file}, ep)
		files = append(files, file)
	}

	settings, err := config.Open(ctx, b.DB, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	a := New(b.DB, Deps{Catalog: b.Catalog, Media: ffmpeg.New(bin, ""), Tasks: &fakeTasks{}, Settings: settings, CacheDir: t.TempDir()})
	for _, f := range files {
		if err := a.find(t.Context(), f); err != nil {
			t.Fatal(err)
		}
	}
	for i, f := range files {
		rows, err := a.q.StoredMarkers(t.Context(), id.Set(f))
		if err != nil {
			t.Fatal(err)
		}
		at := time.Duration(5+5*i) * time.Second
		if len(rows) != 1 || rows[0].Kind != detect.Intro || rows[0].Source != fromFingerprint ||
			(time.Duration(rows[0].StartMs)*time.Millisecond-at).Abs() > 2500*time.Millisecond ||
			(time.Duration(rows[0].EndMs.Int64)*time.Millisecond-at-40*time.Second).Abs() > 4*time.Second {
			t.Errorf("episode %d: %+v, want an intro from %v to %v", i+1, rows, at, at+40*time.Second)
		}
	}
}

// tune returns d seconds of three voices of notes, as an ffmpeg input. Each
// voice changes at its own rate, to a note picked by a hash of the time and
// seed.
func tune(d, seed int) string {
	note := func(rate, salt int) string {
		return fmt.Sprintf("sin(2*PI*(110+880*abs(sin(floor(%d*t)*12.9898+%d.%d*78.233)))*t)", rate, seed, salt)
	}
	return fmt.Sprintf("aevalsrc=0.3*%s+0.3*%s+0.3*%s:d=%d:s=22050", note(3, 1), note(2, 2), note(5, 3), d)
}
