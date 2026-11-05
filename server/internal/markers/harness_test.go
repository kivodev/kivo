package markers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers/detect"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/media/ffmpeg"
	"github.com/mmcdole/kivo/internal/store"
)

// TestMarkersHarness reads the episodes that detect's TestMarkersHarness
// measures: episodes whose chapters mark their intros and credits. It is
// the first half of an offline measurement, not a test.
//
// It runs only when KIVO_MARKERS_HARNESS names a folder holding these files:
//   - kivo.db, a copy of a server's database, which it migrates;
//   - seasons.txt, with one "Series title|season number" per line;
//   - ffmpeg, a program that runs ffmpeg where the database's paths are.
//
// It reads each file's head and tail with their keyframes' pictures. That
// way an overrun into what the chapters call show is judged by pictures
// for every source. It keeps what it reads in the folder's cache/, so a
// second run reads nothing.
//
// It writes seasons.json. That file holds each season's files as markers
// decides from them, with their chapters hidden, and what their chapters
// mark. It also holds every episode file's path and the kinds its chapters
// mark.
//
// KIVO_MARKERS_SEASONS names another list, and KIVO_MARKERS_CACHED=1 reads
// nothing new. The folder's mode file, read before each file, sets how many
// files are read at once: four for "fast", one for "slow", and two
// otherwise. Run both halves, in order, with
// go test -p 1 -run TestMarkersHarness ./internal/markers/...
func TestMarkersHarness(t *testing.T) {
	dir := os.Getenv("KIVO_MARKERS_HARNESS")
	if dir == "" {
		t.Skip("KIVO_MARKERS_HARNESS not set")
	}
	db, err := store.Open(ctx, filepath.Join(dir, "kivo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cat := catalog.New(db, events.NewBus())
	var m Media = ffmpeg.New(filepath.Join(dir, "ffmpeg"), "")
	if os.Getenv("KIVO_MARKERS_CACHED") != "" {
		m = keptOnly{m}
	}
	settings, err := config.Open(ctx, db, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	a := New(db, Deps{Catalog: cat, Media: m, Tasks: &fakeTasks{}, Settings: settings, CacheDir: filepath.Join(dir, "cache")})
	list := filepath.Join(dir, "seasons.txt")
	if l := os.Getenv("KIVO_MARKERS_SEASONS"); l != "" {
		list = l
	}
	seasons := harnessSeasons(t, db, list)

	var mu sync.Mutex
	var all []seasonFiles
	g := &gate{mode: filepath.Join(dir, "mode")}
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, sid := range seasons {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			s, err := a.season(ctx, sid)
			if err == nil {
				err = readWithPictures(ctx, a, g, &s)
			}
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			all = append(all, s)
			mu.Unlock()
		})
	}
	wg.Wait()
	slices.SortFunc(all, func(a, b seasonFiles) int { return strings.Compare(a.id.String(), b.id.String()) })

	data := harnessData{Library: harnessLibrary(t, db, cat)}
	for _, s := range all {
		data.Seasons = append(data.Seasons, harnessSeasonOf(s))
	}
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "seasons.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// keptOnly is a Media that reads nothing new, so only what earlier runs
// kept is measured.
type keptOnly struct{ Media }

func (keptOnly) Chromaprint(context.Context) bool { return false }

// harnessSeasons returns the IDs of the seasons a list names.
func harnessSeasons(t *testing.T, db *store.DB, list string) []id.ID {
	f, err := os.Open(list)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []id.ID
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		title, n, ok := strings.Cut(strings.TrimSpace(sc.Text()), "|")
		if !ok {
			continue
		}
		num, _ := strconv.Atoi(n)
		var sid id.ID
		err := db.Reader().QueryRowContext(ctx, `SELECT s.id FROM work s JOIN work r ON r.id = s.root_id
			WHERE r.title = ? AND s.kind = 'season' AND s.number = ?`, title, num).Scan(&sid)
		if err != nil {
			t.Errorf("%s season %d: %v", title, num, err)
			continue
		}
		out = append(out, sid)
	}
	return out
}

// harnessData is what the markers harness measures, as seasons.json keeps
// it.
type harnessData struct {
	Seasons []harnessSeason
	Library []harnessLibraryFile
}

// harnessSeason is a season's name and its files.
type harnessSeason struct {
	Name  string
	Files []harnessFile
}

// harnessFile is a file as markers decides from it, with its chapters
// hidden. It also holds what the harness needs: its episode's number, the
// works its version covers, its path, the length of the stretch read for
// its head, and its chapters' markers, by kind.
type harnessFile struct {
	detect.File
	Number, Works int
	Path          string
	HeadLength    time.Duration
	Truth         map[string][2]time.Duration
}

// harnessLibraryFile is one of Kivo's present episode files: its path and
// the kinds of marker its chapters give it.
type harnessLibraryFile struct {
	Path  string
	Kinds []string
}

// harnessSeasonOf is a season with its files' chapters hidden.
func harnessSeasonOf(s seasonFiles) harnessSeason {
	out := harnessSeason{Name: seasonName(s)}
	hidden := s
	hidden.files = slices.Clone(s.files)
	for i := range hidden.files {
		hidden.files[i].Chapters = nil
	}
	for i, mf := range hidden.markerFiles() {
		f := s.files[i]
		out.Files = append(out.Files, harnessFile{
			File: mf, Number: f.number, Works: f.works, Path: f.File.Path(),
			HeadLength: min(time.Duration(float64(f.Probe.Duration)*headShare), headMax), Truth: truth(f.File),
		})
	}
	return out
}

// truth returns the markers a file's chapters give it, by kind.
func truth(f catalog.File) map[string][2]time.Duration {
	out := map[string][2]time.Duration{}
	for _, m := range versionMarkers(catalog.KindEpisode, []catalog.File{f}) {
		out[string(m.Kind)] = [2]time.Duration{m.Start, m.End}
	}
	return out
}

func seasonName(s seasonFiles) string {
	if len(s.files) == 0 {
		return s.id.String()
	}
	return fmt.Sprintf("%s S%d", strings.Split(filepath.ToSlash(s.files[0].RelPath), "/")[0], s.number)
}

// harnessLibrary lists Kivo's present episode files and the kinds of
// marker their chapters give them.
func harnessLibrary(t *testing.T, db *store.DB, cat *catalog.Catalog) []harnessLibraryFile {
	rows, err := db.Reader().QueryContext(ctx, `SELECT DISTINCT f.id FROM media_file f
		JOIN media_version_work vw ON vw.version_id = f.version_id
		JOIN work e ON e.id = vw.work_id
		WHERE e.kind = 'episode' AND f.status = 'present'`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []id.ID
	for rows.Next() {
		var fid id.ID
		if err := rows.Scan(&fid); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, fid)
	}
	rows.Close()
	var out []harnessLibraryFile
	for _, fid := range ids {
		f, err := cat.File(ctx, fid)
		if err != nil {
			continue
		}
		lf := harnessLibraryFile{Path: f.Path()}
		for _, m := range versionMarkers(catalog.KindEpisode, []catalog.File{f}) {
			lf.Kinds = append(lf.Kinds, string(m.Kind))
		}
		out = append(out, lf)
	}
	return out
}

// gate bounds how many files are read at once, as its mode file says.
type gate struct {
	mode    string
	mu      sync.Mutex
	running int
}

func (g *gate) limit() int {
	b, _ := os.ReadFile(g.mode)
	switch strings.TrimSpace(string(b)) {
	case "fast":
		return 4
	case "slow":
		return 1
	}
	return 2
}

func (g *gate) acquire(ctx context.Context) error {
	for {
		g.mu.Lock()
		if g.running < g.limit() {
			g.running++
			g.mu.Unlock()
			return nil
		}
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func (g *gate) release() {
	g.mu.Lock()
	g.running--
	g.mu.Unlock()
}

// readWithPictures fingerprints each file not yet fingerprinted, as find
// does. Unlike find, it takes keyframes' pictures in both the head and the
// tail, whatever the chapters mark. Files pass through the gate one at a
// time.
func readWithPictures(ctx context.Context, a *Service, g *gate, s *seasonFiles) error {
	can := a.deps.Media.Chromaprint(ctx)
	for i := range s.files {
		f := &s.files[i]
		if r, ok := a.fingerprintAt(f.ID, f.Generation); ok {
			f.fingerprint = r
			continue
		}
		if !can {
			continue
		}
		if err := g.acquire(ctx); err != nil {
			return err
		}
		r, err := readPictures(ctx, a, *f, s.language)
		g.release()
		if errors.Is(err, context.Canceled) {
			return err
		}
		if err != nil {
			slog.WarnContext(ctx, "harness: read a file", "file", f.RelPath, "err", err)
			continue
		}
		if err := a.keepFingerprint(ctx, f.File, r); err != nil {
			return err
		}
		f.fingerprint = r
	}
	return nil
}

// readPictures is fingerprint with pictures in the head too, and in the
// tail whatever the chapters mark.
func readPictures(ctx context.Context, a *Service, f seasonFile, language string) (*detect.Fingerprint, error) {
	audio, ok := audioTrack(f.File, language)
	if !ok {
		return &detect.Fingerprint{}, nil
	}
	var video *media.SampleVideo
	if v, ok := videoTrack(f.File); ok {
		video = &media.SampleVideo{Track: v.Index, Source: v, Accelerator: a.decoder(v)}
	}
	d := f.Probe.Duration
	var r detect.Fingerprint
	var err error
	if f.part == 1 {
		r.Head, err = a.sample(ctx, media.SampleRequest{Path: f.File.Path(), AudioTrack: audio, Length: min(time.Duration(float64(d)*headShare), headMax), Video: video})
	}
	if err == nil && f.part == f.parts {
		length := min(time.Duration(float64(d)*tailShare), tailMax)
		var v *media.SampleVideo
		if video != nil {
			c := *video
			v = &c
		}
		r.Tail, err = a.sample(ctx, media.SampleRequest{Path: f.File.Path(), AudioTrack: audio, Start: d - length, Length: length, Video: v})
	}
	if errors.Is(err, media.ErrSourceCorrupt) || errors.Is(err, media.ErrUnsupported) {
		return &detect.Fingerprint{}, nil
	}
	return &r, err
}
