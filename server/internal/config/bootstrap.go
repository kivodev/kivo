package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Bootstrap holds the settings needed before the database opens. Each key
// can also be set by an environment variable named KIVO_ plus the key in
// capitals, such as KIVO_DATA_DIR. The variable wins over the file, so a
// container can be configured without a file. Lists in environment
// variables are comma-separated.
type Bootstrap struct {
	Listen   []string `toml:"listen"`    // addresses to listen on
	BasePath string   `toml:"base_path"` // serve under a subpath, such as /kivo, behind a reverse proxy
	// TLSCert and TLSKey are the certificate and key for built-in TLS.
	TLSCert        string   `toml:"tls_cert"`
	TLSKey         string   `toml:"tls_key"`
	TrustedProxies []string `toml:"trusted_proxies"` // addresses or CIDR ranges whose X-Forwarded-For is trusted
	DataDir        string   `toml:"data_dir"`        // the database, uploaded images and downloaded subtitles; must be a local disk
	CacheDir       string   `toml:"cache_dir"`       // everything that can be rebuilt; safe to delete
	BackupDir      string   `toml:"backup_dir"`      // nightly database backups
	FFmpeg         string   `toml:"ffmpeg"`          // path to ffmpeg
	FFprobe        string   `toml:"ffprobe"`         // path to ffprobe
	LogLevel       string   `toml:"log_level"`       // debug, info, warn or error
	LogFormat      string   `toml:"log_format"`      // text (logfmt) or json, for log tools
	TMDbKey        string   `toml:"tmdb_key"`        // a TMDb API key or read access token, replacing the built-in one
	// Audible, when off, describes books from their files alone and never
	// asks Audible or Audnexus. The end-to-end tests run that way.
	Audible bool `toml:"audible"`
}

// DefaultBootstrap is used for every key that neither the file nor the
// environment sets.
func DefaultBootstrap() Bootstrap {
	return Bootstrap{
		Listen:    []string{":8484"},
		DataDir:   "data",
		CacheDir:  "cache",
		BackupDir: "backups",
		FFmpeg:    "ffmpeg",
		FFprobe:   "ffprobe",
		LogLevel:  "info",
		LogFormat: "text",
		Audible:   true,
	}
}

// DatabasePath is where the database lives, in DataDir.
func (b Bootstrap) DatabasePath() string { return filepath.Join(b.DataDir, "kivo.db") }

// LoadBootstrap reads path, applies KIVO_* variables, normalizes the base
// path and validates the result. A missing file is fine, because everything
// can come from the environment. Unknown keys are errors, reported with
// their line.
func LoadBootstrap(path string) (Bootstrap, error) {
	b, err := loadBootstrap(path, os.LookupEnv)
	if err != nil {
		return b, fmt.Errorf("load config %s: %w", path, err)
	}
	return b, nil
}

func loadBootstrap(path string, env func(string) (string, bool)) (Bootstrap, error) {
	b := DefaultBootstrap()
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return b, err
	default:
		dec := toml.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&b); err != nil {
			return b, errors.New(describeTOMLError(err))
		}
	}
	if err := applyEnv(&b, env); err != nil {
		return b, err
	}
	b.normalize()
	return b, b.Validate()
}

func describeTOMLError(err error) string {
	var strict *toml.StrictMissingError
	if errors.As(err, &strict) {
		var lines []string
		for _, e := range strict.Errors {
			row, _ := e.Position()
			lines = append(lines, fmt.Sprintf("line %d: unknown key %q", row, strings.Join(e.Key(), ".")))
		}
		return strings.Join(lines, "; ")
	}
	var dec *toml.DecodeError
	if errors.As(err, &dec) {
		row, col := dec.Position()
		return fmt.Sprintf("line %d, column %d: %s", row, col, dec.Error())
	}
	return err.Error()
}

// applyEnv overrides each field from KIVO_<TOML KEY>, when that variable is
// set.
func applyEnv(b *Bootstrap, env func(string) (string, bool)) error {
	v := reflect.ValueOf(b).Elem()
	t := v.Type()
	for i := range t.NumField() {
		key := t.Field(i).Tag.Get("toml")
		val, ok := env("KIVO_" + strings.ToUpper(key))
		if !ok {
			continue
		}
		f := v.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString(val)
		case reflect.Slice:
			var items []string
			for s := range strings.SplitSeq(val, ",") {
				if s = strings.TrimSpace(s); s != "" {
					items = append(items, s)
				}
			}
			f.Set(reflect.ValueOf(items))
		case reflect.Bool:
			on, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("environment variable KIVO_%s: %q is neither true nor false", strings.ToUpper(key), val)
			}
			f.SetBool(on)
		}
	}
	return nil
}

// normalize gives the base path one leading slash and no trailing slash. It
// turns "/" into the empty root.
func (b *Bootstrap) normalize() {
	if b.BasePath != "" {
		b.BasePath = strings.TrimSuffix("/"+strings.Trim(b.BasePath, "/"), "/")
	}
}

// logLevels are the values log_level accepts, which are slog's level names.
var logLevels = []string{"debug", "info", "warn", "error"}

// Validate checks every key, returning a *ValidationError if any fails.
func (b Bootstrap) Validate() error {
	var errs []error
	if len(b.Listen) == 0 {
		errs = append(errs, errors.New("listen: at least one address is needed"))
	}
	if b.BasePath != "" && (!strings.HasPrefix(b.BasePath, "/") || strings.HasSuffix(b.BasePath, "/")) {
		errs = append(errs, fmt.Errorf("base_path: %q must start with / and not end with one", b.BasePath))
	}
	if (b.TLSCert == "") != (b.TLSKey == "") {
		errs = append(errs, errors.New("tls_cert and tls_key must be set together"))
	}
	for _, p := range b.TrustedProxies {
		if _, err := parseProxy(p); err != nil {
			errs = append(errs, fmt.Errorf("trusted_proxies: %q is not an address or CIDR range", p))
		}
	}
	for _, d := range []struct{ name, dir string }{{"data_dir", b.DataDir}, {"cache_dir", b.CacheDir}, {"backup_dir", b.BackupDir}} {
		if d.dir == "" {
			errs = append(errs, fmt.Errorf("%s must be set", d.name))
		}
	}
	if !slices.Contains(logLevels, b.LogLevel) {
		errs = append(errs, fmt.Errorf("log_level: %q is not one of %s", b.LogLevel, strings.Join(logLevels, ", ")))
	}
	if b.LogFormat != "text" && b.LogFormat != "json" {
		errs = append(errs, fmt.Errorf("log_format: %q is not text or json", b.LogFormat))
	}
	return invalid(errs)
}

// TrustedPrefixes returns TrustedProxies as prefixes. A single address
// becomes a prefix of its full length. Entries that Validate rejects are
// left out.
func (b Bootstrap) TrustedPrefixes() []netip.Prefix {
	var out []netip.Prefix
	for _, p := range b.TrustedProxies {
		if pf, err := parseProxy(p); err == nil {
			out = append(out, pf)
		}
	}
	return out
}

func parseProxy(p string) (netip.Prefix, error) {
	if strings.Contains(p, "/") {
		pf, err := netip.ParsePrefix(p)
		return pf.Masked(), err
	}
	a, err := netip.ParseAddr(p)
	if err != nil {
		return netip.Prefix{}, err
	}
	a = a.Unmap()
	return netip.PrefixFrom(a, a.BitLen()), nil
}
