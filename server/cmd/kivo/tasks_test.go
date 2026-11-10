package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/identify"
)

// TestTaskNames checks the tasks the server lists against the names used
// outside it. The OpenAPI spec's TaskName enum must match in the order the
// server lists them. The web client's task names must match too, in any
// order, since the web client lists them in its own order.
func TestTaskNames(t *testing.T) {
	dir := t.TempDir()
	boot := config.DefaultBootstrap()
	boot.DataDir, boot.CacheDir, boot.BackupDir = filepath.Join(dir, "data"), filepath.Join(dir, "cache"), filepath.Join(dir, "backups")
	boot.FFmpeg, boot.FFprobe = filepath.Join(dir, "ffmpeg"), filepath.Join(dir, "ffprobe")
	bus := events.NewBus()
	s, err := wire(context.Background(), boot, health.NewLogs(10, bus), bus)
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()

	rows, err := s.runner.Tasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range rows {
		// The tasks list shows identification on the scan's row.
		if r.Name != identify.TaskIdentifyMedia {
			names = append(names, r.Name)
		}
	}
	spec := read(t, "../../internal/api/openapi.yaml")
	same(t, "the spec's TaskName enum", names, specEnum(t, spec, "TaskName"))
	web := labelKeys(t, read(t, "../../web/src/features/admin/useAdminLabels.ts"), "taskNames")
	same(t, "the web's taskNames", slices.Sorted(slices.Values(names)), slices.Sorted(slices.Values(web)))
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// same reports the names that are in only one of want and got. If both have
// the same names, it reports when they're in a different order.
func same(t *testing.T, what string, want, got []string) {
	t.Helper()
	if slices.Equal(want, got) {
		return
	}
	differ := false
	for _, n := range want {
		if !slices.Contains(got, n) {
			t.Errorf("%s: missing %s", what, n)
			differ = true
		}
	}
	for _, n := range got {
		if !slices.Contains(want, n) {
			t.Errorf("%s: %s isn't the server's", what, n)
			differ = true
		}
	}
	if !differ {
		t.Errorf("%s: in the order %v, want %v", what, got, want)
	}
}

// specEnum reads a component schema's enum from the spec, written as
// "enum: [a, b, ...]" over one or more lines.
func specEnum(t *testing.T, spec, schema string) []string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^    ` + schema + `:\n(?:      .*\n)*?      enum: \[([^\]]*)\]`).FindStringSubmatch(spec)
	if m == nil {
		t.Fatalf("no %s enum in the spec", schema)
	}
	var out []string
	for v := range strings.SplitSeq(m[1], ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// labelKeys reads the keys of a labels record in the web's source:
// `const name: Record<string, string> = { "key": ..., };`.
func labelKeys(t *testing.T, src, name string) []string {
	t.Helper()
	m := regexp.MustCompile(`(?s)const ` + name + `: Record<string, string> = \{(.*?)\n  \};`).FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("no %s in the web's labels", name)
	}
	var out []string
	for _, k := range regexp.MustCompile(`(?m)^\s*"?([a-z-]+)"?:`).FindAllStringSubmatch(m[1], -1) {
		out = append(out, k[1])
	}
	return out
}
