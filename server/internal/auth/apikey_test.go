package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAPIKeys(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	admin := f.setUp().Session.Caller()

	for _, name := range []string{"", "  ", strings.Repeat("x", maxName+1)} {
		if _, err := f.auth.CreateAPIKey(ctx, admin, NewAPIKey{Name: name}); !errors.Is(err, ErrInvalidName) {
			t.Errorf("name %q: err = %v, want ErrInvalidName", name, err)
		}
	}
	issued, err := f.auth.CreateAPIKey(ctx, admin, NewAPIKey{Name: " Home Assistant "})
	if err != nil {
		t.Fatal(err)
	}
	k, token := issued.Key, issued.Token
	if k.Name != "Home Assistant" || k.ID.IsZero() || token == "" {
		t.Fatalf("CreateAPIKey = %+v, %q", k, token)
	}
	f.audit(actionAPIKeyCreated, 1)

	keys, err := f.auth.APIKeys(ctx)
	if err != nil || len(keys) != 1 || keys[0].ID != k.ID || keys[0].Name != k.Name || !keys[0].LastUsed.IsZero() {
		t.Fatalf("APIKeys before use = %+v, %v", keys, err)
	}

	// Its caller is the admin who made it, through the key.
	c, err := f.auth.Authenticate(ctx, token)
	if err != nil || c != (Caller{User: admin.User, APIKey: k.ID, Name: "Home Assistant"}) {
		t.Fatalf("Authenticate = %+v, %v", c, err)
	}
	if got, err := f.auth.APIKey(ctx, k.ID); err != nil || got.LastUsed.IsZero() {
		t.Fatalf("APIKey after use = %+v, %v", got, err)
	}
	if alive, err := f.auth.Alive(ctx, c); err != nil || !alive {
		t.Fatalf("Alive = %v, %v", alive, err)
	}
	if err := f.auth.SignOut(ctx, c); !errors.Is(err, ErrNotFound) {
		t.Fatalf("signing a key out: err = %v, want ErrNotFound", err)
	}

	// It isn't a session, and a new password keeps it.
	sessions, err := f.auth.Sessions(ctx, admin, admin.User)
	if err != nil || len(sessions) != 1 || sessions[0].ID != admin.Session {
		t.Fatalf("Sessions = %+v, %v", sessions, err)
	}
	if err := f.auth.RevokeSession(ctx, admin, k.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RevokeSession of a key: err = %v, want ErrNotFound", err)
	}
	if err := f.auth.SetPassword(ctx, admin, admin.User, "another password"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.Authenticate(ctx, token); err != nil {
		t.Fatalf("after a new password: %v", err)
	}

	if err := f.auth.RevokeAPIKey(ctx, admin, k.ID); err != nil {
		t.Fatal(err)
	}
	f.audit(actionAPIKeyRevoked, 1)
	if _, err := f.auth.Authenticate(ctx, token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("revoked key: err = %v", err)
	}
	if alive, err := f.auth.Alive(ctx, c); err != nil || alive {
		t.Fatalf("Alive after revoking = %v, %v", alive, err)
	}
	if err := f.auth.RevokeAPIKey(ctx, admin, k.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoking again: err = %v, want ErrNotFound", err)
	}
	if keys, err := f.auth.APIKeys(ctx); err != nil || len(keys) != 0 {
		t.Fatalf("APIKeys after revoking = %+v, %v", keys, err)
	}
}
