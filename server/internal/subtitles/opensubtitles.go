package subtitles

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/text/language"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/egress"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/media"
)

const (
	openSubtitlesAPI = "https://api.opensubtitles.com/api/v1"
	// openSubtitlesAgent names Kivo to OpenSubtitles, which requires an
	// application name and version.
	openSubtitlesAgent = "Kivo v1"

	day            = 24 * time.Hour
	ttlSearch      = day     // how long a search that found subtitles is cached
	ttlNoSubtitles = 7 * day // a search that found nothing isn't tried again for a week
	ttlNotFound    = day     // how long a search OpenSubtitles answered with not found is cached
	keepStale      = 30 * day

	spacing = 250 * time.Millisecond // the minimum time between requests

	loginRetry = time.Hour // a rejected account is tried again this often, in case it was fixed
)

var (
	// ErrNotConfigured means OpenSubtitles has no key, so it's unavailable
	// until an admin sets one.
	ErrNotConfigured = fault.Coded(fault.Unavailable, "notConfigured", "provider not configured")
	// ErrUnavailable means OpenSubtitles is down, is limiting our requests,
	// or rejected our key or account, and no cached search can stand in.
	// Try again later.
	ErrUnavailable = fault.New(fault.Unavailable, "provider unavailable")
	// ErrQuota means OpenSubtitles' downloads for the day are used up. A
	// *QuotaError says until when.
	ErrQuota = fault.Coded(fault.Unavailable, "quotaUsed", "download quota used up")
	// ErrNotFound means a subtitle language isn't valid, or a subtitle
	// result ID wasn't given by SearchSubtitles or is no longer on
	// OpenSubtitles.
	ErrNotFound = fault.New(fault.NotFound, "not found")
	// errLoginRejected means OpenSubtitles refused the account's username or
	// password.
	errLoginRejected = fmt.Errorf("%w: OpenSubtitles rejected the account", ErrUnavailable)
)

// QuotaError is ErrQuota with the time the quota resets. The time is zero
// when OpenSubtitles didn't say.
type QuotaError struct {
	Reset time.Time
}

func (e *QuotaError) Error() string {
	if e.Reset.IsZero() {
		return ErrQuota.Error()
	}
	return ErrQuota.Error() + " until " + e.Reset.UTC().Format(time.RFC3339)
}

func (e *QuotaError) Unwrap() error { return ErrQuota }

// getter is the egress client. Tests swap in a fake that answers the way
// OpenSubtitles does.
type getter interface {
	Get(ctx context.Context, url string, header http.Header) (egress.Response, error)
	Post(ctx context.Context, url string, header http.Header, body []byte) (egress.Response, error)
}

// openSubtitles is the server's client for the OpenSubtitles REST API. It
// spaces requests apart, sends none while it backs off after failures or a
// refused key, and caches searches. Downloads need a key. More than a
// handful of downloads a day also need an account to sign in with.
type openSubtitles struct {
	get   getter
	limit *egress.Limiter // paces requests and tracks outages

	mu       sync.Mutex
	acct     *account                // the account for the settings' credentials
	searches map[string]cachedSearch // keyed by query
}

// cachedSearch is a search's results. They're kept for keepStale after
// they expire, so they can be served while OpenSubtitles is down.
type cachedSearch struct {
	results []SubtitleResult
	expires time.Time
}

// account holds the credentials from the settings, the sign-in token, and
// the download quota OpenSubtitles last reported for them.
type account struct {
	key, user, password string

	mu            sync.Mutex
	token         string    // the token from signing in, when there's an account
	remaining     int       // downloads left today; -1 until OpenSubtitles reports it
	reset         time.Time // when the quota resets
	loginRejected time.Time // when OpenSubtitles last refused the account; zero once it accepts it
}

func newOpenSubtitles() *openSubtitles {
	return &openSubtitles{get: egress.New(), limit: &egress.Limiter{Every: spacing}, searches: map[string]cachedSearch{}}
}

