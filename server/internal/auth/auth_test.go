package auth

import (
	"bytes"
	"context"
	"errors"
	"net/netip"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/store/storetest"
	"github.com/mmcdole/kivo/internal/users"
)

const password = "correct horse battery"

var (
	home  = netip.MustParseAddr("192.168.1.20")
	other = netip.MustParseAddr("203.0.113.9")
)

type fixture struct {
	t        *testing.T
	db       *store.DB
	users    *users.Service
	settings *config.Store
	activity *activity.Service
	auth     *Service
}

func newFixture(t *testing.T) *fixture {
	db := storetest.New(t)
	u := users.New(db, events.NewBus())
	settings, err := config.Open(context.Background(), db, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, db: db, users: u, settings: settings}
	f.activity = activity.New(db, events.NewBus())
	f.auth = New(db, Deps{Users: u, Settings: settings, Activity: f.activity})
	return f
}

// setUp runs first-run setup for "admin".
func (f *fixture) setUp() IssuedSession {
	f.t.Helper()
	iss, err := f.auth.Setup(context.Background(), SignInRequest{Username: "admin", Password: password, RemoteIP: home})
	if err != nil {
		f.t.Fatal(err)
	}
	return iss
}

func (f *fixture) signIn(r SignInRequest) IssuedSession {
	f.t.Helper()
	iss, err := f.auth.SignIn(context.Background(), r)
	if err != nil {
		f.t.Fatal(err)
	}
	return iss
}

func TestSetupCreatesTheAdminOnce(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	if need, err := f.auth.NeedsSetup(ctx); err != nil || !need {
		t.Fatalf("NeedsSetup = %v, %v before setup", need, err)
	}
	if _, err := f.auth.Setup(ctx, SignInRequest{Username: "admin", Password: "short"}); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("short password: err = %v, want ErrWeakPassword", err)
	}

	iss := f.setUp()
	p := iss.Session.Caller()
	if iss.Session.Kind != SessionBrowser || p.User.IsZero() || p.Session.IsZero() {
		t.Fatalf("Session = %+v", iss.Session)
	}
	got, err := f.auth.Authenticate(ctx, iss.Token)
	p.Device = Device{Platform: PlatformBrowser} // the session's own, as it signed in
	if err != nil || got != p {
		t.Fatalf("Authenticate = %+v, %v; want %+v", got, err, p)
	}
	v, err := f.users.Viewer(ctx, p.User)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Admin() || !v.AllLibraries() {
		t.Fatalf("the first admin's Viewer isn't an admin over every library")
	}

	if need, _ := f.auth.NeedsSetup(ctx); need {
		t.Fatal("NeedsSetup after setup")
	}
	_, err = f.auth.Setup(ctx, SignInRequest{Username: "mallory", Password: password})
	if !errors.Is(err, ErrAlreadySetUp) {
		t.Fatalf("second setup: err = %v, want ErrAlreadySetUp", err)
	}
}

func TestSignInAndSignOut(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	admin := f.setUp().Session.Caller()

	iss := f.signIn(SignInRequest{Username: "ADMIN", Password: password, RemoteIP: home, Kind: SessionApp, Name: " Living Room "})
	p := iss.Session.Caller()
	if p.User != admin.User || iss.Session.Kind != SessionApp || p.Session == admin.Session {
		t.Fatalf("Session = %+v, setup's was %+v", iss.Session, admin)
	}
	if got, err := f.auth.Authenticate(ctx, iss.Token); err != nil || got != p || got.Name != "Living Room" {
		t.Fatalf("Authenticate = %+v, %v; want %+v", got, err, p)
	}

	if err := f.auth.SignOut(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.Authenticate(ctx, iss.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("revoked token: err = %v", err)
	}
	if alive, err := f.auth.Alive(ctx, p); err != nil || alive {
		t.Fatalf("Alive after SignOut = %v, %v", alive, err)
	}
	if err := f.auth.SignOut(ctx, p); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second SignOut: err = %v, want ErrNotFound", err)
	}
	if alive, err := f.auth.Alive(ctx, admin); err != nil || !alive {
		t.Fatalf("SignOut ended another session: Alive = %v, %v", alive, err)
	}

	if _, err := f.auth.SignIn(ctx, SignInRequest{Username: "admin", Password: password, Kind: "api_key"}); err == nil {
		t.Fatal("signed in as an API key")
	}
}

