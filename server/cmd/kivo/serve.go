package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/api"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/discovery"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/markers"
	"github.com/mmcdole/kivo/internal/media/ffmpeg"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/playback"
	"github.com/mmcdole/kivo/internal/previews"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/release"
	"github.com/mmcdole/kivo/internal/scan"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/subtitles"
	"github.com/mmcdole/kivo/internal/tasks"
	"github.com/mmcdole/kivo/internal/transcode"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
	"github.com/mmcdole/kivo/web"
)

// server is every component, wired.
type server struct {
	boot       config.Bootstrap
	logs       *health.Logs
	db         *store.DB
	settings   *config.Store
	runner     *tasks.Runner
	activity   *activity.Service
	catalog    *catalog.Catalog
	users      *users.Service
	auth       *auth.Service
	query      *query.Service
	userstate  *userstate.Service
	metadata   *metadata.Service
	scanner    *scan.Scanner
	identifier *identify.Identifier
	markers    *markers.Service
	previews   *previews.Service
	subtitles  *subtitles.Service
	playback   *playback.Service
	ffmpeg     *ffmpeg.Runner
	problems   *health.Registry
	discovery  *discovery.Responder
	release    *release.Checker
	api        *api.Server
}

func serve(configPath string) error {
	boot, err := config.LoadBootstrap(configPath)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return inTray(ctx, boot, func(ctx context.Context) error {
		bus := events.NewBus()
		defer bus.Close()
		logs := setupLogging(boot, bus)
		slog.Info("kivo: starting", "version", version)
		return run(ctx, boot, logs, bus)
	})
}

// logLines is how many recent log lines are kept for admins to read.
const logLines = 5000

// setupLogging logs to stderr as text or JSON. It also keeps the newest
// lines at the log level in memory, and those publish on bus.
func setupLogging(boot config.Bootstrap, bus *events.Bus) *health.Logs {
	var level slog.Level
	_ = level.UnmarshalText([]byte(boot.LogLevel)) // validated by config
	opt := &slog.HandlerOptions{Level: level}
	var h slog.Handler = slog.NewTextHandler(os.Stderr, opt)
	if boot.LogFormat == "json" {
		h = slog.NewJSONHandler(os.Stderr, opt)
	}
	logs := health.NewLogs(logLines, bus)
	slog.SetDefault(slog.New(logs.Handler(h)))
	return logs
}

// run wires the server, runs every part and serves the API on boot.Listen,
// until ctx ends.
func run(ctx context.Context, boot config.Bootstrap, logs *health.Logs, bus *events.Bus) error {
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	s, err := wire(ctx, boot, logs, bus)
	if err != nil {
		return err
	}
	defer s.db.Close()
	if err := s.scanner.Resume(ctx); err != nil {
		return err
	}

	// Every long-running part has an owner here and stops with ctx.
	runners := map[string]func(context.Context) error{
		"tasks":    s.runner.Run,
		"auth":     s.auth.Run,
		"problems": s.problems.Run,
		"playback": s.playback.Run,
		"watcher":  s.scanner.Watch,
		"release":  s.release.Run,
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(runners)+len(boot.Listen)) // one slot per runner and listener, so no send ever blocks
	for name, run := range runners {
		wg.Go(func() {
			if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errs <- fmt.Errorf("%s: %w", name, err)
				stop()
			}
		})
	}
	// Capabilities stays empty until the accelerator test finishes, seconds after start.
	wg.Go(func() { s.ffmpeg.DetectAccelerators(ctx, filepath.Join(boot.CacheDir, "media")) })
	if err := s.listen(ctx, &wg, errs, stop); err != nil {
		stop()
		wg.Wait()
		return err
	}
	s.advertise(ctx, &wg)

	<-ctx.Done()
	slog.InfoContext(ctx, "kivo: stopping")
	stopping := time.Now()
	wg.Wait() // playback ends its plays, writing their positions, and stops their ffmpeg jobs
	slog.InfoContext(ctx, "kivo: stopped", "took", time.Since(stopping).Round(time.Millisecond))
	close(errs)
	return errors.Join(collect(errs)...)
}

