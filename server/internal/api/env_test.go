package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"io/fs"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/discovery"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/markers"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/playback"
	"github.com/mmcdole/kivo/internal/previews"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/release"
	"github.com/mmcdole/kivo/internal/scan"
	"github.com/mmcdole/kivo/internal/subtitles"
	"github.com/mmcdole/kivo/internal/tasks"
	"github.com/mmcdole/kivo/internal/transcode"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
)

var ctx = context.Background()

// noMedia refuses every transcode, because these tests play files
// directly. A conversion writes the source's name as its output. It probes
// nothing and makes no previews, subtitles or fingerprints.
type noMedia struct{}

func (noMedia) Probe(context.Context, string) (media.ProbeResult, error) {
	return media.ProbeResult{}, errors.New("no probing in api tests")
}

func (noMedia) Transcode(context.Context, media.TranscodeRequest) (media.Output, error) {
	return nil, errors.New("no transcoding in api tests")
}

func (noMedia) Convert(_ context.Context, req media.ConvertRequest) error {
	return os.WriteFile(req.Dst, []byte(req.Path), 0o644)
}

func (noMedia) Capabilities() media.Capabilities {
	return media.Capabilities{Tested: true, CPU: media.Caps{Backend: media.Software, Name: "CPU",
		Encoders: []media.EncoderCaps{{Codec: media.H264, GOP: media.GridGOP}}, ToneMap: media.OnCPU, BurnIn: media.OnCPU}}
}

func (noMedia) Keyframes(context.Context, string) ([]media.Keyframe, bool, error) {
	return nil, false, nil
}

func (noMedia) Sample(context.Context, media.SampleRequest) (media.Sample, error) {
	return media.Sample{}, errors.New("no sampling in api tests")
}

// Chromaprint is false, so no markers are fingerprinted.
func (noMedia) Chromaprint(context.Context) bool { return false }

func (noMedia) Previews(context.Context, media.PreviewsRequest) (media.Previews, error) {
	return media.Previews{}, errors.New("no previews in api tests")
}

func (noMedia) Subtitles(context.Context, media.SubtitlesRequest) error {
	return errors.New("no subtitle extraction in api tests")
}

// fakeImager stands in for ffmpeg's artwork re-encoding. It writes a file
// for each width and returns a small frame for the blurhash.
type fakeImager struct{}

func (fakeImager) Image(ctx context.Context, src string, outs []media.ImageOut) ([]byte, error) {
	for _, o := range outs {
		if err := os.WriteFile(o.Path, []byte("RIFF webp"), 0o644); err != nil {
			return nil, err
		}
	}
	var b bytes.Buffer
	err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 32, 48)))
	return b.Bytes(), err
}

// logBuffer captures the server's log, safely across goroutines.
type logBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// env is a server over real services and a seeded catalog. The catalog
// has an admin, a user granted nothing, a movies library with a playable
// file on disk, and a series.
type env struct {
	t    *testing.T
	b    *catalogtest.Builder
	d    Deps
	srv  *Server
	logs *logBuffer

	adminToken, nobodyToken string
	adminSession            id.ID
	admin                   id.ID // the user of adminToken
	nobody                  id.ID // the user of nobodyToken

	lib, folder  id.ID
	heat         catalogtest.Item // a movie whose file exists
	series       id.ID
	episode      catalogtest.Item
	seededTitles []string
	cache        string    // the cache folder
	reported     *reported // problems a test reports, among the services'
	asks         *asks     // what the services asked of the runner
	// openFolder, when set, opens a library folder for a scan instead of
	// reading the disk.
	openFolder func(path string) (fs.FS, func() error, error)
}

// asks records what is asked of the runner, by task, and passes each
// request on.
type asks struct {
	r  *tasks.Runner
	mu sync.Mutex
	by map[string][]id.ID
}

func (a *asks) Ask(task string, items ...id.ID) {
	a.mu.Lock()
	a.by[task] = append(a.by[task], items...)
	a.mu.Unlock()
	a.r.Ask(task, items...)
}

// of is what was asked of a task, in order.
func (a *asks) of(task string) []id.ID {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]id.ID(nil), a.by[task]...)
}

// reported is a component whose problems a test sets.
type reported struct{ problems []health.Problem }

func (r *reported) Health(context.Context) []health.Problem { return r.problems }

// makePreviews puts the seeded film's scrub previews, a single sheet,
// where the previews package keeps them once made. It returns the folder
// for their sheets.
func (e *env) makePreviews() string {
	e.t.Helper()
	f, err := e.b.Catalog.File(ctx, e.heat.File)
	if err != nil {
		e.t.Fatal(err)
	}
	dir := config.CachePath(filepath.Join(e.cache, "previews"), f.ID, f.ID.String(), config.CacheLeaf(f.Generation, 0))
	b, err := json.Marshal(media.Previews{Interval: 10 * time.Second, Width: 320, Height: 180, Columns: 10, Rows: 10, Sheets: 1})
	if err != nil {
		e.t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o644); err != nil {
		e.t.Fatal(err)
	}
	return dir
}

