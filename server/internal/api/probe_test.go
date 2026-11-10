package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/config"
)

// probeOf returns a session token's key and probe ID, derived the way an
// app derives them.
func probeOf(t *testing.T, token string) (key []byte, probe string) {
	t.Helper()
	body, ok := strings.CutPrefix(token, "kivo_s_")
	if !ok {
		t.Fatalf("%q isn't a session token", token)
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	id := sha256.Sum256(append([]byte("kivo-probe-id"), hash[:]...))
	return hash[:], base64.RawURLEncoding.EncodeToString(id[:16])
}

// wantMAC computes the proof's MAC the way an app checks it, independently
// of the server's code.
func wantMAC(key, challenge []byte, serverID, host string, local bool) string {
	msg := []byte("kivo-probe-v1")
	for _, f := range [][]byte{challenge, []byte(serverID), []byte(host)} {
		msg = append(msg, byte(len(f)>>8), byte(len(f)))
		msg = append(msg, f...)
	}
	if local {
		msg = append(msg, 1)
	} else {
		msg = append(msg, 0)
	}
	m := hmac.New(sha256.New, key)
	m.Write(msg)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

var probeChallenge = []byte("0123456789abcdef0123456789abcdef")

// probe sends the unauthenticated server probe from remote to host.
// headers holds name and value pairs.
func probe(t *testing.T, e *env, probeID, remote, host string, headers ...string) (int, gen.ServerInfo) {
	t.Helper()
	q := url.Values{"challenge": {base64.RawURLEncoding.EncodeToString(probeChallenge)}, "probeId": {probeID}}
	r := httptest.NewRequest("GET", "/api/v1/server?"+q.Encode(), nil)
	r.RemoteAddr, r.Host = remote, host
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	e.srv.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		return w.Code, gen.ServerInfo{}
	}
	return w.Code, decode[gen.ServerInfo](t, w)
}

func TestProbeProvesTheServer(t *testing.T) {
	e := newEnv(t, serving{})
	key, id := probeOf(t, e.adminToken)
	serverID := e.d.Settings.ServerID().String()
	for _, tc := range []struct {
		remote string
		local  bool
	}{
		{"192.168.1.5:5000", true},
		{"10.1.2.3:5000", true},
		{"172.16.0.9:5000", true},
		{"127.0.0.1:5000", true},
		{"169.254.3.4:5000", true},
		{"[fd00::5]:5000", true},
		{"[fe80::1]:5000", true},
		{"[::1]:5000", true},
		{"[::ffff:192.168.1.5]:5000", true},
		{"203.0.113.5:5000", false},
		{"100.64.1.1:5000", false},
		{"100.128.0.1:5000", false},
		{"172.32.0.1:5000", false},
		{"[2001:db8::1]:5000", false},
	} {
		code, info := probe(t, e, id, tc.remote, "192.168.1.20:8484")
		if code != http.StatusOK || info.Proof == nil {
			t.Fatalf("%s: %d, proof %v", tc.remote, code, info.Proof)
		}
		p := info.Proof
		if p.Local != tc.local || p.Host != "192.168.1.20:8484" || p.Mac != wantMAC(key, probeChallenge, serverID, p.Host, tc.local) {
			t.Errorf("%s: %+v", tc.remote, *p)
		}
		if info.LocalUrls != nil || info.PublicURL != nil {
			t.Errorf("%s: a probe without a token got the addresses", tc.remote)
		}
	}
}

// A Tailscale address is remote to both the probe and playback. It becomes
// local to both once it's in the local networks.
func TestProbeAndPlaybackAgree(t *testing.T) {
	e := newEnv(t, serving{})
	_, id := probeOf(t, e.adminToken)
	const tailscale = "100.101.102.103:5000"
	for _, nets := range [][]string{nil, {"100.64.0.0/10"}} {
		if err := e.d.Settings.Update(context.Background(), func(s *config.Settings) error { s.LocalNetworks = nets; return nil }); err != nil {
			t.Fatal(err)
		}
		local := nets != nil
		if _, info := probe(t, e, id, tailscale, "kivo.example.com"); info.Proof == nil || info.Proof.Local != local {
			t.Errorf("local networks %v: probe proof %+v", nets, info.Proof)
		}
		b, err := json.Marshal(map[string]any{"workId": e.heat.Work, "capabilities": clientCaps})
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", "/api/v1/plays", bytes.NewReader(b))
		r.RemoteAddr = tailscale
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+e.adminToken)
		w := httptest.NewRecorder()
		e.srv.ServeHTTP(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("local networks %v: start: %d %s", nets, w.Code, w.Body.String())
		}
		if ps := decode[gen.PlayStarted](t, w); ps.Local != local {
			t.Errorf("local networks %v: play local %v", nets, ps.Local)
		}
	}
}

