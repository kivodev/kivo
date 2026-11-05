package metadata

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/health"
)

const searchPath = "/3/search/movie"

func TestCacheServesRepeats(t *testing.T) {
	e := newEnv(t)
	for range 3 {
		if _, err := e.s.SearchMovies(ctx, Query{Title: "Dune"}); err != nil {
			t.Fatal(err)
		}
	}
	if n := e.fx.count(searchPath); n != 1 {
		t.Fatalf("%d requests for one search", n)
	}
	// A different query is a different entry.
	if _, err := e.s.SearchMovies(ctx, Query{Title: "Dune", Year: 1984}); err != nil {
		t.Fatal(err)
	}
	if n := e.fx.count(searchPath); n != 2 {
		t.Fatalf("%d requests for two searches", n)
	}
}

func TestCacheKeepsNotFound(t *testing.T) {
	e := newEnv(t)
	for range 2 {
		if _, err := e.s.Resolve(ctx, catalogID("tmdb", "movie", "999"), ""); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
	}
	if n := e.fx.count("/3/movie/999"); n != 1 {
		t.Fatalf("%d requests for a cached not-found", n)
	}
}

func TestCacheLifetimes(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		date string
		want time.Duration
	}{
		{"2026-03-01", ttlRecent}, {"2027-01-01", ttlRecent}, {"", ttlRecent}, {"2019-05-01", ttlOlder},
	} {
		if got := movieTTL(tc.date, now); got != tc.want {
			t.Errorf("movieTTL(%q) = %v, want %v", tc.date, got, tc.want)
		}
	}
	if seriesTTL(false) != day || seriesTTL(true) != 30*day {
		t.Error("series lifetimes")
	}

	// What the pipeline records for each response.
	e := newEnv(t)
	if _, err := e.s.SearchMovies(ctx, Query{Title: "Dune"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Episodes(ctx, catalogID("tmdb", "series", "63639"), Aired, "", 0); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]time.Duration{
		"/search/movie?include_adult=false&language=en&query=Dune": ttlSearch,
		"/tv/63639/season/1?language=en":                           ttlEnded, // The Expanse has ended
	} {
		var fetched, expires int64
		err := e.b.DB.Reader().QueryRow(`SELECT fetched_at, expires_at FROM provider_cache WHERE provider = 'tmdb' AND key = ?`, key).
			Scan(&fetched, &expires)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if got := time.Duration(expires-fetched) * time.Millisecond; got != want {
			t.Errorf("%s: cached for %v, want %v", key, got, want)
		}
	}
}

func TestSingleFlight(t *testing.T) {
	e := newEnv(t)
	release := make(chan struct{})
	var once sync.Once
	arrived := make(chan struct{})
	e.fx.hook = func(path string) {
		once.Do(func() { close(arrived) })
		<-release
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			_, err := e.s.SearchMovies(ctx, Query{Title: "Dune"})
			errs <- err
		})
	}
	<-arrived
	time.Sleep(50 * time.Millisecond) // the others reach the flight
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := e.fx.count(searchPath); n != 1 {
		t.Fatalf("%d requests for 8 concurrent identical searches", n)
	}
}

func TestOutageServesStaleThenFails(t *testing.T) {
	e := newEnv(t)
	if _, err := e.s.SearchMovies(ctx, Query{Title: "Dune"}); err != nil {
		t.Fatal(err)
	}
	expireCache(t, e)
	e.fx.set(searchPath, http.StatusServiceUnavailable)

	got, err := e.s.SearchMovies(ctx, Query{Title: "Dune"})
	if err != nil || len(got) != 3 {
		t.Fatalf("expired entry during an outage: %v, %v", got, err)
	}
	if n := e.fx.count(searchPath); n != 2 {
		t.Fatalf("%d requests, want the cache, one failed try, then the stale entry", n)
	}
	// With nothing cached, the outage is the answer, and the provider rests.
	_, err = e.s.SearchSeries(ctx, Query{Title: "Firefly"})
	var o *outage
	if !errors.Is(err, ErrUnavailable) || !errors.As(err, &o) || !o.until.After(time.Now()) {
		t.Fatalf("got %v, want an outage", err)
	}
	if n := e.fx.count("/3/search/tv"); n != 0 {
		t.Fatalf("%d requests to a resting provider", n)
	}
	if p := e.s.Health(ctx); len(p) != 1 || p[0].Code != "tmdbUnavailable" || p[0].Severity != health.Warning {
		t.Fatalf("health: %+v", p)
	}

	// Once the rest is over, a success clears the problem.
	e.fx.set(searchPath, 0)
	e.s.fetcher.provider("tmdb").Failed(time.Now().Add(-time.Hour), 0) // a rest that's over
	if _, err := e.s.SearchMovies(ctx, Query{Title: "Dune"}); err != nil {
		t.Fatal(err)
	}
	if p := e.s.Health(ctx); len(p) != 0 {
		t.Fatalf("health after recovery: %+v", p)
	}
}

func TestRejectedKeyPausesProvider(t *testing.T) {
	e := newEnv(t)
	e.s.video = newTMDb(e.s.fetcher, "revoked")
	_, err := e.s.SearchMovies(ctx, Query{Title: "Dune"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v, want ErrUnavailable", err)
	}
	p := e.s.Health(ctx)
	if len(p) != 1 || p[0].Code != "tmdbKeyRejected" || p[0].Severity != health.Error {
		t.Fatalf("health: %+v", p)
	}
	_, err = e.s.Resolve(ctx, catalogID("tmdb", "movie", "438631"), "")
	var o *outage
	if !errors.As(err, &o) || time.Until(o.until) < 50*time.Minute {
		t.Fatalf("got %v, want a pause of about an hour", err)
	}
	if n := e.fx.count("/3/movie/438631"); n != 0 {
		t.Fatalf("%d requests while paused", n)
	}
}

func TestRetryAfter(t *testing.T) {
	e := newEnv(t)
	e.fx.set(searchPath, http.StatusTooManyRequests)
	// The fixture sends no Retry-After, so the first backoff applies.
	_, err := e.s.SearchMovies(ctx, Query{Title: "Dune"})
	var o *outage
	if !errors.As(err, &o) {
		t.Fatalf("got %v", err)
	}
	if d := time.Until(o.until); d < 25*time.Second || d > 30*time.Second {
		t.Fatalf("rest of %v", d)
	}
}

func TestPruneKeepsRecentlyExpired(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	for key, expires := range map[string]time.Time{
		"fresh": now.Add(time.Hour), "expired": now.Add(-day), "ancient": now.Add(-keepStale - day),
	} {
		e.s.fetcher.store(ctx, request{provider: "tmdb", key: key}, []byte("{}"), time.Until(expires))
	}
	n, err := e.s.pruneProviderCache(ctx)
	if err != nil || n != 1 {
		t.Fatalf("pruned %d, %v", n, err)
	}
	var left int
	e.b.DB.Reader().QueryRow(`SELECT count(*) FROM provider_cache`).Scan(&left)
	if left != 2 {
		t.Fatalf("%d entries left", left)
	}
}

func expireCache(t *testing.T, e *env) {
	t.Helper()
	err := e.b.DB.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE provider_cache SET expires_at = 0`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