// account returns the account for the credentials in st, or nil if there's
// no key. Changed credentials get a new account, which signs in again. A
// new key is tried at once, even if the previous key was refused.
func (o *openSubtitles) account(st config.Settings) *account {
	if st.OpenSubtitlesKey == "" {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if a := o.acct; a == nil || a.key != st.OpenSubtitlesKey || a.user != st.OpenSubtitlesUsername || a.password != st.OpenSubtitlesPassword {
		if a != nil && a.key != st.OpenSubtitlesKey {
			o.limit.Recovered()
		}
		o.acct = &account{key: st.OpenSubtitlesKey, user: st.OpenSubtitlesUsername, password: st.OpenSubtitlesPassword, remaining: -1}
	}
	return o.acct
}

func (a *account) header() http.Header {
	return http.Header{"Api-Key": {a.key}, "User-Agent": {openSubtitlesAgent}, "Accept": {"application/json"}}
}

// SubtitleResult is a subtitle OpenSubtitles offers for a file.
type SubtitleResult struct {
	ID              string // opaque; pass it to DownloadSubtitle
	Language        string // a BCP 47 tag
	Release         string // the release the subtitle was made for
	HearingImpaired bool
	Forced          bool // covers only the foreign-language parts
	Downloads       int
	HashMatch       bool // made for this exact file
}

// search searches OpenSubtitles for subtitles for a file, by its hash and
// its work's IDs. It searches in languages (BCP 47 tags), or in the
// library's metadata language when languages is empty. Without an
// OpenSubtitles key it returns ErrNotConfigured. An invalid language
// returns ErrNotFound.
func (s *Service) search(ctx context.Context, f catalog.File, languages []string) ([]SubtitleResult, error) {
	st := s.deps.Settings.Get()
	a := s.openSubtitles.account(st)
	if a == nil {
		return nil, fmt.Errorf("%w: OpenSubtitles", ErrNotConfigured)
	}
	params, err := s.subtitleQuery(ctx, f)
	if err != nil {
		return nil, err
	}
	var langs []string
	for _, l := range languages {
		tag, err := language.Parse(l)
		if err != nil {
			return nil, fmt.Errorf("%w: subtitle language %q", ErrNotFound, l)
		}
		if l := openSubtitlesLanguage(tag); l != "" {
			langs = append(langs, l)
		}
	}
	if len(langs) == 0 {
		lib, err := s.deps.Catalog.Library(ctx, f.Library)
		if err != nil {
			return nil, err
		}
		langs = []string{media.BaseLanguage(cmp.Or(lib.MetadataLanguage, st.MetadataLanguage))}
	}
	slices.Sort(langs)
	params.Set("languages", strings.Join(slices.Compact(langs), ","))
	return s.openSubtitles.search(ctx, a, params)
}

// openSubtitlesRegions are the regional languages OpenSubtitles
// distinguishes. Any other region searches its base language.
var openSubtitlesRegions = []string{"pt-br", "pt-pt", "zh-cn", "zh-tw"}

// openSubtitlesLanguage returns the language code OpenSubtitles uses for a
// tag.
func openSubtitlesLanguage(tag language.Tag) string {
	if t := strings.ToLower(tag.String()); slices.Contains(openSubtitlesRegions, t) {
		return t
	}
	return media.BaseLanguage(tag.String())
}

// subtitleQuery describes a file to OpenSubtitles. It sends the file's
// hash, plus one of these: the movie's TMDb or IMDb ID, the series' TMDb
// ID with season and episode, or, failing those, the title.
func (s *Service) subtitleQuery(ctx context.Context, f catalog.File) (url.Values, error) {
	v, err := s.deps.Catalog.VersionOfFile(ctx, f.ID)
	if err != nil {
		return nil, err
	}
	w, err := s.deps.Catalog.Work(ctx, v.Works[0])
	if err != nil {
		return nil, err
	}
	p := url.Values{}
	if f.Probe.OSHash != 0 {
		p.Set("moviehash", fmt.Sprintf("%016x", f.Probe.OSHash))
	}
	ext := func(w catalog.Work, provider string) string {
		for _, x := range w.ExternalIDs {
			if x.Provider == provider {
				return x.ID
			}
		}
		return ""
	}
	switch {
	case w.Kind == catalog.KindEpisode && !w.Root.IsZero():
		series, err := s.deps.Catalog.Work(ctx, w.Root)
		if err != nil {
			return nil, err
		}
		if id := ext(series, "tmdb"); id != "" {
			p.Set("parent_tmdb_id", id)
			p.Set("season_number", strconv.Itoa(w.ParentNumber))
			p.Set("episode_number", strconv.Itoa(w.Number))
			return p, nil
		}
	case ext(w, "tmdb") != "":
		p.Set("tmdb_id", ext(w, "tmdb"))
		return p, nil
	case ext(w, "imdb") != "":
		p.Set("imdb_id", strings.TrimPrefix(ext(w, "imdb"), "tt"))
		return p, nil
	}
	p.Set("query", strings.ToLower(w.Title))
	return p, nil
}

type osSearch struct {
	Data []struct {
		Attributes struct {
			Language         string `json:"language"`
			DownloadCount    int    `json:"download_count"`
			HearingImpaired  bool   `json:"hearing_impaired"`
			ForeignPartsOnly bool   `json:"foreign_parts_only"`
			Release          string `json:"release"`
			MoviehashMatch   bool   `json:"moviehash_match"`
			Files            []struct {
				FileID int `json:"file_id"`
			} `json:"files"`
		} `json:"attributes"`
	} `json:"data"`
}

// search runs one search and caches it, for a week when it found nothing
// and for a day otherwise. Parameters go in alphabetical order, as
// OpenSubtitles asks, so equal searches share a cache entry. An expired
// search is still served while OpenSubtitles is down.
func (o *openSubtitles) search(ctx context.Context, a *account, params url.Values) ([]SubtitleResult, error) {
	q := params.Encode()
	o.mu.Lock()
	c, cached := o.searches[q]
	o.mu.Unlock()
	if cached && time.Now().Before(c.expires) {
		return slices.Clone(c.results), nil
	}
	results, ttl, err := o.searchRemote(ctx, a, q)
	if errors.Is(err, ErrUnavailable) && cached {
		return slices.Clone(c.results), nil // while OpenSubtitles is down, an expired answer is better than none
	}
	if err != nil {
		return nil, err
	}
	now := time.Now()
	o.mu.Lock()
	for k, c := range o.searches {
		if now.Sub(c.expires) > keepStale {
			delete(o.searches, k)
		}
	}
	o.searches[q] = cachedSearch{results: results, expires: now.Add(ttl)}
	o.mu.Unlock()
	return slices.Clone(results), nil
}

// searchRemote asks OpenSubtitles for a search, and returns its results
// and how long to cache them.
func (o *openSubtitles) searchRemote(ctx context.Context, a *account, q string) ([]SubtitleResult, time.Duration, error) {
	if err := o.down(); err != nil {
		return nil, 0, err
	}
	if err := o.limit.Wait(ctx); err != nil {
		return nil, 0, err
	}
	resp, err := o.get.Get(ctx, openSubtitlesAPI+"/subtitles?"+q, a.header())
	switch {
	case ctx.Err() != nil:
		return nil, 0, ctx.Err()
	case errors.Is(err, egress.ErrRefused) || errors.Is(err, egress.ErrTooLarge):
		return nil, 0, err // the problem is this response, not OpenSubtitles as a whole
	case err != nil:
		return nil, 0, o.failed(0, err)
	}
	switch {
	case resp.Status == http.StatusOK:
		o.limit.Recovered()
		return searchResults(resp.Body)
	case resp.Status == http.StatusNotFound:
		o.limit.Recovered()
		return nil, ttlNotFound, nil
	case resp.Status == http.StatusUnauthorized:
		return nil, 0, o.reject()
	case resp.Status == http.StatusTooManyRequests || resp.Status >= 500:
		return nil, 0, o.failed(resp.RetryAfter(), fmt.Errorf("status %d", resp.Status))
	}
	return nil, 0, fmt.Errorf("the OpenSubtitles search answered %d", resp.Status)
}

// searchResults reads a search's answer, and returns its results and how
// long to cache them.
func searchResults(body []byte) ([]SubtitleResult, time.Duration, error) {
	var r osSearch
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, 0, err
	}
	var out []SubtitleResult
	for _, d := range r.Data {
		a := d.Attributes
		if len(a.Files) == 0 {
			continue
		}
		tag, err := language.Parse(a.Language)
		if err != nil {
			continue
		}
		r := SubtitleResult{
			Language: tag.String(), Release: a.Release, HearingImpaired: a.HearingImpaired, Forced: a.ForeignPartsOnly,
			Downloads: a.DownloadCount, HashMatch: a.MoviehashMatch,
		}
		r.ID = subtitleResultID(a.Files[0].FileID, r)
		out = append(out, r)
	}
	if len(r.Data) == 0 {
		return out, ttlNoSubtitles, nil
	}
	return out, ttlSearch, nil
}