// An untrusted peer's forwarded headers are ignored. A trusted proxy's
// headers count, so a proxy relaying a client on the internet never makes
// that client local.
func TestProbeForwarded(t *testing.T) {
	e := newEnv(t, serving{Options: Options{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.1/32")}}})
	key, id := probeOf(t, e.adminToken)
	serverID := e.d.Settings.ServerID().String()
	for _, tc := range []struct {
		name, remote, reached, xff, xfh string
		host                            string // what the proof names
		local                           bool
	}{
		{"spoofed by a peer on the internet", "203.0.113.5:5000", "kivo.example.com", "192.168.1.5", "192.168.1.20:8484", "kivo.example.com", false},
		{"spoofed by a peer on the LAN", "192.168.1.7:5000", "192.168.1.20:8484", "203.0.113.5", "kivo.example.com", "192.168.1.20:8484", true},
		{"relayed from the internet", "10.0.0.1:443", "backend:8484", "203.0.113.5", "kivo.example.com", "kivo.example.com", false},
		{"relayed through two proxies", "10.0.0.1:443", "backend:8484", "192.168.1.5, 203.0.113.5, 10.0.0.1", "kivo.example.com", "kivo.example.com", false},
		{"relayed from the LAN", "10.0.0.1:443", "backend:8484", "192.168.1.5", "kivo.lan", "kivo.lan", true},
	} {
		code, info := probe(t, e, id, tc.remote, tc.reached, "X-Forwarded-For", tc.xff, "X-Forwarded-Host", tc.xfh)
		if code != http.StatusOK || info.Proof == nil {
			t.Fatalf("%s: %d", tc.name, code)
		}
		p := info.Proof
		if p.Local != tc.local || p.Host != tc.host || p.Mac != wantMAC(key, probeChallenge, serverID, tc.host, tc.local) {
			t.Errorf("%s: %+v", tc.name, *p)
		}
	}
}

func TestProbeWithoutASession(t *testing.T) {
	e := newEnv(t, serving{})
	_, id := probeOf(t, "kivo_s_"+base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	code, info := probe(t, e, id, "192.168.1.5:5000", "192.168.1.20:8484")
	if code != http.StatusOK || info.Proof != nil || info.ID != e.d.Settings.ServerID() || info.Name == "" {
		t.Errorf("unknown probe ID: %d %+v", code, info)
	}
	// A session's probe ID stops working when the session signs out.
	_, id = probeOf(t, e.adminToken)
	if w := e.do("DELETE", "/api/v1/sessions/current", e.adminToken, nil); w.Code != http.StatusNoContent {
		t.Fatalf("sign out: %d %s", w.Code, w.Body.String())
	}
	if _, info := probe(t, e, id, "192.168.1.5:5000", "192.168.1.20:8484"); info.Proof != nil {
		t.Error("a signed-out session's probe ID still proves")
	}
	for _, q := range []string{"challenge=AAAA&probeId=" + id, "challenge=" + base64.RawURLEncoding.EncodeToString(probeChallenge), "probeId=" + id,
		"challenge=" + base64.RawURLEncoding.EncodeToString(probeChallenge) + "&probeId=AAAA"} {
		if w := e.do("GET", "/api/v1/server?"+q, "", nil); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", q, w.Code)
		}
	}
}