// A user signing in again from the same client replaces that client's
// session. Another user on the same client keeps their session. A client
// without an ID adds a new session each time.
func TestSignInAgainFromAClientReplacesItsSession(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	admin := f.setUp().Session.Caller()
	f.newUser("anna")
	phone := SignInRequest{Username: "anna", Password: password, RemoteIP: home, Kind: SessionApp, Name: "Phone", ClientID: "phone-1"}

	first := f.signIn(phone)
	second := f.signIn(phone)
	if _, err := f.auth.Authenticate(ctx, first.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("the replaced token: err = %v", err)
	}
	if _, err := f.auth.Authenticate(ctx, second.Token); err != nil {
		t.Fatal(err)
	}
	if list, err := f.auth.Sessions(ctx, admin, second.Session.User); err != nil || len(list) != 1 {
		t.Fatalf("anna's sessions = %+v, %v", list, err)
	}

	// The admin signing in on anna's phone adds a second session there, and
	// anna's session stays.
	f.signIn(SignInRequest{Username: "admin", Password: password, RemoteIP: home, Kind: SessionApp, ClientID: "phone-1"})
	if alive, err := f.auth.Alive(ctx, second.Session.Caller()); err != nil || !alive {
		t.Fatalf("anna's session after the admin signed in on her phone: Alive = %v, %v", alive, err)
	}

	// Without a client ID, each sign-in is another session.
	f.signIn(SignInRequest{Username: "anna", Password: password, RemoteIP: home})
	f.signIn(SignInRequest{Username: "anna", Password: password, RemoteIP: home})
	if list, _ := f.auth.Sessions(ctx, admin, second.Session.User); len(list) != 3 {
		t.Fatalf("anna has %d sessions, want 3", len(list))
	}

	long := phone
	long.ClientID = strings.Repeat("x", maxClientID+1)
	if _, err := f.auth.SignIn(ctx, long); !errors.Is(err, ErrInvalidClientID) {
		t.Fatalf("an overlong client ID: err = %v", err)
	}
}

// Signing out everywhere else ends every other session of the user. It
// leaves the caller's session, other users' sessions and API keys alone.
func TestSignOutOthers(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	admin := f.setUp()
	key, err := f.auth.CreateAPIKey(ctx, admin.Session.Caller(), NewAPIKey{Name: "Script"})
	if err != nil {
		t.Fatal(err)
	}
	tv := f.signIn(SignInRequest{Username: "admin", Password: password, RemoteIP: home, Kind: SessionApp, Name: "TV"})
	browser := f.signIn(SignInRequest{Username: "admin", Password: password, RemoteIP: home})
	f.newUser("anna")
	anna := f.signIn(SignInRequest{Username: "anna", Password: password, RemoteIP: home})

	if err := f.auth.SignOutOthers(ctx, browser.Session.Caller()); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{admin.Token, tv.Token} {
		if _, err := f.auth.Authenticate(ctx, tok); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("a signed-out token: err = %v", err)
		}
	}
	for _, tok := range []string{browser.Token, key.Token, anna.Token} {
		if _, err := f.auth.Authenticate(ctx, tok); err != nil {
			t.Fatalf("signed out what it shouldn't have: %v", err)
		}
	}

	if err := f.auth.SignOutOthers(ctx, Caller{User: admin.Session.User, APIKey: key.Key.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an API key's caller: err = %v, want ErrNotFound", err)
	}
}

