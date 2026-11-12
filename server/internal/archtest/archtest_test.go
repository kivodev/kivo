package archtest

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRepoLayers(t *testing.T) {
	for _, v := range checkLayers(layers) {
		t.Error(v)
	}
}

func TestRepoImports(t *testing.T) {
	files, err := readFiles("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range checkImports(files, layers) {
		t.Error(v)
	}
	for _, v := range checkCycles(files) {
		t.Error(v)
	}
	for _, v := range checkSubpackages(files) {
		t.Error(v)
	}
	for _, v := range checkPure(files, pure) {
		t.Error(v)
	}
}

func TestRepoOutbound(t *testing.T) {
	vs, err := checkOutbound("..", outbound)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		t.Error(v)
	}
}

func TestRepoWrites(t *testing.T) {
	vs, err := checkWrites("..", owns)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		t.Error(v)
	}
}

func TestRepoTransactions(t *testing.T) {
	vs, err := checkTransactions("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		t.Error(v)
	}
}

func TestEveryComponentHasAPackage(t *testing.T) {
	for _, l := range layers {
		for _, c := range l.Components {
			if _, err := os.Stat(filepath.Join("..", c)); err != nil {
				t.Errorf("component %s has no package: %v", c, err)
			}
		}
	}
	if _, err := os.Stat(filepath.Join("..", egress)); err != nil {
		t.Errorf("%s has no package: %v", egress, err)
	}
}

func TestCheckLayersFindsTwice(t *testing.T) {
	vs := checkLayers([]layer{{"low", []string{"a", "b"}}, {"high", []string{"c", "a"}}})
	if len(vs) != 1 || !strings.Contains(vs[0].Msg, "a is in both low and high") {
		t.Fatalf("got %v, want a in two layers", vs)
	}
}

func TestCheckCyclesFindsCycle(t *testing.T) {
	files := []goFile{
		{Path: "a/a.go", Imports: []string{module + "/internal/b", module + "/internal/a/sub"}},
		{Path: "b/b.go", Imports: []string{module + "/internal/c"}},
		{Path: "c/sub/c.go", Imports: []string{module + "/internal/a", "fmt"}},
		{Path: "d/d.go", Imports: []string{module + "/internal/a"}},
	}
	vs := checkCycles(files)
	if len(vs) != 1 || vs[0].Msg != "cycle: a → b → c → a" {
		t.Fatalf("got %v, want the cycle a → b → c → a only", vs)
	}
}

func TestCheckImportsFindsUpwardImport(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "catalog/catalog.go", `package catalog
import _ "github.com/mmcdole/kivo/internal/store"
import _ "github.com/mmcdole/kivo/internal/users"
import _ "github.com/mmcdole/kivo/internal/id"
import _ "github.com/mmcdole/kivo/internal/catalog/sub"`)
	write(t, dir, "catalog/sub/sub.go", `package sub`)
	write(t, dir, "users/users.go", `package users`)
	write(t, dir, "store/store.go", `package store
import _ "github.com/mmcdole/kivo/internal/scan"`)
	write(t, dir, "scan/scan.go", `package scan
import _ "github.com/mmcdole/kivo/internal/catalog"`)
	write(t, dir, "scan/scan_test.go", `package scan
import _ "github.com/mmcdole/kivo/internal/api"`)
	write(t, dir, "id/id.go", `package id
import _ "github.com/mmcdole/kivo/internal/catalog"`)
	write(t, dir, "media/probe.go", `package media
import _ "github.com/mmcdole/kivo/internal/media/ffmpeg"`)
	write(t, dir, "media/ffmpeg/run.go", `package ffmpeg
import _ "github.com/mmcdole/kivo/internal/media"`)
	write(t, dir, "nowhere/n.go", `package nowhere`)
	write(t, dir, "nowhere/m.go", `package nowhere`)
	layers := []layer{
		{"helpers", []string{"id"}},
		{"domain", []string{"store", "media", "users", "catalog"}},
		{"features", []string{"scan"}},
	}

	files, err := readFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	vs := checkImports(files, layers)
	got := map[string]bool{}
	for _, v := range vs {
		got[v.File] = true
	}
	if len(vs) != 4 || !got["store/store.go"] || !got["id/id.go"] || !got["media/probe.go"] || !got["layers"] {
		t.Fatalf("got %v, want violations in store/store.go, id/id.go, media/probe.go and nowhere's layer only", vs)
	}
}

func TestCheckSubpackagesFindsReach(t *testing.T) {
	files := []goFile{
		{Path: "api/a.go", Pkg: "api", Imports: []string{
			module + "/internal/markers/detect",
			module + "/internal/media/container",
			module + "/internal/api/internal/gen",
			module + "/internal/catalog",
		}},
		{Path: "scan/s.go", Pkg: "scan", Imports: []string{module + "/internal/media/container"}},
		{Path: "query/q.go", Pkg: "query", Imports: []string{module + "/internal/catalog", module + "/internal/catalog/dbq"}},
	}
	vs := checkSubpackages(files)
	if len(vs) != 3 || vs[0].File != "api/a.go" || !strings.Contains(vs[0].Msg, "markers/detect") || !strings.Contains(vs[1].Msg, "media/container") ||
		vs[2].File != "query/q.go" || !strings.Contains(vs[2].Msg, "catalog/dbq") {
		t.Fatalf("got %v, want violations for api's markers/detect and media/container and query's catalog/dbq only", vs)
	}
}

