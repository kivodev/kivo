package metadata

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/egress"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/metadata/dbq"
	"github.com/mmcdole/kivo/internal/store"
)

// spacing is the minimum gap between requests to each provider. Each gap
// keeps well under the provider's published limit, since a whole library's
// refresh shares it.
var spacing = map[string]time.Duration{
	provTMDb:     50 * time.Millisecond,
	provAudible:  500 * time.Millisecond,
	provAudnexus: 600 * time.Millisecond,
}

// getter is the egress client. Tests swap in one that targets a fixture
// server.
type getter interface {
	Get(ctx context.Context, url string, header http.Header) (egress.Response, error)
}

// fetcher is the only path to a provider. Each request goes through the
// cache, then the outage check, single-flight, rate limit and egress, and
// finally the cache write. Provider clients make one attempt each and leave
// retrying to their callers.
type fetcher struct {
	db  *store.DB
	q   *dbq.Queries
	get getter

	mu        sync.Mutex
	providers map[string]*egress.Limiter // each provider's pacing and outage state
	calls     map[string]*call
}

// call is one request in flight, shared by every caller asking for it.
type call struct {
	done chan struct{}
	body []byte
	err  error
}

// request is one GET through the pipeline.
type request struct {
	provider string
	url      string
	header   http.Header // carries the API key, so the key stays out of URLs, logs and cache keys
	key      string      // the cache key; empty for artwork, which the image table tracks instead
	ttl      func(body []byte) time.Duration
	fresh    Freshness
}

// outage is an ErrUnavailable that carries when the provider may be asked
// again.
type outage struct {
	provider string
	until    time.Time
	cause    error
}

func (o *outage) Error() string {
	return fmt.Sprintf("%v: %s until %s: %v", ErrUnavailable, o.provider, o.until.Format(time.RFC3339), o.cause)
}

func (o *outage) Is(target error) bool { return target == ErrUnavailable }

func newFetcher(db *store.DB, get getter) *fetcher {
	return &fetcher{db: db, q: dbq.New(db.Reader()), get: get, providers: map[string]*egress.Limiter{}, calls: map[string]*call{}}
}

// refetchAfter is the age past which a refresh asks the provider again
// rather than using a cached response. A younger response is used as it is,
// such as a season that its episodes' refreshes share.
const refetchAfter = day

// refreshing is the freshness a refresh asks of its provider calls. They use
// no cached response older than refetchAfter, and no stale response while
// the provider is down.
var refreshing = Freshness{MaxAge: refetchAfter, Strict: true}

// fetch returns a response's body. Every error it returns, including an
// outage or ErrNotFound, names the request, as in "fetch tmdb /movie/603:
// not found". A refresh's stored failure keeps that text.
func (f *fetcher) fetch(ctx context.Context, r request) ([]byte, error) {
	body, err := f.answer(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("fetch %s %s: %w", r.provider, r.name(), err)
	}
	return body, nil
}

// name describes what a request asks for. It is the cache key, which leaves
// out any API key, or the URL for artwork.
func (r request) name() string { return cmp.Or(r.key, r.url) }

// answer does fetch's work. It returns the cache's answer or the
// provider's.
func (f *fetcher) answer(ctx context.Context, r request) ([]byte, error) {
	var stale *dbq.GetCachedRow
	if r.key != "" {
		c, err := f.q.GetCached(ctx, dbq.GetCachedParams{Provider: r.provider, Key: r.key})
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return nil, err
		case fresh(c, time.Now(), r.fresh.MaxAge):
			return cachedResult(c)
		case !r.fresh.Strict:
			stale = &c
		}
	}
	if err := f.down(r.provider); err != nil {
		if stale != nil {
			return cachedResult(*stale) // an expired answer beats none while the provider is down
		}
		return nil, err
	}
	body, err := f.once(ctx, r.provider+" "+r.url, func() ([]byte, error) { return f.remote(ctx, r) })
	if errors.Is(err, ErrUnavailable) && stale != nil {
		return cachedResult(*stale)
	}
	return body, err
}

func fresh(c dbq.GetCachedRow, now time.Time, maxAge time.Duration) bool {
	if maxAge > 0 && now.Sub(time.UnixMilli(c.FetchedAt)) > maxAge {
		return false
	}
	return now.UnixMilli() < c.ExpiresAt
}

// cachedResult returns a cache entry's answer. An empty body is a cached
// not-found. The driver may read back a stored nil as empty, and no useful
// response is empty.
func cachedResult(c dbq.GetCachedRow) ([]byte, error) {
	if len(c.Body) == 0 {
		return nil, ErrNotFound
	}
	return unpack(c.Body)
}

// pack gzips a response for the cache. A movie with its credits, images and
// translations is around 100 KB of JSON, and a tenth of that compressed.
func pack(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var b bytes.Buffer
	w, _ := gzip.NewWriterLevel(&b, gzip.BestSpeed)
	_, _ = w.Write(body)
	_ = w.Close()
	return b.Bytes()
}

