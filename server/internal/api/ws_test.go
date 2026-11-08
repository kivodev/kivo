package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/playback"
	"github.com/mmcdole/kivo/internal/users"
)

func (e *env) dial(token string) (*websocket.Conn, context.Context) {
	e.t.Helper()
	ts := httptest.NewServer(e.srv)
	e.t.Cleanup(ts.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	e.t.Cleanup(cancel)
	h := http.Header{}
	h.Set("Authorization", "Bearer "+token)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/api/v1/events", &websocket.DialOptions{HTTPHeader: h})
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { conn.CloseNow() })
	return conn, ctx
}

// recheckSoon shortens the sockets' recheck for one test.
func recheckSoon(t *testing.T) {
	d := recheckEvery
	recheckEvery = 20 * time.Millisecond
	t.Cleanup(func() { recheckEvery = d })
}

func read(t *testing.T, ctx context.Context, conn *websocket.Conn) map[string]any {
	t.Helper()
	var m map[string]any
	if err := wsjson.Read(ctx, conn, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestEventStream(t *testing.T) {
	e := newEnv(t, serving{Version: "1.2.3"})
	conn, ctx := e.dial(e.adminToken)
	if m := read(t, ctx, conn); m["type"] != "snapshot" || m["serverVersion"] != "1.2.3" || m["problemCount"] == nil {
		t.Fatalf("first frame %v", m)
	}

	// An event for another user, or for a library the user can't see,
	// never arrives. An event the user can see does.
	e.d.Events.Publish(events.Event{Kind: events.WorkChanged, Scope: events.Scope{User: id.New()}, Data: e.series})
	e.d.Events.Publish(events.Event{Kind: events.WorkChanged, Scope: events.Scope{Libraries: []id.ID{e.lib}}, Data: e.heat.Work})
	if m := read(t, ctx, conn); m["kind"] != "workChanged" || m["workId"] != e.heat.Work.String() {
		t.Fatalf("event %v", m)
	}

	// Marking played reaches the user's other devices, naming the work.
	w := e.do("PATCH", "/api/v1/works/"+e.heat.Work.String()+"/user-state", e.adminToken, map[string]any{"played": true})
	if w.Code != http.StatusOK {
		t.Fatalf("user state: %d %s", w.Code, w.Body.String())
	}
	m := read(t, ctx, conn)
	if m["kind"] != "userStateChanged" || !reflect.DeepEqual(m["workIds"], []any{e.heat.Work.String()}) || m["userState"] != nil {
		t.Fatalf("user state event %v", m)
	}

	// A play ended elsewhere.
	sid := id.New()
	e.d.Events.Publish(events.Event{Kind: events.PlayEnded, Data: playback.Ended{Play: sid, Message: "Bedtime"}})
	if m := read(t, ctx, conn); m["kind"] != "playEnded" || m["playId"] != sid.String() || m["message"] != "Bedtime" {
		t.Fatalf("play ended %v", m)
	}

	// Admin-only hints. problemsChanged arrives at once with its count.
	// The others arrive as their interval ends.
	e.d.Events.Publish(events.Event{Kind: events.TasksChanged, Scope: events.Scope{AdminOnly: true}})
	e.d.Events.Publish(events.Event{Kind: events.PlaysChanged, Scope: events.Scope{AdminOnly: true}})
	e.d.Events.Publish(events.Event{Kind: events.LogsChanged, Scope: events.Scope{AdminOnly: true}})
	e.d.Events.Publish(events.Event{Kind: events.ProblemsChanged, Scope: events.Scope{AdminOnly: true}, Data: 2})
	if m := read(t, ctx, conn); m["kind"] != "problemsChanged" || m["problemCount"] != 2.0 {
		t.Fatalf("got %v; want problemsChanged with its count", m)
	}
	got := map[any]bool{}
	for range 3 {
		got[read(t, ctx, conn)["kind"]] = true
	}
	if !got["tasksChanged"] || !got["playsChanged"] || !got["logsChanged"] {
		t.Fatalf("got %v; want tasksChanged, playsChanged and logsChanged", got)
	}

	// Falling behind gets a resync and a fresh snapshot. Far more events
	// than the buffer holds are left unread, which fills the socket too, so
	// the subscription lags however fast the writer is.
	for range 10000 { // far more than a subscriber buffers
		e.d.Events.Publish(events.Event{Kind: events.WorkChanged, Data: e.heat.Work})
	}
	for {
		m := read(t, ctx, conn)
		if m["type"] == "resync" {
			break
		}
	}
	if m := read(t, ctx, conn); m["type"] != "snapshot" {
		t.Fatalf("after resync %v", m)
	}
}

// The snapshot carries the version to everyone. It carries the count of
// open problems to admins only.
func TestSnapshotVersion(t *testing.T) {
	e := newEnv(t, serving{Version: "999eb08"})
	conn, ctx := e.dial(e.adminToken)
	if m := read(t, ctx, conn); m["serverVersion"] != "999eb08" || m["problemCount"] == nil {
		t.Errorf("admin snapshot %v", m)
	}
	conn, ctx = e.dial(e.nobodyToken)
	if m := read(t, ctx, conn); m["serverVersion"] != "999eb08" || m["problemCount"] != nil {
		t.Errorf("viewer snapshot %v", m)
	}
}

// A socket follows current permissions. A library granted after the socket
// opened brings a resync, and from then on that library's events.
func TestEventStreamFollowsGrants(t *testing.T) {
	recheckSoon(t)
	e := newEnv(t, serving{})
	conn, ctx := e.dial(e.nobodyToken)
	read(t, ctx, conn) // the snapshot
	if err := e.d.Users.Update(ctx, e.nobody, users.Change{Grants: &users.Grants{Libraries: []id.ID{e.lib}}}); err != nil {
		t.Fatal(err)
	}
	if m := read(t, ctx, conn); m["type"] != "resync" {
		t.Fatalf("after a grant %v", m)
	}
	if m := read(t, ctx, conn); m["type"] != "snapshot" {
		t.Fatalf("after the resync %v", m)
	}
	e.d.Events.Publish(events.Event{Kind: events.TasksChanged, Scope: events.Scope{AdminOnly: true}})
	e.d.Events.Publish(events.Event{Kind: events.WorkChanged, Scope: events.Scope{Libraries: []id.ID{e.lib}}, Data: e.heat.Work})
	if m := read(t, ctx, conn); m["kind"] != "workChanged" {
		t.Fatalf("event %v", m)
	}
}

// homeChanged goes to the user's own sockets when the layout changes.
func TestHomeRowsPublishHomeChanged(t *testing.T) {
	e := newEnv(t, serving{})
	admin, actx := e.dial(e.adminToken)
	other, octx := e.dial(e.nobodyToken)
	read(t, actx, admin)
	read(t, octx, other)
	if w := e.do("POST", "/api/v1/home/rows/reset", e.adminToken, nil); w.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", w.Code, w.Body.String())
	}
	if m := read(t, actx, admin); m["kind"] != "homeChanged" {
		t.Errorf("admin got %v; want homeChanged", m)
	}
	if w := e.do("PATCH", "/api/v1/home/rows", e.adminToken, map[string]any{"items": []map[string]any{{"id": "continueWatching", "shown": false}}}); w.Code != http.StatusOK {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	if m := read(t, actx, admin); m["kind"] != "homeChanged" {
		t.Errorf("admin got %v; want homeChanged after an update", m)
	}
	e.d.Events.Publish(events.Event{Kind: events.PlayEnded, Scope: events.Scope{User: e.nobody}, Data: playback.Ended{Play: id.New()}})
	if m := read(t, octx, other); m["kind"] != "playEnded" {
		t.Errorf("another user got %v; want no homeChanged", m)
	}
}

// libraryChanged names to each socket only the libraries it sees, admin
// sockets included. It doesn't reach a socket that sees none of them.
func TestLibraryChangedNamesWhatEachSees(t *testing.T) {
	e := newEnv(t, serving{})
	other, _ := e.b.Library(catalog.Movies)
	var friend users.User
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		var err error
		friend, err = e.d.Users.CreateUser(ctx, tx, users.NewUser{Username: "friend", Grants: users.Grants{Libraries: []id.ID{e.lib}}})
		return err
	})
	admin, actx := e.dial(e.adminToken)
	granted, gctx := e.dial(e.session(friend.ID))
	nobody, nctx := e.dial(e.nobodyToken)
	for _, s := range []struct {
		conn *websocket.Conn
		ctx  context.Context
	}{{admin, actx}, {granted, gctx}, {nobody, nctx}} {
		read(t, s.ctx, s.conn) // the snapshot
	}

	libs := []id.ID{e.lib, other}
	e.d.Events.Publish(events.Event{Kind: events.LibraryChanged, Scope: events.Scope{Libraries: libs}})
	sid := id.New()
	e.d.Events.Publish(events.Event{Kind: events.PlayEnded, Scope: events.Scope{User: e.nobody}, Data: playback.Ended{Play: sid}})
	want := func(ids ...id.ID) []any {
		var out []any
		for _, x := range ids {
			out = append(out, x.String())
		}
		return out
	}
	if m := read(t, actx, admin); m["kind"] != "libraryChanged" || !reflect.DeepEqual(m["libraryIds"], want(e.lib, other)) {
		t.Errorf("admin got %v; want both libraries", m)
	}
	if m := read(t, gctx, granted); m["kind"] != "libraryChanged" || !reflect.DeepEqual(m["libraryIds"], want(e.lib)) {
		t.Errorf("granted viewer got %v; want only its library", m)
	}
	if m := read(t, nctx, nobody); m["kind"] != "playEnded" {
		t.Errorf("a viewer of neither library got %v; want the next event", m)
	}
}

