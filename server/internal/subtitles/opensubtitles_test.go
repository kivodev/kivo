package subtitles

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/egress"
)

// fakeOpenSubtitles answers requests the way OpenSubtitles does. Its
// searches come from testdata, in the shape the API returns, trimmed to
// the fields Kivo reads. No test touches the network. The egress client's
// own rules are tested in its package.
type fakeOpenSubtitles struct {
	mu     sync.Mutex
	hits   map[string]int // path → requests seen
	status int            // answered to every request instead, when set
	quota  int            // downloads left today
}

func newFakeOpenSubtitles() *fakeOpenSubtitles {
	return &fakeOpenSubtitles{hits: map[string]int{}, quota: 5}
}

func (o *fakeOpenSubtitles) Get(_ context.Context, raw string, h http.Header) (egress.Response, error) {
	return o.answer(raw, h, nil), nil
}

func (o *fakeOpenSubtitles) Post(_ context.Context, raw string, h http.Header, body []byte) (egress.Response, error) {
	return o.answer(raw, h, body), nil
}

func (o *fakeOpenSubtitles) answer(raw string, h http.Header, body []byte) egress.Response {
	u, err := url.Parse(raw)
	if err != nil {
		return egress.Response{Status: http.StatusBadRequest}
	}
	o.mu.Lock()
	o.hits[u.Path]++
	status := o.status
	o.mu.Unlock()
	ok := func(b string) egress.Response {
		return egress.Response{Status: http.StatusOK, Header: http.Header{}, Body: []byte(b)}
	}
	switch {
	case status != 0:
		return egress.Response{Status: status, Header: http.Header{}}
	case u.Host == "dl.opensubtitles.com":
		return ok("1\r\n00:00:01,000 --> 00:00:02,000\r\nThe spice must flow.\r\n")
	case h.Get("Api-Key") != "os-key" || !strings.HasPrefix(h.Get("User-Agent"), "Kivo v"):
		return egress.Response{Status: http.StatusUnauthorized, Header: http.Header{}}
	}
	switch u.Path {
	case "/api/v1/subtitles":
		if u.Query().Get("languages") != "en" {
			return ok(`{"total_count": 0, "data": []}`)
		}
		b, _ := os.ReadFile("testdata/opensubtitles/subtitles.json")
		return ok(string(b))
	case "/api/v1/login":
		if string(body) != `{"password":"pw","username":"anna"}` {
			return egress.Response{Status: http.StatusUnauthorized, Header: http.Header{}, Body: []byte(`{"message": "Error, invalid username/password", "status": 401}`)}
		}
		return ok(`{"user": {"allowed_downloads": 20, "level": "Sub leecher", "vip": false}, "token": "os-token", "status": 200}`)
	case "/api/v1/download":
		if strings.Contains(string(body), "9999") {
			return egress.Response{Status: http.StatusNotFound, Header: http.Header{}}
		}
		o.mu.Lock()
		left := o.quota - 1
		if left >= 0 {
			o.quota = left
		}
		o.mu.Unlock()
		if left < 0 {
			return egress.Response{Status: http.StatusNotAcceptable, Header: http.Header{}, Body: []byte(`{"requests": 6, "remaining": -1, "message": "You have downloaded your allowed 5 subtitles for 24h", "reset_time_utc": "2099-01-01T00:00:00.000Z"}`)}
		}
		return ok(fmt.Sprintf(`{"link": "https://dl.opensubtitles.com/file/1/Dune.srt", "file_name": "Dune.srt", "requests": 1, "remaining": %d, "reset_time_utc": "2099-01-01T00:00:00.000Z"}`, left))
	}
	return egress.Response{Status: http.StatusNotFound, Header: http.Header{}}
}

func (o *fakeOpenSubtitles) set(status int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.status = status
}

func (o *fakeOpenSubtitles) count(path string) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.hits[path]
}

// catalogFile is the fixture's file, in the form search takes.
func (d *downloads) catalogFile() catalog.File {
	d.t.Helper()
	f, err := d.b.Catalog.File(ctx, d.file)
	if err != nil {
		d.t.Fatal(err)
	}
	return f
}