// However a session or API key is revoked, it is refused at once.
func TestEveryRevocationRefusesAtOnce(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	admin := f.setUp()
	anna := f.newUser("anna")
	app := func(name, client string) IssuedSession {
		return f.signIn(SignInRequest{Username: "anna", Password: password, RemoteIP: home, Kind: SessionApp, Name: name, ClientID: client})
	}
	expect := func(what string, revoked ...string) {
		t.Helper()
		for _, tok := range revoked {
			if _, err := f.auth.Authenticate(ctx, tok); !errors.Is(err, ErrUnauthenticated) {
				t.Errorf("%s: err = %v, want ErrUnauthenticated", what, err)
			}
		}
	}

	tv := app("TV", "")
	if err := f.auth.SignOut(ctx, tv.Session.Caller()); err != nil {
		t.Fatal(err)
	}
	expect("signing out", tv.Token)

	tv = app("TV", "")
	if err := f.auth.RevokeSession(ctx, admin.Session.Caller(), tv.Session.ID); err != nil {
		t.Fatal(err)
	}
	expect("an admin revoking a session", tv.Token)

	phone := app("Phone", "phone-1")
	again := app("Phone", "phone-1")
	expect("a sign-in on the same client", phone.Token)

	tv = app("TV", "")
	if err := f.auth.SignOutOthers(ctx, again.Session.Caller()); err != nil {
		t.Fatal(err)
	}
	expect("signing out everywhere else", tv.Token)

	tv = app("TV", "")
	if err := f.auth.ChangePassword(ctx, again.Session.Caller(), PasswordChange{Current: password, New: "a new password", SignOutOthers: true, RemoteIP: home}); err != nil {
		t.Fatal(err)
	}
	expect("a password change that signs out the others", tv.Token)

	if err := f.auth.SetPassword(ctx, admin.Session.Caller(), anna, password); err != nil {
		t.Fatal(err)
	}
	expect("an admin setting the password", again.Token)

	tv, phone = app("TV", ""), app("Phone", "phone-1")
	if err := f.auth.SetPassword(ctx, Caller{}, anna, password); err != nil {
		t.Fatal(err)
	}
	expect("setting the password with no session to keep", tv.Token, phone.Token)

	key, err := f.auth.CreateAPIKey(ctx, admin.Session.Caller(), NewAPIKey{Name: "Script"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.auth.RevokeAPIKey(ctx, key.Key.ID); err != nil {
		t.Fatal(err)
	}
	expect("revoking an API key", key.Token)

	if err := f.auth.SetPassword(ctx, admin.Session.Caller(), id.New(), password); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("an unknown user: err = %v", err)
	}
	if err := f.auth.RevokeSession(ctx, admin.Session.Caller(), tv.Session.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a session already revoked: err = %v", err)
	}
}

// Changing your own password needs the current one. It signs out the
// user's other sessions only when asked. The caller's own session and the
// API keys stay.
func TestChangePassword(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.setUp()
	anna := f.newUser("anna")
	tv := f.signIn(SignInRequest{Username: "anna", Password: password, RemoteIP: home, Kind: SessionApp, Name: "TV"})
	browser := f.signIn(SignInRequest{Username: "anna", Password: password, RemoteIP: home})
	me := browser.Session.Caller()

	if err := f.auth.ChangePassword(ctx, me, PasswordChange{Current: "wrong password", New: "a new password", RemoteIP: home}); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("a wrong current password: err = %v, want ErrWrongPassword", err)
	}
	if err := f.auth.ChangePassword(ctx, me, PasswordChange{Current: password, New: "short", RemoteIP: home}); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("a short new password: err = %v, want ErrWeakPassword", err)
	}

	if err := f.auth.ChangePassword(ctx, me, PasswordChange{Current: password, New: "a new password", RemoteIP: home}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.Authenticate(ctx, tv.Token); err != nil {
		t.Fatalf("the TV was signed out: %v", err)
	}
	if _, err := f.auth.SignIn(ctx, SignInRequest{Username: "anna", Password: password, RemoteIP: other}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("the old password: err = %v", err)
	}

	if err := f.auth.ChangePassword(ctx, me, PasswordChange{Current: "a new password", New: "another password", SignOutOthers: true, RemoteIP: home}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.Authenticate(ctx, tv.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("the TV after signing out the others: err = %v", err)
	}
	if alive, _ := f.auth.Alive(ctx, me); !alive {
		t.Fatal("the session that changed the password was signed out")
	}
	if u := f.signIn(SignInRequest{Username: "anna", Password: "another password", RemoteIP: home}); u.Session.User != anna {
		t.Fatalf("signed in as %v, want %v", u.Session.User, anna)
	}
}

// Wrong current passwords back off the same way wrong sign-ins do, and
// share the same count.
func TestChangePasswordBacksOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := newFixture(t)
		me := f.setUp().Session.Caller()
		wrong := PasswordChange{Current: "wrong password", New: "a new password", RemoteIP: other}
		for range userFreeFailures + 1 {
			if err := f.auth.ChangePassword(ctx, me, wrong); !errors.Is(err, ErrWrongPassword) {
				t.Fatalf("err = %v, want ErrWrongPassword", err)
			}
		}
		right := PasswordChange{Current: password, New: "a new password", RemoteIP: other}
		var te *ThrottledError
		if err := f.auth.ChangePassword(ctx, me, right); !errors.As(err, &te) {
			t.Fatalf("err = %v, want a ThrottledError", err)
		}
		if _, err := f.auth.SignIn(ctx, SignInRequest{Username: "admin", Password: password, RemoteIP: other}); !errors.Is(err, ErrThrottled) {
			t.Fatalf("sign-in from the same address: err = %v, want ErrThrottled", err)
		}
		time.Sleep(te.RetryAfter)
		if err := f.auth.ChangePassword(ctx, me, right); err != nil {
			t.Fatal(err)
		}
	})
}

