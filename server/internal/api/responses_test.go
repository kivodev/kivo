package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// transcript records requests and their answers, for a golden file.
type transcript struct {
	e *env
	b strings.Builder
}

// do sends a request, records its status and body, and returns the
// decoded body. It returns nil when the body isn't a JSON object.
func (tr *transcript) do(method, path, token string, body any) map[string]any {
	tr.e.t.Helper()
	w := tr.e.do(method, path, token, body)
	fmt.Fprintf(&tr.b, "%s %s\n%d\n", method, path, w.Code)
	var out bytes.Buffer
	if json.Indent(&out, w.Body.Bytes(), "", "  ") == nil {
		tr.b.Write(out.Bytes())
	} else {
		tr.b.Write(w.Body.Bytes())
	}
	tr.b.WriteString("\n\n")
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return m
}

var (
	uuidPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	timePattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`)
	tmpPattern  = regexp.MustCompile(regexp.QuoteMeta(slashed(os.TempDir())) + `/[^"/]+/\d+`)
	// A Windows path, in JSON or a request line, is written as the golden
	// file has it: with / and no drive.
	winPath    = regexp.MustCompile(`[A-Z]:\\[^"\n]*`)
	msPattern  = regexp.MustCompile(`"(remainingMs|ageMs)": \d+`)
	reqPattern = regexp.MustCompile(`"requestId": "[0-9a-f]+"`)
	// An activity cursor is an event's time in milliseconds and its ID.
	activityCursor = regexp.MustCompile(`\b\d{13}\.(<id\d+>)`)
	// An opaque cursor is base64-encoded JSON that holds IDs.
	cursorPattern = regexp.MustCompile(`"eyJ[A-Za-z0-9_-]*=*"`)
)

// String returns the transcript with the values that change from run to
// run (IDs, times and temporary folders) replaced by stable names.
func (tr *transcript) String() string {
	s := cursorPattern.ReplaceAllStringFunc(tr.b.String(), func(c string) string {
		b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(strings.Trim(c, `"`), "="))
		if err != nil {
			return c
		}
		return strconv.Quote("<cursor " + string(b) + ">")
	})
	names := map[string]string{}
	s = uuidPattern.ReplaceAllStringFunc(s, func(u string) string {
		if n, ok := names[u]; ok {
			return n
		}
		names[u] = fmt.Sprintf("<id%d>", len(names)+1)
		return names[u]
	})
	s = timePattern.ReplaceAllString(s, "<time>")
	s = winPath.ReplaceAllStringFunc(s, func(p string) string { return slashed(strings.ReplaceAll(p, `\\`, `\`)) })
	s = tmpPattern.ReplaceAllString(s, "<tmp>")
	s = reqPattern.ReplaceAllString(s, `"requestId": "<request>"`)
	s = activityCursor.ReplaceAllString(s, `<ms>.$1`)
	return msPattern.ReplaceAllString(s, `"$1": <ms>`)
}

// slashed is an absolute path written with / and no drive.
func slashed(p string) string { return filepath.ToSlash(strings.TrimPrefix(p, filepath.VolumeName(p))) }

// TestResponses keeps the status and body of the work, library, task,
// home, directory, play and paging endpoints in a golden file.
func TestResponses(t *testing.T) {
	e := newEnv(t, serving{})
	tr := &transcript{e: e}
	admin, nobody := e.adminToken, e.nobodyToken
	var friend users.User
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		var err error
		friend, err = e.d.Users.CreateUser(ctx, tx, users.NewUser{Username: "friend", Grants: users.Grants{All: true}})
		return err
	})
	friendToken := e.session(friend.ID)
	books, folder := e.b.Library(catalog.Audiobooks)
	book := e.b.Work(catalog.WorkInput{Kind: catalog.KindBook, Title: "Leviathan Wakes"})
	e.b.Version([]id.ID{e.b.File(folder, "James S. A. Corey/Leviathan Wakes/a.m4b")}, book)
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.Catalog.UpdateMetadata(ctx, tx, book, catalog.WorkMetadata{Title: "Leviathan Wakes",
			Book: &catalog.BookMetadata{Authors: []string{"James S. A. Corey"}, Narrators: []string{"Jefferson Mays"}}})
	})
	heat, series, episode := e.heat.Work.String(), e.series.String(), e.episode.Work.String()
	works := "/api/v1/works/"

	// Works.
	libs := "/api/v1/libraries/"
	tr.do("GET", libs+e.lib.String()+"/works", admin, nil)
	tr.do("GET", libs+e.lib.String()+"/works?limit=1", admin, nil)
	tr.do("GET", libs+e.lib.String()+"/works?limit=0", admin, nil)
	tr.do("GET", libs+e.lib.String()+"/works?offset=1&yearFrom=1990&yearTo=2030", admin, nil)
	tr.do("GET", libs+e.lib.String()+"/works?yearFrom=2000&yearTo=1999", admin, nil)
	tr.do("GET", libs+books.String()+"/authors", admin, nil)
	tr.do("GET", libs+books.String()+"/book-series", admin, nil)
	tr.do("GET", libs+books.String()+"/collections", admin, nil)
	tr.do("GET", libs+id.New().String()+"/collections", admin, nil)
	tr.do("GET", works+series+"/children", admin, nil)
	tr.do("GET", works+id.New().String()+"/children", admin, nil)
	tr.do("GET", "/api/v1/collections/"+id.New().String(), admin, nil)
	tr.do("GET", "/api/v1/unmatched-works", admin, nil)
	tr.do("GET", "/api/v1/unmatched-works", nobody, nil)
	tr.do("GET", "/api/v1/lists", admin, nil)
	tr.do("POST", "/api/v1/works/query", admin, map[string]any{"query": map[string]any{"kinds": []string{"movie"}, "sort": []map[string]any{{"field": "year"}}}, "limit": 5})
	tr.do("POST", "/api/v1/works/query", admin, map[string]any{"query": map[string]any{"kinds": []string{"movie"}, "where": map[string]any{"match": "all", "rules": []map[string]any{{"field": "nope", "op": "eq"}}}}})
	tr.do("GET", "/api/v1/work-fields/contentRating/values", admin, nil)
	tr.do("GET", works+heat, admin, nil)
	tr.do("GET", "/api/v1/search?q=hea", admin, nil)
	tr.do("GET", works+heat, nobody, nil)
	tr.do("PATCH", works+heat, admin, map[string]any{"title": "Heat", "year": 1996, "overview": "A heist."})
	tr.do("PATCH", works+heat, admin, map[string]any{"contentRating": "R"})
	tr.do("PATCH", works+heat, admin, map[string]any{"contentRating": "Nonsense"})
	tr.do("PATCH", works+heat, admin, map[string]any{"contentRating": ""})
	tr.do("PATCH", works+heat, admin, map[string]any{"studios": []string{"Warner"}, "genres": []string{"Crime"}})
	tr.do("PATCH", works+heat, admin, map[string]any{"book": map[string]any{"subtitle": "No"}})
	tr.do("PATCH", works+heat, nobody, map[string]any{"title": "Nope"})
	tr.do("PATCH", works+id.New().String(), admin, map[string]any{"title": "Nope"})
	tr.do("PATCH", works+series, admin, map[string]any{"series": map[string]any{"ordering": "sideways"}})
	tr.do("PATCH", works+series, admin, map[string]any{"title": "Renamed", "series": map[string]any{"ordering": "group:5e5b7b4f"}})
	tr.do("PATCH", works+series, admin, map[string]any{"title": "Seeded Series", "series": map[string]any{"ordering": "dvd"}})
	tr.do("PATCH", works+episode, admin, map[string]any{"episode": map[string]any{"airDate": "2001-02-03"}})
	tr.do("PATCH", works+book.String(), admin, map[string]any{"book": map[string]any{
		"subtitle": "Expanse 1", "publisher": " Orbit ", "releaseDate": "2011-06-02", "asin": "B0058ZZZ5A",
		"authors": []string{"James S. A. Corey"}, "narrators": []string{"Jefferson Mays"},
		"series": []map[string]any{{"name": "The Expanse", "position": "1", "primary": true}},
	}})
	tr.do("PATCH", works+book.String(), admin, map[string]any{"book": map[string]any{"publisher": ""}})
	tr.do("PATCH", "/api/v1/works", admin, map[string]any{"workIds": []string{book.String()}, "genres": []string{"Science Fiction"}})
	tr.do("PATCH", "/api/v1/works", admin, map[string]any{"workIds": []string{heat}, "genres": []string{"Crime"}})
	tr.do("PATCH", works+heat+"/user-state", admin, map[string]any{"played": true})
	tr.do("PATCH", works+heat+"/user-state", admin, map[string]any{"positionMs": 1000})
	tr.do("PATCH", works+heat+"/user-state", admin, map[string]any{"positionMs": -1, "playedAt": "2026-01-02T03:04:05Z", "versionId": id.New()})
	tr.do("PATCH", works+heat+"/user-state", nobody, map[string]any{"played": true})
	tr.do("GET", works+heat+"/matches?provider=audible", admin, nil)
	tr.do("GET", works+episode+"/matches?provider=tmdb&query=tmdb-0", admin, nil)
	tr.do("GET", works+book.String()+"/matches?provider=tmdb", admin, nil)
	tr.do("GET", works+book.String()+"/matches?query=B0058ZZZ5A", admin, nil)
	tr.do("GET", works+id.New().String()+"/matches", admin, nil)
	tr.do("POST", works+heat+"/match", admin, map[string]any{"matchId": "nonsense"})
	tr.do("POST", works+heat+"/match", admin, map[string]any{"matchId": "tmdb:movie:1"})
	tr.do("POST", works+book.String()+"/match", admin, map[string]any{"matchId": "tmdb:movie:1"})
	tr.do("POST", works+book.String()+"/match", admin, map[string]any{"matchId": "audible:book:B0058ZZZ5A"})
	tr.do("POST", works+id.New().String()+"/match", admin, map[string]any{"matchId": "tmdb:movie:1"})
	tr.do("POST", works+episode+"/unmatch", admin, nil)
	tr.do("POST", works+id.New().String()+"/unmatch", admin, nil)
	tr.do("POST", works+heat+"/refresh-metadata", admin, nil)
	tr.do("POST", works+id.New().String()+"/refresh-metadata", admin, nil)
	tr.do("GET", "/api/v1/content-ratings", admin, nil)
	tr.do("GET", "/api/v1/content-ratings?workId="+heat, admin, nil)
	tr.do("GET", "/api/v1/content-ratings?workId="+heat, nobody, nil)
	tr.do("GET", works+heat+"/artwork", admin, nil)
	tr.do("GET", works+heat+"/artwork?limit=1&after=bogus", admin, nil) // an older app's paging: the list comes back whole
	tr.do("GET", works+id.New().String()+"/artwork", admin, nil)
	tr.do("POST", works+id.New().String()+"/artwork/"+id.New().String()+"/select", admin, nil)

	// Libraries.
	tr.do("GET", "/api/v1/libraries", admin, nil)
	tr.do("GET", "/api/v1/libraries", friendToken, nil)
	tr.do("GET", "/api/v1/libraries", nobody, nil)
	tr.do("GET", "/api/v1/libraries?limit=1&after=bogus", admin, nil) // an older app's paging: the list comes back whole
	tr.do("GET", "/api/v1/libraries/"+e.lib.String(), admin, nil)
	tr.do("GET", "/api/v1/libraries/"+e.lib.String(), nobody, nil)
	tr.do("GET", "/api/v1/libraries/"+e.lib.String()+"/facets", admin, nil)
	dir := t.TempDir()
	created := tr.do("POST", "/api/v1/libraries", admin, map[string]any{"name": "Shows", "kind": "shows", "folders": []string{dir}, "previews": "never"})
	lib, _ := created["id"].(string)
	tr.do("PATCH", "/api/v1/libraries/"+lib, admin, map[string]any{"name": "TV", "useLocalMetadata": false})
	tr.do("POST", "/api/v1/libraries/"+lib+"/scan", admin, map[string]any{"full": true})
	tr.do("POST", "/api/v1/libraries/"+lib+"/scan", admin, nil)
	tr.do("GET", "/api/v1/libraries/"+lib, admin, nil)
	tr.do("POST", "/api/v1/libraries/"+lib+"/stop-scan", admin, nil)
	tr.do("POST", "/api/v1/libraries/"+lib+"/refresh-metadata", admin, nil)
	tr.do("POST", "/api/v1/libraries/"+id.New().String()+"/refresh-metadata", admin, nil)
	tr.do("POST", "/api/v1/libraries/"+id.New().String()+"/scan", admin, nil)
	tr.do("POST", "/api/v1/libraries/"+id.New().String()+"/stop-scan", admin, nil)
	tr.do("DELETE", "/api/v1/libraries/"+lib, admin, nil)
	tr.do("DELETE", "/api/v1/libraries/"+id.New().String(), admin, nil)

	// Tasks.
	tr.do("GET", "/api/v1/tasks", admin, nil)
	tr.do("POST", "/api/v1/tasks/make-previews/run", admin, nil)
	tr.do("POST", "/api/v1/tasks/make-previews/stop", admin, nil)
	tr.do("POST", "/api/v1/tasks/prepare-downloads/stop", admin, nil)
	tr.do("POST", "/api/v1/tasks/library-changes/run", admin, nil)
	tr.do("POST", "/api/v1/tasks/nope/run", admin, nil)
	tr.do("GET", "/api/v1/tasks/refresh-metadata/failures", admin, nil)
	tr.do("GET", "/api/v1/tasks/prune-previews/failures", admin, nil)
	tr.do("POST", "/api/v1/tasks/refresh-metadata/retry", admin, nil)
	tr.do("POST", "/api/v1/tasks/refresh-metadata/retry?item=bogus", admin, nil)

	// Directories.
	for _, d := range []string{"b", "a", ".hidden"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tr.do("GET", "/api/v1/directories?path="+dir, admin, nil)
	tr.do("GET", "/api/v1/directories?path=relative", admin, nil)
	tr.do("GET", "/api/v1/directories?path="+filepath.Join(dir, "nope"), admin, nil)

	// Plays.
	st := tr.do("POST", "/api/v1/plays", friendToken, map[string]any{"workId": e.heat.Work, "capabilities": clientCaps, "positionMs": 5000})
	play, _ := st["id"].(string)
	tr.do("POST", "/api/v1/plays", admin, map[string]any{"workId": e.heat.Work, "capabilities": clientCaps, "subtitleTrackId": "nonsense"})
	tr.do("POST", "/api/v1/plays", admin, map[string]any{"workId": e.heat.Work, "capabilities": clientCaps, "quality": map[string]any{"maxBitrateBps": -1}})
	tr.do("POST", "/api/v1/plays", nobody, map[string]any{"workId": e.heat.Work, "capabilities": clientCaps})
	tr.do("PATCH", "/api/v1/plays/"+play, friendToken, map[string]any{"positionMs": 60000, "state": "paused", "seekToFirstFrameMs": 250, "bandwidthBps": 9000000})
	tr.do("PATCH", "/api/v1/plays/"+play, friendToken, map[string]any{"positionMs": 61000, "state": "playing", "subtitleTrackId": "nonsense"})
	tr.do("PATCH", "/api/v1/plays/"+play, nobody, map[string]any{"positionMs": 0, "state": "playing"})
	tr.do("GET", "/api/v1/plays", admin, nil)
	tr.do("GET", "/api/v1/plays/"+play, admin, nil)
	tr.do("GET", "/api/v1/plays/"+play, friendToken, nil)
	tr.do("GET", "/api/v1/plays/"+id.New().String(), admin, nil)
	tr.do("POST", "/api/v1/plays/"+play+"/stop", friendToken, map[string]any{"message": strings.Repeat("x", 501)})
	tr.do("POST", "/api/v1/plays/"+play+"/stop", nobody, map[string]any{})
	tr.do("POST", "/api/v1/plays/"+play+"/fallback", friendToken, map[string]any{"positionMs": 62000, "code": "mediaDecodeError", "detail": "bad"})
	tr.do("GET", "/api/v1/plays", admin, nil)
	for _, pl := range e.d.Playback.Plays() {
		tr.do("POST", "/api/v1/plays/"+pl.ID.String()+"/stop", admin, map[string]any{"message": "Closing up"})
	}
	tr.do("POST", "/api/v1/plays/"+play+"/stop", friendToken, map[string]any{"positionMs": 1000})
	own := tr.do("POST", "/api/v1/plays", admin, map[string]any{"workId": e.heat.Work, "capabilities": clientCaps})
	if p, ok := own["id"].(string); ok {
		tr.do("POST", "/api/v1/plays/"+p+"/stop", admin, map[string]any{"positionMs": 600000})
	}

	// Home, once there's something to continue.
	time.Sleep(time.Millisecond)
	tr.do("GET", "/api/v1/home", admin, nil)
	tr.do("GET", "/api/v1/home", friendToken, nil)
	tr.do("GET", "/api/v1/home", nobody, nil)
	tr.do("GET", "/api/v1/home?kind=recentlyAdded", admin, nil)
	tr.do("GET", "/api/v1/home/rows", friendToken, nil)
	tr.do("PATCH", "/api/v1/home/rows", friendToken, map[string]any{"items": []map[string]any{{"id": "continueWatching", "shown": false}}})
	tr.do("POST", "/api/v1/home/rows/reset", friendToken, nil)

	// Activity lists the plays above, a page at a time.
	page := tr.do("GET", "/api/v1/activity?show=plays&limit=1", admin, nil)
	if next, ok := page["next"].(string); ok {
		tr.do("GET", "/api/v1/activity?show=plays&limit=1&after="+next, admin, nil)
	}
	tr.do("GET", "/api/v1/activity?show=nope", admin, nil)
	tr.do("GET", "/api/v1/activity?after=bogus", admin, nil)
	tr.do("GET", "/api/v1/activity", nobody, nil)

	// Logs, for their paging.
	tr.do("GET", "/api/v1/logs?after=bogus", admin, nil)
	tr.do("GET", "/api/v1/logs?limit=0", admin, nil)
	tr.do("GET", "/api/v1/logs?level=nope", admin, nil)

	// A batch edit with a work that isn't there is refused as not found,
	// even when the batch also holds a movie.
	tr.do("PATCH", "/api/v1/works", admin, map[string]any{"workIds": []string{book.String(), id.New().String()}, "genres": []string{"x"}})
	tr.do("PATCH", "/api/v1/works", admin, map[string]any{"workIds": []string{id.New().String(), heat}, "genres": []string{"x"}})

	// An admin can't remove admin from, disable or delete their own user.
	me := "/api/v1/users/" + e.admin.String()
	tr.do("PATCH", me, admin, map[string]any{"admin": false})
	tr.do("PATCH", me, admin, map[string]any{"disabled": true})
	tr.do("PATCH", me, admin, map[string]any{"admin": false, "libraries": map[string]any{"all": false, "libraryIds": []string{}}})
	tr.do("DELETE", me, admin, nil)

	// A subtitle search without a language.
	tr.do("GET", "/api/v1/files/"+e.heat.File.String()+"/subtitle-results", admin, nil)

	golden(t, "responses.golden", tr.String())
	if code := e.do("GET", "/api/v1/home", admin, nil).Code; code != http.StatusOK {
		t.Errorf("home: %d", code)
	}
}
