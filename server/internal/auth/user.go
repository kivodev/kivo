package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/netip"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/auth/dbq"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// CreateUser creates a user with a password, as when an admin adds someone
// directly. by is the admin, recorded as who added the user.
func (s *Service) CreateUser(ctx context.Context, by Caller, u users.NewUser, password string) (users.User, error) {
	if err := checkPassword(password); err != nil {
		return users.User{}, err
	}
	phc, err := s.hasher.hash(password)
	if err != nil {
		return users.User{}, err
	}
	var out users.User
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		if out, err = s.createUser(ctx, tx, u, phc); err != nil {
			return err
		}
		return s.deps.Activity.Joined(ctx, tx, out.ID, by.User)
	})
	if err != nil {
		return users.User{}, err
	}
	return out, nil
}

// SetPassword replaces a user's password and signs out the user's sessions
// except by's own, so a user setting their own password stays signed in.
// When by has no session, such as an API key or the zero Caller, every
// session is signed out. The user's API keys stay. The change is recorded
// with by as who made it, unless by is the user or names nobody.
func (s *Service) SetPassword(ctx context.Context, by Caller, user id.ID, password string) error {
	if err := checkPassword(password); err != nil {
		return err
	}
	phc, err := s.hasher.hash(password)
	if err != nil {
		return err
	}
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		n, err := q.SetPassword(ctx, dbq.SetPasswordParams{ID: id.New(), Secret: phc, Now: time.Now().UnixMilli(), UserID: user})
		if err != nil {
			return err
		}
		if n == 0 {
			return users.ErrNotFound
		}
		if err := q.DeleteOtherSessions(ctx, dbq.DeleteOtherSessionsParams{UserID: user, Keep: by.Session}); err != nil {
			return err
		}
		return s.deps.Activity.PasswordSet(ctx, tx, user, by.User, netip.Addr{})
	})
	return err
}

// PasswordChange is a user changing their own password.
type PasswordChange struct {
	Current string
	New     string
	// SignOutOthers signs out the user's other sessions. The caller's own
	// session and the user's API keys stay.
	SignOutOthers bool
	RemoteIP      netip.Addr
}

// ChangePassword replaces the caller's own password after checking the
// current one. A wrong current password returns ErrWrongPassword. It counts
// against the user and address as a failed sign-in does, and enough of them
// return a *ThrottledError.
func (s *Service) ChangePassword(ctx context.Context, c Caller, r PasswordChange) error {
	if err := checkPassword(r.New); err != nil {
		return err
	}
	user, err := s.q.UserPassword(ctx, c.User)
	if errors.Is(err, sql.ErrNoRows) {
		return users.ErrNotFound
	}
	if err != nil {
		return err
	}
	try, d := s.throttle.begin(r.RemoteIP, user.Username)
	if d > 0 {
		return &ThrottledError{RetryAfter: d}
	}
	defer try.end()
	ok, err := s.hasher.verify(r.Current[:min(len(r.Current), maxPassword)], user.Secret.String)
	if err != nil {
		return err
	}
	if !ok || len(r.Current) > maxPassword {
		try.fail()
		return ErrWrongPassword
	}
	try.succeed()

	phc, err := s.hasher.hash(r.New)
	if err != nil {
		return err
	}
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		n, err := q.SetPassword(ctx, dbq.SetPasswordParams{ID: id.New(), Secret: phc, Now: time.Now().UnixMilli(), UserID: c.User})
		if err != nil {
			return err
		}
		if n == 0 {
			return users.ErrNotFound
		}
		if r.SignOutOthers {
			if err := q.DeleteOtherSessions(ctx, dbq.DeleteOtherSessionsParams{UserID: c.User, Keep: c.Session}); err != nil {
				return err
			}
		}
		return s.deps.Activity.PasswordSet(ctx, tx, c.User, id.ID{}, r.RemoteIP)
	})
}

// createUser creates a user with a password hash in the caller's
// transaction.
func (s *Service) createUser(ctx context.Context, tx *store.Tx, u users.NewUser, phc string) (users.User, error) {
	out, err := s.deps.Users.CreateUser(ctx, tx, u)
	if err != nil {
		return users.User{}, err
	}
	err = dbq.New(tx).InsertPassword(ctx, dbq.InsertPasswordParams{
		ID: id.New(), UserID: out.ID, Secret: phc, Now: time.Now().UnixMilli(),
	})
	if err != nil {
		return users.User{}, err
	}
	return out, nil
}

// The bounds of a password's length in bytes. argon2 hashes any length, but
// nobody needs more than maxPassword.
const (
	minPassword = 8
	maxPassword = 1024
)

func checkPassword(p string) error {
	if n := len(p); n < minPassword || n > maxPassword {
		return ErrWeakPassword
	}
	return nil
}
