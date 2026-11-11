package api

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/config"
)

// signed is the message a proof's signature covers, built the way an app
// builds it, independently of the server's code.
func signed(challenge []byte, serverID, host string, local bool) []byte {
	msg := []byte("kivo-probe-v2")
	for _, f := range [][]byte{challenge, []byte(serverID), []byte(host)} {
		msg = append(msg, byte(len(f)>>8), byte(len(f)))
		msg = append(msg, f...)
	}
	if local {
		return append(msg, 1)
	}
	return append(msg, 0)
}

// serverKey is the server's public key, as the server's settings hold it.
func serverKey(e *env) *ecdsa.PublicKey { return &e.d.Settings.ServerKey().PublicKey }

// verifies reports whether sig is key's signature of msg.
func verifies(key *ecdsa.PublicKey, msg, sig []byte) bool {
	digest := sha256.Sum256(msg)
	return ecdsa.VerifyASN1(key, digest[:], sig)
}

// proves reports whether p is the server's signature over challenge, its
// ID, host and local, by its key, and names that key as an app reads it.
func proves(t *testing.T, e *env, p *gen.ServerProof, challenge []byte, host string, local bool) bool {
	t.Helper()
	sig, err := base64.RawURLEncoding.DecodeString(p.Signature)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(p.Key)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), raw)
	return err == nil && key.Equal(serverKey(e)) && verifies(key, signed(challenge, e.d.Settings.ServerID().String(), host, local), sig)
}

var probeChallenge = []byte("0123456789abcdef0123456789abcdef")

// probe sends the unauthenticated server probe from remote to host.
// headers holds name and value pairs.
func probe(t *testing.T, e *env, remote, host string, headers ...string) (int, gen.ServerInfo) {
	t.Helper()
	q := url.Values{"challenge": {base64.RawURLEncoding.EncodeToString(probeChallenge)}}
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
		code, info := probe(t, e, tc.remote, "192.168.1.20:8484")
		if code != http.StatusOK || info.Proof == nil {
			t.Fatalf("%s: %d, proof %v", tc.remote, code, info.Proof)
		}
		p := info.Proof
		if p.Local != tc.local || p.Host != "192.168.1.20:8484" || !proves(t, e, p, probeChallenge, p.Host, tc.local) {
			t.Errorf("%s: %+v", tc.remote, *p)
		}
		if info.LocalUrls != nil || info.PublicURL != nil {
			t.Errorf("%s: a probe without a token got the addresses", tc.remote)
		}
	}
}

// The signature covers every field: a proof doesn't verify for another
// challenge, server, host or locality.
func TestProbeProofCoversItsClaim(t *testing.T) {
	e := newEnv(t, serving{})
	_, info := probe(t, e, "192.168.1.5:5000", "192.168.1.20:8484")
	sig, err := base64.RawURLEncoding.DecodeString(info.Proof.Signature)
	if err != nil {
		t.Fatal(err)
	}
	id := e.d.Settings.ServerID().String()
	if !verifies(serverKey(e), signed(probeChallenge, id, "192.168.1.20:8484", true), sig) {
		t.Fatal("the proof doesn't verify")
	}
	for name, msg := range map[string][]byte{
		"challenge": signed([]byte("another challenge, sixteen bytes"), id, "192.168.1.20:8484", true),
		"server":    signed(probeChallenge, "another-server", "192.168.1.20:8484", true),
		"host":      signed(probeChallenge, id, "192.168.1.21:8484", true),
		"local":     signed(probeChallenge, id, "192.168.1.20:8484", false),
	} {
		if verifies(serverKey(e), msg, sig) {
			t.Errorf("verifies with another %s", name)
		}
	}
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if verifies(&other.PublicKey, signed(probeChallenge, id, "192.168.1.20:8484", true), sig) {
		t.Error("verifies with another key")
	}
}

// A Tailscale address is remote to both the probe and playback. It becomes
// local to both once it's in the local networks.
func TestProbeAndPlaybackAgree(t *testing.T) {
	e := newEnv(t, serving{})
	const tailscale = "100.101.102.103:5000"
	for _, nets := range [][]string{nil, {"100.64.0.0/10"}} {
		if err := e.d.Settings.Update(context.Background(), func(s *config.Settings) error { s.LocalNetworks = nets; return nil }); err != nil {
			t.Fatal(err)
		}
		local := nets != nil
		if _, info := probe(t, e, tailscale, "kivo.example.com"); info.Proof == nil || info.Proof.Local != local {
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
		code, info := probe(t, e, tc.remote, tc.reached, "X-Forwarded-For", tc.xff, "X-Forwarded-Host", tc.xfh)
		if code != http.StatusOK || info.Proof == nil {
			t.Fatalf("%s: %d", tc.name, code)
		}
		p := info.Proof
		if p.Local != tc.local || p.Host != tc.host || !proves(t, e, p, probeChallenge, tc.host, tc.local) {
			t.Errorf("%s: %+v", tc.name, *p)
		}
	}
}

// The proof needs no session: it still comes once every session is gone,
// so an app whose session was removed hears the server and then its 401.
func TestProbeWithoutASession(t *testing.T) {
	e := newEnv(t, serving{})
	if w := e.do("DELETE", "/api/v1/sessions/current", e.adminToken, nil); w.Code != http.StatusNoContent {
		t.Fatalf("sign out: %d %s", w.Code, w.Body.String())
	}
	if _, info := probe(t, e, "192.168.1.5:5000", "192.168.1.20:8484"); info.Proof == nil || !proves(t, e, info.Proof, probeChallenge, "192.168.1.20:8484", true) {
		t.Errorf("no proof once the session is gone: %+v", info.Proof)
	}
	if w := e.do("GET", "/api/v1/users/me", e.adminToken, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("the removed session's token: %d", w.Code)
	}
	if info := decode[gen.ServerInfo](t, e.do("GET", "/api/v1/server", "", nil)); info.Proof != nil {
		t.Errorf("a proof without a challenge: %+v", info.Proof)
	}
	for _, q := range []string{"challenge=AAAA", "challenge=not+base64url!"} {
		if w := e.do("GET", "/api/v1/server?"+q, "", nil); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", q, w.Code)
		}
	}
}

// An app's sign-in gives it the key the server's proofs are signed by; a
// browser's doesn't.
func TestSignInGivesTheServerKey(t *testing.T) {
	e := newEnv(t, serving{})
	raw, err := serverKey(e).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	want := base64.RawURLEncoding.EncodeToString(raw)
	w := e.do("POST", "/api/v1/sessions", "", map[string]string{"username": "admin", "password": adminPassword, "kind": "app"})
	if s := decode[gen.Session](t, w); w.Code != http.StatusCreated || s.ServerKey == nil || *s.ServerKey != want {
		t.Errorf("app sign-in: %d %+v", w.Code, s)
	}
	w = e.do("POST", "/api/v1/sessions", "", map[string]string{"username": "admin", "password": adminPassword, "kind": "browser"})
	if s := decode[gen.Session](t, w); w.Code != http.StatusCreated || s.ServerKey != nil {
		t.Errorf("browser sign-in: %d %+v", w.Code, s)
	}
}