func TestSearchOpenSubtitles(t *testing.T) {
	d := newDownloads(t)
	f := d.catalogFile()
	got, err := d.s.search(ctx, f, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	// The results are in OpenSubtitles' order. Results without a file are
	// left out.
	if want := []string{"7001.en", "7002.en.hi", "7003.en.forced", "7005.en.hi"}; !slices.Equal(resultIDs(got), want) {
		t.Fatalf("results %v, want %v", resultIDs(got), want)
	}
	if !got[1].HashMatch || !got[1].HearingImpaired || got[0].Downloads != 52000 || got[1].Language != "en" {
		t.Fatalf("result %+v", got[1])
	}
	// Searches are cached.
	if _, err := d.s.search(ctx, f, []string{"en-US"}); err != nil || d.remote.count("/api/v1/subtitles") != 1 {
		t.Fatalf("searched again: %v, %d requests", err, d.remote.count("/api/v1/subtitles"))
	}
	if _, err := d.s.search(ctx, f, []string{"not a language!"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a bad language: %v", err)
	}
}

// An expired search is served while OpenSubtitles is down, and the outage
// raises a problem. No request is sent until the backoff passes.
func TestSearchWhileOpenSubtitlesIsDown(t *testing.T) {
	d := newDownloads(t)
	f := d.catalogFile()
	if _, err := d.s.search(ctx, f, []string{"en"}); err != nil {
		t.Fatal(err)
	}
	d.s.openSubtitles.mu.Lock()
	for q, c := range d.s.openSubtitles.searches {
		c.expires = time.Now().Add(-time.Hour)
		d.s.openSubtitles.searches[q] = c
	}
	d.s.openSubtitles.mu.Unlock()
	d.remote.set(http.StatusServiceUnavailable)
	if got, err := d.s.search(ctx, f, []string{"en"}); err != nil || len(got) != 4 || d.remote.count("/api/v1/subtitles") != 2 {
		t.Fatalf("an expired search while down: %v %v, %d requests", resultIDs(got), err, d.remote.count("/api/v1/subtitles"))
	}
	if !slices.Contains(problemCodes(d), "openSubtitlesUnavailable") {
		t.Fatalf("health %v", problemCodes(d))
	}
	if _, err := d.s.search(ctx, f, []string{"de"}); !errors.Is(err, ErrUnavailable) || d.remote.count("/api/v1/subtitles") != 2 {
		t.Fatalf("a new search while resting: %v, %d requests", err, d.remote.count("/api/v1/subtitles"))
	}
}

func resultIDs(rs []SubtitleResult) []string {
	var ids []string
	for _, r := range rs {
		head, _, _ := strings.Cut(r.ID, "~")
		ids = append(ids, head)
	}
	return ids
}

func TestDownloadWithinTheQuota(t *testing.T) {
	d := newDownloads(t)
	d.remote.quota = 1
	got, err := d.s.fetch(ctx, "7002.en.hi")
	if err != nil {
		t.Fatal(err)
	}
	if got.name != "en.7002.srt" || string(got.data) != "1\r\n00:00:01,000 --> 00:00:02,000\r\nThe spice must flow.\r\n" {
		t.Fatalf("downloaded %s: %q", got.name, got.data)
	}
	if r := got.result; r.Language != "en" || !r.HearingImpaired || r.Forced {
		t.Fatalf("result %+v", r)
	}

	// The download said none are left, so OpenSubtitles isn't asked again
	// until the quota resets.
	_, err = d.s.fetch(ctx, "7001.en")
	if q, ok := errors.AsType[*QuotaError](err); !ok || !errors.Is(err, ErrQuota) || q.Reset.Year() != 2099 || d.remote.count("/api/v1/download") != 1 {
		t.Fatalf("over the quota: %v, %d requests", err, d.remote.count("/api/v1/download"))
	}
	// A quota used up elsewhere, such as by another server on the same
	// account, is learned from OpenSubtitles' refusal.
	d.s.openSubtitles.account(d.settings.Get()).setQuota(3, time.Now())
	if _, err := d.s.fetch(ctx, "7001.en"); !errors.Is(err, ErrQuota) || d.remote.count("/api/v1/download") != 2 {
		t.Fatalf("refused by OpenSubtitles: %v, %d requests", err, d.remote.count("/api/v1/download"))
	}
	if _, err := d.s.fetch(ctx, "7001.en"); !errors.Is(err, ErrQuota) || d.remote.count("/api/v1/download") != 2 {
		t.Fatalf("asked again after a refusal: %v, %d requests", err, d.remote.count("/api/v1/download"))
	}
	if _, err := d.s.fetch(ctx, "x.en"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a bad result: %v", err)
	}
}

// A result ID carries its release back, and a download uses it. An ID
// whose release isn't in the form the server writes is refused.
func TestDownloadRelease(t *testing.T) {
	d := newDownloads(t)
	results, err := d.s.search(ctx, d.catalogFile(), []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.s.fetch(ctx, results[0].ID)
	if err != nil || got.result.Release != "Dune.Part.One.2021.1080p.WEB-DL" {
		t.Fatalf("downloaded %+v %v", got.result, err)
	}
	if _, err := d.s.fetch(ctx, "7001.en~!!"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a bad release: %v", err)
	}
}

func TestNoOpenSubtitlesKey(t *testing.T) {
	d := newDownloads(t)
	d.setOpenSubtitles("", "", "")
	if _, err := d.s.search(ctx, d.catalogFile(), nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("search without a key: %v", err)
	}
	if _, err := d.s.fetch(ctx, "7001.en"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("download without a key: %v", err)
	}
	if codes := problemCodes(d); len(codes) != 0 {
		t.Fatalf("health %v", codes)
	}
}

func TestDownloadNoLongerThere(t *testing.T) {
	d := newDownloads(t)
	if _, err := d.s.fetch(ctx, "9999.en"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a result OpenSubtitles no longer has: %v", err)
	}
}

// An account OpenSubtitles refuses raises a problem and isn't tried again
// right away. Changed credentials clear the problem, and are tried at the
// next download.
func TestOpenSubtitlesAccountRejected(t *testing.T) {
	d := newDownloads(t)
	f := d.catalogFile()
	d.setOpenSubtitles("os-key", "anna", "wrong")
	if _, err := d.s.fetch(ctx, "7001.en"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("refused account: %v", err)
	}
	if !slices.Contains(problemCodes(d), "openSubtitlesAccountRejected") {
		t.Fatalf("health %v", problemCodes(d))
	}
	// Searching needs no account, so it keeps working.
	if _, err := d.s.search(ctx, f, nil); err != nil {
		t.Fatalf("search with a refused account: %v", err)
	}
	if _, err := d.s.fetch(ctx, "7001.en"); !errors.Is(err, ErrUnavailable) || d.remote.count("/api/v1/login") != 1 {
		t.Fatalf("asked again at once: %v, %d logins", err, d.remote.count("/api/v1/login"))
	}

	d.setOpenSubtitles("os-key", "anna", "pw")
	if slices.Contains(problemCodes(d), "openSubtitlesAccountRejected") {
		t.Fatalf("still raised after a change: %v", problemCodes(d))
	}
	if _, err := d.s.fetch(ctx, "7001.en"); err != nil {
		t.Fatal(err)
	}
	if len(problemCodes(d)) != 0 || d.remote.count("/api/v1/download") != 1 {
		t.Fatalf("health %v, %d downloads", problemCodes(d), d.remote.count("/api/v1/download"))
	}
}

// A key OpenSubtitles refuses raises a problem. A new key is tried at
// once, and clears the problem when it works.
func TestOpenSubtitlesKeyRejected(t *testing.T) {
	d := newDownloads(t)
	f := d.catalogFile()
	d.setOpenSubtitles("bad-key", "", "")
	if _, err := d.s.search(ctx, f, nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("a refused key: %v", err)
	}
	if !slices.Contains(problemCodes(d), "openSubtitlesKeyRejected") {
		t.Fatalf("health %v", problemCodes(d))
	}
	d.setOpenSubtitles("os-key", "", "")
	if _, err := d.s.search(ctx, f, nil); err != nil {
		t.Fatalf("a new key: %v", err)
	}
	if len(problemCodes(d)) != 0 {
		t.Fatalf("health %v", problemCodes(d))
	}
}

func problemCodes(d *downloads) []string {
	var codes []string
	for _, p := range d.s.Health(ctx) {
		codes = append(codes, p.Code)
	}
	return codes
}

// setOpenSubtitles sets OpenSubtitles' key and account, as an admin would.
func (d *downloads) setOpenSubtitles(key, user, password string) {
	d.t.Helper()
	err := d.settings.Update(ctx, func(s *config.Settings) error {
		s.OpenSubtitlesKey, s.OpenSubtitlesUsername, s.OpenSubtitlesPassword = key, user, password
		return nil
	})
	if err != nil {
		d.t.Fatal(err)
	}
}
