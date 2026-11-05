package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/egress"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/media/subtitle"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/playback"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/scan"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/subtitles"
	"github.com/mmcdole/kivo/internal/tasks"
	"github.com/mmcdole/kivo/internal/transcode"
	"github.com/mmcdole/kivo/internal/users"
)

// errorProblems are the status and code that answer every exported error,
// wrapped the way a service returns it.
var errorProblems = []struct {
	name   string
	err    error
	status int
	code   string
}{
	{"query.ErrNotFound", query.ErrNotFound, 404, "notFound"},
	{"query.ErrInvalid", query.ErrInvalid, 400, "invalidRequest"},

	{"catalog.ErrNotFound", catalog.ErrNotFound, 404, "notFound"},
	{"catalog.ErrStale", catalog.ErrStale, 409, "conflict"},
	{"catalog.ErrInvalid", catalog.ErrInvalid, 400, "invalidRequest"},
	{"catalog.ErrFolderInUse", catalog.ErrFolderInUse, 409, "conflict"},
	{"catalog.ErrPresent", catalog.ErrPresent, 409, "fileStillPresent"},
	{"catalog.ErrUnreachable", catalog.ErrUnreachable, 500, "internal"},

	{"users.ErrNotFound", users.ErrNotFound, 404, "notFound"},
	{"users.ErrDisabled", users.ErrDisabled, 401, "unauthenticated"},
	{"users.ErrNameTaken", users.ErrNameTaken, 409, "nameTaken"},
	{"users.ErrInvalidName", users.ErrInvalidName, 400, "invalidRequest"},
	{"users.ErrLastAdmin", users.ErrLastAdmin, 409, "lastAdmin"},
	{"users.ErrNoLibraries", users.ErrNoLibraries, 400, "invalidRequest"},
	{"users.ErrUnknownLibrary", users.ErrUnknownLibrary, 400, "invalidRequest"},
	{"users.ErrInvalidPlayback", users.ErrInvalidPlayback, 400, "invalidRequest"},

	{"auth.ErrInvalidCredentials", auth.ErrInvalidCredentials, 401, "invalidCredentials"},
	{"auth.ErrWrongPassword", auth.ErrWrongPassword, 400, "wrongPassword"},
	{"auth.ErrUnauthenticated", auth.ErrUnauthenticated, 401, "unauthenticated"},
	{"auth.ErrBusy", auth.ErrBusy, 429, "rateLimited"},
	{"auth.ErrThrottled", auth.ErrThrottled, 429, "throttled"},
	{"auth.ThrottledError", &auth.ThrottledError{RetryAfter: 1500 * time.Millisecond}, 429, "throttled"},
	{"auth.ErrAlreadySetUp", auth.ErrAlreadySetUp, 409, "alreadySetUp"},
	{"auth.ErrWeakPassword", auth.ErrWeakPassword, 400, "invalidRequest"},
	{"auth.ErrNotFound", auth.ErrNotFound, 404, "notFound"},
	{"auth.ErrInvalidExpiry", auth.ErrInvalidExpiry, 400, "invalidRequest"},
	{"auth.ErrInvalidName", auth.ErrInvalidName, 400, "invalidRequest"},
	{"auth.ErrInvalidClientID", auth.ErrInvalidClientID, 400, "invalidRequest"},
	{"auth.ErrInvalidKind", auth.ErrInvalidKind, 400, "invalidRequest"},
	{"auth.ErrInviteNotFound", auth.ErrInviteNotFound, 404, "notFound"},
	{"auth.ErrCodeNotFound", auth.ErrCodeNotFound, 404, "notFound"},
	{"auth.ErrCodeGone", auth.ErrCodeGone, 410, "expired"},
	{"auth.ErrDenied", auth.ErrDenied, 403, "denied"},

	{"tasks.ErrNotFound", tasks.ErrNotFound, 404, "notFound"},
	{"tasks.ErrNotStoppable", tasks.ErrNotStoppable, 409, "notStoppable"},

	{"identify.ErrInvalid", identify.ErrInvalid, 400, "invalidRequest"},
	{"identify.ErrContentRating", identify.ErrContentRating, 400, "invalidRequest"},
	{"identify.ErrOrdering", identify.ErrOrdering, 400, "invalidRequest"},
	{"identify.ErrNotMatchable", identify.ErrNotMatchable, 409, "kindMismatch"},
	{"identify.ErrBookMatch", identify.ErrBookMatch, 400, "invalidRequest"},

	{"scan.ErrRelativePath", scan.ErrRelativePath, 400, "invalidRequest"},
	{"scan.ErrUnreadable", scan.ErrUnreadable, 400, "invalidRequest"},

	{"metadata.ErrNotFound", metadata.ErrNotFound, 404, "notFound"},
	{"metadata.ErrUnavailable", metadata.ErrUnavailable, 503, "providerUnavailable"},
	{"metadata.ErrBadImage", metadata.ErrBadImage, 400, "invalidRequest"},
	{"metadata.ErrNoSuchArtwork", metadata.ErrNoSuchArtwork, 400, "invalidRequest"},
	{"subtitles.ErrNotFound", subtitles.ErrNotFound, 404, "notFound"},
	{"subtitles.ErrUnavailable", subtitles.ErrUnavailable, 503, "providerUnavailable"},
	{"subtitles.ErrNotConfigured", subtitles.ErrNotConfigured, 503, "notConfigured"},
	{"subtitles.ErrQuota", subtitles.ErrQuota, 503, "quotaUsed"},

	{"playback.ErrNoPlay", playback.ErrNoPlay, 404, "notFound"},
	{"playback.ErrNotFound", playback.ErrNotFound, 404, "notFound"},
	{"playback.ErrNotReady", playback.ErrNotReady, 404, "notReady"},
	{"playback.ErrStreamLimit", playback.ErrStreamLimit, 409, "streamLimit"},
	{"playback.StreamLimitError", &playback.StreamLimitError{}, 409, "streamLimit"},
	{"playback.ErrTranscodeBusy", playback.ErrTranscodeBusy, 503, "transcodeBusy"},
	{"playback.RefusedError busy", &playback.RefusedError{Err: playback.ErrTranscodeBusy, Reason: playback.RefuseStorageFull}, 503, "transcodeBusy"},
	{"playback.ErrNotPlayable", playback.ErrNotPlayable, 409, "notPlayable"},
	{"playback.RefusedError not playable", &playback.RefusedError{Err: playback.ErrNotPlayable, Reason: playback.RefuseNoTracks}, 409, "notPlayable"},
	{"playback.ErrNoFallback", playback.ErrNoFallback, 409, "noFallback"},
	{"playback.ErrJobFailed", playback.ErrJobFailed, 500, "transcodeFailed"},

	{"config.ValidationError", &config.ValidationError{Errs: []error{fmt.Errorf("played_threshold: 3 is not between 0.5 and 1")}}, 400, "invalidRequest"},
	{"config.ErrInvalid", config.ErrInvalid, 400, "invalidRequest"},
	{"context.DeadlineExceeded", context.DeadlineExceeded, 503, "providerUnavailable"},

	{"transcode.ErrStorageFull", transcode.ErrStorageFull, 500, "internal"},
	{"transcode.ErrUnsupported", transcode.ErrUnsupported, 500, "internal"},
	{"transcode.ErrAcceleratorFailed", transcode.ErrAcceleratorFailed, 500, "internal"},
	{"transcode.ErrJobFailed", transcode.ErrJobFailed, 500, "internal"},
	{"transcode.ErrNotFound", transcode.ErrNotFound, 404, "notFound"},
	{"transcode.ErrClosed", transcode.ErrClosed, 500, "internal"},
	{"media.ErrSourceUnreadable", media.ErrSourceUnreadable, 500, "internal"},
	{"media.ErrSourceCorrupt", media.ErrSourceCorrupt, 500, "internal"},
	{"media.ErrUnsupported", media.ErrUnsupported, 500, "internal"},
	{"media.ErrEncoderBusy", media.ErrEncoderBusy, 500, "internal"},
	{"media.ErrEncoderFailed", media.ErrEncoderFailed, 500, "internal"},
	{"media.ErrTimeout", media.ErrTimeout, 500, "internal"},
	{"media.ErrCancelled", media.ErrCancelled, 500, "internal"},
	{"subtitle.ErrFormat", subtitle.ErrFormat, 500, "internal"},
	{"store.ErrNewerDatabase", store.ErrNewerDatabase, 500, "internal"},
	{"store.ErrOlderDatabase", store.ErrOlderDatabase, 500, "internal"},
	{"store.ErrNestedWrite", store.ErrNestedWrite, 500, "internal"},
	{"store.ErrLowSpace", store.ErrLowSpace, 500, "internal"},
	{"store.ErrInUse", store.ErrInUse, 500, "internal"},
	{"events.ErrLagged", events.ErrLagged, 500, "internal"},
	{"egress.ErrRefused", egress.ErrRefused, 500, "internal"},
	{"egress.ErrTooLarge", egress.ErrTooLarge, 500, "internal"},
	{"id.ErrInvalid", id.ErrInvalid, 500, "internal"},
}

