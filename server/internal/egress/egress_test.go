package egress

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var ctx = context.Background()

func TestPublic(t *testing.T) {
	for _, tc := range []struct {
		addr string
		ok   bool
	}{
		{"93.184.216.34", true},
		{"2606:2800:220:1:248:1893:25c8:1946", true},
		{"127.0.0.1", false},
		{"127.8.9.10", false},
		{"::1", false},
		{"10.1.2.3", false},
		{"172.16.0.1", false},
		{"192.168.1.10", false},
		{"169.254.169.254", false}, // cloud metadata
		{"fe80::1", false},
		{"fc00::1", false},
		{"fd12:3456::1", false},
		{"0.0.0.0", false},
		{"0.1.2.3", false},
		{"::", false},
		{"100.64.0.1", false},
		{"100.100.100.100", false},
		{"192.0.2.1", false},
		{"198.18.0.1", false},
		{"224.0.0.1", false},
		{"255.255.255.255", false},
		{"240.0.0.1", false},
		{"ff02::1", false},
		{"64:ff9b::a00:1", false},  // NAT64 to 10.0.0.1
		{"2002:a00:1::1", false},   // 6to4 from 10.0.0.1
		{"::ffff:10.0.0.1", false}, // checked after unmapping, as control does
	} {
		a := netip.MustParseAddr(tc.addr).Unmap()
		if got := public(a); got != tc.ok {
			t.Errorf("public(%s) = %v, want %v", tc.addr, got, tc.ok)
		}
	}
}

func TestRetryAfter(t *testing.T) {
	r := Response{Header: http.Header{}}
	if r.RetryAfter() != 0 {
		t.Error("no Retry-After")
	}
	r.Header.Set("Retry-After", "120")
	if r.RetryAfter() != 2*time.Minute {
		t.Error("Retry-After in seconds")
	}
	r.Header.Set("Retry-After", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))
	if d := r.RetryAfter(); d < 59*time.Minute || d > time.Hour {
		t.Errorf("Retry-After as a date: %v", d)
	}
}

func TestRefusesPlainHTTP(t *testing.T) {
	c := New()
	for _, u := range []string{"http://example.com/x", "ftp://example.com/x", "file:///etc/passwd", "://bad"} {
		if _, err := c.Get(ctx, u, nil); !errors.Is(err, ErrRefused) {
			t.Errorf("Get(%s) = %v, want ErrRefused", u, err)
		}
	}
}

func TestRefusesLoopbackAndHidesURL(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the request reached the server")
	}))
	defer srv.Close()
	_, err := New().Get(ctx, srv.URL+"/x?api_key=SECRET", nil)
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("got %v, want ErrRefused", err)
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("the error shows the URL: %v", err)
	}
}

// fakeDNS makes c resolve every name to ip, as a hostile or rebinding DNS
// server would, without any network.
func fakeDNS(c *Client, ip string, lookups *atomic.Int32) {
	addr := netip.MustParseAddr(ip)
	c.dialer.Resolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		client, server := net.Pipe()
		go serveDNS(server, addr, lookups)
		return client, nil
	}}
}

// serveDNS answers DNS queries over a stream connection (length-prefixed)
// with one A record, and AAAA queries with nothing.
func serveDNS(conn net.Conn, addr netip.Addr, lookups *atomic.Int32) {
	defer conn.Close()
	for {
		var n uint16
		if binary.Read(conn, binary.BigEndian, &n) != nil {
			return
		}
		q := make([]byte, n)
		if _, err := io.ReadFull(conn, q); err != nil {
			return
		}
		lookups.Add(1)
		end := 12
		for q[end] != 0 {
			end += int(q[end]) + 1
		}
		qtype := binary.BigEndian.Uint16(q[end+1:])
		end += 5
		resp := append([]byte{q[0], q[1], 0x81, 0x80, 0, 1, 0, 0, 0, 0, 0, 0}, q[12:end]...)
		if qtype == 1 {
			resp[7] = 1
			ip := addr.As4()
			resp = append(resp, 0xc0, 12, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4)
			resp = append(resp, ip[:]...)
		}
		_ = binary.Write(conn, binary.BigEndian, uint16(len(resp)))
		if _, err := conn.Write(resp); err != nil {
			return
		}
	}
}

