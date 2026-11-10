package auth

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

func (f *fixture) library(name string) id.ID {
	f.t.Helper()
	var lib id.ID
	err := f.db.Write(context.Background(), func(ctx context.Context, tx *store.Tx) error {
		l, err := catalog.New(f.db, events.NewBus()).CreateLibrary(ctx, tx, catalog.LibraryInput{Name: name, Kind: catalog.Movies, Folders: []string{catalogtest.Abs("/media/" + name)}})
		lib = l.ID
		return err
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return lib
}

func TestCreateUserAndSetPassword(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	admin := f.setUp().Session.Caller()
	movies := f.library("Movies")

	_, err := f.auth.CreateUser(ctx, admin, users.NewUser{Username: "ben", Grants: users.Grants{Libraries: []id.ID{movies}}}, "short")
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("short password: err = %v, want ErrWeakPassword", err)
	}
	u, err := f.auth.CreateUser(ctx, admin, users.NewUser{Username: "ben", Grants: users.Grants{Libraries: []id.ID{movies}}}, password)
	if err != nil {
		t.Fatal(err)
	}
	ben := f.signIn(SignInRequest{Username: "ben", Password: password, RemoteIP: home})
	if ben.Session.User != u.ID {
		t.Fatalf("signed in as %v, want %v", ben.Session.User, u.ID)
	}

	// A new password signs out every session but the setter's own.
	if err := f.auth.SetPassword(ctx, admin, u.ID, "another password"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.Authenticate(ctx, ben.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("session after a new password: err = %v", err)
	}
	if _, err := f.auth.SignIn(ctx, SignInRequest{Username: "ben", Password: password, RemoteIP: other}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password: err = %v", err)
	}
	f.signIn(SignInRequest{Username: "ben", Password: "another password", RemoteIP: home})

	self := f.signIn(SignInRequest{Username: "admin", Password: password, RemoteIP: home}).Session.Caller()
	if err := f.auth.SetPassword(ctx, self, self.User, "a new admin password"); err != nil {
		t.Fatal(err)
	}
	if alive, _ := f.auth.Alive(ctx, self); !alive {
		t.Fatal("setting one's own password signed out the session that set it")
	}
	if alive, _ := f.auth.Alive(ctx, admin); alive {
		t.Fatal("setting one's own password kept another session")
	}
	if err := f.auth.SetPassword(ctx, self, id.New(), password); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("unknown user: err = %v, want users.ErrNotFound", err)
	}
}

func TestInvites(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	admin := f.setUp().Session.Caller()
	movies, tv := f.library("Movies"), f.library("TV")
	week := time.Now().Add(7 * 24 * time.Hour)

	if _, err := f.auth.CreateInvite(ctx, admin, NewInvite{Expires: week}); !errors.Is(err, users.ErrNoLibraries) {
		t.Fatalf("no libraries: err = %v", err)
	}
	if _, err := f.auth.CreateInvite(ctx, admin, NewInvite{Grants: users.Grants{All: true}, Expires: time.Now().Add(-time.Minute)}); !errors.Is(err, ErrInvalidExpiry) {
		t.Fatalf("expired: err = %v", err)
	}
	issued, err := f.auth.CreateInvite(ctx, admin, NewInvite{
		Grants: users.Grants{Libraries: []id.ID{tv, movies}}, Limits: users.Limits{StreamLimit: 2}, Expires: week,
	})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := f.auth.Invites(ctx)
	if err != nil || len(pending) != 1 || pending[0].ID != issued.Invite.ID || pending[0].CreatedBy != "admin" || pending[0].Limits.StreamLimit != 2 {
		t.Fatalf("Invites = %+v, %v", pending, err)
	}
	if got, err := f.auth.InviteByToken(ctx, issued.Token); err != nil || got.CreatedBy != "admin" {
		t.Fatalf("InviteByToken = %+v, %v", got, err)
	}
	if _, err := f.auth.InviteByToken(ctx, "nonsense"); !errors.Is(err, ErrInviteNotFound) {
		t.Fatalf("bad token: err = %v", err)
	}

	// Accepting gives exactly the invite's access, and signs in.
	iss, err := f.auth.AcceptInvite(ctx, issued.Token, SignInRequest{Username: "cara", Password: password, RemoteIP: other})
	if err != nil {
		t.Fatal(err)
	}
	u, err := f.users.User(ctx, iss.Session.User)
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "cara" || u.Admin || u.Grants.All || !slices.Contains(u.Grants.Libraries, movies) ||
		!slices.Contains(u.Grants.Libraries, tv) || u.Limits.StreamLimit != 2 || u.InvitedBy != admin.User {
		t.Fatalf("invited user = %+v", u)
	}
	if _, err := f.auth.Authenticate(ctx, iss.Token); err != nil {
		t.Fatal(err)
	}
	// The link works once.
	if _, err := f.auth.AcceptInvite(ctx, issued.Token, SignInRequest{Username: "dan", Password: password}); !errors.Is(err, ErrInviteNotFound) {
		t.Fatalf("second accept: err = %v", err)
	}
	if pending, _ := f.auth.Invites(ctx); len(pending) != 0 {
		t.Fatalf("a used invite is pending: %+v", pending)
	}

	// A revoked link stops working.
	issued, err = f.auth.CreateInvite(ctx, admin, NewInvite{Grants: users.Grants{All: true}, Expires: week})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.auth.RevokeInvite(ctx, issued.Invite.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.InviteByToken(ctx, issued.Token); !errors.Is(err, ErrInviteNotFound) {
		t.Fatalf("revoked: err = %v", err)
	}
	if err := f.auth.RevokeInvite(ctx, issued.Invite.ID); !errors.Is(err, ErrInviteNotFound) {
		t.Fatalf("second revoke: err = %v", err)
	}
}