// TestErrorProblems checks the status and code of every exported error
// that no handler answers in its own way.
func TestErrorProblems(t *testing.T) {
	for _, tc := range errorProblems {
		p, _ := problemFor(fmt.Errorf("pkg: operation: %w", tc.err))
		if p.status != tc.status || p.code != tc.code {
			t.Errorf("%s: %d %s, want %d %s", tc.name, p.status, p.code, tc.status, tc.code)
		}
	}
	for _, tc := range []struct {
		name       string
		err        error
		retryAfter time.Duration
		reason     string
	}{
		{"auth.ErrBusy", auth.ErrBusy, time.Second, ""},
		{"auth.ThrottledError", &auth.ThrottledError{RetryAfter: 1500 * time.Millisecond}, 1500 * time.Millisecond, ""},
		{"playback.RefusedError", &playback.RefusedError{Err: playback.ErrNotPlayable, Reason: playback.RefuseNoTracks}, 0, "noTracks"},
	} {
		if p, _ := problemFor(tc.err); p.retryAfter != tc.retryAfter || p.reason != tc.reason {
			t.Errorf("%s: retry after %v, reason %q", tc.name, p.retryAfter, p.reason)
		}
	}
	reset := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	if p, _ := problemFor(fmt.Errorf("download subtitle: %w", &subtitles.QuotaError{Reset: reset})); p.code != "quotaUsed" || !p.resetsAt.Equal(reset) {
		t.Errorf("subtitles.QuotaError: %s, resets at %v", p.code, p.resetsAt)
	}
	if p, known := problemFor(errors.New("disk on fire: /secret/path")); known || p.status != 500 || p.code != "internal" || p.detail != "" {
		t.Errorf("an unknown error: %+v, known %v", p, known)
	}
}

