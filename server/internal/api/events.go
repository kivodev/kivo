package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/playback"
	"github.com/mmcdole/kivo/internal/users"
)

// eventsRoute is the live-events socket's route.
const eventsRoute = "GET /api/v1/events"

// eventsProtocol is the subprotocol the live-events socket answers to a
// client that offers its token as one.
const eventsProtocol = "kivo"

// serveEvents serves the live-events socket. Each connection runs one
// goroutine over a subscription filtered by the caller's Viewer. A
// snapshot is sent first. If the subscription lags, the socket sends a
// resync frame and a fresh snapshot.
func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request) {
	c := callerOf(r.Context())
	s.sockMu.Lock()
	if s.stopping.Load() {
		s.sockMu.Unlock()
		s.writeProblem(w, r, problem{status: http.StatusServiceUnavailable, code: codeInternal, detail: "the server is stopping"})
		return
	}
	s.sockets.Add(1)
	s.sockMu.Unlock()
	defer s.sockets.Done()

	// guard has already required cookie sessions to send a same-origin
	// Origin, including through a trusted proxy's Host. Native clients and
	// the packaged TV app send a token and no Origin. A client that offers
	// its token as a subprotocol also offers eventsProtocol, which is the
	// one the answer names, so the token is never sent back.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true, Subprotocols: []string{eventsProtocol}})
	if err != nil {
		return // Accept answered the request
	}
	defer conn.CloseNow()
	ctx := conn.CloseRead(context.WithoutCancel(r.Context())) // clients send nothing; this notices them leave
	s.stream(ctx, conn, c)
}

// recheckEvery is how often a socket's session and permissions are checked
// again. It is a variable so tests can shorten it.
var recheckEvery = 30 * time.Second

// keepAliveEvery is how often a socket is sent a keep-alive frame. A TV in
// standby, or a client whose address changed, leaves a half-open
// connection that never closes on its side, so the client needs something
// to arrive to tell a quiet socket from a dead one. A WebSocket ping
// wouldn't do: browsers answer pings without showing them to the page.
// So it is an application frame, a KeepAliveMessage, which changes nothing
// on the client. A client that hears nothing for a few intervals
// reconnects. The writes also make the server's TCP stack give up on a
// peer that has gone, so its socket closes rather than waiting forever. It
// is a variable so tests can shorten it.
var keepAliveEvery = 25 * time.Second

const (
	writeTimeout = 10 * time.Second
	// closeSessionEnded closes a socket whose session no longer
	// authenticates. Clients respond by signing in again.
	closeSessionEnded websocket.StatusCode = 4401
)

// stream sends the events c may see on conn until the client leaves, the
// server stops, or the credential stops authenticating.
func (s *Server) stream(ctx context.Context, conn *websocket.Conn, c caller) {
	v := c.viewer
	sub := s.deps.Events.Subscribe(scopeFilter(v))
	defer func() { sub.Close() }()
	if s.send(ctx, conn, s.snapshot(ctx, v)) != nil {
		return
	}
	// resync starts over with the current Viewer. It opens a new
	// subscription, sends a resync frame so the client refetches, and
	// sends a fresh snapshot.
	resync := func() bool {
		sub.Close()
		sub = s.deps.Events.Subscribe(scopeFilter(v))
		return s.send(ctx, conn, gen.ResyncMessage{Type: gen.ResyncMessageTypeResync}) == nil &&
			s.send(ctx, conn, s.snapshot(ctx, v)) == nil
	}
	tick := time.NewTicker(recheckEvery)
	defer tick.Stop()
	alive := time.NewTicker(keepAliveEvery)
	defer alive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.closing:
			_ = conn.Close(websocket.StatusGoingAway, "server stopping")
			return
		case <-alive.C:
			if s.send(ctx, conn, gen.KeepAliveMessage{Type: gen.KeepAliveMessageTypeKeepAlive}) != nil {
				return
			}
		case <-tick.C:
			now, ok := s.stillSignedIn(ctx, c)
			if !ok {
				_ = conn.Close(closeSessionEnded, "session ended")
				return
			}
			// Current permissions decide what arrives, as on every request.
			if !sameScope(v, now) {
				v = now
				if !resync() {
					return
				}
			}
		case e, ok := <-sub.C():
			if !ok {
				if !errors.Is(sub.Err(), events.ErrLagged) || !resync() {
					return
				}
				continue
			}
			if msg, ok := eventMessage(e, v); ok && s.send(ctx, conn, msg) != nil {
				return
			}
		}
	}
}