func TestEventStreamClosesOnShutdown(t *testing.T) {
	e := newEnv(t, serving{})
	conn, ctx := e.dial(e.adminToken)
	read(t, ctx, conn)
	e.srv.Close()
	_, _, err := conn.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusGoingAway {
		t.Errorf("read after Close: %v", err)
	}
	if w := e.do("GET", "/healthz", "", nil); w.Code != http.StatusServiceUnavailable {
		t.Errorf("healthz while stopping: %d", w.Code)
	}
}

func TestEventStreamNoCredentialsNoSocket(t *testing.T) {
	e := newEnv(t, serving{})
	ts := httptest.NewServer(e.srv)
	defer ts.Close()
	_, resp, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(ts.URL, "http")+"/api/v1/events", nil)
	if err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("without credentials: %v %v", err, resp)
	}
}

// The packaged TV app's page can't set a header on its socket, so it offers
// its token as a kivo-bearer subprotocol beside kivo. The upgrade
// authenticates with it, sends no Origin, and is answered with kivo, never
// the token. No other route reads the subprotocol.
func TestEventStreamBearerSubprotocol(t *testing.T) {
	e := newEnv(t, serving{})
	ts := httptest.NewServer(e.srv)
	defer ts.Close()
	addr := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/events"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, addr, &websocket.DialOptions{Subprotocols: []string{"kivo", "kivo-bearer." + e.adminToken}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if got := resp.Header.Values("Sec-WebSocket-Protocol"); conn.Subprotocol() != "kivo" || len(got) != 1 || got[0] != "kivo" {
		t.Errorf("answered subprotocol %q, header %q", conn.Subprotocol(), got)
	}
	if m := read(t, ctx, conn); m["type"] != "snapshot" {
		t.Errorf("first frame %v", m)
	}

	for name, protocols := range map[string][]string{
		"a bad token": {"kivo", "kivo-bearer.not-a-token"},
		"no token":    {"kivo", "kivo-bearer."},
	} {
		_, resp, err := websocket.Dial(ctx, addr, &websocket.DialOptions{Subprotocols: protocols})
		if err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: %v %v", name, err, resp)
		}
	}

	// Another route, upgrade or not, never takes the token from it.
	for _, upgrade := range []bool{true, false} {
		h := []string{"Sec-WebSocket-Protocol", "kivo, kivo-bearer." + e.adminToken}
		if upgrade {
			h = append(h, "Connection", "Upgrade", "Upgrade", "websocket", "Sec-WebSocket-Version", "13", "Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		}
		if w := e.do("GET", "/api/v1/home", "", nil, h...); w.Code != http.StatusUnauthorized {
			t.Errorf("home with the subprotocol (upgrade %v): %d", upgrade, w.Code)
		}
	}
}

// A quiet socket is sent a keep-alive frame on an interval, so a client can
// tell a quiet socket from a dead one. Events still arrive between them.
func TestEventStreamKeepAlive(t *testing.T) {
	d := keepAliveEvery
	keepAliveEvery = 20 * time.Millisecond
	t.Cleanup(func() { keepAliveEvery = d })
	e := newEnv(t, serving{})
	conn, ctx := e.dial(e.adminToken)
	read(t, ctx, conn) // the snapshot
	for range 2 {
		if m := read(t, ctx, conn); !reflect.DeepEqual(m, map[string]any{"type": "keepAlive"}) {
			t.Fatalf("quiet socket got %v; want a keep-alive", m)
		}
	}
	e.d.Events.Publish(events.Event{Kind: events.WorkChanged, Scope: events.Scope{Libraries: []id.ID{e.lib}}, Data: e.heat.Work})
	for {
		m := read(t, ctx, conn)
		if m["type"] == "keepAlive" {
			continue
		}
		if m["kind"] != "workChanged" {
			t.Fatalf("got %v; want the event", m)
		}
		break
	}
}
