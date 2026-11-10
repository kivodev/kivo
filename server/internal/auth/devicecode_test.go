package auth

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mmcdole/kivo/internal/config"
)

// start starts a device code from ip, failing the test on an error.
func (f *fixture) start(r DeviceCodeStart) DeviceCode {
	f.t.Helper()
	if r.Name == "" {
		r.Name = "Den"
	}
	if r.Device.Platform == "" {
		r.Device.Platform = PlatformAndroidTV
	}
	dl, err := f.auth.StartDeviceCode(context.Background(), r)
	if err != nil {
		f.t.Fatal(err)
	}
	return dl
}

func TestDeviceCode(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	admin := f.setUp().Session.Caller()

	started := time.Now()
	dl := f.start(DeviceCodeStart{Name: " Living Room ", Device: Device{Platform: PlatformAndroidTV}, RemoteIP: home})
	if len(dl.Code) != codeLen || strings.Trim(dl.Code, codeAlphabet) != "" || len(dl.Secret) < 40 || dl.Interval != 2*time.Second {
		t.Fatalf("started %+v", dl)
	}
	if _, ok, err := f.auth.PollDeviceCode(ctx, dl.Secret, home); ok || err != nil {
		t.Fatalf("poll before approval = %v, %v", ok, err)
	}

	// Typed in lower case with a space or a hyphen, as people do.
	typed := strings.ToLower(dl.Code[:3] + " " + dl.Code[3:])
	got, err := f.auth.LookUpDeviceCode(ctx, admin, typed, home)
	if err != nil || got.Name != "Living Room" || got.Platform != PlatformAndroidTV || !got.Nearby || got.Started.Before(started.Add(-time.Second)) {
		t.Fatalf("look up = %+v, %v", got, err)
	}
	if err := f.auth.ApproveDeviceCode(ctx, admin, dl.Code[:3]+"-"+dl.Code[3:], home); err != nil {
		t.Fatal(err)
	}
	// Once answered, the code is no longer waiting.
	if err := f.auth.ApproveDeviceCode(ctx, admin, dl.Code, home); !errors.Is(err, ErrCodeNotFound) {
		t.Fatalf("second approval: err = %v", err)
	}
	if err := f.auth.DenyDeviceCode(ctx, admin, dl.Code, home); !errors.Is(err, ErrCodeNotFound) {
		t.Fatalf("denying an approved code: err = %v", err)
	}
	if _, ok, err := f.auth.PollDeviceCode(ctx, "wrong", home); ok || !errors.Is(err, ErrCodeGone) {
		t.Fatalf("wrong secret = %v, %v", ok, err)
	}

	iss, ok, err := f.auth.PollDeviceCode(ctx, dl.Secret, home)
	if err != nil || !ok {
		t.Fatalf("poll after approval = %v, %v", ok, err)
	}
	p, err := f.auth.Authenticate(ctx, iss.Token)
	if err != nil || p.User != admin.User || iss.Session.Kind != SessionApp || p.Name != "Living Room" {
		t.Fatalf("linked session = %+v, %v", p, err)
	}
	list, err := f.auth.Sessions(ctx, admin, admin.User)
	i := slices.IndexFunc(list, func(s Session) bool { return s.ID == p.Session })
	if err != nil || len(list) != 2 || i < 0 || list[i].Name != "Living Room" || list[i].Device.Platform != PlatformAndroidTV {
		t.Fatalf("sessions = %+v, %v", list, err)
	}
	// Once used, the secret gets nothing more.
	if _, ok, err := f.auth.PollDeviceCode(ctx, dl.Secret, home); ok || !errors.Is(err, ErrCodeGone) {
		t.Fatalf("poll after use = %v, %v", ok, err)
	}

	// A linked device can be signed out like any other.
	if err := f.auth.RevokeSession(ctx, admin, p.Session); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.Authenticate(ctx, iss.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("revoked linked session: err = %v", err)
	}

	for _, r := range []DeviceCodeStart{
		{Name: "  "},
		{Name: "Den", Kind: "robot"},
	} {
		r.RemoteIP = home
		if _, err := f.auth.StartDeviceCode(ctx, r); err == nil {
			t.Errorf("started %+v", r)
		}
	}
	if _, err := f.auth.StartDeviceCode(ctx, DeviceCodeStart{Name: "Den", RemoteIP: home}); err != nil {
		t.Errorf("start without a platform: %v", err)
	}
}