// wire builds every component in dependency order, giving each the parts
// built before it and its Options. Consumers get the small interfaces they
// declare. Tasks are added here, before the runner runs. Each service
// publishes its own changes on bus.
//
// A callback is used only where a direct call would close a cycle or go up
// a layer:
//   - The runner's callback records a run's end in activity.
//   - The subtitles callback moves playback's downloads.
//   - The settings callback notifies the watcher and the advertisement,
//     which are built after settings and reached through s.
//   - The health callbacks publish and record problems, and announce log
//     lines.
//
// None of the callbacks is called before run.
func wire(ctx context.Context, boot config.Bootstrap, logs *health.Logs, bus *events.Bus) (*server, error) {
	for _, dir := range []string{boot.DataDir, boot.CacheDir, boot.BackupDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := store.Open(ctx, boot.DatabasePath())
	if err != nil {
		return nil, err
	}
	s := &server{boot: boot, logs: logs, db: db}
	if s.settings, err = config.Open(ctx, db, config.Options{Changed: s.settingsChanged}); err != nil {
		return nil, err
	}
	s.ffmpeg = ffmpeg.New(boot.FFmpeg, boot.FFprobe)
	logTool(ctx, boot.FFmpeg)
	logTool(ctx, boot.FFprobe)
	s.activity = activity.New(db, bus)
	s.runner = tasks.New(db, s.settings, bus, tasks.Options{Ended: s.activity.TaskRunEnded})
	s.catalog = catalog.New(db, bus)
	s.users = users.New(db, bus)
	s.auth = auth.New(db, auth.Deps{Users: s.users, Settings: s.settings, Activity: s.activity})
	s.query = query.New(db)
	s.userstate = userstate.New(db, userstate.Deps{Catalog: s.catalog, Settings: s.settings, Activity: s.activity, Events: bus})
	s.metadata = metadata.New(db, metadata.Deps{
		Catalog: s.catalog, Imager: s.ffmpeg, Settings: s.settings, Events: bus, Tasks: s.runner,
		CacheDir: boot.CacheDir, ImageDir: filepath.Join(boot.DataDir, "images"),
	}, metadata.Options{Audible: boot.Audible, TMDbKey: boot.TMDbKey})
	s.previews = previews.New(db, previews.Deps{Catalog: s.catalog, Media: s.ffmpeg, Tasks: s.runner, CacheDir: boot.CacheDir})
	s.subtitles = subtitles.New(db, subtitles.Deps{
		Catalog: s.catalog, Settings: s.settings, Media: s.ffmpeg, Tasks: s.runner,
		CacheDir: boot.CacheDir, SubtitleDir: filepath.Join(boot.DataDir, "subtitles"),
	}, subtitles.Options{
		Replaced: func(ctx context.Context, tx *store.Tx, replaced []id.ID, by id.ID) error {
			return s.playback.SubtitlesReplaced(ctx, tx, replaced, by)
		},
	})
	s.markers = markers.New(db, markers.Deps{
		Catalog: s.catalog, Media: s.ffmpeg, Tasks: s.runner, Settings: s.settings, CacheDir: boot.CacheDir,
	})
	s.identifier = identify.New(db, identify.Deps{
		Catalog: s.catalog, Metadata: s.metadata, Userstate: s.userstate, Activity: s.activity, Tasks: s.runner, Markers: s.markers,
	}, identify.Options{})
	s.scanner = scan.New(db, scan.Deps{
		Catalog: s.catalog, Tasks: s.runner, Prober: s.ffmpeg, Settings: s.settings, Identifier: s.identifier,
		Previews: s.previews, Subtitles: s.subtitles,
	}, scan.Options{})
	if !s.ffmpeg.Chromaprint(ctx) {
		slog.WarnContext(ctx, "kivo: this ffmpeg writes no Chromaprint fingerprints; intro and credits markers come from chapters only", "path", boot.FFmpeg)
	}
	tc, err := transcode.New(transcode.Deps{
		Media: s.ffmpeg, Keys: transcode.ContainerIndex{},
		Dir: filepath.Join(boot.CacheDir, "transcode"), Downloads: filepath.Join(boot.CacheDir, "downloads"),
	}, transcode.Options{})
	if err != nil {
		return nil, fmt.Errorf("set up transcoding: %w", err)
	}
	s.playback = playback.New(db, playback.Deps{
		Catalog: s.catalog, Users: s.users, Auth: s.auth, Userstate: s.userstate, Transcoder: tc, Settings: s.settings, Markers: s.markers, Tasks: s.runner,
		Activity: s.activity, Subtitles: s.subtitles, Previews: s.previews, Accelerators: s.ffmpeg, Events: bus,
	})
	s.problems = health.NewRegistry([]health.Reporter{db, s.metadata, s.scanner, s.previews, s.subtitles, s.markers, s.playback, s.activity}, bus, health.RegistryOptions{
		Raised: func(ctx context.Context, raised []health.OpenProblem) {
			if err := s.activity.ProblemsRaised(ctx, raised); err != nil {
				slog.WarnContext(ctx, "kivo: record problems raised", "err", err)
			}
		},
	})

	for _, t := range s.tasks() {
		s.runner.Add(t)
	}
	s.discovery = discovery.New(discovery.Deps{
		Settings: s.settings, Version: version, Listen: boot.Listen, TLS: boot.TLSCert != "", BasePath: boot.BasePath,
	})
	s.release = release.New(release.Options{Version: version, Check: boot.UpdateCheck})
	s.api, err = api.New(api.Deps{
		Users: s.users, Auth: s.auth, Activity: s.activity, Query: s.query, Playback: s.playback,
		Scan: s.scanner, Identify: s.identifier, Metadata: s.metadata, Userstate: s.userstate,
		Catalog: s.catalog, Subtitles: s.subtitles, Markers: s.markers, Events: bus, Tasks: s.runner, Problems: s.problems, Logs: logs,
		Settings: s.settings, Discovery: s.discovery, Accelerators: s.ffmpeg, Version: version, Release: s.release, BasePath: boot.BasePath,
	}, api.Options{TrustedProxies: boot.TrustedPrefixes(), Logger: slog.Default(), Web: web.Files()})
	if err != nil {
		return nil, fmt.Errorf("set up the API: %w", err)
	}
	return s, nil
}

// logTool logs which ffmpeg or ffprobe the server will run, or warns that
// it can't run it. That is the first thing to check when nothing probes or
// plays.
func logTool(ctx context.Context, path string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	v, err := ffmpeg.Version(ctx, path)
	if err != nil {
		slog.WarnContext(ctx, "kivo: can't run "+filepath.Base(path)+"; probing and playback will fail", "path", path, "err", err)
		return
	}
	slog.InfoContext(ctx, "kivo: using "+filepath.Base(path), "path", path, "version", v)
}

// settingsChanged tells the watcher when file watching is turned on or off,
// and tells the advertisement when the server is renamed.
func (s *server) settingsChanged(was, is config.Settings) {
	if was.FileWatching != is.FileWatching {
		s.scanner.FileWatchingChanged()
	}
	if was.ServerName != is.ServerName {
		s.discovery.Renamed()
	}
}

// listen serves the API on every listen address until ctx ends. The HTTP
// servers time out reading headers and idle kept-alive connections. They
// set no read or write timeout, because that would end a long media
// stream. The api package limits each request's body and each write
// instead. Live-event sockets are closed first on shutdown, because
// http.Server.Shutdown doesn't close hijacked connections.
func (s *server) listen(ctx context.Context, wg *sync.WaitGroup, errs chan<- error, stop func()) error {
	for _, addr := range s.boot.Listen {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}
		srv := &http.Server{Handler: s.api, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
		slog.InfoContext(ctx, "kivo: listening", "addr", ln.Addr().String(), "tls", s.boot.TLSCert != "")
		wg.Go(func() {
			var err error
			if s.boot.TLSCert != "" {
				err = srv.ServeTLS(ln, s.boot.TLSCert, s.boot.TLSKey)
			} else {
				err = srv.Serve(ln)
			}
			if !errors.Is(err, http.ErrServerClosed) {
				errs <- fmt.Errorf("http %s: %w", addr, err)
				stop()
			}
		})
		wg.Go(func() {
			<-ctx.Done()
			s.api.Close()
			sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			_ = srv.Shutdown(sctx)
		})
	}
	return nil
}

// advertise announces the server on the local network until ctx ends.
// Being seen there needs host networking. Without multicast DNS the server
// carries on, and apps reach it by its address.
func (s *server) advertise(ctx context.Context, wg *sync.WaitGroup) {
	wg.Go(func() {
		if err := s.discovery.Advertise(ctx); err != nil {
			slog.WarnContext(ctx, "kivo: LAN discovery is off; it needs host networking", "err", err)
		}
	})
}

func collect(errs <-chan error) []error {
	var out []error
	for err := range errs {
		out = append(out, err)
	}
	return out
}