func TestSignInFailuresLookAlike(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.setUp()
	if err := f.users.Update(ctx, f.newUser("friend"), users.Change{Disabled: ptr(true)}); err != nil {
		t.Fatal(err)
	}

	cases := []SignInRequest{
		{Username: "nobody", Password: password},
		{Username: "admin", Password: "wrong password"},
		{Username: "admin", Password: ""},
		{Username: "", Password: password},
		{Username: "friend", Password: password}, // disabled
	}
	var first string
	for _, r := range cases {
		r.RemoteIP = home
		_, err := f.auth.SignIn(ctx, r)
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("%+v: err = %v, want ErrInvalidCredentials", r, err)
		}
		if first == "" {
			first = err.Error()
		} else if err.Error() != first {
			t.Errorf("%+v: error %q differs from %q", r, err, first)
		}
	}
}

func TestUnknownUsernameCostsAHash(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.setUp()
	// With the hashing capacity taken, a sign-in that hashes fails with ErrBusy,
	// so ErrBusy proves an unknown username still reaches the hash.
	for range cap(f.auth.hasher.sem) {
		f.auth.hasher.sem <- struct{}{}
	}
	_, err := f.auth.SignIn(ctx, SignInRequest{Username: "nobody", Password: password, RemoteIP: home})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("unknown username: err = %v, want ErrBusy", err)
	}
	_, err = f.auth.SignIn(ctx, SignInRequest{Username: "admin", Password: password, RemoteIP: home})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("known username: err = %v, want ErrBusy", err)
	}
	for range cap(f.auth.hasher.sem) {
		f.auth.hasher.release()
	}
	f.signIn(SignInRequest{Username: "admin", Password: password, RemoteIP: home})
}

func TestDisablingAUserFailsAuthenticateAtOnce(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	admin := f.setUp()
	friend := f.newUser("friend")
	iss := f.signIn(SignInRequest{Username: "friend", Password: password, RemoteIP: home})

	if err := f.users.Update(ctx, friend, users.Change{Disabled: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.Authenticate(ctx, iss.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("disabled user's token: err = %v, want ErrUnauthenticated", err)
	}
	if alive, err := f.auth.Alive(ctx, iss.Session.Caller()); err != nil || alive {
		t.Fatalf("Alive = %v, %v for a disabled user's session", alive, err)
	}
	_, err := f.auth.SignIn(ctx, SignInRequest{Username: "friend", Password: password, RemoteIP: home})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled user's sign-in: err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := f.auth.Authenticate(ctx, admin.Token); err != nil {
		t.Fatalf("another user's token stopped working: %v", err)
	}

	if err := f.users.Update(ctx, friend, users.Change{Disabled: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.Authenticate(ctx, iss.Token); err != nil {
		t.Fatalf("re-enabled user's token: %v", err)
	}
}

func TestNoRawTokenInTheDatabase(t *testing.T) {
	f := newFixture(t)
	admin := f.setUp()
	tokens := []string{admin.Token}
	for range 3 {
		tokens = append(tokens, f.signIn(SignInRequest{Username: "admin", Password: password, RemoteIP: home}).Token)
	}
	key, err := f.auth.CreateAPIKey(context.Background(), admin.Session.Caller(), NewAPIKey{Name: "Home Assistant"})
	if err != nil {
		t.Fatal(err)
	}
	tokens = append(tokens, key.Token)
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	var file []byte
	for _, suffix := range []string{"", "-wal"} {
		b, err := os.ReadFile(f.db.Path() + suffix)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		file = append(file, b...)
	}
	if len(file) == 0 {
		t.Fatal("read nothing")
	}
	for _, tok := range tokens {
		raw, err := tokenEncoding.DecodeString(tok)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(file, []byte(tok)) || bytes.Contains(file, raw) {
			t.Fatal("a token is in the database file")
		}
	}
	if bytes.Contains(file, []byte(password)) {
		t.Fatal("the password is in the database file")
	}
}

func TestMalformedTokens(t *testing.T) {
	f := newFixture(t)
	good := f.setUp().Token
	for _, tok := range []string{"", "x", good + "A", good[:len(good)-1], "Bearer " + good, tokenEncoding.EncodeToString(make([]byte, tokenLen))} {
		if _, err := f.auth.Authenticate(context.Background(), tok); !errors.Is(err, ErrUnauthenticated) {
			t.Errorf("token %q: err = %v, want ErrUnauthenticated", tok, err)
		}
	}
}

func TestBrowserSessionsExpireWhenIdleAndAppsDont(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		f := newFixture(t)
		browser := f.setUp()
		busy := f.signIn(SignInRequest{Username: "admin", Password: password, RemoteIP: home})
		app := f.signIn(SignInRequest{Username: "admin", Password: password, RemoteIP: home, Kind: SessionApp})
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = f.auth.Run(ctx)
		}()

		// busy is used every 10 days, so it never goes idle long enough.
		for range 4 {
			time.Sleep(10 * 24 * time.Hour)
			if _, err := f.auth.Authenticate(ctx, busy.Token); err != nil {
				t.Fatalf("a browser session in use expired: %v", err)
			}
		}
		if _, err := f.auth.Authenticate(ctx, browser.Token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("idle browser session: err = %v, want ErrUnauthenticated", err)
		}
		if alive, _ := f.auth.Alive(ctx, browser.Session.Caller()); alive {
			t.Fatal("idle browser session is Alive")
		}
		if _, err := f.auth.Authenticate(ctx, app.Token); err != nil {
			t.Fatalf("idle app session expired: %v", err)
		}
		time.Sleep(flushEvery)
		synctest.Wait()
		var n int
		if err := f.db.Reader().QueryRow(`SELECT count(*) FROM session WHERE id = ?`, browser.Session.ID).Scan(&n); err != nil || n != 0 {
			t.Fatalf("idle browser session still stored (n=%d, err=%v)", n, err)
		}
		cancel()
		<-done
	})
}