// unpack reverses pack. Entries cached before packing are plain JSON.
func unpack(body []byte) ([]byte, error) {
	if len(body) < 2 || body[0] != 0x1f || body[1] != 0x8b {
		return body, nil
	}
	r, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

// once runs fn for key, unless a call for key is already in flight. In that
// case it waits for that call's result.
func (f *fetcher) once(ctx context.Context, key string, fn func() ([]byte, error)) ([]byte, error) {
	f.mu.Lock()
	if c := f.calls[key]; c != nil {
		f.mu.Unlock()
		select {
		case <-c.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if (errors.Is(c.err, context.Canceled) || errors.Is(c.err, context.DeadlineExceeded)) && ctx.Err() == nil {
			return f.once(ctx, key, fn) // the first caller gave up, not the provider, so try again
		}
		return c.body, c.err
	}
	c := &call{done: make(chan struct{})}
	f.calls[key] = c
	f.mu.Unlock()

	c.body, c.err = fn()
	f.mu.Lock()
	delete(f.calls, key)
	f.mu.Unlock()
	close(c.done)
	return c.body, c.err
}

func (f *fetcher) remote(ctx context.Context, r request) ([]byte, error) {
	p := f.provider(r.provider)
	if err := p.Wait(ctx); err != nil {
		return nil, err
	}
	resp, err := f.get.Get(ctx, r.url, r.header)
	switch {
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case errors.Is(err, egress.ErrRefused) || errors.Is(err, egress.ErrTooLarge):
		return nil, err // this response is the problem, not the provider
	case err != nil:
		return nil, failed(r.provider, p, 0, err)
	}

	switch {
	case resp.Status == http.StatusOK:
		p.Recovered()
		if r.key != "" {
			f.store(ctx, r, resp.Body, r.ttl(resp.Body))
		}
		return resp.Body, nil
	case resp.Status == http.StatusNotFound:
		p.Recovered()
		if r.key != "" {
			f.store(ctx, r, nil, ttlNotFound)
		}
		return nil, ErrNotFound
	case resp.Status == http.StatusUnauthorized:
		return nil, reject(r.provider, p)
	case resp.Status == http.StatusTooManyRequests || resp.Status >= 500:
		return nil, failed(r.provider, p, resp.RetryAfter(), fmt.Errorf("status %d", resp.Status))
	}
	return nil, fmt.Errorf("answered %d", resp.Status)
}

// store writes a response to the cache. A failure only costs a refetch, so
// it is logged rather than returned.
func (f *fetcher) store(ctx context.Context, r request, body []byte, ttl time.Duration) {
	now := time.Now()
	err := f.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return dbq.New(tx).PutCached(ctx, dbq.PutCachedParams{
			Provider: r.provider, Key: r.key, Body: pack(body), FetchedAt: now.UnixMilli(), ExpiresAt: now.Add(ttl).UnixMilli(),
		})
	})
	if err != nil {
		slog.WarnContext(ctx, "metadata: write cache", "provider", r.provider, "err", err)
	}
}

func (f *fetcher) provider(name string) *egress.Limiter {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.providers[name]
	if p == nil {
		every, ok := spacing[name]
		if !ok {
			every = 100 * time.Millisecond
		}
		p = &egress.Limiter{Every: every}
		f.providers[name] = p
	}
	return p
}

// down returns an outage error while the provider is resting. Once the
// rest is over, the next request serves as the trial.
func (f *fetcher) down(name string) error {
	o := f.provider(name).Outage()
	if !time.Now().Before(o.Until) {
		return nil
	}
	cause := errors.New("resting after failures")
	if o.Rejected {
		cause = errors.New("the API key was rejected")
	}
	return &outage{provider: name, until: o.Until, cause: cause}
}

// failed records an outage. Requests stop until a backoff, or the
// provider's Retry-After, has passed.
func failed(name string, p *egress.Limiter, after time.Duration, cause error) error {
	o := p.Failed(time.Now(), after)
	slog.Warn("metadata: provider unavailable", "provider", name, "until", o.Until, "err", cause)
	return &outage{provider: name, until: o.Until, cause: cause}
}

// reject pauses a provider that refused our key, and raises a problem.
func reject(name string, p *egress.Limiter) error {
	o := p.Rejected(time.Now())
	slog.Error("metadata: API key rejected", "provider", name)
	return &outage{provider: name, until: o.Until, cause: errors.New("the API key was rejected")}
}

var displayNames = map[string]string{
	provTMDb: "TMDb", provAnimeLists: "Anime-Lists",
	provAudible: "Audible", provAudnexus: "Audnexus",
	provYouTube: "YouTube",
}

// health lists providers that are down or have refused our key.
func (f *fetcher) health() []health.Problem {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []health.Problem
	for prov, p := range f.providers {
		o, name := p.Outage(), displayNames[prov]
		switch {
		case o.Rejected:
			out = append(out, health.Problem{
				Component: "metadata", Code: prov + "KeyRejected", Severity: health.Error, RaisedAt: o.Since,
				Message: name + " rejected the API key. Metadata and artwork updates from " + name + " are paused.",
			})
		case o.Failures > 0:
			out = append(out, health.Problem{
				Component: "metadata", Code: prov + "Unavailable", Severity: health.Warning, RaisedAt: o.Since,
				Message: name + " isn't responding. Metadata updates from " + name + " are paused.",
			})
		}
	}
	return out
}