const (
	adminPassword = "correct horse battery"
	heatBytes     = "0123456789abcdefghijklmnopqrstuvwxyz"
)

// serving is how a test's server is served. It holds api's Options, and
// the base path and version the server is built with.
type serving struct {
	Options
	BasePath, Version string
	Newer             release.Release // the newer release, if its version is set
}

// newer is the release checker's answer.
type newer release.Release

func (n newer) Newer() (release.Release, bool) { return release.Release(n), n.Version != "" }

func newEnv(t *testing.T, o serving) *env {
	t.Helper()
	e := &env{t: t, b: catalogtest.New(t), logs: &logBuffer{}}
	db := e.b.DB
	settings, err := config.Open(ctx, db, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus()
	e.d = Deps{
		Catalog: e.b.Catalog, Users: users.New(db, bus), Query: query.New(db), Settings: settings,
		Tasks: tasks.New(db, settings, events.NewBus(), tasks.Options{}), Events: bus,
		Logs: health.NewLogs(1000, events.NewBus()),
	}
	e.asks = &asks{r: e.d.Tasks, by: map[string][]id.ID{}}
	e.d.Activity = activity.New(db, events.NewBus())
	e.d.Auth = auth.New(db, auth.Deps{Users: e.d.Users, Settings: settings, Activity: e.d.Activity})
	e.d.Metadata = metadata.New(db, metadata.Deps{Catalog: e.b.Catalog, Imager: fakeImager{}, Settings: e.d.Settings, Events: e.d.Events, Tasks: e.asks, CacheDir: t.TempDir(), ImageDir: t.TempDir()}, metadata.Options{})
	e.d.Userstate = userstate.New(db, userstate.Deps{Catalog: e.b.Catalog, Settings: settings, Activity: e.d.Activity, Events: e.d.Events})
	e.cache = t.TempDir()
	e.d.Markers = markers.New(db, markers.Deps{Catalog: e.b.Catalog, Media: noMedia{}, Tasks: e.asks, Settings: e.d.Settings, CacheDir: e.cache})
	pv := previews.New(db, previews.Deps{Catalog: e.b.Catalog, Media: noMedia{}, Tasks: e.asks, CacheDir: e.cache})
	e.d.Subtitles = subtitles.New(db, subtitles.Deps{Catalog: e.b.Catalog, Settings: e.d.Settings, Media: noMedia{}, Tasks: e.asks, CacheDir: e.cache, SubtitleDir: t.TempDir()}, subtitles.Options{})
	e.d.Identify = identify.New(db, identify.Deps{
		Catalog: e.b.Catalog, Metadata: e.d.Metadata, Userstate: e.d.Userstate, Activity: e.d.Activity, Tasks: e.asks, Markers: e.d.Markers,
	}, identify.Options{})
	e.d.Scan = scan.New(db, scan.Deps{
		Catalog: e.b.Catalog, Tasks: e.asks, Prober: noMedia{}, Settings: settings, Identifier: e.d.Identify, Previews: pv, Subtitles: e.d.Subtitles,
	}, scan.Options{
		OpenFolder: func(path string) (fs.FS, func() error, error) {
			if e.openFolder != nil {
				return e.openFolder(path)
			}
			return catalog.OpenFolder(path)
		},
	})
	tc, err := transcode.New(transcode.Deps{Media: noMedia{}, Keys: noMedia{}, Dir: t.TempDir(), Downloads: t.TempDir()}, transcode.Options{Floor: 1})
	if err != nil {
		t.Fatal(err)
	}
	e.d.Accelerators = noMedia{}
	e.d.Playback = playback.New(db, playback.Deps{
		Catalog: e.b.Catalog, Users: e.d.Users, Auth: e.d.Auth, Userstate: e.d.Userstate, Transcoder: tc, Settings: e.d.Settings, Markers: e.d.Markers, Tasks: e.asks,
		Activity: e.d.Activity, Subtitles: e.d.Subtitles, Previews: pv, Accelerators: e.d.Accelerators, Events: e.d.Events,
	})
	for _, ts := range [][]tasks.Task{e.d.Scan.Tasks(), e.d.Metadata.Tasks(), pv.Tasks(), e.d.Markers.Tasks(), e.d.Subtitles.Tasks(), e.d.Playback.Tasks(), e.b.Catalog.Tasks()} {
		for _, task := range ts {
			e.d.Tasks.Add(task)
		}
	}
	e.d.Version, e.d.BasePath, e.d.Release = o.Version, o.BasePath, newer(o.Newer)
	e.d.Discovery = discovery.New(discovery.Deps{Settings: settings, Version: o.Version, Listen: []string{":8484", "127.0.0.1:9000"}, BasePath: o.BasePath})
	e.reported = &reported{}
	e.d.Problems = health.NewRegistry([]health.Reporter{e.d.Scan, e.d.Playback, e.reported}, e.d.Events, health.RegistryOptions{})

	iss, err := e.d.Auth.Setup(ctx, auth.SignInRequest{Username: "admin", Password: adminPassword, Kind: auth.SessionApp, Name: "Test"})
	if err != nil {
		t.Fatal(err)
	}
	e.adminToken, e.adminSession, e.admin = iss.Token, iss.Session.ID, iss.Session.User

	// nobody has no grants, as happens when every library they were
	// granted is deleted.
	var nobody users.User
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		if nobody, err = e.d.Users.CreateUser(ctx, tx, users.NewUser{Username: "nobody", Grants: users.Grants{All: true}}); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE user SET all_libraries = 0 WHERE id = ?`, nobody.ID)
		return err
	})
	e.nobody, e.nobodyToken = nobody.ID, e.session(nobody.ID)

	e.seed()
	if o.Logger == nil {
		o.Logger = slog.New(e.d.Logs.Handler(slog.NewTextHandler(e.logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}
	if e.srv, err = New(e.d, o.Options); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.srv.Close)
	return e
}

// run runs the task runner until the test ends. Without it, requests to
// the runner wait, and Run now begins a run that does nothing.
func (e *env) run() {
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = e.d.Tasks.Run(runCtx); close(done) }()
	e.t.Cleanup(func() { cancel(); <-done })
}

// session starts an app session for a user directly, without its
// password.
func (e *env) session(user id.ID) string {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	hash := sha256.Sum256(raw)
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO session (id, token_hash, user_id, kind, name, created_at, last_used_at)
			VALUES (?, ?, ?, 'app', 'test', ?, ?)`, id.New(), hash[:], user, time.Now().UnixMilli(), time.Now().UnixMilli())
		return err
	})
	return "kivo_s_" + base64.RawURLEncoding.EncodeToString(raw)
}