// Once a code is denied, every poll gets the denial, and the code can't be
// approved afterwards.
func TestDeviceCodeDenied(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	admin := f.setUp().Session.Caller()
	dl := f.start(DeviceCodeStart{RemoteIP: home})
	if err := f.auth.DenyDeviceCode(ctx, admin, dl.Code, home); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, ok, err := f.auth.PollDeviceCode(ctx, dl.Secret, home); ok || !errors.Is(err, ErrDenied) {
			t.Fatalf("poll after denial = %v, %v", ok, err)
		}
	}
	if err := f.auth.ApproveDeviceCode(ctx, admin, dl.Code, home); !errors.Is(err, ErrCodeNotFound) {
		t.Fatalf("approving a denied code: err = %v", err)
	}
	if _, err := f.auth.LookUpDeviceCode(ctx, admin, dl.Code, home); !errors.Is(err, ErrCodeNotFound) {
		t.Fatalf("looking up a denied code: err = %v", err)
	}
}

// When a code is approved and denied at the same time, exactly one wins.
func TestDeviceCodeAnsweredOnce(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	admin := f.setUp().Session.Caller()
	for i := range 20 {
		// The loser counts as a guess, so each round is from its own address.
		ip := netip.AddrFrom4([4]byte{192, 168, 2, byte(i)})
		dl := f.start(DeviceCodeStart{RemoteIP: ip})
		var wg sync.WaitGroup
		errs := make([]error, 2)
		wg.Go(func() { errs[0] = f.auth.ApproveDeviceCode(ctx, admin, dl.Code, ip) })
		wg.Go(func() { errs[1] = f.auth.DenyDeviceCode(ctx, admin, dl.Code, ip) })
		wg.Wait()
		if (errs[0] == nil) == (errs[1] == nil) {
			t.Fatalf("approve %v, deny %v", errs[0], errs[1])
		}
		_, ok, err := f.auth.PollDeviceCode(ctx, dl.Secret, ip)
		if ok != (errs[0] == nil) || !ok && !errors.Is(err, ErrDenied) {
			t.Fatalf("poll = %v, %v after approve %v, deny %v", ok, err, errs[0], errs[1])
		}
	}
}

