package auth

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mmcdole/kivo/internal/store"
)

func (f *fixture) castToken(by Caller) IssuedCastToken {
	f.t.Helper()
	ct, err := f.auth.IssueCastToken(context.Background(), by, home, NewCastToken{Name: "Living Room TV", Receiver: CastWeb})
	if err != nil {
		f.t.Fatal(err)
	}
	return ct
}

func TestCastTokenPlaysAsTheSessionsUser(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	iss := f.setUp()
	ct := f.castToken(iss.Session.Caller())
	c, err := f.auth.Authenticate(ctx, ct.Token)
	if err != nil {
		t.Fatal(err)
	}
	if c.User != iss.Session.User || c.CastToken.IsZero() || !c.Session.IsZero() || !c.APIKey.IsZero() ||
		c.Name != "Living Room TV" {
		t.Fatalf("caller = %+v", c)
	}
	if alive, err := f.auth.Alive(ctx, c); err != nil || !alive {
		t.Fatalf("Alive = %v, %v", alive, err)
	}
	f.audit(actionCastTokenCreated, 1)

	// A cast token or an API key can't make one.
	if _, err := f.auth.IssueCastToken(ctx, c, home, NewCastToken{Name: "TV", Receiver: CastWeb}); !errors.Is(err, ErrNotFound) {
		t.Errorf("a cast token's cast token: %v", err)
	}
	if _, err := f.auth.IssueCastToken(ctx, Caller{User: c.User, APIKey: c.CastToken}, home, NewCastToken{Name: "TV", Receiver: CastWeb}); !errors.Is(err, ErrNotFound) {
		t.Errorf("an API key's cast token: %v", err)
	}
	if _, err := f.auth.IssueCastToken(ctx, iss.Session.Caller(), home, NewCastToken{Name: "TV", Receiver: "toaster"}); !errors.Is(err, ErrInvalidReceiver) {
		t.Errorf("an unknown receiver: %v", err)
	}

	// Only the cast token itself can revoke it, and revoking it leaves the
	// session working.
	if err := f.auth.RevokeCastToken(ctx, iss.Session.Caller()); !errors.Is(err, ErrNotFound) {
		t.Errorf("a session revoking a cast token: %v", err)
	}
	if err := f.auth.RevokeCastToken(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.Authenticate(ctx, ct.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("revoked: %v", err)
	}
	if _, err := f.auth.Authenticate(ctx, iss.Token); err != nil {
		t.Errorf("the session after its cast token's revocation: %v", err)
	}

	// Signing the session out ends its cast tokens.
	ct = f.castToken(iss.Session.Caller())
	c, _ = f.auth.Authenticate(ctx, ct.Token)
	if err := f.auth.SignOut(ctx, iss.Session.Caller()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.Authenticate(ctx, ct.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("after sign-out: %v", err)
	}
	if alive, _ := f.auth.Alive(ctx, c); alive {
		t.Error("Alive after sign-out")
	}
}

func TestCastTokenExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		f := newFixture(t)
		iss := f.setUp()
		app := f.signIn(SignInRequest{Username: "admin", Password: password, RemoteIP: home, Kind: SessionApp})
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = f.auth.Run(ctx)
		}()

		// Used every 10 minutes, it lasts until 24 hours after it was made.
		busy := f.castToken(app.Session.Caller())
		idle := f.castToken(app.Session.Caller())
		idleCaller, _ := f.auth.Authenticate(ctx, idle.Token)
		var busyCaller Caller
		for i := range 24*6 - 1 {
			time.Sleep(10 * time.Minute)
			c, err := f.auth.Authenticate(ctx, busy.Token)
			if err != nil {
				t.Fatalf("a cast token in use expired after %v: %v", time.Duration(i+1)*10*time.Minute, err)
			}
			busyCaller = c
		}
		if _, err := f.auth.Authenticate(ctx, idle.Token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("idle cast token: %v", err)
		}
		if alive, _ := f.auth.Alive(ctx, idleCaller); alive {
			t.Fatal("idle cast token is Alive")
		}
		time.Sleep(10*time.Minute + time.Second)
		if _, err := f.auth.Authenticate(ctx, busy.Token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("a cast token past 24 hours: %v", err)
		}
		if alive, _ := f.auth.Alive(ctx, busyCaller); alive {
			t.Fatal("a cast token past 24 hours is Alive")
		}

		// Expired tokens are dropped at the next flush.
		time.Sleep(flushEvery)
		synctest.Wait()
		var n int
		if err := f.db.Reader().QueryRow(`SELECT count(*) FROM cast_token`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%d expired cast tokens stored (err=%v)", n, err)
		}

		// A cast token's browser session expiring ends it too.
		ct := f.castToken(iss.Session.Caller())
		err := f.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
			_, err := tx.ExecContext(ctx, `UPDATE session SET last_used_at = ? WHERE id = ?`,
				time.Now().Add(-browserIdle-time.Minute).UnixMilli(), iss.Session.ID)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.auth.Authenticate(ctx, ct.Token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("a cast token of an expired browser session: %v", err)
		}
		cancel()
		<-done
	})
}
