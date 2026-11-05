package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// events returns the activity of a group, the newest first.
func (f *fixture) events(g activity.Group) []activity.Event {
	f.t.Helper()
	got, _, err := f.activity.List(context.Background(), g, activity.Position{}, 50)
	if err != nil {
		f.t.Fatal(err)
	}
	return got
}

// Sign-ins, failed sign-ins and users joining are recorded in the same
// writes that make them. A new session is recorded with its client and
// address. Failed sign-ins are counted per user and address. A new user is
// recorded with who added them.
func TestActivity(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	admin := f.setUp().Session.Caller()
	if got := f.events(""); len(got) != 0 {
		t.Fatalf("setup recorded %+v", got)
	}

	if _, err := f.auth.CreateUser(ctx, admin, users.NewUser{Username: "ben", Grants: users.Grants{All: true}}, password); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := f.auth.SignIn(ctx, SignInRequest{Username: "ben", Password: "wrong password", RemoteIP: other}); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("a wrong password: %v", err)
		}
	}
	f.signIn(SignInRequest{Username: "ben", Password: password, Name: "ben's iPhone", Kind: SessionApp, RemoteIP: other})

	dl := f.start(DeviceCodeStart{Name: "Living Room", RemoteIP: home})
	if err := f.auth.ApproveDeviceCode(ctx, admin, dl.Code, home); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := f.auth.PollDeviceCode(ctx, dl.Secret, home); !ok || err != nil {
		t.Fatalf("poll = %v, %v", ok, err)
	}

	lib := f.library("Movies")
	issued, err := f.auth.CreateInvite(ctx, admin, NewInvite{Grants: users.Grants{Libraries: []id.ID{lib}}, Expires: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.AcceptInvite(ctx, issued.Token, SignInRequest{Username: "dana", Password: password, RemoteIP: other}); err != nil {
		t.Fatal(err)
	}

	got := f.events(activity.GroupSignIns)
	if len(got) != 5 {
		t.Fatalf("sign-ins %+v", got)
	}
	if e := got[0]; e.Kind != activity.UserJoined || e.Username != "dana" || e.By != "" {
		t.Errorf("joined from an invite %+v", e)
	}
	if e := got[1]; e.Kind != activity.SignedIn || e.Username != "admin" || e.Client != "Living Room" || e.Address != home.String() || !e.Local {
		t.Errorf("signed in with a code %+v", e)
	}
	if e := got[2]; e.Kind != activity.SignedIn || e.Username != "ben" || e.Client != "ben's iPhone" || e.Address != other.String() || e.Local {
		t.Errorf("signed in %+v", e)
	}
	if e := got[3]; e.Kind != activity.SignInFailed || e.Username != "ben" || e.Count != 2 || e.Address != other.String() {
		t.Errorf("failed sign-ins %+v", e)
	}
	if e := got[4]; e.Kind != activity.UserJoined || e.Username != "ben" || e.By != "admin" {
		t.Errorf("added by an admin %+v", e)
	}
}
