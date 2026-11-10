package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

// passwordServer is a database with two signed-in users, an admin and anna.
// It is held open the way the running server holds it.
type passwordServer struct {
	t        *testing.T
	config   string
	auth     *auth.Service
	activity *activity.Service
	admin    string // the admin's token
	anna     []string
}

const oldPassword = "correct horse battery"

func newPasswordServer(t *testing.T) *passwordServer {
	ctx := context.Background()
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "server.toml")
	if err := os.WriteFile(cfg, fmt.Appendf(nil, "data_dir = %q\n", data), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, filepath.Join(data, "kivo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	settings, err := config.Open(ctx, db, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	u := users.New(db, events.NewBus())
	act := activity.New(db, events.NewBus())
	s := &passwordServer{t: t, config: cfg, auth: auth.New(db, auth.Deps{Users: u, Settings: settings, Activity: act}), activity: act}
	home := netip.MustParseAddr("192.168.1.2")
	admin, err := s.auth.Setup(ctx, auth.SignInRequest{Username: "admin", Password: oldPassword, RemoteIP: home})
	if err != nil {
		t.Fatal(err)
	}
	s.admin = admin.Token
	if _, err := s.auth.CreateUser(ctx, admin.Session.Caller(), users.NewUser{Username: "Anna", Grants: users.Grants{All: true}}, oldPassword); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []auth.SessionKind{auth.SessionBrowser, auth.SessionApp} {
		iss, err := s.auth.SignIn(ctx, auth.SignInRequest{Username: "anna", Password: oldPassword, RemoteIP: home, Kind: kind, Name: "Device"})
		if err != nil {
			t.Fatal(err)
		}
		s.anna = append(s.anna, iss.Token)
	}
	return s
}

// signedIn reports whether a token still authenticates.
func (s *passwordServer) signedIn(token string) bool {
	_, err := s.auth.Authenticate(context.Background(), token)
	if err != nil && !errors.Is(err, auth.ErrUnauthenticated) {
		s.t.Fatal(err)
	}
	return err == nil
}

// signsIn reports whether anna signs in with password.
func (s *passwordServer) signsIn(password string) bool {
	_, err := s.auth.SignIn(context.Background(), auth.SignInRequest{Username: "anna", Password: password, RemoteIP: netip.MustParseAddr("192.168.1.3")})
	if err != nil && !errors.Is(err, auth.ErrInvalidCredentials) {
		s.t.Fatal(err)
	}
	return err == nil
}

// stdin returns a file holding text, to read as stdin.
func stdin(t *testing.T, text string) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// reset-password sets the password and signs out all of the user's
// sessions, while the database is held open elsewhere as by the running
// server. The username is matched case-insensitively, and other users stay
// signed in. The activity records it as a reset from the command line.
func TestResetPassword(t *testing.T) {
	s := newPasswordServer(t)
	var out strings.Builder
	if err := resetPassword(s.config, "anna", stdin(t, "a new password\n"), &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "Set Anna's password and signed out all of their sessions.\n" {
		t.Errorf("printed %q", got)
	}
	for _, token := range s.anna {
		if s.signedIn(token) {
			t.Error("a session from before the reset still authenticates")
		}
	}
	if !s.signedIn(s.admin) {
		t.Error("another user was signed out")
	}
	if s.signsIn(oldPassword) || !s.signsIn("a new password") {
		t.Error("the new password isn't the one that signs in")
	}
	rows, _, err := s.activity.List(context.Background(), activity.GroupSignIns, activity.Position{}, 50)
	if err != nil {
		t.Fatal(err)
	}
	var changed []activity.Event
	for _, e := range rows {
		if e.Kind == activity.PasswordChanged {
			changed = append(changed, e)
		}
	}
	if len(changed) != 1 {
		t.Fatalf("passwordChanged events %+v, want one", changed)
	}
	if e := changed[0]; e.Username != "Anna" || !e.CommandLine || e.By != "" || e.Address != "" {
		t.Errorf("recorded %+v", e)
	}
}

// An unknown user gives an error naming only the username given, and
// changes nothing.
func TestResetPasswordUnknownUser(t *testing.T) {
	s := newPasswordServer(t)
	err := resetPassword(s.config, "bob", stdin(t, "a new password\n"), io.Discard)
	if err == nil || err.Error() != `no user named "bob"` {
		t.Fatalf("err = %v", err)
	}
	if !s.signedIn(s.anna[0]) || !s.signsIn(oldPassword) {
		t.Error("an unknown user's reset changed anna")
	}
}

// A password the policy refuses gives an error, and changes nothing.
func TestResetPasswordPolicy(t *testing.T) {
	s := newPasswordServer(t)
	for _, p := range []string{"short\n", "\n", strings.Repeat("x", 1025) + "\n"} {
		if err := resetPassword(s.config, "anna", stdin(t, p), io.Discard); !errors.Is(err, auth.ErrWeakPassword) {
			t.Errorf("%d bytes: err = %v, want ErrWeakPassword", len(p)-1, err)
		}
	}
	if !s.signedIn(s.anna[0]) || !s.signsIn(oldPassword) {
		t.Error("a refused password changed anna")
	}
}

// When stdin isn't a terminal, the password is stdin's first line without
// its line ending. Empty stdin is an error.
func TestReadPasswordFromStdin(t *testing.T) {
	for in, want := range map[string]string{
		"a new password\n":        "a new password",
		"a new password\r\n":      "a new password",
		"a new password":          "a new password",
		" spaced \nsecond line\n": " spaced ",
	} {
		if got, err := readPassword(stdin(t, in), io.Discard); err != nil || got != want {
			t.Errorf("%q: %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := readPassword(stdin(t, ""), io.Discard); err == nil {
		t.Error("empty stdin gave a password")
	}
}

// The database must already exist. A wrong data directory gives an error,
// not a new, empty database.
func TestResetPasswordNoDatabase(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KIVO_DATA_DIR", dir)
	if err := resetPassword(filepath.Join(dir, "none.toml"), "anna", stdin(t, "a new password\n"), io.Discard); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "kivo.db")); !errors.Is(err, os.ErrNotExist) {
		t.Error("made a database")
	}
}

// TestResetPasswordWhileServing runs the real server and resets the admin's
// password from alongside it, as `docker exec` does. The sessions signed in
// before are refused at once, and only the new password signs in.
func TestResetPasswordWhileServing(t *testing.T) {
	if testing.Short() {
		t.Skip("end to end")
	}
	dir := t.TempDir()
	boot := config.DefaultBootstrap()
	boot.Listen = []string{freeAddr(t)}
	boot.DataDir, boot.CacheDir, boot.BackupDir = filepath.Join(dir, "data"), filepath.Join(dir, "cache"), filepath.Join(dir, "backups")
	cfg := filepath.Join(dir, "server.toml")
	if err := os.WriteFile(cfg, fmt.Appendf(nil, "data_dir = %q\n", boot.DataDir), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	bus := events.NewBus()
	go func() { done <- run(ctx, boot, health.NewLogs(100, bus), bus) }()
	defer func() {
		stop()
		if err := <-done; err != nil {
			t.Errorf("shutdown: %v", err)
		}
	}()

	base := "http://" + boot.Listen[0]
	setup, other := newClient(t, base), newClient(t, base)
	setup.waitHealthy()
	setup.call("POST", "/api/v1/setup", map[string]any{"username": "admin", "password": oldPassword}, http.StatusCreated, nil)
	other.call("POST", "/api/v1/sessions", map[string]any{"username": "admin", "password": oldPassword}, http.StatusCreated, nil)
	for _, c := range []*client{setup, other} {
		c.call("GET", "/api/v1/users/me", nil, http.StatusOK, nil)
	}

	if err := resetPassword(cfg, "admin", stdin(t, "a new password\n"), io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*client{setup, other} {
		c.call("GET", "/api/v1/users/me", nil, http.StatusUnauthorized, nil)
	}
	again := newClient(t, base)
	again.call("POST", "/api/v1/sessions", map[string]any{"username": "admin", "password": oldPassword}, http.StatusUnauthorized, nil)
	again.call("POST", "/api/v1/sessions", map[string]any{"username": "admin", "password": "a new password"}, http.StatusCreated, nil)
	again.call("GET", "/api/v1/users/me", nil, http.StatusOK, nil)
}
