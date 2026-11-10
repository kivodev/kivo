package api

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/auth"
)

// identity is an Android TV app's identity headers, as name and value
// pairs, at version.
func identity(version string) []string {
	return []string{appHeader, "kivo-android", appVersionHeader, version, platformHeader, "androidTv",
		osVersionHeader, "14", deviceModelHeader, "SHIELD Android TV"}
}

func TestParseDevice(t *testing.T) {
	h, pairs := http.Header{}, identity("0.3.0+78")
	for i := 0; i < len(pairs); i += 2 {
		h.Set(pairs[i], pairs[i+1])
	}
	want := auth.Device{App: "kivo-android", AppVersion: "0.3.0+78", Platform: auth.PlatformAndroidTV, OSVersion: "14", Model: "SHIELD Android TV"}
	if got := parseDevice(h); got != want {
		t.Errorf("%+v, want %+v", got, want)
	}

	// A value that's too long or isn't printable ASCII is left out alone.
	h.Set(deviceModelHeader, strings.Repeat("a", maxDeviceValue+1))
	h.Set(osVersionHeader, "é")
	want.Model, want.OSVersion = "", ""
	if got := parseDevice(h); got != want {
		t.Errorf("%+v, want %+v", got, want)
	}
	if got := parseDevice(http.Header{}); got != (auth.Device{}) {
		t.Errorf("no headers: %+v", got)
	}
}

// TestDevice checks that a session keeps the device it signed in from,
// takes a changed one from a later request, and that a play takes it from
// its start request.
func TestDevice(t *testing.T) {
	e := newEnv(t, serving{})
	w := e.do("POST", "/api/v1/sessions", "", map[string]string{"username": "admin", "password": adminPassword, "kind": "app", "name": "Living Room"},
		identity("0.3.0+78")...)
	if w.Code != http.StatusCreated {
		t.Fatalf("sign in: %d %s", w.Code, w.Body.String())
	}
	s := decode[gen.Session](t, w)
	want := gen.ClientDevice{App: ptr("kivo-android"), AppVersion: ptr("0.3.0+78"), Platform: ptr(gen.ClientPlatformAndroidTv), OsVersion: ptr("14"),
		DeviceModel: ptr("SHIELD Android TV")}
	if s.Device == nil || !reflect.DeepEqual(*s.Device, want) {
		t.Fatalf("the session: %+v", s)
	}
	sessions := func() gen.Session {
		t.Helper()
		page := decode[gen.SessionList](t, e.do("GET", "/api/v1/sessions", *s.Token, nil))
		for _, x := range page.Items {
			if x.ID == s.ID {
				return x
			}
		}
		t.Fatalf("no session %v in %+v", s.ID, page)
		return gen.Session{}
	}
	// A request without the headers changes nothing; one with a new version
	// updates the session.
	if got := sessions(); !reflect.DeepEqual(*got.Device, want) {
		t.Fatalf("after a request without the headers: %+v", got.Device)
	}
	e.do("GET", "/api/v1/users/me", *s.Token, nil, identity("0.3.0+79")...)
	want.AppVersion = ptr("0.3.0+79")
	if got := sessions(); !reflect.DeepEqual(*got.Device, want) {
		t.Fatalf("after an update: %+v", got.Device)
	}

	w = e.do("POST", "/api/v1/plays", *s.Token, map[string]any{"workId": e.heat.Work, "capabilities": clientCaps}, identity("0.3.0+79")...)
	if w.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	pl := decode[gen.Play](t, e.do("GET", "/api/v1/plays/"+decode[gen.PlayStarted](t, w).ID.String(), e.adminToken, nil))
	if pl.Device == nil || !reflect.DeepEqual(*pl.Device, want) || pl.Delivery != gen.PlayDeliveryFile || len(pl.Tracks) != 3 {
		t.Fatalf("now playing: %+v", pl)
	}
}
