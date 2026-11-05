package metadata

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/egress"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/health"
)

func TestV3KeyIsScrubbedFromErrors(t *testing.T) {
	const k = "0123456789abcdef0123456789abcdef"
	if !v3Key(k) || v3Key("eyJhbGciOiJIUzI1NiJ9.x.y") {
		t.Fatal("v3Key")
	}
	err := scrub(errors.New(`Get "https://api.themoviedb.org/3/x?api_key=`+k+`": dial tcp: refused`), k)
	if strings.Contains(err.Error(), k) {
		t.Fatalf("key left in %q", err)
	}
}

// A v3 key travels in the URL, so a failed fetch's error could carry it into
// the log, and from there onto the Logs page and its download. The egress
// client's errors name only the host, and tmdb scrubs the key from the
// errors it returns. So neither shows the key when logged.
func TestV3KeyStaysOutOfTheLog(t *testing.T) {
	const k = "0123456789abcdef0123456789abcdef"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	_, fetchErr := egress.New().Get(ctx, srv.URL+"/3/movie/1?api_key="+k, nil)
	if fetchErr == nil {
		t.Fatal("egress fetched from loopback")
	}
	tmdbErr := scrub(errors.New(`Get "https://api.themoviedb.org/3/movie/1?api_key=`+k+`": dial tcp: refused`), k)

	var out bytes.Buffer
	logs := health.NewLogs(10, events.NewBus())
	log := slog.New(logs.Handler(slog.NewTextHandler(&out, nil)))
	log.Warn("metadata: provider unavailable", "provider", "tmdb", "err", fetchErr)
	log.Warn("identify: match failed", "err", tmdbErr)

	var page strings.Builder
	lines := logs.Lines(health.LogFilter{})
	if err := health.WriteText(&page, lines); err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		for _, a := range l.Attrs {
			page.WriteString(a.Value.String())
		}
	}
	if len(lines) != 2 || strings.Contains(out.String(), k) || strings.Contains(page.String(), k) {
		t.Errorf("the key reached the log:\n%s\n%s", out.String(), page.String())
	}
}