func (e *env) seed() {
	t := e.t
	dir := t.TempDir()
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		l, err := e.b.Catalog.CreateLibrary(ctx, tx, catalog.LibraryInput{Name: "Seeded Films", Kind: catalog.Movies, Folders: []string{dir}})
		e.lib, e.folder = l.ID, l.Folders[0].ID
		return err
	})
	e.heat = e.b.Movie(e.folder, "Heatwave Seeded", 1995)
	f, err := e.b.Catalog.File(ctx, e.heat.File)
	if err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(dir, f.RelPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(heatBytes), 0o644); err != nil {
		t.Fatal(err)
	}
	track := func(tr media.Track) catalog.Track {
		info, _ := json.Marshal(tr)
		return catalog.Track{Index: tr.Index, Type: string(tr.Kind), Codec: string(tr.Codec), Default: tr.Default, Info: info}
	}
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.Catalog.SetProbe(ctx, tx, e.heat.File, f.Generation, catalog.Probe{
			Summary: catalog.ProbeSummary{Container: "mov,mp4,m4a,3gp,3g2,mj2", Duration: 2 * time.Hour, Bitrate: 8_000_000},
			Tracks: []catalog.Track{
				track(media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8}),
				track(media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Default: true}),
			},
			Chapters: []catalog.Chapter{
				{Index: 0, Start: 0, End: 3 * time.Minute, Title: "Opening Credits"},
				{Index: 1, Start: 3 * time.Minute, End: 110 * time.Minute, Title: "Film"},
				{Index: 2, Start: 110 * time.Minute, End: 2 * time.Hour, Title: "End Credits"},
			},
		})
	})
	e.series = e.b.Series("Seeded Series")
	e.episode = e.b.Episode(e.folder, e.series, 1, 1)
	e.seededTitles = []string{"Heatwave Seeded", "Seeded Series", "Seeded Films"}
}

// do sends a request through the server.
func (e *env) do(method, path, token string, body any, headers ...string) *httptest.ResponseRecorder {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	r := httptest.NewRequest(method, path, rd)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	e.srv.ServeHTTP(w, r)
	return w
}

// problemCode returns a problem response's code, or "" for any other body.
func problemCode(w *httptest.ResponseRecorder) string {
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/problem+json") {
		return ""
	}
	var p struct{ Code string }
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	return p.Code
}

// clientCaps are capabilities that play the seeded MP4 directly.
var clientCaps = map[string]any{
	"file":      map[string]any{"containers": []string{"mp4"}},
	"video":     []map[string]any{{"codec": "h264", "delivery": []string{"file", "stream"}}},
	"audio":     []map[string]any{{"codec": "aac", "delivery": []string{"file", "stream"}, "maxChannels": 2}},
	"subtitles": []map[string]any{{"codec": "webvtt", "delivery": []string{"stream", "external"}}},
}