// subtitleResultID encodes what a later download needs into the result ID
// the client sends back. It holds the provider's file ID, the language,
// the flags and the release, in the form "123.en.hi.forced~" followed by
// the release in unpadded URL-safe base64.
func subtitleResultID(file int, r SubtitleResult) string {
	s := strconv.Itoa(file) + "." + r.Language
	if r.HearingImpaired {
		s += ".hi"
	}
	if r.Forced {
		s += ".forced"
	}
	if r.Release != "" {
		s += "~" + base64.RawURLEncoding.EncodeToString([]byte(r.Release))
	}
	return s
}

// maxRelease bounds the release a result ID carries back, in bytes.
const maxRelease = 255

// parseSubtitleResult reads a subtitleResultID back.
func parseSubtitleResult(s string) (int, SubtitleResult, bool) {
	head, release, _ := strings.Cut(s, "~")
	b, err := base64.RawURLEncoding.DecodeString(release)
	if err != nil || len(b) > maxRelease || !utf8.Valid(b) {
		return 0, SubtitleResult{}, false
	}
	parts := strings.Split(head, ".")
	file, err := strconv.Atoi(parts[0])
	if err != nil || file <= 0 || len(parts) < 2 {
		return 0, SubtitleResult{}, false
	}
	tag, err := language.Parse(parts[1])
	if err != nil {
		return 0, SubtitleResult{}, false
	}
	r := SubtitleResult{ID: s, Language: tag.String(), Release: strings.TrimSpace(string(b))}
	for _, flag := range parts[2:] {
		switch flag {
		case "hi":
			r.HearingImpaired = true
		case "forced":
			r.Forced = true
		default:
			return 0, SubtitleResult{}, false
		}
	}
	return file, r, true
}

