package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"maps"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/scan"
	"github.com/mmcdole/kivo/internal/tasks"
	"github.com/mmcdole/kivo/internal/users"
)

type list struct {
	Items   []map[string]any
	Next    string
	Total   *int
	Letters []struct {
		Letter string
		Offset int
	}
}

func TestSetupAndSessions(t *testing.T) {
	e := newEnv(t, serving{})
	st := decode[map[string]any](t, e.do("GET", "/api/v1/setup", "", nil))
	if st["required"] != false || st["serverName"] == "" {
		t.Errorf("setup state %v", st)
	}
	w := e.do("POST", "/api/v1/setup", "", map[string]string{"username": "again", "password": "long enough password"})
	if w.Code != http.StatusConflict || problemCode(w) != "alreadySetUp" {
		t.Errorf("second setup: %d %s", w.Code, problemCode(w))
	}
	w = e.do("POST", "/api/v1/sessions", "", map[string]string{"username": "admin", "password": "wrong password", "kind": "app"})
	if w.Code != http.StatusUnauthorized || problemCode(w) != "invalidCredentials" {
		t.Errorf("wrong password: %d %s", w.Code, problemCode(w))
	}
	w = e.do("POST", "/api/v1/sessions", "", map[string]string{"username": "admin", "password": adminPassword, "kind": "app", "name": "TV"})
	s := decode[map[string]any](t, w)
	token, _ := s["token"].(string)
	if w.Code != http.StatusCreated || token == "" || len(w.Result().Cookies()) != 0 || s["kind"] != "app" || s["name"] != "TV" || s["current"] != true {
		t.Fatalf("app sign-in: %d %v", w.Code, s)
	}
	cur := decode[map[string]any](t, e.do("GET", "/api/v1/sessions/current", token, nil))
	if cur["id"] != s["id"] || cur["name"] != "TV" || cur["current"] != true || cur["lastUsedAt"] == nil || cur["token"] != nil {
		t.Errorf("current session %v", cur)
	}
	if me := decode[map[string]any](t, e.do("GET", "/api/v1/users/me", token, nil)); me["username"] != "admin" || me["admin"] != true {
		t.Errorf("me %v", me)
	}
	if w := e.do("DELETE", "/api/v1/sessions/current", token, nil); w.Code != http.StatusNoContent {
		t.Errorf("sign out: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/sessions/current", token, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("after signing out: %d", w.Code)
	}
}

func TestSettings(t *testing.T) {
	e := newEnv(t, serving{})
	w := e.do("PATCH", "/api/v1/settings", e.adminToken, map[string]any{"serverName": "Den", "playedThreshold": 0.8})
	st := decode[map[string]any](t, w)
	if w.Code != http.StatusOK || st["serverName"] != "Den" || st["playedThreshold"] != 0.8 {
		t.Errorf("update: %d %v", w.Code, st)
	}
	w = e.do("PATCH", "/api/v1/settings", e.adminToken, map[string]any{
		"fileWatching": false, "maintenanceStartHour": 23, "maintenanceEndHour": 4, "previewsAtOnce": 6,
		"markersAtOnce": 3,
	})
	st = decode[map[string]any](t, w)
	if w.Code != http.StatusOK || st["fileWatching"] != false || st["maintenanceStartHour"] != 23.0 || st["maintenanceEndHour"] != 4.0 ||
		st["previewsAtOnce"] != 6.0 || st["markersAtOnce"] != 3.0 {
		t.Errorf("update: %d %v", w.Code, st)
	}
	// The OpenSubtitles password is written, never read back.
	w = e.do("PATCH", "/api/v1/settings", e.adminToken, map[string]any{"openSubtitlesPassword": "hunter22"})
	if st = decode[map[string]any](t, w); st["openSubtitlesPasswordSet"] != true || strings.Contains(w.Body.String(), "hunter22") {
		t.Errorf("password set: %v", st)
	}
	w = e.do("PATCH", "/api/v1/settings", e.adminToken, map[string]any{"openSubtitlesPassword": ""})
	if st = decode[map[string]any](t, w); st["openSubtitlesPasswordSet"] != false {
		t.Errorf("password cleared: %v", st)
	}
	for _, bad := range []map[string]any{{"playedThreshold": 3}, {"maintenanceEndHour": 23}, {"maintenanceStartHour": 24}, {"previewsAtOnce": 0}, {"previewsAtOnce": 9},
		{"markersAtOnce": 0}, {"markersAtOnce": 5}} {
		w = e.do("PATCH", "/api/v1/settings", e.adminToken, bad)
		if w.Code != http.StatusBadRequest || problemCode(w) != codeInvalidRequest {
			t.Errorf("%v: %d %s", bad, w.Code, w.Body.String())
		}
	}
	if got := decode[map[string]any](t, e.do("GET", "/api/v1/setup", "", nil))["serverName"]; got != "Den" {
		t.Errorf("public server name %v", got)
	}
}

func TestCurrentUserPlayback(t *testing.T) {
	e := newEnv(t, serving{})
	pb := decode[map[string]any](t, e.do("GET", "/api/v1/users/me", e.adminToken, nil))["playback"]
	want := map[string]any{"subtitleMode": "forced", "rememberTracks": true, "autoplayNext": true, "bookSpeed": 1.0}
	if !maps.Equal(pb.(map[string]any), want) {
		t.Errorf("defaults %v", pb)
	}

	// Settings merge, so only the fields sent change.
	w := e.do("PATCH", "/api/v1/users/me", e.adminToken, map[string]any{"playback": map[string]any{
		"audioLanguage": "ja", "subtitleLanguage": "en-gb", "subtitleMode": "smart", "rememberTracks": false, "autoplayNext": false,
		"bookSpeed": 1.5,
	}})
	want = map[string]any{
		"audioLanguage": "ja", "subtitleLanguage": "en-GB", "subtitleMode": "smart", "rememberTracks": false, "autoplayNext": false,
		"bookSpeed": 1.5,
	}
	if got := decode[map[string]any](t, w)["playback"]; w.Code != http.StatusOK || !maps.Equal(got.(map[string]any), want) {
		t.Errorf("update: %d %v", w.Code, got)
	}
	// An empty language is the default again.
	e.do("PATCH", "/api/v1/users/me", e.adminToken, map[string]any{"playback": map[string]any{"audioLanguage": ""}})
	cur := decode[map[string]any](t, e.do("GET", "/api/v1/users/me", e.adminToken, nil))
	if pb := cur["playback"].(map[string]any); pb["audioLanguage"] != nil || pb["subtitleMode"] != "smart" {
		t.Errorf("playback %v", pb)
	}

	for _, bad := range []map[string]any{{"subtitleMode": "sometimes"}, {"subtitleLanguage": "?"}, {"bookSpeed": 4}} {
		w := e.do("PATCH", "/api/v1/users/me", e.adminToken, map[string]any{"playback": bad})
		if w.Code != http.StatusBadRequest || problemCode(w) != codeInvalidRequest {
			t.Errorf("%v: %d %s", bad, w.Code, w.Body.String())
		}
	}
}

func TestCurrentUserLibraries(t *testing.T) {
	e := newEnv(t, serving{})
	pinned := func() any {
		items := decode[map[string]any](t, e.do("GET", "/api/v1/libraries", e.adminToken, nil))["items"].([]any)
		return items[0].(map[string]any)["pinned"]
	}
	if got := pinned(); got != true {
		t.Errorf("default pinned = %v", got)
	}
	w := e.do("PATCH", "/api/v1/users/me", e.adminToken, map[string]any{
		"libraries": []map[string]any{{"id": e.lib.String(), "pinned": false}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("set: %d %s", w.Code, w.Body.String())
	}
	if got := pinned(); got != false {
		t.Errorf("pinned after unpinning = %v", got)
	}
	w = e.do("PATCH", "/api/v1/users/me", e.adminToken, map[string]any{
		"libraries": []map[string]any{{"id": id.New().String(), "pinned": true}},
	})
	if w.Code != http.StatusBadRequest || problemCode(w) != codeInvalidRequest {
		t.Errorf("unknown library: %d %s", w.Code, w.Body.String())
	}
}

// A request without the body its operation requires is rejected by the
// generated decoder before any handler runs.
func TestMissingBody(t *testing.T) {
	e := newEnv(t, serving{})
	for _, path := range []string{"/api/v1/users", "/api/v1/plays", "/api/v1/device-codes"} {
		w := e.do("POST", path, e.adminToken, nil)
		if w.Code != http.StatusBadRequest || problemCode(w) != codeInvalidRequest {
			t.Errorf("POST %s without a body: %d %s", path, w.Code, w.Body.String())
		}
	}
	w := e.do("PATCH", "/api/v1/users/me", e.adminToken, nil)
	if w.Code != http.StatusBadRequest || problemCode(w) != codeInvalidRequest {
		t.Errorf("PATCH /users/me without a body: %d %s", w.Code, w.Body.String())
	}
}

func TestUsersAndInvites(t *testing.T) {
	e := newEnv(t, serving{})
	admin := decode[map[string]any](t, e.do("GET", "/api/v1/users/me", e.adminToken, nil))["id"].(string)

	// Saving needs a choice of libraries.
	w := e.do("POST", "/api/v1/users", e.adminToken, map[string]any{
		"username": "ben", "password": "a good password", "libraries": map[string]any{"all": false, "libraryIds": []string{}},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("no libraries: %d %s", w.Code, w.Body.String())
	}
	w = e.do("POST", "/api/v1/users", e.adminToken, map[string]any{
		"username": "ben", "password": "a good password", "libraries": map[string]any{"all": false, "libraryIds": []string{e.lib.String()}},
		"limits": map[string]any{"maxStreams": 2}, "parental": map[string]any{"maxAge": 13, "blockUnrated": true},
	})
	ben := decode[map[string]any](t, w)
	if w.Code != http.StatusCreated || ben["admin"] != false || ben["limits"].(map[string]any)["maxStreams"] != 2.0 ||
		ben["parental"].(map[string]any)["maxAge"] != 13.0 || ben["libraries"].(map[string]any)["libraryIds"].([]any)[0] != e.lib.String() {
		t.Fatalf("create: %d %v", w.Code, ben)
	}
	if w := e.do("POST", "/api/v1/users", e.adminToken, map[string]any{
		"username": "BEN", "password": "a good password", "libraries": map[string]any{"all": true, "libraryIds": []string{}},
	}); w.Code != http.StatusConflict || problemCode(w) != "nameTaken" {
		t.Errorf("a taken name: %d %s", w.Code, problemCode(w))
	}
	w = e.do("POST", "/api/v1/sessions", "", map[string]string{"username": "ben", "password": "a good password", "kind": "app"})
	if w.Code != http.StatusCreated {
		t.Errorf("ben signs in: %d", w.Code)
	}
	// Ben sees his own limits.
	me := decode[map[string]any](t, e.do("GET", "/api/v1/users/me", decode[map[string]any](t, w)["token"].(string), nil))
	if me["limits"].(map[string]any)["maxStreams"] != 2.0 {
		t.Errorf("ben's own limits: %v", me)
	}

	// Edit one section. The rest stays as it was.
	id := ben["id"].(string)
	w = e.do("PATCH", "/api/v1/users/"+id, e.adminToken, map[string]any{"limits": map[string]any{}, "disabled": true})
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["disabled"] != true || len(got["limits"].(map[string]any)) != 0 ||
		got["parental"].(map[string]any)["blockUnrated"] != true {
		t.Errorf("update: %d %v", w.Code, got)
	}
	if w := e.do("POST", "/api/v1/users/"+id+"/set-password", e.adminToken, map[string]any{"password": "another password"}); w.Code != http.StatusNoContent {
		t.Errorf("set password: %d", w.Code)
	}
	if got := decode[map[string]any](t, e.do("GET", "/api/v1/users", e.adminToken, nil))["items"].([]any); len(got) != 3 {
		t.Errorf("users: %v", got)
	}

	// The guard rails. An admin can't do this to their own user, or to the
	// last admin.
	for _, body := range []map[string]any{{"admin": false}, {"disabled": true}} {
		if w := e.do("PATCH", "/api/v1/users/"+admin, e.adminToken, body); w.Code != http.StatusConflict || problemCode(w) != codeOwnUser {
			t.Errorf("%v on oneself: %d %s", body, w.Code, problemCode(w))
		}
	}
	if w := e.do("DELETE", "/api/v1/users/"+admin, e.adminToken, nil); w.Code != http.StatusConflict || problemCode(w) != codeOwnUser {
		t.Errorf("deleting oneself: %d %s", w.Code, problemCode(w))
	}
	if w := e.do("DELETE", "/api/v1/users/"+id, e.adminToken, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/users/"+id, e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("deleted: %d", w.Code)
	}

	// An invite link, previewed and accepted without credentials.
	w = e.do("POST", "/api/v1/invites", e.adminToken, map[string]any{
		"libraries": map[string]any{"all": true, "libraryIds": []string{}}, "limits": map[string]any{"maxStreams": 2},
		"expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	})
	inv := decode[map[string]any](t, w)
	token, _ := inv["token"].(string)
	if w.Code != http.StatusCreated || token == "" || inv["createdBy"] != "admin" {
		t.Fatalf("create invite: %d %v", w.Code, inv)
	}
	if got := decode[map[string]any](t, e.do("GET", "/api/v1/invites", e.adminToken, nil))["items"].([]any); len(got) != 1 || got[0].(map[string]any)["token"] != nil {
		t.Errorf("invites: %v", got)
	}
	if got := decode[map[string]any](t, e.do("GET", "/api/v1/invite-links/"+token, "", nil)); got["invitedBy"] != "admin" {
		t.Errorf("invite link: %v", got)
	}
	w = e.do("POST", "/api/v1/invite-links/"+token+"/accept", "", map[string]any{"username": "cara", "password": "a good password", "kind": "app"})
	s := decode[map[string]any](t, w)
	if w.Code != http.StatusCreated || s["token"] == nil {
		t.Fatalf("accept: %d %v", w.Code, s)
	}
	if me := decode[map[string]any](t, e.do("GET", "/api/v1/users/me", s["token"].(string), nil)); me["username"] != "cara" {
		t.Errorf("accepted as %v", me)
	}
	if w := e.do("GET", "/api/v1/invite-links/"+token, "", nil); w.Code != http.StatusNotFound {
		t.Errorf("a used link: %d", w.Code)
	}
}

func TestLibraries(t *testing.T) {
	e := newEnv(t, serving{})
	libs := decode[list](t, e.do("GET", "/api/v1/libraries", e.adminToken, nil))
	if len(libs.Items) != 1 || libs.Items[0]["workCount"] != float64(2) || libs.Items[0]["folders"] == nil || libs.Total != nil || libs.Next != "" {
		t.Fatalf("libraries %+v", libs)
	}
	dir := t.TempDir()
	w := e.do("POST", "/api/v1/libraries", e.adminToken, map[string]any{"name": "Shows", "kind": "shows", "folders": []string{dir}, "previews": "nightlyAndAdded"})
	lib := decode[map[string]any](t, w)
	if w.Code != http.StatusCreated || lib["previews"] != "nightlyAndAdded" || lib["subtitles"] != "nightlyAndAdded" || lib["useLocalMetadata"] != true {
		t.Fatalf("create: %d %v", w.Code, lib)
	}
	path := "/api/v1/libraries/" + lib["id"].(string)
	w = e.do("PATCH", path, e.adminToken, map[string]any{"name": "TV", "folders": []string{dir, t.TempDir()}})
	lib = decode[map[string]any](t, w)
	if w.Code != http.StatusOK || lib["name"] != "TV" || len(lib["folders"].([]any)) != 2 {
		t.Fatalf("update: %d %v", w.Code, lib)
	}
	if w := e.do("PATCH", path, e.adminToken, map[string]any{"previews": "sometimes"}); w.Code != http.StatusBadRequest {
		t.Errorf("bad previews: %d", w.Code)
	}
	if w := e.do("PATCH", path, e.adminToken, map[string]any{"subtitles": "nightly"}); w.Code != http.StatusOK || decode[map[string]any](t, w)["subtitles"] != "nightly" {
		t.Errorf("subtitles nightly: %d", w.Code)
	}
	if w := e.do("PATCH", path, e.adminToken, map[string]any{"name": strings.Repeat("é", 101)}); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "tooLong") {
		t.Errorf("a name over 100 characters: %d %s", w.Code, w.Body.String())
	}
	w = e.do("POST", path+"/scan", e.adminToken, map[string]any{"full": true})
	acc := decode[map[string]any](t, w)
	if asked := e.asks.of(scan.TaskScanLibraries); w.Code != http.StatusAccepted || len(acc) != 0 || len(asked) == 0 || asked[len(asked)-1].String() != lib["id"] {
		t.Errorf("scan: %d %v, asked for %v", w.Code, acc, asked)
	}
	f := decode[map[string]any](t, e.do("GET", "/api/v1/libraries/"+e.lib.String()+"/facets", e.adminToken, nil))
	if years := f["years"].([]any); len(years) != 1 || years[0] != float64(1995) {
		t.Errorf("facets %v", f)
	}
	// Heat is 1080p SDR, in no collection.
	if r, ok := f["resolutions"].([]any); !ok || len(r) != 1 || r[0] != "1080p" || f["collections"] != false {
		t.Errorf("facets %v", f)
	}
	if h, ok := f["dynamicRanges"].([]any); !ok || len(h) != 0 {
		t.Errorf("facets %v", f)
	}
	w = e.do("POST", path+"/refresh-metadata", e.adminToken, nil)
	if acc := decode[map[string]any](t, w); w.Code != http.StatusAccepted || acc["queued"] != 0.0 {
		t.Errorf("refresh library: %d %v", w.Code, acc)
	}
	// Dropping a folder hides it at once. The library changes task removes
	// it.
	var dropped string
	for _, f := range lib["folders"].([]any) {
		if f := f.(map[string]any); f["path"] != dir {
			dropped = f["id"].(string)
		}
	}
	w = e.do("PATCH", path, e.adminToken, map[string]any{"folders": []string{dir}})
	lib = decode[map[string]any](t, w)
	if w.Code != http.StatusOK || len(lib["folders"].([]any)) != 1 {
		t.Fatalf("drop a folder: %d %v", w.Code, lib)
	}
	if asked := e.asks.of(catalog.TaskLibraryChanges); len(asked) != 1 || asked[0].String() != dropped {
		t.Errorf("library changes asked for %v, want the folder %s", asked, dropped)
	}

	// Deleting a library answers at once, and the library changes task
	// deletes it. The library is gone from every list straight away.
	w = e.do("DELETE", path, e.adminToken, nil)
	if acc := decode[map[string]any](t, w); w.Code != http.StatusAccepted || len(acc) != 0 {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	if asked := e.asks.of(catalog.TaskLibraryChanges); len(asked) != 2 || asked[1].String() != lib["id"] {
		t.Errorf("library changes asked for %v, want the library", asked)
	}
	if w := e.do("GET", path, e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("deleted library: %d", w.Code)
	}
	if libs := decode[list](t, e.do("GET", "/api/v1/libraries", e.adminToken, nil)); len(libs.Items) != 1 {
		t.Errorf("libraries while one is being deleted: %+v", libs)
	}
	if w := e.do("DELETE", path, e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("delete again: %d", w.Code)
	}
}

// A collection's page is read by its ID. Anyone who sees one of its works
// can read it, and it lists the works they see.
func TestCollection(t *testing.T) {
	e := newEnv(t, serving{})
	heat, err := e.b.Catalog.Work(ctx, e.heat.Work)
	if err != nil {
		t.Fatal(err)
	}
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.Catalog.UpdateMetadata(ctx, tx, heat.ID, catalog.WorkMetadata{
			Title: heat.Title, Year: heat.Year, Collection: &catalog.CollectionRef{Key: "tmdb:1", Name: "Heatwave Collection"},
		})
	})
	movie := decode[map[string]any](t, e.do("GET", "/api/v1/works/"+e.heat.Work.String(), e.adminToken, nil))
	colls := movie["collections"].([]any)
	if len(colls) != 1 {
		t.Fatalf("the movie's collections: %v", colls)
	}
	if c := colls[0].(map[string]any); c["name"] != "Heatwave Collection" || c["workCount"] != float64(1) || c["source"] != "provider" {
		t.Errorf("the movie's collection: %v", c)
	}
	coll := colls[0].(map[string]any)["id"].(string)

	w := e.do("GET", "/api/v1/collections/"+coll, e.adminToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("collection: %d %s", w.Code, w.Body.String())
	}
	got := decode[map[string]any](t, w)
	if items := got["items"].([]any); got["name"] != "Heatwave Collection" || len(items) != 1 || items[0].(map[string]any)["id"] != e.heat.Work.String() {
		t.Errorf("collection: %v", got)
	}
	page := decode[list](t, e.do("GET", "/api/v1/libraries/"+e.lib.String()+"/collections?offset=0&limit=5", e.adminToken, nil))
	if len(page.Items) != 1 || page.Items[0]["id"] != coll || *page.Total != 1 {
		t.Errorf("the library's collections: %+v", page)
	}
	if w := e.do("GET", "/api/v1/collections/"+coll, e.nobodyToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("a collection of works the user can't see: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/works/"+coll, e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("a collection as a work: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/collections/"+id.New().String(), e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("no such collection: %d", w.Code)
	}
}

// TestBrowseShowsByLastEpisodeAdded checks that a shows library's grid
// puts the series with the newest episode first.
func TestBrowseShowsByLastEpisodeAdded(t *testing.T) {
	e := newEnv(t, serving{})
	lib, folder := e.b.Library(catalog.Shows)
	older, newer := e.b.Series("Older Show"), e.b.Series("Newer Show")
	e.b.Episode(folder, newer, 1, 1)
	time.Sleep(2 * time.Millisecond)
	e.b.Episode(folder, older, 1, 1)
	time.Sleep(2 * time.Millisecond)
	e.b.Episode(folder, newer, 1, 2)
	w := e.do("GET", "/api/v1/libraries/"+lib.String()+"/works?sort=lastEpisodeAddedAt", e.adminToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("sort=lastEpisodeAddedAt: %d %s", w.Code, w.Body.String())
	}
	page := decode[list](t, w)
	if len(page.Items) != 2 || page.Items[0]["id"] != newer.String() || page.Items[1]["id"] != older.String() {
		t.Errorf("sort=lastEpisodeAddedAt: %+v, want %s then %s", page.Items, newer, older)
	}
}

func TestBrowseAndDetail(t *testing.T) {
	e := newEnv(t, serving{})
	grid := "/api/v1/libraries/" + e.lib.String() + "/works"
	page := decode[list](t, e.do("GET", grid+"?sort=title&limit=10", e.adminToken, nil))
	if *page.Total != 2 || len(page.Items) != 2 || len(page.Letters) == 0 || page.Letters[0].Offset != 0 {
		t.Fatalf("browse %+v", page)
	}
	// A page at an offset starts the grid from there. It has the total but
	// not the letter rail, which only the first page has.
	at := decode[list](t, e.do("GET", grid+"?offset=1&limit=1", e.adminToken, nil))
	if len(at.Items) != 1 || at.Items[0]["id"] != page.Items[1]["id"] || at.Total == nil || *at.Total != 2 || at.Letters != nil || at.Next != "" {
		t.Errorf("offset 1: %+v", at)
	}
	if second := page.Letters[1]; second.Offset != 1 {
		t.Errorf("letters %+v", page.Letters)
	}
	if w := e.do("GET", grid+"?offset=-1", e.adminToken, nil); w.Code != http.StatusBadRequest {
		t.Errorf("offset -1: %d", w.Code)
	}
	for _, q := range []string{"?yearFrom=1990&yearTo=1999", "?yearFrom=1990", "?resolution=1080p&hdr=hdr", "?played=unplayed", "?match=unmatched", "?genre=Crime&genre=Drama"} {
		if w := e.do("GET", grid+q, e.adminToken, nil); w.Code != http.StatusOK {
			t.Errorf("%s: %d %s", q, w.Code, w.Body.String())
		}
	}
	heat, err := e.b.Catalog.Work(ctx, e.heat.Work)
	if err != nil {
		t.Fatal(err)
	}
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.Catalog.UpdateMetadata(ctx, tx, heat.ID, catalog.WorkMetadata{
			Title: heat.Title, Year: heat.Year, Community: &catalog.CommunityRating{Rating: 7.5, Votes: 900},
			TrailerURL: "https://www.youtube.com/watch?v=abc",
		})
	})
	detail := decode[map[string]any](t, e.do("GET", "/api/v1/works/"+e.heat.Work.String(), e.adminToken, nil))
	if movie, _ := detail["movie"].(map[string]any); movie["trailerUrl"] != "https://www.youtube.com/watch?v=abc" {
		t.Errorf("detail's movie: %v", detail["movie"])
	}
	for _, sort := range []string{"rating", "myRating"} {
		page := decode[list](t, e.do("GET", grid+"?sort="+sort, e.adminToken, nil))
		if len(page.Items) != 2 {
			t.Fatalf("sort=%s: %+v", sort, page)
		}
		if sort == "rating" && (page.Items[0]["communityRating"] != 7.5 || page.Items[0]["communityVotes"] != float64(900)) {
			t.Errorf("sort=rating: first %v", page.Items[0])
		}
	}
	for _, q := range []string{"?yearFrom=2000&yearTo=1999", "?matchState=identifying", "?sort=popularity"} {
		if w := e.do("GET", grid+q, e.adminToken, nil); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", q, w.Code)
		}
	}
	if w := e.do("GET", "/api/v1/libraries/"+id.New().String()+"/works", e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("no such library: %d", w.Code)
	}

	second := e.b.Episode(e.folder, e.series, 1, 2)
	seasons := decode[list](t, e.do("GET", "/api/v1/works/"+e.series.String()+"/children", e.adminToken, nil))
	if len(seasons.Items) != 1 || seasons.Items[0]["season"].(map[string]any)["seriesId"] != e.series.String() {
		t.Fatalf("children %+v", seasons)
	}
	if n := seasons.Items[0]["season"].(map[string]any)["episodeCount"]; n != float64(2) {
		t.Errorf("season episodeCount %v, want 2", n)
	}
	ep := decode[map[string]any](t, e.do("GET", "/api/v1/works/"+e.episode.Work.String(), e.adminToken, nil))["episode"].(map[string]any)
	if ep["nextId"] != second.Work.String() || ep["previousId"] != nil {
		t.Errorf("episode detail %v, want next %s and no previous", ep, second.Work)
	}

	w := e.do("GET", "/api/v1/works/"+e.heat.Work.String(), e.adminToken, nil)
	d := decode[map[string]any](t, w)
	versions, _ := d["versions"].([]any)
	if w.Code != http.StatusOK || d["title"] != "Heatwave Seeded" || len(versions) != 1 || d["lockedFields"] == nil {
		t.Fatalf("detail: %d %v", w.Code, d)
	}
	v := versions[0].(map[string]any)
	if v["resolution"] != "1080p" || v["hdr"] != nil || v["workIds"].([]any)[0] != e.heat.Work.String() || len(v["tracks"].([]any)) != 2 {
		t.Errorf("version %v", v)
	}
	if ms, _ := v["markers"].([]any); len(ms) != 1 || ms[0].(map[string]any)["kind"] != "credits" || ms[0].(map[string]any)["startMs"] != 6_600_000.0 {
		t.Errorf("version markers %v", v["markers"])
	}
	sd := decode[map[string]any](t, e.do("GET", "/api/v1/works/"+e.series.String(), e.adminToken, nil))
	if sd["series"].(map[string]any)["seasonCount"] != float64(1) {
		t.Errorf("series detail %v", sd["series"])
	}

	ratings := decode[map[string]any](t, e.do("GET", "/api/v1/content-ratings", e.adminToken, nil))
	if items := ratings["items"].([]any); len(items) != 11 || items[2].(map[string]any)["name"] != "PG-13" || items[2].(map[string]any)["age"] != float64(13) {
		t.Errorf("content ratings %v", ratings)
	}
	if w := e.do("PATCH", "/api/v1/works/"+e.heat.Work.String(), e.adminToken, map[string]any{"contentRating": "PG-14"}); w.Code != http.StatusBadRequest {
		t.Errorf("an unknown content rating: %d", w.Code)
	}
	w = e.do("PATCH", "/api/v1/works/"+e.heat.Work.String(), e.adminToken, map[string]any{"title": "Heat", "contentRating": "R", "genres": []string{"Crime"}})
	d = decode[map[string]any](t, w)
	if w.Code != http.StatusOK || d["title"] != "Heat" || d["contentRating"] != "R" || !strings.Contains(w.Body.String(), `"lockedFields":["contentRating","genres","title"]`) {
		t.Errorf("edit: %d %s", w.Code, w.Body.String())
	}
	var rank int
	if err := e.b.DB.Reader().QueryRow(`SELECT content_rating_age FROM work WHERE id = ?`, e.heat.Work).Scan(&rank); err != nil || rank != 17 {
		t.Errorf("edit: rank %d, %v", rank, err)
	}
	f := decode[map[string]any](t, e.do("GET", "/api/v1/libraries/"+e.lib.String()+"/facets", e.adminToken, nil))
	if r := f["contentRatings"].([]any); len(r) != 1 || r[0] != "R" {
		t.Errorf("facets %v", f)
	}
	page = decode[list](t, e.do("GET", grid+"?contentRating=R&contentRating=PG", e.adminToken, nil))
	if len(page.Items) != 1 || page.Items[0]["id"] != e.heat.Work.String() || page.Items[0]["contentRating"] != "R" {
		t.Errorf("filter by content rating %+v", page)
	}

	homeRows := func() []string {
		home := decode[map[string][]map[string]any](t, e.do("GET", "/api/v1/home", e.adminToken, nil))
		var ids []string
		for _, r := range home["rows"] {
			ids = append(ids, r["id"].(string))
		}
		return ids
	}
	myList := decode[struct {
		Items []struct {
			ID      string `json:"id"`
			BuiltIn bool   `json:"builtIn"`
		} `json:"items"`
	}](t, e.do("GET", "/api/v1/lists", e.adminToken, nil)).Items[0].ID
	if rows := homeRows(); slices.Contains(rows, "list:"+myList) {
		t.Errorf("home without works on My List %v", rows)
	}
	if w := e.do("POST", "/api/v1/lists/"+myList+"/works", e.adminToken, map[string]any{"workId": e.heat.Work.String()}); w.Code != http.StatusNoContent {
		t.Errorf("add to My List: %d %s", w.Code, w.Body.String())
	}
	if rows := homeRows(); len(rows) < 2 || rows[0] != "list:"+myList || !strings.HasPrefix(rows[1], "recentlyAdded:") {
		t.Errorf("home with My List %v", rows)
	}
	inList := decode[list](t, e.do("GET", "/api/v1/lists/"+myList+"/works", e.adminToken, nil))
	if len(inList.Items) != 1 || inList.Items[0]["id"] != e.heat.Work.String() || *inList.Total != 1 {
		t.Errorf("my list works %+v", inList)
	}
	if w := e.do("POST", "/api/v1/lists/"+myList+"/works", e.adminToken, map[string]any{"workId": e.heat.Work.String()}); w.Code != http.StatusNoContent {
		t.Errorf("adding again: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("POST", "/api/v1/works/"+e.heat.Work.String()+"/refresh-metadata", e.adminToken, nil); w.Code != http.StatusAccepted {
		t.Errorf("refresh: %d %s", w.Code, w.Body.String())
	}
}

func TestSetOrdering(t *testing.T) {
	e := newEnv(t, serving{})
	path := "/api/v1/works/" + e.series.String()
	for _, o := range []string{"sideways", "group:"} {
		if w := e.do("PATCH", path, e.adminToken, map[string]any{"series": map[string]any{"ordering": o}}); w.Code != http.StatusBadRequest {
			t.Errorf("ordering %q: %d %s", o, w.Code, w.Body.String())
		}
	}
	// The seeded series has no TMDb ID, so TMDb lists no group for it.
	w := e.do("PATCH", path, e.adminToken, map[string]any{"title": "Renamed", "series": map[string]any{"ordering": "group:5e5b7b4f"}})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "unknownEpisodeGroup") {
		t.Errorf("an unknown group: %d %s", w.Code, w.Body.String())
	}
	if got := decode[map[string]any](t, e.do("GET", path, e.adminToken, nil)); got["title"] == "Renamed" {
		t.Error("a rejected ordering still saved the other fields")
	}
	if w := e.do("PATCH", path, e.adminToken, map[string]any{"series": map[string]any{"ordering": "dvd"}}); w.Code != http.StatusOK {
		t.Errorf("dvd: %d %s", w.Code, w.Body.String())
	}
}

// A season's played check toggles. Marking it played marks its episodes,
// and the season reads back as played. Unmarking it unmarks them, and the
// season reads back as unplayed.
func TestSeasonPlayedToggles(t *testing.T) {
	e := newEnv(t, serving{})
	e2 := e.b.Episode(e.folder, e.series, 1, 2)
	seasons := decode[list](t, e.do("GET", "/api/v1/works/"+e.series.String()+"/children", e.adminToken, nil))
	if len(seasons.Items) != 1 {
		t.Fatalf("seasons %+v", seasons)
	}
	season := seasons.Items[0]["id"].(string)
	played := func(work string) bool {
		t.Helper()
		w := decode[map[string]any](t, e.do("GET", "/api/v1/works/"+work, e.adminToken, nil))
		return w["userState"].(map[string]any)["played"] == true
	}
	for _, mark := range []bool{true, false} {
		st := decode[map[string]any](t, e.do("PATCH", "/api/v1/works/"+season+"/user-state", e.adminToken, map[string]any{"played": mark}))
		if st["played"] != mark {
			t.Errorf("marked %v: season reads %v", mark, st)
		}
		for _, w := range []string{season, e.series.String(), e.episode.Work.String(), e2.Work.String()} {
			if got := played(w); got != mark {
				t.Errorf("marked %v: %s played %v", mark, w, got)
			}
		}
	}
}

func TestUnmatch(t *testing.T) {
	e := newEnv(t, serving{})
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO work_external_id (provider, kind, ext_id, work_id) VALUES ('tmdb', 'movie', '949', ?)`, e.heat.Work)
		return err
	})
	if w := decode[map[string]any](t, e.do("GET", "/api/v1/works/"+e.heat.Work.String(), e.adminToken, nil)); w["matchState"] != "matched" {
		t.Fatalf("seeded movie %v", w)
	}
	w := decode[map[string]any](t, e.do("POST", "/api/v1/works/"+e.heat.Work.String()+"/unmatch", e.adminToken, nil))
	if w["id"] != e.heat.Work.String() || w["matchState"] != "unmatched" || w["title"] != "Heatwave Seeded" {
		t.Errorf("unmatched work %v", w)
	}
	if xs, _ := w["externalIds"].([]any); len(xs) != 0 {
		t.Errorf("external IDs left: %v", xs)
	}
	// The Unmatched list has it, with its file, for admins only.
	un := decode[list](t, e.do("GET", "/api/v1/unmatched-works", e.adminToken, nil))
	if len(un.Items) != 1 || un.Items[0]["id"] != e.heat.Work.String() || un.Total != nil ||
		!strings.HasSuffix(fmt.Sprint(un.Items[0]["path"]), ".mkv") || un.Items[0]["libraryId"] != e.lib.String() {
		t.Errorf("unmatched list %+v", un)
	}
	if r := e.do("GET", "/api/v1/unmatched-works", e.nobodyToken, nil); r.Code != http.StatusForbidden || problemCode(r) != codeForbidden {
		t.Errorf("a viewer's unmatched list: %d %s", r.Code, r.Body.String())
	}
	if r := e.do("GET", "/api/v1/unmatched-works?after=bogus", e.adminToken, nil); r.Code != http.StatusBadRequest {
		t.Errorf("a bogus cursor: %d", r.Code)
	}
	// The library grid's match filter has it too, for admins only.
	grid := "/api/v1/libraries/" + e.lib.String() + "/works?matchState=unmatched"
	if g := decode[list](t, e.do("GET", grid, e.adminToken, nil)); len(g.Items) != 1 || g.Items[0]["id"] != e.heat.Work.String() {
		t.Errorf("the unmatched filter %+v", g)
	}
	// Refresh metadata on an unmatched work sends its folder back to be
	// identified, without a scan of its library.
	if r := e.do("POST", "/api/v1/works/"+e.heat.Work.String()+"/refresh-metadata", e.adminToken, nil); r.Code != http.StatusAccepted {
		t.Errorf("refresh an unmatched work: %d %s", r.Code, r.Body.String())
	}
	if asked := e.asks.of(identify.TaskIdentifyMedia); !slices.Equal(asked, []id.ID{e.heat.File}) {
		t.Errorf("asked to identify %v, want the unmatched work's file", asked)
	}
	if asked := e.asks.of(scan.TaskScanLibraries); len(asked) != 0 {
		t.Errorf("scans asked for: %v, want none", asked)
	}
	if r := e.do("POST", "/api/v1/works/"+e.episode.Work.String()+"/unmatch", e.adminToken, nil); r.Code != http.StatusBadRequest {
		t.Errorf("unmatch an episode: %d %s", r.Code, r.Body.String())
	}
	if r := e.do("POST", "/api/v1/works/"+id.New().String()+"/unmatch", e.adminToken, nil); r.Code != http.StatusNotFound {
		t.Errorf("unmatch an unknown work: %d", r.Code)
	}
}

// upload sends an artwork upload of kind with the file's bytes.
func (e *env) upload(work id.ID, kind string, data []byte) *httptest.ResponseRecorder {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("type", kind)
	fw, _ := mw.CreateFormFile("file", "poster.png")
	_, _ = fw.Write(data)
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/api/v1/works/"+work.String()+"/artwork", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+e.adminToken)
	w := httptest.NewRecorder()
	e.srv.ServeHTTP(w, r)
	return w
}

func pngOf(w, h int) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, h)))
	return b.Bytes()
}

// Choose artwork. An upload becomes the selected poster and is served
// through the image route. Wrong types and sizes are refused. No response
// carries a provider URL or a path.
func TestArtwork(t *testing.T) {
	e := newEnv(t, serving{})
	path := "/api/v1/works/" + e.heat.Work.String() + "/artwork"
	noURLs := func(w *httptest.ResponseRecorder) {
		t.Helper()
		if body := w.Body.String(); strings.Contains(body, "http") || strings.Contains(body, "/") {
			t.Errorf("a URL or path in %s", body)
		}
	}
	empty := e.do("GET", path, e.adminToken, nil)
	if got := decode[list](t, empty); len(got.Items) != 0 {
		t.Errorf("artwork before any: %+v", got)
	}

	up := e.upload(e.heat.Work, "poster", pngOf(400, 600))
	if up.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", up.Code, up.Body.String())
	}
	noURLs(up)
	art := decode[map[string]any](t, up)
	if art["type"] != "poster" || art["source"] != "upload" || art["selected"] != true || art["width"] != float64(400) {
		t.Errorf("uploaded %v", art)
	}
	listed := e.do("GET", path+"?type=poster", e.adminToken, nil)
	noURLs(listed)
	if got := decode[list](t, listed); len(got.Items) != 1 || got.Items[0]["id"] != art["id"] {
		t.Errorf("listed after upload %+v", got)
	}
	detail := decode[map[string]any](t, e.do("GET", "/api/v1/works/"+e.heat.Work.String(), e.adminToken, nil))
	if poster, _ := detail["images"].(map[string]any)["poster"].(map[string]any); poster["id"] != art["id"] {
		t.Errorf("the work's poster %v, want the upload", detail["images"])
	}
	if w := e.do("GET", "/api/v1/images/"+art["id"].(string)+"?width=240", e.adminToken, nil); w.Code != http.StatusOK {
		t.Errorf("serving the upload: %d", w.Code)
	}
	if w := e.do("POST", path+"/"+art["id"].(string)+"/select", e.adminToken, nil); w.Code != http.StatusOK {
		t.Errorf("selecting the current poster: %d %s", w.Code, w.Body.String())
	}

	for name, c := range map[string]struct {
		kind  string
		data  []byte
		field string
	}{
		"text":          {"poster", []byte("not an image"), "unsupported"},
		"an SVG":        {"poster", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "unsupported"},
		"over 25 MB":    {"poster", append(pngOf(4, 4), make([]byte, 25<<20)...), "tooLarge"},
		"a movie still": {"still", pngOf(4, 4), "invalid"},
	} {
		w := e.upload(e.heat.Work, c.kind, c.data)
		var p struct{ Errors []map[string]string }
		_ = json.Unmarshal(w.Body.Bytes(), &p)
		if w.Code != http.StatusBadRequest || len(p.Errors) == 0 || p.Errors[0]["code"] != c.field {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	if w := e.do("GET", path+"?type=still", e.adminToken, nil); w.Code != http.StatusBadRequest {
		t.Errorf("a movie's stills: %d", w.Code)
	}
	if w := e.do("POST", path+"/"+id.New().String()+"/select", e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("selecting an unknown image: %d", w.Code)
	}
	if w := e.do("GET", path, e.nobodyToken, nil); w.Code != http.StatusForbidden {
		t.Errorf("a viewer listing artwork: %d", w.Code)
	}
}

func TestFiles(t *testing.T) {
	e := newEnv(t, serving{})
	files := decode[list](t, e.do("GET", "/api/v1/works/"+e.heat.Work.String()+"/files", e.adminToken, nil))
	if len(files.Items) != 1 || files.Items[0]["status"] != "present" || files.Items[0]["versionId"] != e.heat.Version.String() {
		t.Fatalf("show files %+v", files)
	}
	if w := e.do("DELETE", "/api/v1/files/"+e.heat.File.String(), e.adminToken, nil); w.Code != http.StatusConflict || problemCode(w) != "fileStillPresent" {
		t.Errorf("delete a present file: %d", w.Code)
	}
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.MarkGone(ctx, tx, e.episode.File)
	})
	gone := decode[list](t, e.do("GET", "/api/v1/files?status=gone", e.adminToken, nil))
	if len(gone.Items) != 1 || gone.Items[0]["id"] != e.episode.File.String() || gone.Items[0]["title"] == nil || gone.Items[0]["goneAt"] == nil {
		t.Fatalf("missing files %+v", gone)
	}
	if w := e.do("DELETE", "/api/v1/files/"+e.episode.File.String(), e.adminToken, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/v1/files", e.adminToken, nil); w.Code != http.StatusBadRequest {
		t.Errorf("no filter: %d", w.Code)
	}

	// Forget all removes every missing file, and only those.
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.MarkGone(ctx, tx, e.heat.File)
	})
	if gone := decode[list](t, e.do("GET", "/api/v1/files?status=gone", e.adminToken, nil)); len(gone.Items) != 1 || gone.Items[0]["workId"] != nil {
		t.Fatalf("a work in no library has no workId: %+v", gone)
	}
	if w := e.do("DELETE", "/api/v1/files?status=present", e.adminToken, nil); w.Code != http.StatusBadRequest {
		t.Errorf("delete present files: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/files?status=gone", e.adminToken, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete all missing: %d %s", w.Code, w.Body.String())
	}
	if gone := decode[list](t, e.do("GET", "/api/v1/files?status=gone", e.adminToken, nil)); len(gone.Items) != 0 {
		t.Errorf("after deleting all missing: %+v", gone)
	}
}

func TestProblemsAndMetrics(t *testing.T) {
	e := newEnv(t, serving{})
	since := time.Now().Add(-time.Hour)
	e.reported.problems = []health.Problem{{Component: "scan", Code: "folderUnreachable", Message: "library /mnt/tv unreachable", RaisedAt: since, Severity: health.Error}}
	ps := decode[list](t, e.do("GET", "/api/v1/problems", e.adminToken, nil))
	if len(ps.Items) != 1 || ps.Items[0]["severity"] != "error" {
		t.Fatalf("problems %+v", ps)
	}
	if w := e.do("DELETE", "/api/v1/problems/"+ps.Items[0]["id"].(string), e.adminToken, nil); w.Code != http.StatusNoContent {
		t.Errorf("dismiss: %d", w.Code)
	}
	if ps := decode[list](t, e.do("GET", "/api/v1/problems", e.adminToken, nil)); len(ps.Items) != 0 {
		t.Errorf("after dismissing %+v", ps)
	}
	if w := e.do("DELETE", "/api/v1/problems/nope", e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("dismiss unknown: %d", w.Code)
	}
	w := e.do("GET", "/api/v1/metrics", e.adminToken, nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `kivo_http_requests_total{route="GET /api/v1/problems",status="2xx"} 2`) {
		t.Errorf("metrics: %d %s", w.Code, w.Body.String())
	}
}

func TestDirectories(t *testing.T) {
	e := newEnv(t, serving{})
	dir := t.TempDir()
	for _, d := range []string{"b", "a", ".hidden"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "file.mkv"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	page := decode[list](t, e.do("GET", "/api/v1/directories?path="+dir, e.adminToken, nil))
	if len(page.Items) != 2 || page.Items[0]["name"] != "a" || page.Items[1]["path"] != filepath.Join(dir, "b") {
		t.Errorf("listing %+v", page)
	}
	if roots := decode[list](t, e.do("GET", "/api/v1/directories", e.adminToken, nil)); len(roots.Items) == 0 || roots.Items[0]["path"] != filepath.Dir(roots.Items[0]["path"].(string)) {
		t.Errorf("roots %+v", roots)
	}
	if w := e.do("GET", "/api/v1/directories?path=relative", e.adminToken, nil); w.Code != http.StatusBadRequest {
		t.Errorf("relative: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/directories?path="+filepath.Join(dir, "nope"), e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("missing: %d", w.Code)
	}
}

func TestImages(t *testing.T) {
	e := newEnv(t, serving{})
	img := id.New()
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO image (id, work_id, kind, source, origin, width, height, blurhash, fetched_at)
			VALUES (?, ?, 'poster', 'provider', 'x', 480, 720, '', 0)`, img, e.heat.Work)
		return err
	})
	// The admin sees the work, but the cache has no file, so it's not
	// found.
	if w := e.do("GET", "/api/v1/images/"+img.String()+"?width=300", e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("uncached image: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/images/"+img.String()+"?width=wide", e.adminToken, nil); w.Code != http.StatusBadRequest {
		t.Errorf("bad width: %d", w.Code)
	}
}

// A library's scan status shows its scan in flight, with the step the scan
// reported. Cancel stops the scan, and the library is idle again.
func TestLibraryScanStatusAndStop(t *testing.T) {
	e := newEnv(t, serving{})
	opened, release := make(chan struct{}, 1), make(chan struct{})
	e.openFolder = func(path string) (fs.FS, func() error, error) {
		opened <- struct{}{}
		<-release
		return catalog.OpenFolder(path)
	}
	e.run()
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	t.Cleanup(free) // before the runner stops
	status := func() map[string]any {
		t.Helper()
		lib := decode[map[string]any](t, e.do("GET", "/api/v1/libraries/"+e.lib.String(), e.adminToken, nil))
		st, _ := lib["scan"].(map[string]any)
		return st
	}

	if w := e.do("POST", "/api/v1/libraries/"+e.lib.String()+"/scan", e.adminToken, nil); w.Code != http.StatusAccepted {
		t.Fatalf("scan: %d %s", w.Code, w.Body.String())
	}
	<-opened // walking its one folder
	st := status()
	if p, _ := st["progress"].(map[string]any); st["libraryId"] != e.lib.String() || st["state"] != "scanning" ||
		p["step"] != "walk" || p["done"] != 0.0 || p["total"] != 1.0 || p["remainingMs"] != nil {
		t.Fatalf("status while a scan walks: %v", st)
	}
	scans := e.task("scan-libraries")
	if p, _ := scans["progress"].(map[string]any); scans["running"] != true || scans["stoppable"] != true || p["step"] != "walk" {
		t.Errorf("scans while one walks %v", scans)
	}
	if w := e.do("GET", "/api/v1/metrics", e.adminToken, nil); !strings.Contains(w.Body.String(), `kivo_task_items{task="scan-libraries"} 1`+"\n") {
		t.Errorf("metrics while a scan walks: %s", w.Body.String())
	}

	if w := e.do("POST", "/api/v1/libraries/"+e.lib.String()+"/stop-scan", e.adminToken, nil); w.Code != http.StatusNoContent {
		t.Fatalf("stop: %d %s", w.Code, w.Body.String())
	}
	free()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		st := status()
		if st["state"] == "idle" && st["progress"] == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("after cancelling: %v", st)
		}
	}
	if scans := e.task("scan-libraries"); scans["running"] != false {
		t.Errorf("scans after cancelling %v", scans)
	}
}

