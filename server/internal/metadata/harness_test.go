package metadata

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/egress"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/media/ffmpeg"
	"github.com/mmcdole/kivo/internal/users"
)

var ctx = context.Background()

// The fixtures in testdata are TMDb, Audible and Audnexus responses in the
// shape the APIs return, trimmed to the fields Kivo reads. A local server
// answers every test request from them, so no test touches the network.

const hostHeader = "X-Fixture-Host"

// fixtures answers provider requests from testdata, routed by the host each
// request was meant for. Artwork hosts get a generated PNG.
type fixtures struct {
	srv *httptest.Server

	mu     sync.Mutex
	status map[string]int    // path → status to answer instead
	hits   map[string]int    // path → requests seen
	hook   func(path string) // runs before each answer
}

func newFixtures(t *testing.T) *fixtures {
	f := &fixtures{status: map[string]int{}, hits: map[string]int{}}
	f.srv = httptest.NewServer(f)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fixtures) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.hits[r.URL.Path]++
	status, hook := f.status[r.URL.Path], f.hook
	f.mu.Unlock()
	if hook != nil {
		hook(r.URL.Path)
	}
	if status != 0 {
		w.WriteHeader(status)
		return
	}
	var file string
	switch r.Header.Get(hostHeader) {
	case "api.themoviedb.org":
		if r.Header.Get("Authorization") != "Bearer tmdb-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		file = "testdata/tmdb/" + strings.ReplaceAll(strings.TrimPrefix(r.URL.Path, "/3/"), "/", "_") + ".json"
	case "raw.githubusercontent.com":
		file = "testdata/animelists/" + path.Base(r.URL.Path)
	case "api.audible.com":
		file = "testdata/audible/search_" + strings.ReplaceAll(r.URL.Query().Get("title"), " ", "_") + ".json"
	case "api.audnex.us":
		if r.URL.Query().Get("region") != "us" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		file = "testdata/audnexus/" + strings.ReplaceAll(strings.TrimPrefix(r.URL.Path, "/"), "/", "_") + ".json"
	case "image.tmdb.org", "m.media-amazon.com", "images.example.org":
		w.Write(pngBytes(300, 450))
		return
	default:
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	b, err := os.ReadFile(file)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Write(b)
}

func (f *fixtures) set(path string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[path] = status
}

func (f *fixtures) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[path]
}

// fixtureGetter sends every request to the fixtures, naming the host it was
// meant for. The egress client's own rules are tested in its own package.
type fixtureGetter struct{ f *fixtures }

func (g fixtureGetter) Get(ctx context.Context, raw string, h http.Header) (egress.Response, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return egress.Response{}, fmt.Errorf("test: %w", egress.ErrRefused)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.f.srv.URL+u.RequestURI(), nil)
	if err != nil {
		return egress.Response{}, err
	}
	req.Header = h.Clone()
	if req.Header == nil {
		req.Header = http.Header{}
	}
	req.Header.Set(hostHeader, u.Host)
	resp, err := g.f.srv.Client().Do(req)
	if err != nil {
		return egress.Response{}, err
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	return egress.Response{Status: resp.StatusCode, Header: resp.Header, Body: got}, err
}

// fakeImager stands in for ffmpeg. It decodes with Go's image packages,
// fails as ffmpeg would on a broken file, and writes a marker file for each
// output.
type fakeImager struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeImager) Image(ctx context.Context, src string, outs []media.ImageOut) ([]byte, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, &media.Error{Op: "image " + src, Kind: media.ErrSourceUnreadable}
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, &media.Error{Op: "image " + src, Kind: media.ErrSourceCorrupt, Detail: err.Error()}
	}
	for _, o := range outs {
		if err := os.WriteFile(o.Path, fmt.Appendf(nil, "RIFF webp %d", o.Width), 0o644); err != nil {
			return nil, err
		}
	}
	b := img.Bounds()
	tiny := image.NewNRGBA(image.Rect(0, 0, 32, max(1, 32*b.Dy()/b.Dx())))
	for y := range tiny.Bounds().Dy() {
		for x := range 32 {
			tiny.Set(x, y, img.At(b.Min.X+x*b.Dx()/32, b.Min.Y+y*b.Dy()/tiny.Bounds().Dy()))
		}
	}
	var out bytes.Buffer
	err = png.Encode(&out, tiny)
	return out.Bytes(), err
}

func (f *fakeImager) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// imager returns a real ffmpeg when KIVO_FFMPEG names one, and the fake
// otherwise.
func imager(t *testing.T) Imager {
	if bin := os.Getenv("KIVO_FFMPEG"); bin != "" {
		return ffmpeg.New(bin, os.Getenv("KIVO_FFPROBE"))
	}
	return &fakeImager{}
}