// TestEndpointProblems checks the status, code and field of the errors
// that the endpoints below return.
func TestEndpointProblems(t *testing.T) {
	e := newEnv(t, serving{})
	work := "/api/v1/works/" + e.heat.Work.String()

	var limited users.User
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		var err error
		limited, err = e.d.Users.CreateUser(ctx, tx, users.NewUser{Username: "limited", Grants: users.Grants{All: true}, Limits: users.Limits{StreamLimit: 1}})
		return err
	})
	limitedToken := e.session(limited.ID)
	play := map[string]any{"workId": e.heat.Work, "capabilities": clientCaps}
	if w := e.do("POST", "/api/v1/plays", limitedToken, play); w.Code != http.StatusCreated {
		t.Fatalf("first play: %d %s", w.Code, w.Body.String())
	}

	for _, tc := range []struct {
		name   string
		w      func() *httptest.ResponseRecorder
		status int
		code   string
		field  string
	}{
		{"artwork of no such type", func() *httptest.ResponseRecorder {
			return e.do("GET", work+"/artwork?type=disc", e.adminToken, nil)
		}, 400, "invalidRequest", "type"},
		{"upload of no such type", func() *httptest.ResponseRecorder { return e.upload(e.heat.Work, "disc", pngOf(10, 15)) }, 400, "invalidRequest", "/type"},
		{"upload of no image", func() *httptest.ResponseRecorder { return e.upload(e.heat.Work, "poster", []byte("not an image")) }, 400, "invalidRequest", "/file"},
		{"delete a present file", func() *httptest.ResponseRecorder {
			return e.do("DELETE", "/api/v1/files/"+e.heat.File.String(), e.adminToken, nil)
		}, 409, "fileStillPresent", ""},
		{"run an Activity task", func() *httptest.ResponseRecorder {
			return e.do("POST", "/api/v1/tasks/library-changes/run", e.adminToken, nil)
		}, 404, "notFound", ""},
		{"stop work that always finishes", func() *httptest.ResponseRecorder {
			return e.do("POST", "/api/v1/tasks/prepare-downloads/stop", e.adminToken, nil)
		}, 409, "notStoppable", ""},
		{"failures of a task that keeps none", func() *httptest.ResponseRecorder {
			return e.do("GET", "/api/v1/tasks/prune-previews/failures", e.adminToken, nil)
		}, 404, "notFound", ""},
		{"play over the stream limit", func() *httptest.ResponseRecorder { return e.do("POST", "/api/v1/plays", limitedToken, play) }, 409, "streamLimit", ""},
		{"sign in as no such kind", func() *httptest.ResponseRecorder {
			return e.do("POST", "/api/v1/sessions", "", map[string]string{"username": "admin", "password": adminPassword, "kind": "tv"})
		}, 400, "invalidRequest", "/kind"},
		{"create a library with a relative folder", func() *httptest.ResponseRecorder {
			return e.do("POST", "/api/v1/libraries", e.adminToken, map[string]any{"name": "Films", "kind": "movies", "folders": []string{"films"}})
		}, 400, "invalidRequest", "/folders/0"},
		{"list a missing directory", func() *httptest.ResponseRecorder {
			return e.do("GET", "/api/v1/directories?path="+filepath.Join(t.TempDir(), "nope"), e.adminToken, nil)
		}, 404, "notFound", ""},
	} {
		r := tc.w()
		var p struct {
			Code   string
			Errors []struct{ Field string }
		}
		_ = json.Unmarshal(r.Body.Bytes(), &p)
		field := ""
		if len(p.Errors) > 0 {
			field = p.Errors[0].Field
		}
		if r.Code != tc.status || p.Code != tc.code || field != tc.field {
			t.Errorf("%s: %d %s %q, want %d %s %q", tc.name, r.Code, p.Code, field, tc.status, tc.code, tc.field)
		}
	}
}