func TestRefusesNameResolvingToPrivateAddress(t *testing.T) {
	// The name looks public. Only the dialled address shows where it goes.
	c := New()
	var lookups atomic.Int32
	fakeDNS(c, "10.20.30.40", &lookups)
	_, err := c.Get(ctx, "https://media.example.com/poster.jpg", nil)
	if !errors.Is(err, ErrRefused) || lookups.Load() == 0 {
		t.Fatalf("got %v after %d lookups, want ErrRefused after a lookup", err, lookups.Load())
	}
}

// testServer returns a TLS server and a client that trusts it. The client
// may dial loopback, and the public internet too, though tests never do.
// It resolves every name to the server.
func testServer(t *testing.T, h http.Handler) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	c := New()
	c.allow = func(a netip.Addr) bool { return a.IsLoopback() || public(a) }
	c.hc.Transport.(*http.Transport).TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig
	var lookups atomic.Int32
	fakeDNS(c, "127.0.0.1", &lookups)
	return srv, c
}

func port(srv *httptest.Server) string {
	return srv.Listener.Addr().(*net.TCPAddr).AddrPort().String()[len("127.0.0.1"):]
}

func TestGet(t *testing.T) {
	srv, c := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		io.WriteString(w, "ok")
	}))
	resp, err := c.Get(ctx, "https://api.example.com"+port(srv)+"/3/movie/1", http.Header{"Authorization": {"Bearer k"}})
	if err != nil || resp.Status != 200 || string(resp.Body) != "ok" {
		t.Fatalf("got %+v, %v", resp, err)
	}
	resp, err = c.Get(ctx, "https://api.example.com"+port(srv)+"/3/movie/1", nil)
	if err != nil || resp.Status != http.StatusUnauthorized {
		t.Fatalf("a status is a response, not an error: got %+v, %v", resp, err)
	}
}

func TestIgnoresProxyEnvironment(t *testing.T) {
	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	var proxied atomic.Int32
	go func() {
		for {
			conn, err := proxy.Accept()
			if err != nil {
				return
			}
			proxied.Add(1)
			conn.Close()
		}
	}()
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "ALL_PROXY"} {
		t.Setenv(k, "http://"+proxy.Addr().String())
	}
	t.Setenv("NO_PROXY", "")

	srv, c := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "direct") }))
	resp, err := c.Get(ctx, "https://api.example.com"+port(srv)+"/", nil)
	if err != nil || string(resp.Body) != "direct" || proxied.Load() != 0 {
		t.Fatalf("got %q, %v; proxy saw %d connections", resp.Body, err, proxied.Load())
	}
}

func TestRedirects(t *testing.T) {
	var srv *httptest.Server
	srv, c := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/to-http":
			http.Redirect(w, r, "http://api.example.com"+port(srv)+"/ok", http.StatusFound)
		case "/to-private":
			http.Redirect(w, r, "https://10.0.0.1/ok", http.StatusFound)
		case "/to-https":
			http.Redirect(w, r, "/ok", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/ok":
			io.WriteString(w, "ok")
		}
	}))
	base := "https://api.example.com" + port(srv)
	if resp, err := c.Get(ctx, base+"/to-https", nil); err != nil || string(resp.Body) != "ok" {
		t.Fatalf("an HTTPS redirect: %q, %v", resp.Body, err)
	}
	for _, path := range []string{"/to-http", "/to-private"} {
		if _, err := c.Get(ctx, base+path, nil); !errors.Is(err, ErrRefused) {
			t.Errorf("%s: got %v, want ErrRefused", path, err)
		}
	}
	if _, err := c.Get(ctx, base+"/loop", nil); err == nil {
		t.Error("a redirect loop succeeded")
	}
}

func TestBodyCap(t *testing.T) {
	big := strings.Repeat("x", 2048)
	srv, c := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chunked" {
			w.(http.Flusher).Flush() // no Content-Length
		}
		io.WriteString(w, big)
	}))
	c.max = 1024
	for _, path := range []string{"/sized", "/chunked"} {
		if _, err := c.Get(ctx, "https://api.example.com"+port(srv)+path, nil); !errors.Is(err, ErrTooLarge) {
			t.Errorf("%s: got %v, want ErrTooLarge", path, err)
		}
	}
	c.max = 2048
	if resp, err := c.Get(ctx, "https://api.example.com"+port(srv)+"/sized", nil); err != nil || len(resp.Body) != 2048 {
		t.Fatalf("a body at the cap: %d bytes, %v", len(resp.Body), err)
	}
}
