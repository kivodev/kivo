package config

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/config/dbq"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/store/storetest"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "server.toml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func noEnv(string) (string, bool) { return "", false }

func TestBootstrapUnknownKeyReportsLine(t *testing.T) {
	p := writeFile(t, "listen = [\":1\"]\n\ndata_dri = \"/x\"\n")
	_, err := loadBootstrap(p, noEnv)
	if err == nil || !strings.Contains(err.Error(), `line 3: unknown key "data_dri"`) {
		t.Fatalf("err = %v, want the line and key", err)
	}
}

func TestBootstrapSyntaxErrorReportsLine(t *testing.T) {
	p := writeFile(t, "log_level = \"info\"\nlisten = [\":1\"\n")
	_, err := loadBootstrap(p, noEnv)
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v, want line 2", err)
	}
}

func TestBootstrapMissingFileUsesDefaults(t *testing.T) {
	b, err := loadBootstrap(filepath.Join(t.TempDir(), "absent.toml"), noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if want := DefaultBootstrap(); !slices.Equal(b.Listen, want.Listen) || b.DataDir != want.DataDir {
		t.Fatalf("got %+v, want defaults", b)
	}
}

func TestBootstrapEnvironmentWins(t *testing.T) {
	p := writeFile(t, "data_dir = \"/from/file\"\ncache_dir = \"/cache\"\nupdate_check = false\n")
	env := map[string]string{
		"KIVO_DATA_DIR":        "/from/env",
		"KIVO_LISTEN":          ":1, :2",
		"KIVO_TRUSTED_PROXIES": "10.0.0.0/8,192.168.1.2",
		"KIVO_BASE_PATH":       "kivo/",
		"KIVO_AUDIBLE":         "false",
	}
	b, err := loadBootstrap(p, func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	if b.DataDir != "/from/env" || b.CacheDir != "/cache" {
		t.Errorf("dirs = %q, %q; want the env value over the file, and the file over the default", b.DataDir, b.CacheDir)
	}
	if !slices.Equal(b.Listen, []string{":1", ":2"}) || len(b.TrustedProxies) != 2 {
		t.Errorf("lists = %q, %q", b.Listen, b.TrustedProxies)
	}
	if b.Audible {
		t.Error("KIVO_AUDIBLE=false left Audible on")
	}
	if b.UpdateCheck {
		t.Error("update_check = false left UpdateCheck on")
	}
	if b.BasePath != "/kivo" {
		t.Errorf("base path = %q, want /kivo", b.BasePath)
	}
}

func TestBootstrapBadSwitch(t *testing.T) {
	_, err := loadBootstrap(filepath.Join(t.TempDir(), "absent.toml"), func(k string) (string, bool) { return "maybe", k == "KIVO_AUDIBLE" })
	if err == nil || !strings.Contains(err.Error(), "KIVO_AUDIBLE") {
		t.Fatalf("err = %v, want KIVO_AUDIBLE named", err)
	}
}

func TestEveryBootstrapKeyHasAnEnvironmentVariable(t *testing.T) {
	b := DefaultBootstrap()
	seen := map[string]bool{}
	_ = applyEnv(&b, func(k string) (string, bool) { seen[k] = true; return "", false })
	for _, k := range []string{"KIVO_LISTEN", "KIVO_BASE_PATH", "KIVO_TLS_CERT", "KIVO_TLS_KEY", "KIVO_TRUSTED_PROXIES",
		"KIVO_DATA_DIR", "KIVO_CACHE_DIR", "KIVO_BACKUP_DIR", "KIVO_FFMPEG", "KIVO_FFPROBE", "KIVO_LOG_LEVEL", "KIVO_LOG_FORMAT", "KIVO_AUDIBLE",
		"KIVO_TMDB_KEY", "KIVO_UPDATE_CHECK"} {
		if !seen[k] {
			t.Errorf("%s is never read", k)
		}
	}
}

func TestBootstrapValidate(t *testing.T) {
	for name, mutate := range map[string]func(*Bootstrap){
		"cert without key": func(b *Bootstrap) { b.TLSCert = "c.pem" },
		"bad proxy":        func(b *Bootstrap) { b.TrustedProxies = []string{"nope"} },
		"bad log level":    func(b *Bootstrap) { b.LogLevel = "loud" },
		"no listen":        func(b *Bootstrap) { b.Listen = nil },
		"no data dir":      func(b *Bootstrap) { b.DataDir = "" },
	} {
		b := DefaultBootstrap()
		mutate(&b)
		if b.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// rows are the stored settings, without the server's ID and key.
func rows(t *testing.T, db *store.DB) map[string]string {
	t.Helper()
	rs, err := dbq.New(db.Reader()).ListSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, r := range rs {
		if r.Key != serverIDKey && r.Key != serverKeyKey {
			out[r.Key] = r.Value
		}
	}
	return out
}

func TestServerIdentityIsMadeOnceAndKept(t *testing.T) {
	db := storetest.New(t)
	ctx := context.Background()
	s, err := Open(ctx, db, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if s.ServerID().IsZero() {
		t.Fatal("no server ID")
	}
	if err := s.Update(ctx, func(c *Settings) error { c.ServerName = "Den"; return nil }); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, db, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.ServerID() != s.ServerID() {
		t.Fatalf("server ID %v after a restart, was %v", reopened.ServerID(), s.ServerID())
	}
	if len(s.ServerKey()) != ed25519.PrivateKeySize || !s.ServerKey().Equal(reopened.ServerKey()) {
		t.Fatal("the server key changed across a restart")
	}
	other, err := Open(ctx, storetest.New(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if other.ServerKey().Equal(s.ServerKey()) {
		t.Fatal("two servers made the same key")
	}
}

// Options.Changed is called for each Update that is written, with the
// settings before and after. It isn't called for an Update that fails.
func TestUpdateTellsChanged(t *testing.T) {
	db := storetest.New(t)
	ctx := context.Background()
	var told [][2]Settings
	s, err := Open(ctx, db, Options{Changed: func(was, is Settings) { told = append(told, [2]Settings{was, is}) }})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, func(c *Settings) error { c.FileWatching = !c.FileWatching; return nil }); err != nil {
		t.Fatal(err)
	}
	if len(told) != 1 || told[0][0].FileWatching == told[0][1].FileWatching || told[0][1].FileWatching != s.Get().FileWatching {
		t.Fatalf("told %+v", told)
	}
	if err := s.Update(ctx, func(c *Settings) error { c.PlayedThreshold = 2; return nil }); err == nil {
		t.Fatal("an invalid threshold was written")
	}
	if len(told) != 1 {
		t.Fatalf("told of a failed update: %+v", told[1:])
	}
}

func TestPublicURL(t *testing.T) {
	for u, ok := range map[string]bool{
		"": true, "https://kivo.example.com": true, "http://203.0.113.9:8484/kivo": true,
		"kivo.example.com": false, "ftp://kivo.example.com": false, "https://kivo.example.com/?a=1": false,
		"https://me:pw@kivo.example.com": false,
	} {
		s := Defaults()
		s.PublicURL = u
		if err := s.Validate(); (err == nil) != ok {
			t.Errorf("%q: err = %v", u, err)
		}
	}
}

func TestUpdateWritesOnlyChangedNonDefaultFields(t *testing.T) {
	db := storetest.New(t)
	ctx := context.Background()
	s, err := Open(ctx, db, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get(); got.ServerName != Defaults().ServerName {
		t.Fatalf("fresh server: %+v", got)
	}

	if err := s.Update(ctx, func(c *Settings) error {
		c.ServerName = "Home"
		c.LocalNetworks = []string{"10.0.0.0/8"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := rows(t, db); len(got) != 2 || got["server_name"] != `"Home"` || got["local_networks"] != `["10.0.0.0/8"]` {
		t.Fatalf("rows = %v", got)
	}

	// Setting a field back to its default removes its row.
	if err := s.Update(ctx, func(c *Settings) error { c.ServerName = Defaults().ServerName; return nil }); err != nil {
		t.Fatal(err)
	}
	if got := rows(t, db); len(got) != 1 {
		t.Fatalf("rows = %v, want only local_networks", got)
	}

	reopened, err := Open(ctx, db, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Get(); !slices.Equal(got.LocalNetworks, []string{"10.0.0.0/8"}) || got.ServerName != "Kivo" {
		t.Fatalf("reopened = %+v", got)
	}
}

func TestFailedUpdateLeavesSettingsUnchanged(t *testing.T) {
	db := storetest.New(t)
	ctx := context.Background()
	s, err := Open(ctx, db, Options{})
	if err != nil {
		t.Fatal(err)
	}
	before := s.Get()

	if err := s.Update(ctx, func(c *Settings) error { c.PlayedThreshold = 2; return nil }); err == nil {
		t.Fatal("invalid value accepted")
	}
	boom := errors.New("boom")
	if err := s.Update(ctx, func(c *Settings) error { c.ServerName = "x"; return boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want fn's error", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.Update(cancelled, func(c *Settings) error { c.ServerName = "x"; return nil }); err == nil {
		t.Fatal("write with a cancelled context succeeded")
	}
	// fn mutating a slice it was given must not reach the current settings.
	_ = s.Update(ctx, func(c *Settings) error { c.LocalNetworks = append(c.LocalNetworks, "bad"); return nil })

	if after := s.Get(); after.ServerName != before.ServerName || after.PlayedThreshold != before.PlayedThreshold || len(after.LocalNetworks) != 0 {
		t.Fatalf("settings changed: %+v", after)
	}
	if got := rows(t, db); len(got) != 0 {
		t.Fatalf("rows written: %v", got)
	}
}

// A row that is unknown, unreadable or invalid leaves its own setting at
// the default and every other setting at its stored value.
func TestOpenIgnoresUnknownUnreadableAndInvalidRows(t *testing.T) {
	db := storetest.New(t)
	ctx := context.Background()
	err := db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if err := q.UpsertSetting(ctx, dbq.UpsertSettingParams{Key: "from_the_future", Value: `1`}); err != nil {
			return err
		}
		if err := q.UpsertSetting(ctx, dbq.UpsertSettingParams{Key: "played_threshold", Value: `"ninety"`}); err != nil {
			return err
		}
		if err := q.UpsertSetting(ctx, dbq.UpsertSettingParams{Key: "video_transcode_limit", Value: `-1`}); err != nil {
			return err
		}
		if err := q.UpsertSetting(ctx, dbq.UpsertSettingParams{Key: "local_networks", Value: `["10.0.0.0/8"]`}); err != nil {
			return err
		}
		return q.UpsertSetting(ctx, dbq.UpsertSettingParams{Key: "server_name", Value: `"Den"`})
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, db, Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := s.Get()
	if got.ServerName != "Den" || got.PlayedThreshold != Defaults().PlayedThreshold || got.VideoTranscodeLimit != 0 ||
		!slices.Equal(got.LocalNetworks, []string{"10.0.0.0/8"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestDefaultsAreValid(t *testing.T) {
	if err := Defaults().Validate(); err != nil {
		t.Fatal(err)
	}
	b := DefaultBootstrap()
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAcceleratorSettings(t *testing.T) {
	for _, ok := range []string{"auto", "none", "qsv:/dev/dri/renderD128", "vaapi:/dev/dri/renderD129", "nvenc:0"} {
		s := Defaults()
		s.Accelerator = ok
		if err := s.Validate(); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "off", "on", "qsv:", "cuda:0", "/dev/dri/renderD128"} {
		s := Defaults()
		s.Accelerator = bad
		if s.Validate() == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	s := Defaults()
	s.VideoTranscodeLimit = -1
	if s.Validate() == nil {
		t.Error("a negative transcode limit accepted")
	}
}

func TestMaintenanceWindow(t *testing.T) {
	for _, w := range [][2]int{{2, 6}, {23, 4}, {0, 23}} {
		s := Defaults()
		s.MaintenanceStart, s.MaintenanceEnd = w[0], w[1]
		if err := s.Validate(); err != nil {
			t.Errorf("%v: %v", w, err)
		}
	}
	for _, w := range [][2]int{{3, 3}, {-1, 4}, {2, 24}} {
		s := Defaults()
		s.MaintenanceStart, s.MaintenanceEnd = w[0], w[1]
		if s.Validate() == nil {
			t.Errorf("%v accepted", w)
		}
	}
}

func TestPreviewsAtOnce(t *testing.T) {
	for n, ok := range map[int]bool{0: false, 1: true, 3: true, 8: true, 9: false} {
		s := Defaults()
		s.PreviewsAtOnce = n
		if err := s.Validate(); (err == nil) != ok {
			t.Errorf("%d: %v", n, err)
		}
	}
}

func TestMarkersAtOnce(t *testing.T) {
	for n, ok := range map[int]bool{0: false, 1: true, 4: true, 5: false} {
		s := Defaults()
		s.MarkersAtOnce = n
		if err := s.Validate(); (err == nil) != ok {
			t.Errorf("%d: %v", n, err)
		}
	}
}

// By default a client is local on a private, loopback or link-local
// address. Once local networks are set, only those ranges count. Tailscale's
// range is local only when it is listed.
func TestIsLocal(t *testing.T) {
	addr := netip.MustParseAddr
	for _, c := range []struct {
		nets  []string
		ip    string
		local bool
	}{
		{nil, "192.168.1.20", true},
		{nil, "::ffff:10.0.0.5", true},
		{nil, "::1", true},
		{nil, "fe80::1", true},
		{nil, "fd00::5", true},
		{nil, "100.101.102.103", false},
		{nil, "203.0.113.9", false},
		{nil, "2001:db8::1", false},
		{[]string{"203.0.113.0/24"}, "203.0.113.9", true},
		{[]string{"203.0.113.0/24"}, "192.168.1.20", false},
		{[]string{"100.64.0.0/10"}, "100.101.102.103", true},
	} {
		if got := (Settings{LocalNetworks: c.nets}).IsLocal(addr(c.ip)); got != c.local {
			t.Errorf("%s with local networks %v: %v", c.ip, c.nets, got)
		}
	}
}
