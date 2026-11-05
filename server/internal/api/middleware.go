package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/metadata"
)

// chain wraps h in the middleware. From outermost to innermost, the layers
// are observe (request ID, log and metrics), recover, security headers,
// the base path, and guard (authentication and access).
func (s *Server) chain(mux *http.ServeMux) http.Handler {
	return s.observe(s.recoverer(s.headers(s.strip(s.guard(mux)))))
}

// request holds what the middleware learns about a request. Every layer
// shares it through the context.
type request struct {
	id     string
	route  string // the mux pattern, never the raw path, which can hold a play's ID
	op     string // the route's operation ID, if it's an API route
	ip     netip.Addr
	secure bool // TLS, here or at a trusted proxy
	host   string
	// clientCookie is the browser's client ID from its client cookie. It
	// is empty if the browser sent none.
	clientCookie string
	device       auth.Device // the app and device, from the identity headers
}

func requestOf(ctx context.Context) *request {
	if r, ok := ctx.Value(keyRequest).(*request); ok {
		return r
	}
	return &request{}
}

func requestID(ctx context.Context) string { return requestOf(ctx).id }
func routeOf(r *http.Request) string       { return requestOf(r.Context()).route }

// observe gives the request its ID and context, then logs it and counts it
// in the metrics.
func (s *Server) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		var b [8]byte
		_, _ = rand.Read(b[:])
		req := &request{id: hex.EncodeToString(b[:]), route: "unmatched"}
		req.ip, req.secure, req.host = s.client(r)
		if c, err := r.Cookie(cookieName(clientCookieName, req.secure)); err == nil {
			req.clientCookie = c.Value
		}
		req.device = parseDevice(r.Header)
		w.Header().Set("X-Request-Id", req.id)
		sw := &statusWriter{ResponseWriter: w, rc: http.NewResponseController(w)}
		_ = sw.rc.SetWriteDeadline(time.Time{}) // the connection's previous response may have set one
		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), keyRequest, req)))
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		d := time.Since(start)
		s.metrics.observe(req.route, sw.status, d)
		s.log.Log(r.Context(), requestLevel(req.route, sw.status, d), "api: request",
			"request", req.id, "method", r.Method, "route", req.route, "status", sw.status, "duration", d.Round(time.Microsecond))
	})
}

// client resolves the client's address, whether the connection is secure,
// and the host it reached. Forwarded headers count only from a trusted
// proxy.
func (s *Server) client(r *http.Request) (ip netip.Addr, secure bool, host string) {
	addr, _ := netip.ParseAddrPort(r.RemoteAddr)
	ip, secure, host = addr.Addr().Unmap(), r.TLS != nil, r.Host
	if !s.trusted(ip) {
		return ip, secure, host
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		ip = hop.Unmap()
		if !s.trusted(ip) {
			break
		}
	}
	if proto, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ","); strings.TrimSpace(proto) != "" {
		secure = strings.EqualFold(strings.TrimSpace(proto), "https")
	}
	if fh, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Host"), ","); strings.TrimSpace(fh) != "" {
		host = strings.TrimSpace(fh)
	}
	return ip, secure, host
}

func (s *Server) trusted(ip netip.Addr) bool {
	for _, p := range s.proxies {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// writeStall is how long one write of a response may block on a client
// that stops reading. A response as a whole has no time limit, since media
// streams for as long as the client plays.
const writeStall = 10 * time.Minute

// statusWriter records the status a handler sent, and gives each write
// writeStall to finish.
type statusWriter struct {
	http.ResponseWriter
	rc     *http.ResponseController
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	_ = w.rc.SetWriteDeadline(time.Now().Add(writeStall))
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// slowRequest is how long an API call (not a media stream) can take before
// it's worth a line at Info.
const slowRequest = time.Second

// requestLevel keeps the log readable on a busy server. Every request is
// logged at Debug. Server errors and slow API calls are logged at Info or
// above. Media routes stream for as long as the client reads, so their
// duration isn't counted as slow.
func requestLevel(route string, status int, d time.Duration) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelWarn
	case d >= slowRequest && !strings.Contains(route, "/plays/{id}/") && !strings.Contains(route, "/events"):
		return slog.LevelInfo
	}
	return slog.LevelDebug
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.log.Error("api: handler panicked", "request", requestID(r.Context()), "route", routeOf(r),
					"panic", fmt.Sprint(v), "stack", string(debug.Stack()))
				s.writeProblem(w, r, problem{status: http.StatusInternalServerError, code: codeInternal})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// contentSecurityPolicy keeps the web client to its own origin. It also
// allows blob and data URLs for media and images, WebAssembly, and
// Google's Cast sender library over the page's own scheme. It forbids
// framing.
const contentSecurityPolicy = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval' www.gstatic.com; worker-src 'self' blob:; " +
	"media-src 'self' blob: data:; img-src 'self' data: blob:; connect-src 'self'; style-src 'self'; " +
	"object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

func (s *Server) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

// strip serves everything under the base path, and nothing outside it.
func (s *Server) strip(next http.Handler) http.Handler {
	if s.deps.BasePath == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == s.deps.BasePath {
			http.Redirect(w, r, s.deps.BasePath+"/", http.StatusMovedPermanently)
			return
		}
		rest, ok := strings.CutPrefix(r.URL.Path, s.deps.BasePath+"/")
		if !ok {
			s.writeProblem(w, r, errNotFound)
			return
		}
		r2 := new(http.Request)
		*r2 = *r
		u := *r.URL
		u.Path = "/" + rest
		u.RawPath = ""
		r2.URL = &u
		next.ServeHTTP(w, r2)
	})
}

