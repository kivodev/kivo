package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"unicode/utf8"

	"github.com/mmcdole/kivo/internal/auth/dbq"
	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// NeedsSetup reports whether first-run setup hasn't created the admin yet.
func (s *Service) NeedsSetup(ctx context.Context) (bool, error) {
	exists, err := s.q.AnyUser(ctx)
	if err != nil {
		return false, err
	}
	return exists == 0, nil
}

// Setup is first-run setup. It creates the admin user with access to every
// library, and signs it in. It fails with ErrAlreadySetUp once any user
// exists.
func (s *Service) Setup(ctx context.Context, r SignInRequest) (IssuedSession, error) {
	c, err := checkRequest(r)
	if err != nil {
		return IssuedSession{}, err
	}
	if err := checkPassword(r.Password); err != nil {
		return IssuedSession{}, err
	}
	// This is also checked here, so a late request doesn't spend a hash.
	if exists, err := s.q.AnyUser(ctx); err != nil {
		return IssuedSession{}, err
	} else if exists == 1 {
		return IssuedSession{}, ErrAlreadySetUp
	}
	phc, err := s.hasher.hash(r.Password)
	if err != nil {
		return IssuedSession{}, err
	}

	var out IssuedSession
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if exists, err := q.AnyUser(ctx); err != nil {
			return err
		} else if exists == 1 {
			return ErrAlreadySetUp
		}
		u, err := s.createUser(ctx, tx, users.NewUser{
			Username: r.Username, Admin: true, Grants: users.Grants{All: true},
		}, phc)
		if err != nil {
			return err
		}
		out, err = s.issue(ctx, tx, u.ID, c)
		if err != nil {
			return err
		}
		return audit(ctx, q, actionSetup, u.ID, out.Session.Caller(), r.RemoteIP)
	})
	if err != nil {
		return IssuedSession{}, err
	}
	return out, nil
}

// SignIn checks a username and password and starts a session. The new
// session replaces the user's session on the same client. Every failure
// returns ErrInvalidCredentials and costs one hash, whether or not the user
// exists. Before checking anything, it may refuse with a *ThrottledError.
// It fails with ErrBusy when all hashing capacity is in use.
func (s *Service) SignIn(ctx context.Context, r SignInRequest) (IssuedSession, error) {
	c, err := checkRequest(r)
	if err != nil {
		return IssuedSession{}, err
	}
	try, d := s.throttle.begin(r.RemoteIP, r.Username)
	if d > 0 {
		return IssuedSession{}, &ThrottledError{RetryAfter: d}
	}
	defer try.end()

	user, err := s.q.SignInUser(ctx, r.Username)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return IssuedSession{}, err
	}
	// An overlong password is truncated and still hashed, so it costs the
	// same.
	ok, err := s.hasher.verify(r.Password[:min(len(r.Password), maxPassword)], user.Secret.String)
	if err != nil {
		return IssuedSession{}, err
	}
	if !ok || user.Disabled || len(r.Password) > maxPassword {
		try.fail()
		return IssuedSession{}, s.signInFailed(ctx, user.ID, r.RemoteIP)
	}
	try.succeed()

	var out IssuedSession
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		var err error
		if out, err = s.issue(ctx, tx, user.ID, c); err != nil {
			return err
		}
		if err := audit(ctx, q, actionSignIn, user.ID, out.Session.Caller(), r.RemoteIP); err != nil {
			return err
		}
		return s.deps.Activity.SignedIn(ctx, tx, s.signIn(user.ID, c.name, r.RemoteIP))
	})
	if err != nil {
		return IssuedSession{}, err
	}
	return out, nil
}

// signInFailed records a failed sign-in by user, which is the zero ID for
// an unknown name. Once the failure is recorded, it returns
// ErrInvalidCredentials.
func (s *Service) signInFailed(ctx context.Context, user id.ID, ip netip.Addr) error {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		if err := audit(ctx, dbq.New(tx), actionSignInFailed, user, Caller{}, ip); err != nil {
			return err
		}
		return s.deps.Activity.Failed(ctx, tx, s.signIn(user, "", ip))
	})
	if err != nil {
		return err
	}
	return ErrInvalidCredentials
}

// signIn builds the activity record of a sign-in.
func (s *Service) signIn(user id.ID, client string, ip netip.Addr) activity.SignIn {
	return activity.SignIn{User: user, Client: client, Address: ip, Local: s.deps.Settings.Get().IsLocal(ip)}
}

// checkRequest checks and returns the client a sign-in is from. Its session
// kind defaults to browser.
func checkRequest(r SignInRequest) (client, error) {
	return checkClient(client{kind: r.Kind, name: r.Name, id: r.ClientID, device: r.Device}, SessionBrowser)
}

// checkClient checks the client a session is for and returns it cleaned
// up. The kind defaults to defaultKind. The name is trimmed. A platform
// the server doesn't know is left out, and a browser session that names
// none gets the browser platform.
func checkClient(c client, defaultKind SessionKind) (client, error) {
	switch c.kind {
	case "":
		c.kind = defaultKind
	case SessionBrowser, SessionApp:
	default:
		return client{}, fault.About(fmt.Errorf("%w: %q", ErrInvalidKind, c.kind), fault.Field{Name: "/kind", Code: "invalid"})
	}
	if !validClientID(c.id) {
		return client{}, ErrInvalidClientID
	}
	if !c.device.Platform.valid() {
		c.device.Platform = ""
	}
	if c.device.Platform == "" && c.kind == SessionBrowser {
		c.device.Platform = PlatformBrowser
	}
	var err error
	c.name, err = checkName(c.name)
	return c, err
}

// maxName is the most runes the name of a session or API key may have.
const maxName = 100

// checkName returns the trimmed name of a session or API key.
func checkName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > maxName {
		return "", ErrInvalidName
	}
	return name, nil
}

// maxClientID is the most runes a client ID may have.
const maxClientID = 100

// validClientID reports whether id is a valid client ID or empty.
func validClientID(id string) bool {
	return utf8.ValidString(id) && utf8.RuneCountInString(id) <= maxClientID
}
