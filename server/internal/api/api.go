package api

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"sync"
	"sync/atomic"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/discovery"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/markers"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/playback"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/release"
	"github.com/mmcdole/kivo/internal/scan"
	"github.com/mmcdole/kivo/internal/subtitles"
	"github.com/mmcdole/kivo/internal/tasks"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
)

// Deps are the services the handlers call and facts about the server.
// Each handler parses the request, calls the service that owns the answer,
// and converts the result.
type Deps struct {
	Users     *users.Service
	Auth      *auth.Service
	Activity  *activity.Service
	Query     *query.Service
	Playback  *playback.Service
	Scan      *scan.Scanner
	Identify  *identify.Identifier
	Metadata  *metadata.Service
	Userstate *userstate.Service
	Catalog   *catalog.Catalog
	Subtitles *subtitles.Service
	Markers   *markers.Service
	Events    *events.Bus
	Tasks     *tasks.Runner
	// Problems lists the open problems of each component, for the problems
	// page.
	Problems *health.Registry
	// Logs are the recent log lines, for admins to read.
	Logs     *health.Logs
	Settings *config.Store
	// Discovery names the server's local URLs, for the server info.
	Discovery *discovery.Responder
	// Accelerators is what the startup accelerator test found, for the
	// transcoding settings.
	Accelerators playback.Accelerators
	// Version is the server's version, as the server info reports it.
	Version string
	// Release has the newer release, if one is out, for admins' server info.
	Release interface {
		Newer() (release.Release, bool)
	}
	// BasePath serves everything under a subpath, such as /kivo, behind a
	// reverse proxy. It is empty for the root. Otherwise it has a leading
	// slash and no trailing one, as config normalizes it.
	BasePath string
}

// Options are the HTTP layer's optional settings.
type Options struct {
	// TrustedProxies are the address ranges whose X-Forwarded-For and
	// X-Forwarded-Proto headers are believed. When empty, no proxy is
	// trusted.
	TrustedProxies []netip.Prefix
	// Logger logs requests and their failures; nil is slog.Default().
	Logger *slog.Logger
	// Web is the built web client (index.html at its root). Nil serves a
	// placeholder page.
	Web fs.FS
}

// Server is Kivo's HTTP handler: the API, media, the live-events socket and
// the web client, behind one middleware chain.
type Server struct {
	deps    Deps
	proxies []netip.Prefix
	log     *slog.Logger
	routes  map[string]route // by mux pattern
	mux     *http.ServeMux
	handler http.Handler
	web     *webApp
	metrics *metrics
	// errorReports holds the client error reports' rate limits.
	errorReports errorReports

	// Live-event sockets, closed on Close.
	sockMu   sync.Mutex // orders adding a socket with Close
	sockets  sync.WaitGroup
	closing  chan struct{}
	stopping atomic.Bool
}

// New builds the server. It fails if an operation in the spec has no known
// x-kivo-access, so a route can never be served without one.
func New(d Deps, opt Options) (*Server, error) {
	routes, err := embeddedRoutes()
	if err != nil {
		return nil, err
	}
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	web, err := newWebApp(opt.Web, d.BasePath)
	if err != nil {
		return nil, fmt.Errorf("load the web app: %w", err)
	}
	s := &Server{
		deps: d, proxies: opt.TrustedProxies,
		log: opt.Logger, routes: routes, mux: http.NewServeMux(), web: web, metrics: newMetrics(),
		closing: make(chan struct{}),
	}
	s.register()
	s.handler = s.chain(s.mux)
	return s, nil
}

// ServeHTTP serves one request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

// Close marks the server as stopping for /healthz, closes every live-events
// socket and waits for their goroutines. cmd/kivo calls it when shutting
// down, because http.Server.Shutdown doesn't close hijacked connections.
func (s *Server) Close() {
	s.sockMu.Lock()
	if !s.stopping.Swap(true) {
		close(s.closing)
	}
	s.sockMu.Unlock()
	s.sockets.Wait()
}

// register puts every route on the mux. These are the generated API
// routes, the hand-written ones (media, images and events), the API's 404
// and the web app.
func (s *Server) register() {
	strict := gen.NewStrictHandlerWithOptions(&handlers{s}, nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  s.requestError,
		ResponseErrorHandlerFunc: s.writeError,
	})
	gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseRouter:       s.mux,
		ErrorHandlerFunc: s.requestError,
	})
	s.mux.HandleFunc("GET /api/v1/images/{id}", s.serveImage)
	s.mux.HandleFunc("GET /api/v1/fonts/{sha256}", s.serveFont)
	s.mux.HandleFunc(eventsRoute, s.serveEvents)
	s.mux.HandleFunc("GET /api/v1/plays/{id}/hls/{name}", s.serveHLS)
	s.mux.HandleFunc("GET /api/v1/plays/{id}/files/{part}", s.serveFile)
	s.mux.HandleFunc("GET /api/v1/plays/{id}/subtitles/{name}", s.serveSubtitle)
	s.mux.HandleFunc("GET /api/v1/plays/{id}/previews/{name}", s.servePreview)
	s.mux.HandleFunc("GET /api/v1/downloads/{id}/files/{part}", s.serveDownloadFile)
	s.mux.HandleFunc("GET /api/v1/downloads/{id}/subtitles/{name}", s.serveDownloadSubtitle)
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		s.writeProblem(w, r, problem{status: http.StatusNotFound, code: codeNotFound})
	})
	s.mux.Handle("/", s.web)
}

// handlers implements the generated strict server interface.
type handlers struct{ *Server }

var _ gen.StrictServerInterface = (*handlers)(nil)

// ctxKey keys this package's values in a request context.
type ctxKey int

const (
	keyRequest ctxKey = iota
	keyCaller
)
