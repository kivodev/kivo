package config

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"reflect"
	"regexp"
	"sync"
	"sync/atomic"

	"github.com/mmcdole/kivo/internal/config/dbq"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
)

// Settings are everything an admin can change at runtime. Defaults live in
// code. The setting table holds a row only for a setting that differs from
// its default. Each field's `setting` tag is its key. A new field needs a
// default in Defaults and a check in Validate, and is then stored
// automatically.
type Settings struct {
	ServerName           string  `setting:"server_name"`
	MetadataLanguage     string  `setting:"metadata_language"`      // BCP 47, such as en or pt-BR
	ContentRatingCountry string  `setting:"content_rating_country"` // ISO 3166-1 alpha-2, for content ratings
	PlayedThreshold      float64 `setting:"played_threshold"`       // fraction watched that marks a work played
	// Without an OpenSubtitles key, subtitles aren't searched or downloaded.
	// An account's username and password raise the daily download quota.
	OpenSubtitlesKey      string `setting:"opensubtitles_key"`
	OpenSubtitlesUsername string `setting:"opensubtitles_username"`
	OpenSubtitlesPassword string `setting:"opensubtitles_password"`
	// FileWatching scans a folder as soon as its files change. When it is
	// off, the rescan every 12 hours finds changes.
	FileWatching bool `setting:"file_watching"`
	// LocalNetworks are the CIDR ranges whose clients count as local. Empty
	// means private, loopback and link-local addresses. Any other range,
	// such as Tailscale's 100.64.0.0/10, is local only once listed here.
	LocalNetworks []string `setting:"local_networks"`
	// Accelerator is the video accelerator that transcodes use. It is auto
	// (the accelerators that passed the accelerator test, best first), none
	// (the CPU), or one accelerator as the test names it, such as
	// qsv:/dev/dri/renderD128.
	Accelerator         string `setting:"accelerator"`
	VideoTranscodeLimit int    `setting:"video_transcode_limit"` // simultaneous video transcodes; zero means no limit
	// MaintenanceStart and MaintenanceEnd are the nightly maintenance
	// window, in whole hours of the server's local time. The scheduled night
	// tasks start inside it, and maintenance work stops when it closes. An
	// end that is not after the start falls on the next day.
	MaintenanceStart int `setting:"maintenance_start"`
	MaintenanceEnd   int `setting:"maintenance_end"`
	// PreviewsAtOnce is how many files have preview thumbnails made at
	// once, from 1 to 8. It applies both in the window and outside it.
	PreviewsAtOnce int `setting:"previews_at_once"`
	// MarkersAtOnce is how many files are fingerprinted for intro and
	// credits markers at once, from 1 to 4. It applies both in the window
	// and outside it.
	MarkersAtOnce int `setting:"markers_at_once"`
	// PublicURL is the address apps use away from home, such as
	// https://kivo.example.com. It is empty when the server isn't reachable
	// from outside.
	PublicURL string `setting:"public_url"`
	// RemoteQualityCap is the most a stream outside the local networks may
	// use, in bits per second, for a user without a cap of their own. Zero
	// means no cap.
	RemoteQualityCap int64 `setting:"remote_quality_cap"`
}

// Accelerator settings other than a chosen accelerator.
const (
	AcceleratorAuto = "auto"
	AcceleratorNone = "none"
)

// Defaults returns the settings of a fresh server.
func Defaults() Settings {
	return Settings{
		ServerName:           "Kivo",
		MetadataLanguage:     "en",
		ContentRatingCountry: "US",
		PlayedThreshold:      0.9,
		FileWatching:         true,
		Accelerator:          AcceleratorAuto,
		MaintenanceStart:     2,
		MaintenanceEnd:       6,
		PreviewsAtOnce:       2,
		MarkersAtOnce:        1,
	}
}