// download is a subtitle file downloaded from OpenSubtitles.
type download struct {
	result SubtitleResult // decoded from the result ID: the language, release and flags
	// name is the file's name among a file's downloaded subtitles. It's
	// built from the language and OpenSubtitles' file ID, with the
	// format's extension.
	name string
	data []byte
}

// fetch downloads a result that search returned, by its ID. ErrQuota means
// the day's downloads are used up. An ID that search didn't give returns
// ErrNotFound.
func (s *Service) fetch(ctx context.Context, result string) (download, error) {
	a := s.openSubtitles.account(s.deps.Settings.Get())
	if a == nil {
		return download{}, fmt.Errorf("%w: OpenSubtitles", ErrNotConfigured)
	}
	fileID, r, ok := parseSubtitleResult(result)
	if !ok {
		return download{}, fmt.Errorf("%w: subtitle result %q", ErrNotFound, result)
	}
	name, data, err := s.openSubtitles.download(ctx, a, fileID)
	if err != nil {
		return download{}, err
	}
	format := strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")
	if !slices.Contains(downloadFormats, format) {
		format = "srt" // OpenSubtitles serves SubRip unless asked otherwise
	}
	return download{result: r, name: fmt.Sprintf("%s.%d.%s", r.Language, fileID, format), data: data}, nil
}

