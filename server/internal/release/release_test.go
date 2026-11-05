package release

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mmcdole/kivo/internal/egress"
)

// github answers the latest release with body, recording each request.
type github struct {
	mu      sync.Mutex
	body    string
	status  int
	err     error
	asked   int
	headers []http.Header
}

func (g *github) Get(_ context.Context, url string, h http.Header) (egress.Response, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.asked++
	g.headers = append(g.headers, h)
	if url != latestURL {
		return egress.Response{Status: http.StatusNotFound}, nil
	}
	if g.err != nil {
		return egress.Response{}, g.err
	}
	status := g.status
	if status == 0 {
		status = http.StatusOK
	}
	return egress.Response{Status: status, Body: []byte(g.body)}, nil
}

func (g *github) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.asked
}

func checker(opt Options, g *github) *Checker {
	c := New(opt)
	c.get = g
	return c
}

const latest = `{"tag_name": "v0.5.0", "html_url": "https://example.com/elsewhere", "draft": false, "prerelease": false}`

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"v0.5.0", "0.4.0", true},
		{"0.4.1", "0.4.0", true},
		{"1.0.0", "0.10.9", true},
		{"0.10.0", "0.9.0", true},
		{"v0.4.0", "0.4.0", false},
		{"0.4.0", "0.5.0", false},
		{"v0.5.0", "v0.4.0-3-gabc1234-dirty", true},
		{"v0.4.0", "v0.4.0-3-gabc1234", false},
		{"v0.5.0", "dev", false},
		{"v0.5.0", "d76b7e0", false},
		{"latest", "0.4.0", false},
		{"0.5", "0.4.0", false},
	} {
		if got := Newer(tc.a, tc.b); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCheck(t *testing.T) {
	g := &github{body: latest}
	c := checker(Options{Version: "0.4.0", Check: true}, g)
	if _, ok := c.Newer(); ok {
		t.Fatal("newer before any check")
	}
	if err := c.check(t.Context()); err != nil {
		t.Fatal(err)
	}
	// The link is made from the tag, whatever html_url says.
	want := Release{Version: "0.5.0", URL: "https://github.com/kivodev/kivo/releases/tag/v0.5.0"}
	if got, ok := c.Newer(); !ok || got != want {
		t.Fatalf("Newer() = %+v, %v, want %+v", got, ok, want)
	}
	// Only GitHub's own Accept header goes out; egress sets the generic
	// User-Agent.
	if h := g.headers[0]; len(h) != 1 || h.Get("Accept") != "application/vnd.github+json" {
		t.Errorf("headers = %v", h)
	}
}

func TestCheckKeepsNothing(t *testing.T) {
	for name, g := range map[string]*github{
		"same version": {body: `{"tag_name": "v0.4.0"}`},
		"older":        {body: `{"tag_name": "v0.3.9"}`},
		"draft":        {body: `{"tag_name": "v0.5.0", "draft": true}`},
		"prerelease":   {body: `{"tag_name": "v0.5.0", "prerelease": true}`},
		"not a tag":    {body: `{"tag_name": "nightly"}`},
	} {
		c := checker(Options{Version: "0.4.0", Check: true}, g)
		if err := c.check(t.Context()); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if r, ok := c.Newer(); ok {
			t.Errorf("%s: newer %+v", name, r)
		}
	}
}

func TestCheckFails(t *testing.T) {
	for name, g := range map[string]*github{
		"unreachable":  {err: errors.New("dial: no route")},
		"rate limited": {status: http.StatusForbidden},
		"no releases":  {status: http.StatusNotFound},
		"garbled":      {body: "<html>"},
	} {
		c := checker(Options{Version: "0.4.0", Check: true}, g)
		if err := c.check(t.Context()); err == nil {
			t.Errorf("%s: no error", name)
		}
		if _, ok := c.Newer(); ok {
			t.Errorf("%s: newer", name)
		}
	}
}

// TestRun checks a minute after startup and then daily.
func TestRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := &github{body: latest}
		c := checker(Options{Version: "0.4.0", Check: true}, g)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error)
		go func() { done <- c.Run(ctx) }()
		time.Sleep(firstCheck - time.Second)
		synctest.Wait()
		if n := g.count(); n != 0 {
			t.Fatalf("asked %d times before the first check", n)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if _, ok := c.Newer(); !ok || g.count() != 1 {
			t.Fatalf("after the first check: asked %d times, newer %v", g.count(), ok)
		}
		time.Sleep(checkEvery)
		synctest.Wait()
		if n := g.count(); n != 2 {
			t.Fatalf("asked %d times after a day, want 2", n)
		}
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}

// TestRunSkips checks that Run returns at once, without asking, for a
// build that isn't a release and when the check is off.
func TestRunSkips(t *testing.T) {
	for name, opt := range map[string]Options{
		"dev":    {Version: "dev", Check: true},
		"commit": {Version: "d76b7e0", Check: true},
		"off":    {Version: "0.4.0", Check: false},
	} {
		g := &github{body: latest}
		if err := checker(opt, g).Run(t.Context()); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if n := g.count(); n != 0 {
			t.Errorf("%s: asked %d times", name, n)
		}
	}
}