// A library's scan status reads its scan's progress. That is the step, how
// far the scan has got, and the time left in the step at the rate so far.
// Another library's scan doesn't show. A scan that hasn't reported a step
// has no progress.
func TestLibraryScanStatusFromItsScan(t *testing.T) {
	now := time.Now()
	lib := query.AdminLibrary{Library: query.Library{ID: id.New()}, Folders: []catalog.Folder{{Reachable: true}}}
	if a := libraryScanStatus(lib, nil, nil, now); a.LibraryID != lib.ID || a.State != "idle" || a.Progress != nil {
		t.Errorf("no scan: %+v", a)
	}
	other := tasks.Item{ID: id.New(), Step: "probe", Done: 1, Total: 2, Since: now.Add(-time.Minute)}
	if a := libraryScanStatus(lib, []tasks.Item{other}, nil, now); a.State != "idle" || a.Progress != nil {
		t.Errorf("another library's scan: %+v", a)
	}
	if a := libraryScanStatus(lib, nil, []id.ID{lib.ID}, now); a.State != "scanning" || a.Progress != nil {
		t.Errorf("a scan asked for, not started: %+v", a)
	}
	started := tasks.Item{ID: lib.ID}
	if a := libraryScanStatus(lib, []tasks.Item{other, started}, nil, now); a.State != "scanning" || a.Progress != nil {
		t.Errorf("a scan with no step yet: %+v", a)
	}
	probing := tasks.Item{ID: lib.ID, Step: "probe", Done: 10, Total: 40, Since: now.Add(-time.Minute)}
	a := libraryScanStatus(lib, []tasks.Item{other, probing}, nil, now)
	if p := a.Progress; a.State != "scanning" || p == nil || p.Step != gen.ScanStep("probe") || p.Done != 10 || p.Total != 40 ||
		p.RemainingMs == nil || *p.RemainingMs != int((3*time.Minute).Milliseconds()) {
		t.Errorf("probing 10 of 40 in a minute: %+v %+v", a, a.Progress)
	}
	lib.Folders = append(lib.Folders, catalog.Folder{Reachable: false})
	if a := libraryScanStatus(lib, nil, nil, now); a.State != "unreachable" {
		t.Errorf("a folder unreachable: %+v", a)
	}
}