// guard enforces the route's access level. It answers CORS preflights,
// then checks credentials, the same-origin rule for cookie sessions, and
// the level.
func (s *Server) guard(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPreflight(r) {
			s.preflight(w, r, mux)
			return
		}
		_, pattern := mux.Handler(r)
		rt, known := s.routes[pattern]
		if pattern != "" {
			requestOf(r.Context()).route = pattern
		}
		requestOf(r.Context()).op = rt.op
		if !known {
			mux.ServeHTTP(w, r) // the web app and the API's 404
			return
		}
		limitBody(w, r, rt.upload)
		if rt.level != play {
			// An answer can carry a token or session data. A route that
			// may be cached, as images and fonts are, says so itself.
			w.Header().Set("Cache-Control", "no-store")
		}
		if rt.receiver {
			allowReceiver(w, r)
		}
		switch rt.level {
		case public:
			if unsafeMethod(r.Method) && r.Header.Get("Origin") != "" && !sameOrigin(r) {
				s.writeProblem(w, r, problem{status: http.StatusForbidden, code: codeCrossOrigin})
				return
			}
			// A public route still learns about a signed-in caller. Server
			// info uses this to add the server's addresses.
			if c, err := s.authenticate(r, signedIn); err == nil {
				r = r.WithContext(context.WithValue(r.Context(), keyCaller, c))
			}
		case play:
			allowAnyOrigin(w)
		default:
			c, err := s.authenticate(r, rt.level)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), keyCaller, c))
		}
		mux.ServeHTTP(w, r)
	})
}

// bodyLimit is how large a request's body may be and how long reading it
// may take.
type bodyLimit struct {
	size    int64
	timeout time.Duration
}

// The limits on a JSON body, and on an upload's file and form.
var (
	jsonBody   = bodyLimit{size: 1 << 20, timeout: 30 * time.Second}
	uploadBody = bodyLimit{size: metadata.MaxUpload + 1<<20, timeout: 10 * time.Minute}
)

// limitBody caps the size of the request's body and the time to read it.
func limitBody(w http.ResponseWriter, r *http.Request, upload bool) {
	if r.Body == nil || r.Body == http.NoBody {
		return
	}
	l := jsonBody
	if upload {
		l = uploadBody
	}
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(l.timeout))
	r.Body = timedBody{ReadCloser: http.MaxBytesReader(w, r.Body, l.size), rc: rc}
}

// timedBody clears the read deadline once the body ends. The server keeps
// reading after the body to notice a client hanging up. A deadline left
// set would cancel a slow response when it passed.
type timedBody struct {
	io.ReadCloser
	rc *http.ResponseController
}

func (b timedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err == io.EOF {
		_ = b.rc.SetReadDeadline(time.Time{})
	}
	return n, err
}

// sameOrigin reports whether the request's Origin is this server at the
// address the client used to reach it.
func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return false
	}
	u, err := url.Parse(o)
	if err != nil {
		return false
	}
	req := requestOf(r.Context())
	scheme := "http"
	if req.secure {
		scheme = "https"
	}
	return u.Scheme == scheme && strings.EqualFold(hostPort(u.Host, scheme), hostPort(req.host, scheme))
}

// hostPort adds the scheme's default port, so "kivo" and "kivo:80" match.
func hostPort(h, scheme string) string {
	if _, _, err := net.SplitHostPort(h); err == nil {
		return h
	}
	if scheme == "https" {
		return net.JoinHostPort(strings.Trim(h, "[]"), "443")
	}
	return net.JoinHostPort(strings.Trim(h, "[]"), "80")
}

func unsafeMethod(m string) bool {
	return m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions
}

func isUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}