// The server decides whether the device is nearby, by checking its address
// against the local networks.
func TestDeviceCodeNearby(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	admin := f.setUp().Session.Caller()
	nearby := func(ip netip.Addr) bool {
		t.Helper()
		got, err := f.auth.LookUpDeviceCode(ctx, admin, f.start(DeviceCodeStart{RemoteIP: ip}).Code, home)
		if err != nil {
			t.Fatal(err)
		}
		return got.Nearby
	}
	if !nearby(home) || nearby(other) || !nearby(netip.MustParseAddr("::1")) {
		t.Error("by default, private and loopback addresses are nearby and others aren't")
	}
	err := f.settings.Update(ctx, func(s *config.Settings) error {
		s.LocalNetworks = []string{"203.0.113.0/24"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if nearby(home) || !nearby(other) {
		t.Error("with local networks set, only addresses in them are nearby")
	}
}

// A browser that signs in by code gets a browser session on its own client.
func TestDeviceCodeBrowser(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	admin := f.setUp().Session.Caller()
	// A browser that names no platform gets the browser platform.
	dl, err := f.auth.StartDeviceCode(ctx, DeviceCodeStart{Name: "Firefox on Linux", Kind: SessionBrowser, ClientID: "browser-1", RemoteIP: home})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := f.auth.LookUpDeviceCode(ctx, admin, dl.Code, home); err != nil || got.Platform != PlatformBrowser {
		t.Fatalf("look up = %+v, %v", got, err)
	}
	if err := f.auth.ApproveDeviceCode(ctx, admin, dl.Code, home); err != nil {
		t.Fatal(err)
	}
	iss, ok, err := f.auth.PollDeviceCode(ctx, dl.Secret, home)
	if err != nil || !ok || iss.Session.Kind != SessionBrowser || iss.Session.Device.Platform != PlatformBrowser || iss.ClientID != "browser-1" {
		t.Fatalf("poll = %+v, %v, %v", iss, ok, err)
	}
}

// A device that signs in by code again with its client ID replaces the
// approving user's session on that device.
func TestDeviceCodeReplacesTheClientsSession(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	admin := f.setUp().Session.Caller()
	link := func() IssuedSession {
		t.Helper()
		dl := f.start(DeviceCodeStart{ClientID: "tv-1", RemoteIP: other})
		if err := f.auth.ApproveDeviceCode(ctx, admin, dl.Code, home); err != nil {
			t.Fatal(err)
		}
		iss, ok, err := f.auth.PollDeviceCode(ctx, dl.Secret, other)
		if err != nil || !ok {
			t.Fatalf("poll = %v, %v", ok, err)
		}
		return iss
	}
	first := link()
	link()
	if _, err := f.auth.Authenticate(ctx, first.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("the replaced session: err = %v", err)
	}
	if list, err := f.auth.Sessions(ctx, admin, admin.User); err != nil || len(list) != 2 {
		t.Fatalf("sessions = %+v, %v; want setup's and the TV's", list, err)
	}
}

func TestUserCodesExpire(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := newFixture(t)
		admin := f.setUp().Session.Caller()
		dl := f.start(DeviceCodeStart{RemoteIP: other})
		if !dl.Expires.Equal(time.Now().Add(5 * time.Minute)) {
			t.Fatalf("expires %v", dl.Expires)
		}
		time.Sleep(codeTTL)
		if _, err := f.auth.LookUpDeviceCode(ctx, admin, dl.Code, home); !errors.Is(err, ErrCodeNotFound) {
			t.Fatalf("expired code: err = %v", err)
		}
		if err := f.auth.ApproveDeviceCode(ctx, admin, dl.Code, home); !errors.Is(err, ErrCodeNotFound) {
			t.Fatalf("approving an expired code: err = %v", err)
		}
		if err := f.auth.DenyDeviceCode(ctx, admin, dl.Code, home); !errors.Is(err, ErrCodeNotFound) {
			t.Fatalf("denying an expired code: err = %v", err)
		}
		if _, ok, err := f.auth.PollDeviceCode(ctx, dl.Secret, other); ok || !errors.Is(err, ErrCodeGone) {
			t.Fatalf("poll after expiry = %v, %v", ok, err)
		}
	})
}

func TestDeviceCodeIsThrottled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := newFixture(t)
		admin := f.setUp().Session.Caller()

		// Guessed codes back off as wrong passwords do, whether approving
		// or denying.
		for i := range userFreeFailures {
			guess := f.auth.ApproveDeviceCode
			if i%2 == 1 {
				guess = f.auth.DenyDeviceCode
			}
			if err := guess(ctx, admin, "AAAAAA", home); !errors.Is(err, ErrCodeNotFound) {
				t.Fatalf("err = %v", err)
			}
		}
		dl := f.start(DeviceCodeStart{RemoteIP: other})
		if err := f.auth.DenyDeviceCode(ctx, admin, "AAAAAA", home); !errors.Is(err, ErrCodeNotFound) {
			t.Fatal(err)
		}
		if err := f.auth.DenyDeviceCode(ctx, admin, dl.Code, home); !errors.Is(err, ErrThrottled) {
			t.Fatalf("after guessing: err = %v, want ErrThrottled", err)
		}

		// So are guessed secrets.
		for range userFreeFailures + 1 {
			_, _, _ = f.auth.PollDeviceCode(ctx, "guess", other)
		}
		if _, _, err := f.auth.PollDeviceCode(ctx, dl.Secret, other); !errors.Is(err, ErrThrottled) {
			t.Fatalf("after guessing secrets: err = %v, want ErrThrottled", err)
		}
	})
}

// One address may have only a few codes pending at once. An IPv6 client is
// counted by its /64, so rotating through its addresses gains nothing.
func TestUserCodesPerAddress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := newFixture(t)
		for _, c := range []struct {
			name  string
			addr  func(i int) netip.Addr
			other netip.Addr
		}{
			{"IPv4", func(int) netip.Addr { return other }, home},
			{"IPv6 /64", func(i int) netip.Addr {
				return netip.AddrFrom16([16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, byte(i + 1)})
			}, netip.MustParseAddr("2001:db8:0:2::1")},
		} {
			for i := range maxCodesPerAddr {
				f.start(DeviceCodeStart{RemoteIP: c.addr(i)})
			}
			_, err := f.auth.StartDeviceCode(ctx, DeviceCodeStart{Name: "Den", RemoteIP: c.addr(maxCodesPerAddr)})
			var te *ThrottledError
			if !errors.As(err, &te) || te.RetryAfter != codeTTL {
				t.Fatalf("%s: too many codes: err = %v", c.name, err)
			}
			f.start(DeviceCodeStart{RemoteIP: c.other})
		}
		// Once the oldest expire, the address may start again.
		time.Sleep(codeTTL)
		f.start(DeviceCodeStart{RemoteIP: other})
	})
}