func TestCheckOutboundFindsCall(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "api/api.go", `package api

import "net/http"

func serve(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", http.StatusTeapot) }`)
	write(t, dir, "catalog/fetch.go", `package catalog

import (
	"net"
	web "net/http"
)

func fetch() {
	c := &web.Client{}
	_, _ = web.Get("https://example.com")
	_, _ = net.Dial("tcp", "example.com:80")
	_ = c
}`)
	write(t, dir, "catalog/fetch_test.go", `package catalog

import "net/http"

var _ = http.DefaultClient`)
	write(t, dir, "egress/egress.go", `package egress

import "net/http"

var c = &http.Client{}`)

	vs, err := checkOutbound(dir, outbound)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range vs {
		got = append(got, v.File)
	}
	if want := []string{"catalog/fetch.go:9", "catalog/fetch.go:10", "catalog/fetch.go:11"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", vs, want)
	}
}

func TestCheckPureFindsImport(t *testing.T) {
	files := []goFile{
		{Path: "markers/detect/a.go", Pkg: "markers/detect", Imports: []string{
			"time", "path/filepath", module + "/internal/media", module + "/internal/fault",
		}},
		{Path: "markers/detect/b.go", Pkg: "markers/detect", Imports: []string{
			"os", "os/exec", "database/sql", module + "/internal/store", module + "/internal/media/ffmpeg",
		}, Now: []int{12}},
		{Path: "markers/a.go", Pkg: "markers", Imports: []string{"os", module + "/internal/store"}},
	}
	vs := checkPure(files, map[string][]string{"markers/detect": {"media"}, "rules": {}})
	var got []string
	for _, v := range vs {
		got = append(got, v.File)
	}
	if want := slices.Repeat([]string{"markers/detect/b.go"}, 6); !slices.Equal(got, append(want, "pure")) {
		t.Fatalf("got %v, want b.go's five imports, its clock read and rules' missing package", vs)
	}
}

func TestCheckWritesFindsForeignWrite(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "catalog/queries/ok.sql", `-- name: Put :exec
INSERT INTO work (id) VALUES (?) ON CONFLICT (id) DO UPDATE SET title = excluded.title;
-- name: Del :exec
DELETE FROM media_file WHERE id = ?;`)
	write(t, dir, "catalog/queries/bad.sql", `-- name: Bad :exec
UPDATE user_state SET played = 1;
-- name: Due :exec
UPDATE work SET title = ?, refreshed_at = NULL WHERE id = ?;
-- name: Upsert :exec
INSERT INTO work (id) VALUES (?) ON CONFLICT (id) DO UPDATE SET refresh_error = NULL;`)
	write(t, dir, "metadata/queries/ok.sql", `-- name: Refreshed :exec
UPDATE work SET refreshed_at = ?, refresh_error = (SELECT NULL FROM work WHERE id = ?) WHERE id = ?;`)
	write(t, dir, "metadata/queries/bad.sql", `-- name: Retitle :exec
UPDATE work SET refreshed_at = ?, title = ? WHERE id = ?;
-- name: Add :exec
INSERT INTO work (id, refreshed_at) VALUES (?, ?);`)
	write(t, dir, "query/queries/read.sql", `-- name: Home :many
WITH x AS (SELECT 1) SELECT * FROM x;`)
	write(t, dir, "query/queries/bad.sql", `-- name: Sneaky :exec
INSERT INTO play_event (id) VALUES (?);`)

	vs, err := checkWrites(dir, owns)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range vs {
		got = append(got, v.File)
	}
	slices.Sort(got)
	want := []string{"catalog/queries/bad.sql", "catalog/queries/bad.sql", "catalog/queries/bad.sql",
		"metadata/queries/bad.sql", "metadata/queries/bad.sql", "query/queries/bad.sql"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want three in catalog's bad.sql, two in metadata's and one in query's", vs)
	}
}

func TestCheckTransactionsFindsControl(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "store/tx.go", `package store

func (db *DB) Write() {
	tx, _ := db.writer.BeginTx(ctx, nil)
	_ = tx.Commit()
}`)
	write(t, dir, "catalog/catalog.go", `package catalog

func (c *Catalog) write() {
	tx, _ := c.db.Reader().BeginTx(ctx, nil)
	defer tx.Rollback()
	_ = c.db.Write(ctx, fn)
}`)
	write(t, dir, "catalog/catalog_test.go", `package catalog

func fixture() { _ = tx.Commit() }`)

	vs, err := checkTransactions(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range vs {
		got = append(got, v.File)
	}
	if want := []string{"catalog/catalog.go:4", "catalog/catalog.go:5"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", vs, want)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