var (
	languageTag = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z]{2})?$`)
	countryCode = regexp.MustCompile(`^[A-Z]{2}$`)
	accelerator = regexp.MustCompile(`^(vaapi|qsv|nvenc|videotoolbox):\S+$`)
)

// ValidMetadataLanguage reports whether s is a valid metadata language for
// the server or a library. It must be a language tag such as en or pt-BR.
func ValidMetadataLanguage(s string) bool { return languageTag.MatchString(s) }

// ValidContentRatingCountry reports whether s is a valid content rating
// country for the server or a library. It must be a two-letter country
// code such as US.
func ValidContentRatingCountry(s string) bool { return countryCode.MatchString(s) }

// IsLocal reports whether a client address is on a local network. When
// LocalNetworks is set, only those ranges count. Otherwise private (RFC 1918
// and unique local), loopback and link-local addresses count.
func (s Settings) IsLocal(ip netip.Addr) bool {
	ip = ip.Unmap()
	if len(s.LocalNetworks) > 0 {
		for _, n := range s.LocalNetworks {
			if p, err := netip.ParsePrefix(n); err == nil && p.Contains(ip) {
				return true
			}
		}
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}

// Validate checks every setting, returning a *ValidationError if any fails.
func (s Settings) Validate() error {
	var errs []error
	if s.ServerName == "" {
		errs = append(errs, errors.New("server_name must not be empty"))
	}
	if !ValidMetadataLanguage(s.MetadataLanguage) {
		errs = append(errs, fmt.Errorf("metadata_language: %q is not a language tag such as en or pt-BR", s.MetadataLanguage))
	}
	if !ValidContentRatingCountry(s.ContentRatingCountry) {
		errs = append(errs, fmt.Errorf("content_rating_country: %q is not a two-letter country code", s.ContentRatingCountry))
	}
	if s.PlayedThreshold < 0.5 || s.PlayedThreshold > 1 {
		errs = append(errs, fmt.Errorf("played_threshold: %v is not between 0.5 and 1", s.PlayedThreshold))
	}
	for _, n := range s.LocalNetworks {
		if _, err := netip.ParsePrefix(n); err != nil {
			errs = append(errs, fmt.Errorf("local_networks: %q is not a CIDR range", n))
		}
	}
	if a := s.Accelerator; a != AcceleratorAuto && a != AcceleratorNone && !accelerator.MatchString(a) {
		errs = append(errs, fmt.Errorf("accelerator: %q is not auto, none or an accelerator such as qsv:/dev/dri/renderD128", a))
	}
	if s.VideoTranscodeLimit < 0 {
		errs = append(errs, fmt.Errorf("video_transcode_limit: %d is negative", s.VideoTranscodeLimit))
	}
	if s.RemoteQualityCap < 0 {
		errs = append(errs, fmt.Errorf("remote_quality_cap: %d is negative", s.RemoteQualityCap))
	}
	for _, h := range []struct {
		key  string
		hour int
	}{{"maintenance_start", s.MaintenanceStart}, {"maintenance_end", s.MaintenanceEnd}} {
		if h.hour < 0 || h.hour > 23 {
			errs = append(errs, fmt.Errorf("%s: %d is not an hour from 0 to 23", h.key, h.hour))
		}
	}
	if s.MaintenanceStart == s.MaintenanceEnd {
		errs = append(errs, errors.New("maintenance_end: the window must end at another hour than it starts"))
	}
	if s.PreviewsAtOnce < 1 || s.PreviewsAtOnce > 8 {
		errs = append(errs, fmt.Errorf("previews_at_once: %d is not from 1 to 8", s.PreviewsAtOnce))
	}
	if s.MarkersAtOnce < 1 || s.MarkersAtOnce > 4 {
		errs = append(errs, fmt.Errorf("markers_at_once: %d is not from 1 to 4", s.MarkersAtOnce))
	}
	if u := s.PublicURL; u != "" {
		p, err := url.Parse(u)
		if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" || p.User != nil ||
			p.RawQuery != "" || p.Fragment != "" {
			errs = append(errs, fmt.Errorf("public_url: %q is not an http or https address such as https://kivo.example.com", u))
		}
	}
	return invalid(errs)
}

// serverIDKey and serverKeyKey are the setting table's rows for the
// server's ID and its identity key's private scalar. Neither is a setting. Both are
// made on first start and never change.
const (
	serverIDKey  = "server_id"
	serverKeyKey = "server_key"
)

// Store serves the current settings and persists changes.
type Store struct {
	db       *store.DB
	q        *dbq.Queries
	serverID id.ID
	key      *ecdsa.PrivateKey
	cur      atomic.Pointer[Settings]
	mu       sync.Mutex             // serializes Update
	changed  func(was, is Settings) // Options.Changed
}

// Options are the store's optional hooks.
type Options struct {
	// Changed receives the settings before and after each Update, once the
	// new settings are written and current. It runs before Update returns
	// and must not call Update.
	Changed func(was, is Settings)
}

// Open loads the stored settings over the defaults. Some rows are logged
// and ignored: a key this release doesn't know (from a newer release, or a
// removed setting), and a value that no longer parses or is no longer
// valid. That setting keeps its default and the others keep their stored
// values, so a bad row never stops startup.
func Open(ctx context.Context, db *store.DB, opt Options) (*Store, error) {
	if opt.Changed == nil {
		opt.Changed = func(was, is Settings) {}
	}
	s := &Store{db: db, q: dbq.New(db.Reader()), changed: opt.Changed}
	if err := s.loadIdentity(ctx); err != nil {
		return nil, fmt.Errorf("load server identity: %w", err)
	}
	rows, err := s.q.ListSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	cur := Defaults()
	fields := fieldsByKey(&cur)
	for _, r := range rows {
		if r.Key == serverIDKey || r.Key == serverKeyKey {
			continue
		}
		f, ok := fields[r.Key]
		if !ok {
			slog.WarnContext(ctx, "config: ignoring unknown setting", "key", r.Key)
			continue
		}
		v := reflect.New(f.Type())
		if err := json.Unmarshal([]byte(r.Value), v.Interface()); err != nil {
			slog.WarnContext(ctx, "config: ignoring unreadable setting", "key", r.Key, "err", err)
			continue
		}
		// The value is checked alone over the defaults. The defaults are
		// valid, so any error comes from this value.
		alone := Defaults()
		fieldsByKey(&alone)[r.Key].Set(v.Elem())
		if err := alone.Validate(); err != nil {
			slog.WarnContext(ctx, "config: ignoring invalid setting", "key", r.Key, "err", err)
			continue
		}
		f.Set(v.Elem())
	}
	s.cur.Store(&cur)
	return s, nil
}

// loadIdentity reads the server's ID and identity key, making them on
// first start.
func (s *Store) loadIdentity(ctx context.Context) error {
	if err := s.loadOrMake(ctx, serverIDKey, id.New(), &s.serverID); err != nil {
		return err
	}
	made, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	scalar, err := made.Bytes()
	if err != nil {
		return err
	}
	if err := s.loadOrMake(ctx, serverKeyKey, scalar, &scalar); err != nil {
		return err
	}
	s.key, err = ecdsa.ParseRawPrivateKey(elliptic.P256(), scalar)
	if err != nil {
		return fmt.Errorf("server key: %w", err)
	}
	return nil
}

// loadOrMake reads the setting table's row key into v, writing made there
// first when the row is absent.
func (s *Store) loadOrMake(ctx context.Context, key string, made, v any) error {
	b, err := json.Marshal(made)
	if err != nil {
		return err
	}
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return dbq.New(tx).InsertSettingIfAbsent(ctx, dbq.InsertSettingIfAbsentParams{Key: key, Value: string(b)})
	})
	if err != nil {
		return err
	}
	stored, err := s.q.Setting(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(stored), v)
}

// ServerID is the server's own ID, which apps use to recognize it. It stays
// the same across restarts and address changes, and a backup carries it.
func (s *Store) ServerID() id.ID { return s.serverID }

// ServerKey is the server's identity key, an ECDSA P-256 key, which signs
// its answer to an app's probe. Like the ID, it stays the same and a backup
// carries it.
func (s *Store) ServerKey() *ecdsa.PrivateKey { return s.key }

// Get returns the current settings. It never blocks. Treat slices in the
// result as read-only.
func (s *Store) Get() Settings { return *s.cur.Load() }

// Update applies fn to a copy of the current settings, validates the result,
// writes the fields that changed, and only then makes the result current.
// If fn, validation or the write fails, Get is unchanged. A result that
// fails validation returns a *ValidationError.
func (s *Store) Update(ctx context.Context, fn func(*Settings) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	old := s.Get()
	next := clone(old)
	if err := fn(&next); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}

	defaults := Defaults()
	oldF, newF, defF := fieldsByKey(&old), fieldsByKey(&next), fieldsByKey(&defaults)
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		for key, nf := range newF {
			nv := mustJSON(nf)
			if bytes.Equal(nv, mustJSON(oldF[key])) {
				continue
			}
			var err error
			if bytes.Equal(nv, mustJSON(defF[key])) {
				err = q.DeleteSetting(ctx, key)
			} else {
				err = q.UpsertSetting(ctx, dbq.UpsertSettingParams{Key: key, Value: string(nv)})
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.cur.Store(&next)
	s.changed(old, next)
	return nil
}

// fieldsByKey maps each setting key to its addressable field in s.
func fieldsByKey(s *Settings) map[string]reflect.Value {
	v := reflect.ValueOf(s).Elem()
	t := v.Type()
	out := make(map[string]reflect.Value, t.NumField())
	for i := range t.NumField() {
		out[t.Field(i).Tag.Get("setting")] = v.Field(i)
	}
	return out
}

// clone deep-copies s, so fn in Update can't alter the current settings'
// slices.
func clone(s Settings) Settings {
	var out Settings
	if err := json.Unmarshal(mustJSON(reflect.ValueOf(s)), &out); err != nil {
		panic(err)
	}
	return out
}

func mustJSON(v reflect.Value) []byte {
	b, err := json.Marshal(v.Interface())
	if err != nil {
		panic(err) // settings are plain data, so marshalling can't fail
	}
	return b
}