func pngBytes(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.NRGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// env is a service over a fresh database that answers from the fixtures.
type env struct {
	t       *testing.T
	b       *catalogtest.Builder
	s       *Service
	fx      *fixtures
	img     Imager
	tasks   *fakeTasks
	cfg     *config.Store
	cache   string
	uploads string // data/images
}

func newEnv(t *testing.T) *env {
	return newEnvWith(t, &fakeImager{})
}

func newEnvWith(t *testing.T, img Imager) *env {
	t.Helper()
	b := catalogtest.New(t)
	cfg, err := config.Open(ctx, b.DB, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, b: b, fx: newFixtures(t), img: img, tasks: &fakeTasks{}, cfg: cfg, cache: t.TempDir(), uploads: t.TempDir()}
	e.s = New(b.DB, Deps{Catalog: b.Catalog, Imager: img, Settings: cfg, Events: events.NewBus(), Tasks: e.tasks, CacheDir: e.cache, ImageDir: e.uploads}, Options{
		Audible: true, TMDbKey: "tmdb-key",
	})
	e.s.fetcher.get = fixtureGetter{e.fx}
	return e
}

// fakeTasks records the items asked for, by task.
type fakeTasks struct {
	mu    sync.Mutex
	asked map[string][]id.ID
}

func (f *fakeTasks) Ask(task string, items ...id.ID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.asked == nil {
		f.asked = map[string][]id.ID{}
	}
	f.asked[task] = append(f.asked[task], items...)
}

// take returns the items asked for on task since the last take.
func (f *fakeTasks) take(task string) []id.ID {
	f.mu.Lock()
	defer f.mu.Unlock()
	items := f.asked[task]
	delete(f.asked, task)
	return items
}

// viewer returns the Viewer of a new user who can see every library.
func (e *env) viewer() users.Viewer {
	e.t.Helper()
	svc := users.New(e.b.DB, events.NewBus())
	var user users.User
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		var err error
		user, err = svc.CreateUser(ctx, tx, users.NewUser{Username: "viewer-" + id.New().String(), Grants: users.Grants{All: true}})
		return err
	})
	v, err := svc.Viewer(ctx, user.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	return v
}

// movie adds a version of Dune (2021), matched to its TMDb ID, at rel under
// folder. Every call adds to the same work.
func (e *env) movie(folder id.ID, rel string) id.ID {
	e.t.Helper()
	w := e.b.Work(catalog.WorkInput{
		Kind: catalog.KindMovie, Title: "Dune", Year: 2021,
		ExternalIDs: []catalog.ExternalID{{Provider: "tmdb", Kind: string(catalog.KindMovie), ID: "438631"}},
	})
	e.b.Version([]id.ID{e.b.File(folder, rel)}, w)
	return w
}

func catalogID(provider, kind, x string) catalog.ExternalID {
	return catalog.ExternalID{Provider: provider, Kind: kind, ID: x}
}

// refresh runs a work's refresh.
func (e *env) refresh(work id.ID) error {
	e.t.Helper()
	return e.s.refresh(ctx, work, Freshness{})
}

func (e *env) mustRefresh(work id.ID) {
	e.t.Helper()
	if err := e.refresh(work); err != nil {
		e.t.Fatal(err)
	}
}

// column reads one column of a work row.
func (e *env) column(work id.ID, col string) string {
	e.t.Helper()
	var v sql.NullString
	if err := e.b.DB.Reader().QueryRow(`SELECT CAST(`+col+` AS TEXT) FROM work WHERE id = ?`, work).Scan(&v); err != nil {
		e.t.Fatal(err)
	}
	return v.String
}

// credits lists a work's credits as "role:name" in order.
func (e *env) credits(work id.ID) []string {
	e.t.Helper()
	rows, err := e.b.DB.Reader().Query(`SELECT c.role, p.name FROM credit c JOIN person p ON p.id = c.person_id
		WHERE c.work_id = ? ORDER BY c.role, c.ord`, work)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var role, name string
		if err := rows.Scan(&role, &name); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, role+":"+name)
	}
	return out
}

// images returns a work's image rows by kind.
func (e *env) images(work id.ID) map[string]imageRow {
	e.t.Helper()
	rows, err := e.s.q.WorkImages(ctx, work)
	if err != nil {
		e.t.Fatal(err)
	}
	out := map[string]imageRow{}
	for _, r := range rows {
		out[r.Kind] = imageRow{ID: r.ID, Source: r.Source, Origin: r.Origin, Chosen: r.Chosen, Blurhash: r.Blurhash}
	}
	return out
}

type imageRow struct {
	ID       id.ID
	Source   string
	Origin   string
	Chosen   bool
	Blurhash string
}

// localLibrary makes a library that uses local metadata, over a real
// temporary folder. It returns the library, its folder's ID and the
// folder's path.
func (e *env) localLibrary(kind catalog.LibraryKind) (lib, folder id.ID, dir string) {
	e.t.Helper()
	dir = e.t.TempDir()
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		l, err := e.b.Catalog.CreateLibrary(ctx, tx, catalog.LibraryInput{
			Name: "Local " + filepath.Base(dir), Kind: kind, Folders: []string{dir}, UseLocalMetadata: true,
		})
		lib, folder = l.ID, l.Folders[0].ID
		return err
	})
	return lib, folder, dir
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