type osDownload struct {
	Link         string `json:"link"`
	FileName     string `json:"file_name"`
	Remaining    int    `json:"remaining"`
	ResetTimeUTC string `json:"reset_time_utc"`
}

// download fetches a subtitle file by OpenSubtitles' file ID. Once the
// day's quota is known to be used up, it refuses without sending a
// request.
func (o *openSubtitles) download(ctx context.Context, a *account, file int) (name string, data []byte, err error) {
	a.mu.Lock()
	if a.remaining == 0 && time.Now().Before(a.reset) {
		reset := a.reset
		a.mu.Unlock()
		return "", nil, &QuotaError{Reset: reset}
	}
	a.mu.Unlock()
	h := a.header()
	h.Set("Content-Type", "application/json")
	if tok, err := o.login(ctx, a); err != nil {
		return "", nil, err
	} else if tok != "" {
		h.Set("Authorization", "Bearer "+tok)
	}
	body, _ := json.Marshal(map[string]int{"file_id": file})
	resp, err := o.post(ctx, openSubtitlesAPI+"/download", h, body)
	if err != nil {
		return "", nil, err
	}
	var d osDownload
	_ = json.Unmarshal(resp.Body, &d)
	reset, _ := time.Parse(time.RFC3339, d.ResetTimeUTC)
	switch resp.Status {
	case http.StatusOK:
	case http.StatusNotAcceptable: // the quota is used up
		a.setQuota(0, reset)
		return "", nil, &QuotaError{Reset: reset}
	case http.StatusNotFound, http.StatusGone:
		return "", nil, fmt.Errorf("%w: subtitle file %d", ErrNotFound, file)
	case http.StatusUnauthorized:
		a.mu.Lock()
		a.token = "" // sign in again next time, in case the token expired
		a.mu.Unlock()
		return "", nil, o.reject()
	default:
		return "", nil, fmt.Errorf("the OpenSubtitles download answered %d", resp.Status)
	}
	a.setQuota(d.Remaining, reset)
	if !strings.HasPrefix(d.Link, "https://") {
		return "", nil, errors.New("the OpenSubtitles download has no link")
	}
	got, err := o.get.Get(ctx, d.Link, http.Header{"User-Agent": {openSubtitlesAgent}})
	if err != nil {
		return "", nil, err
	}
	if got.Status != http.StatusOK || len(got.Body) == 0 {
		return "", nil, fmt.Errorf("subtitle file link answered %d", got.Status)
	}
	return d.FileName, got.Body, nil
}

func (a *account) setQuota(remaining int, reset time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.remaining = remaining
	if reset.IsZero() {
		reset = time.Now().Add(day)
	}
	a.reset = reset
}

// rejected is when OpenSubtitles last refused the account, or zero.
func (a *account) rejected() time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.loginRejected
}

// login signs in once, when an account is configured, to get the account's
// larger download quota. A refused account isn't tried again for
// loginRetry. Changed credentials make a new account, which tries at once.
func (o *openSubtitles) login(ctx context.Context, a *account) (string, error) {
	a.mu.Lock()
	tok, refused := a.token, a.loginRejected
	a.mu.Unlock()
	if a.user == "" || tok != "" {
		return tok, nil
	}
	if !refused.IsZero() && time.Since(refused) < loginRetry {
		return "", errLoginRejected
	}
	h := a.header()
	h.Set("Content-Type", "application/json")
	body, _ := json.Marshal(map[string]string{"username": a.user, "password": a.password})
	resp, err := o.post(ctx, openSubtitlesAPI+"/login", h, body)
	if err != nil {
		return "", err
	}
	if resp.Status == http.StatusUnauthorized || resp.Status == http.StatusBadRequest {
		a.mu.Lock()
		if a.loginRejected.IsZero() {
			slog.ErrorContext(ctx, "subtitles: OpenSubtitles rejected the account")
		}
		a.loginRejected = time.Now()
		a.mu.Unlock()
		return "", errLoginRejected
	}
	var r struct {
		Token string `json:"token"`
	}
	if resp.Status != http.StatusOK || json.Unmarshal(resp.Body, &r) != nil || r.Token == "" {
		return "", fmt.Errorf("login: status %d", resp.Status)
	}
	a.mu.Lock()
	a.token, a.loginRejected = r.Token, time.Time{}
	a.mu.Unlock()
	return r.Token, nil
}