func (s *Server) send(ctx context.Context, conn *websocket.Conn, msg any) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return wsjson.Write(ctx, conn, msg)
}

// stillSignedIn rechecks the socket's session and user, so a revoked
// session or a disabled user loses the socket within one interval. It
// returns the Viewer built from current permissions.
func (s *Server) stillSignedIn(ctx context.Context, c caller) (users.Viewer, bool) {
	if alive, err := s.deps.Auth.Alive(ctx, c.Caller); err != nil || !alive {
		return users.Viewer{}, false
	}
	v, err := s.deps.Users.Viewer(ctx, c.User)
	return v, err == nil
}

// sameScope reports whether two Viewers admit the same events.
func sameScope(a, b users.Viewer) bool {
	return a.Admin() == b.Admin() && a.AllLibraries() == b.AllLibraries() &&
		slices.Equal(a.Libraries(), b.Libraries())
}

// scopeFilter admits the events a Viewer may see. These are events for its
// libraries and its own user, and admin-only events for admins.
func scopeFilter(v users.Viewer) func(events.Scope) bool {
	return func(sc events.Scope) bool {
		switch {
		case sc.AdminOnly && !v.Admin():
			return false
		case !sc.User.IsZero() && sc.User != v.User():
			return false
		case len(sc.Libraries) > 0 && !slices.ContainsFunc(sc.Libraries, v.CanSee):
			return false
		}
		return true
	}
}

func (s *Server) snapshot(ctx context.Context, v users.Viewer) gen.SnapshotMessage {
	msg := gen.SnapshotMessage{Type: gen.SnapshotMessageTypeSnapshot, ServerVersion: s.deps.Version}
	if v.Admin() {
		msg.ProblemCount = ptr(len(s.deps.Problems.Open(ctx)))
	}
	return msg
}

// eventMessage is an event on the wire, as v may see it. Kinds without a
// mapping aren't sent, because clients only know the documented kinds.
func eventMessage(e events.Event, v users.Viewer) (gen.EventMessage, bool) {
	msg := gen.EventMessage{Type: gen.EventMessageTypeEvent, Kind: gen.EventKind(e.Kind)}
	switch e.Kind {
	case events.WorkChanged:
		w, ok := e.Data.(id.ID)
		if !ok {
			return msg, false
		}
		msg.WorkID = &w
	case events.LibraryChanged:
		libs := slices.DeleteFunc(slices.Clone(e.Scope.Libraries), func(l id.ID) bool { return !v.CanSee(l) })
		msg.LibraryIds = &libs
	case events.UserStateChanged:
		works, ok := e.Data.([]id.ID)
		if !ok {
			return msg, false
		}
		msg.WorkIds = &works
	case events.PlayEnded:
		d, ok := e.Data.(playback.Ended)
		if !ok {
			return msg, false
		}
		msg.PlayID, msg.Message = &d.Play, opt(d.Message)
	case events.ProblemsChanged:
		n, ok := e.Data.(int)
		if !ok {
			return msg, false
		}
		msg.ProblemCount = &n
	case events.DownloadsChanged, events.PlaysChanged, events.TasksChanged, events.LogsChanged, events.ActivityChanged, events.HomeChanged, events.ListsChanged:
	default:
		return msg, false
	}
	return msg, true
}