func TestLastUsedIsBatched(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		f := newFixture(t)
		iss := f.setUp()
		created := time.Now()
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = f.auth.Run(ctx)
		}()
		lastUsed := func() time.Time {
			var ms int64
			if err := f.db.Reader().QueryRow(`SELECT last_used_at FROM session WHERE id = ?`, iss.Session.ID).Scan(&ms); err != nil {
				t.Fatal(err)
			}
			return time.UnixMilli(ms)
		}

		time.Sleep(time.Minute)
		used := time.Now()
		if _, err := f.auth.Authenticate(ctx, iss.Token); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if !lastUsed().Equal(created) {
			t.Fatal("last-used written before the batch")
		}
		time.Sleep(flushEvery)
		synctest.Wait()
		if got := lastUsed(); !got.Equal(used) {
			t.Fatalf("last-used = %v after a flush, want %v", got, used)
		}

		// A use just before shutdown is written on the way out.
		time.Sleep(time.Minute)
		final := time.Now()
		if _, err := f.auth.Authenticate(ctx, iss.Token); err != nil {
			t.Fatal(err)
		}
		cancel()
		<-done
		if got := lastUsed(); !got.Equal(final) {
			t.Fatalf("last-used = %v after shutdown, want %v", got, final)
		}
	})
}

func TestSignInBacksOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := newFixture(t)
		f.setUp()
		wrong := SignInRequest{Username: "admin", Password: "wrong password", RemoteIP: other}
		for range userFreeFailures + 1 {
			if _, err := f.auth.SignIn(ctx, wrong); !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("err = %v, want ErrInvalidCredentials", err)
			}
		}
		// Now even the right password waits, and costs no hash.
		right := SignInRequest{Username: "admin", Password: password, RemoteIP: other}
		_, err := f.auth.SignIn(ctx, right)
		var te *ThrottledError
		if !errors.As(err, &te) || !errors.Is(err, ErrThrottled) || te.RetryAfter != firstBackoff {
			t.Fatalf("err = %v, want a ThrottledError after %v", err, firstBackoff)
		}
		// The owner, at home, isn't locked out.
		f.signIn(SignInRequest{Username: "admin", Password: password, RemoteIP: home})

		time.Sleep(te.RetryAfter)
		f.signIn(right)
	})
}

// newUser creates a non-admin user with a password and every library.
func (f *fixture) newUser(name string) id.ID {
	f.t.Helper()
	u, err := f.auth.CreateUser(context.Background(), Caller{}, users.NewUser{Username: name, Grants: users.Grants{All: true}}, password)
	if err != nil {
		f.t.Fatal(err)
	}
	return u.ID
}

func ptr[T any](v T) *T { return &v }