func TestSubtitleDownloads(t *testing.T) {
	e := newEnv(t, serving{})
	// The test server has no OpenSubtitles key.
	w := e.do("GET", "/api/v1/files/"+e.heat.File.String()+"/subtitle-results?language=en", e.adminToken, nil)
	if w.Code != http.StatusServiceUnavailable || problemCode(w) != "notConfigured" {
		t.Errorf("search without a key: %d %s", w.Code, w.Body.String())
	}
	w = e.do("POST", "/api/v1/files/"+e.heat.File.String()+"/subtitles", e.adminToken, map[string]any{"resultId": "1.en"})
	if w.Code != http.StatusServiceUnavailable || problemCode(w) != "notConfigured" {
		t.Errorf("download without a key: %d %s", w.Code, w.Body.String())
	}
}

// Any user who sees a file may search for its subtitles and download one.
// A user who doesn't see the file gets 404 for both.
func TestSubtitleSearchIsForViewers(t *testing.T) {
	e := newEnv(t, serving{})
	var viewer users.User
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		var err error
		viewer, err = e.d.Users.CreateUser(ctx, tx, users.NewUser{Username: "eva", Grants: users.Grants{All: true}})
		return err
	})
	token := e.session(viewer.ID)
	search := "/api/v1/files/" + e.heat.File.String() + "/subtitle-results"
	download := "/api/v1/files/" + e.heat.File.String() + "/subtitles"
	body := map[string]any{"resultId": "1.en"}
	// The test server has no OpenSubtitles key, so this error shows the
	// request got past access.
	if w := e.do("GET", search, token, nil); w.Code != http.StatusServiceUnavailable || problemCode(w) != "notConfigured" {
		t.Errorf("a viewer's search: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("POST", download, token, body); w.Code != http.StatusServiceUnavailable || problemCode(w) != "notConfigured" {
		t.Errorf("a viewer's download: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", search, e.nobodyToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("a search by a user who can't see the file: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("POST", download, e.nobodyToken, body); w.Code != http.StatusNotFound {
		t.Errorf("a download by a user who can't see the file: %d %s", w.Code, w.Body.String())
	}
}

// TestServerLadder checks that signed-in clients read the quality ladder
// from the server, highest first. A signed-out client gets none.
func TestServerLadder(t *testing.T) {
	e := newEnv(t, serving{})
	if info := decode[gen.ServerInfo](t, e.do("GET", "/api/v1/server", "", nil)); info.QualityLadder != nil {
		t.Errorf("signed out: %v", *info.QualityLadder)
	}
	ladder := deref(decode[gen.ServerInfo](t, e.do("GET", "/api/v1/server", e.nobodyToken, nil)).QualityLadder)
	if len(ladder) != 7 || ladder[0] != (gen.QualityRung{BitrateBps: 20_000_000, Height: 1080}) ||
		ladder[6] != (gen.QualityRung{BitrateBps: 720_000, Height: 360}) {
		t.Errorf("ladder %v", ladder)
	}
}