// post sends one POST through OpenSubtitles' spacing and outage rules.
func (o *openSubtitles) post(ctx context.Context, u string, h http.Header, body []byte) (egress.Response, error) {
	if err := o.down(); err != nil {
		return egress.Response{}, err
	}
	if err := o.limit.Wait(ctx); err != nil {
		return egress.Response{}, err
	}
	resp, err := o.get.Post(ctx, u, h, body)
	switch {
	case ctx.Err() != nil:
		return egress.Response{}, ctx.Err()
	case err != nil:
		return egress.Response{}, o.failed(0, err)
	case resp.Status == http.StatusTooManyRequests || resp.Status >= 500:
		return egress.Response{}, o.failed(resp.RetryAfter(), fmt.Errorf("status %d", resp.Status))
	}
	o.limit.Recovered()
	return resp, nil
}

// down returns ErrUnavailable while requests to OpenSubtitles are backed
// off. Once that time passes, the next request is a trial.
func (o *openSubtitles) down() error {
	out := o.limit.Outage()
	if !time.Now().Before(out.Until) {
		return nil
	}
	cause := "resting after failures"
	if out.Rejected {
		cause = "the API key was rejected"
	}
	return fmt.Errorf("%w: OpenSubtitles until %s: %s", ErrUnavailable, out.Until.Format(time.RFC3339), cause)
}

// failed records an outage. Requests stop until a backoff, or
// OpenSubtitles' Retry-After, passes.
func (o *openSubtitles) failed(after time.Duration, cause error) error {
	out := o.limit.Failed(time.Now(), after)
	slog.Warn("subtitles: OpenSubtitles unavailable", "until", out.Until, "err", cause)
	return fmt.Errorf("%w: OpenSubtitles until %s: %w", ErrUnavailable, out.Until.Format(time.RFC3339), cause)
}

// reject pauses requests when OpenSubtitles refused our key, and raises a
// problem.
func (o *openSubtitles) reject() error {
	out := o.limit.Rejected(time.Now())
	slog.Error("subtitles: OpenSubtitles rejected the API key")
	return fmt.Errorf("%w: OpenSubtitles until %s: the API key was rejected", ErrUnavailable, out.Until.Format(time.RFC3339))
}

// health lists OpenSubtitles' problems: it's down, or it refuses the key
// or the account in st.
func (o *openSubtitles) health(st config.Settings) []health.Problem {
	var out []health.Problem
	if a := o.account(st); a != nil {
		if since := a.rejected(); !since.IsZero() {
			out = append(out, health.Problem{
				Component: "subtitles", Code: "openSubtitlesAccountRejected", Severity: health.Error, RaisedAt: since,
				Message: "OpenSubtitles rejected the account's username or password. Subtitle downloads are paused.",
			})
		}
	}
	switch outage := o.limit.Outage(); {
	case outage.Rejected:
		out = append(out, health.Problem{
			Component: "subtitles", Code: "openSubtitlesKeyRejected", Severity: health.Error, RaisedAt: outage.Since,
			Message: "OpenSubtitles rejected the API key. Subtitle search and downloads are paused.",
		})
	case outage.Failures > 0:
		out = append(out, health.Problem{
			Component: "subtitles", Code: "openSubtitlesUnavailable", Severity: health.Warning, RaisedAt: outage.Since,
			Message: "OpenSubtitles isn't responding. Subtitle search and downloads are paused.",
		})
	}
	return out
}
