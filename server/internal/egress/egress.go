package egress

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// MaxBody is the largest response body Get reads. It is big enough for
// full-size artwork, and small enough that a hostile server can't exhaust
// memory.
const MaxBody = 25 << 20

const (
	dialTimeout   = 10 * time.Second
	tlsTimeout    = 10 * time.Second
	headerTimeout = 20 * time.Second
	totalTimeout  = 2 * time.Minute
	maxRedirects  = 5
)

var (
	// ErrRefused means the request would leave HTTPS or reach an address
	// that isn't on the public internet.
	ErrRefused = errors.New("refused")
	// ErrTooLarge means the response body is over MaxBody.
	ErrTooLarge = errors.New("response too large")
)

// Response is a complete response. Callers interpret the status.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// RetryAfter returns how long the response's Retry-After header asks to
// wait. The header may be in seconds or a date. It is zero without one.
func (r Response) RetryAfter() time.Duration {
	v := r.Header.Get("Retry-After")
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return 0
}

// Client makes outbound requests. It is safe for concurrent use.
type Client struct {
	hc     *http.Client
	dialer *net.Dialer
	max    int64
	allow  func(netip.Addr) bool // public, except in this package's tests
}

// New returns a client.
func New() *Client {
	c := &Client{max: MaxBody, allow: public}
	c.dialer = &net.Dialer{Timeout: dialTimeout, Control: c.control}
	c.hc = &http.Client{
		Transport: &http.Transport{
			Proxy:                  nil, // never from the environment, because a proxy would be dialled instead of the checked address
			DialContext:            c.dialer.DialContext,
			TLSHandshakeTimeout:    tlsTimeout,
			ResponseHeaderTimeout:  headerTimeout,
			MaxResponseHeaderBytes: 1 << 20,
			ForceAttemptHTTP2:      true,
			MaxIdleConnsPerHost:    4,
			IdleConnTimeout:        90 * time.Second,
		},
		Timeout:       totalTimeout,
		CheckRedirect: checkRedirect,
	}
	return c
}

// Get fetches rawURL with the given headers. Any status gives a Response.
// An error means no complete response arrived. Errors name the host but
// never the URL, which may carry a key.
func (c *Client) Get(ctx context.Context, rawURL string, header http.Header) (Response, error) {
	return c.do(ctx, http.MethodGet, rawURL, header, nil)
}

// Post sends body to rawURL. It behaves like Get otherwise.
func (c *Client) Post(ctx context.Context, rawURL string, header http.Header, body []byte) (Response, error) {
	return c.do(ctx, http.MethodPost, rawURL, header, body)
}

// host returns a URL's host for use in errors, which never name the URL.
func host(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "a bad URL"
	}
	return u.Host
}

// do sends a request. Its errors give the method and the host, as in
// "get api.example.com".
func (c *Client) do(ctx context.Context, method, rawURL string, header http.Header, body []byte) (Response, error) {
	op := strings.ToLower(method) + " " + host(rawURL)
	u, err := url.Parse(rawURL)
	if err != nil {
		return Response{}, fmt.Errorf("%w: %s", ErrRefused, op)
	}
	if u.Scheme != "https" {
		return Response{}, fmt.Errorf("%w: %s: not HTTPS", ErrRefused, op)
	}
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), r)
	if err != nil {
		return Response{}, fmt.Errorf("%s: %w", op, err)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Kivo")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // url.Error prints the URL
		}
		return Response{}, fmt.Errorf("%s: %w", op, err)
	}
	defer resp.Body.Close()
	if resp.ContentLength > c.max {
		return Response{}, fmt.Errorf("%w: %s", ErrTooLarge, op)
	}
	out, err := io.ReadAll(io.LimitReader(resp.Body, c.max+1))
	if err != nil {
		return Response{}, fmt.Errorf("%s: %w", op, err)
	}
	if int64(len(out)) > c.max {
		return Response{}, fmt.Errorf("%w: %s", ErrTooLarge, op)
	}
	return Response{Status: resp.StatusCode, Header: resp.Header, Body: out}, nil
}

// control runs on every connection after DNS and before connecting, with
// the address about to be dialled. So neither a DNS answer that changes
// between lookups nor a redirect can slip a private address past it.
func (c *Client) control(network, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: unparseable address %q", ErrRefused, address)
	}
	if !c.allow(ap.Addr().Unmap()) {
		return fmt.Errorf("%w: %s is not a public address", ErrRefused, ap.Addr())
	}
	return nil
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("more than %d redirects", maxRedirects)
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("%w: redirect to %s is not HTTPS", ErrRefused, req.URL.Host)
	}
	return nil
}

// special lists ranges that are neither private nor loopback by Go's
// definitions but still aren't the public internet.
var special = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // this network
	netip.MustParsePrefix("100.64.0.0/10"),   // carrier-grade NAT, and VPN overlays
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // documentation
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // documentation
	netip.MustParsePrefix("203.0.113.0/24"),  // documentation
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved, and broadcast
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64, which can reach private IPv4
	netip.MustParsePrefix("64:ff9b:1::/48"),  // local NAT64
	netip.MustParsePrefix("100::/64"),        // discard
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
	netip.MustParsePrefix("2002::/16"),       // 6to4, which embeds any IPv4
}

// public reports whether a is on the public internet.
func public(a netip.Addr) bool {
	if !a.IsValid() || a.IsLoopback() || a.IsPrivate() || a.IsUnspecified() || a.IsMulticast() ||
		a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || a.IsInterfaceLocalMulticast() {
		return false
	}
	for _, p := range special {
		if p.Contains(a) {
			return false
		}
	}
	return true
}
