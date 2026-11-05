package api

import (
	"errors"
	"net/http"
	"regexp"
	"testing"
)

type logPage struct {
	Items []struct {
		ID, Level, Message string
		Attributes         []struct{ Key, Value string }
	}
	Next  string
	Total *int
}

func (p logPage) messages() []string {
	out := make([]string, len(p.Items))
	for i, l := range p.Items {
		out[i] = l.Message
	}
	return out
}

func TestLogsPage(t *testing.T) {
	e := newEnv(t, serving{})
	e.srv.log.Info("scan: finished", "folder", "/mnt/movies")
	e.srv.log.Warn("scan: folder unreachable; nothing in it changes", "folder", "/mnt/tv")
	e.srv.log.Error("store: write failed", "err", errors.New("disk full"))

	get := func(q string) logPage {
		t.Helper()
		w := e.do("GET", "/api/v1/logs"+q, e.adminToken, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", q, w.Code, w.Body.String())
		}
		return decode[logPage](t, w)
	}
	p := get("")
	if got := p.messages(); len(got) != 3 || got[0] != "scan: finished" || got[2] != "store: write failed" || *p.Total != 3 {
		t.Fatalf("info and above, newest last: %v total %v", got, p.Total)
	}
	if a := p.Items[2].Attributes; p.Items[2].Level != "error" || len(a) != 1 || a[0].Key != "err" || a[0].Value != "disk full" {
		t.Errorf("error line %+v", p.Items[2])
	}
	if got := get("?level=warning").messages(); len(got) != 2 || got[0] != "scan: folder unreachable; nothing in it changes" {
		t.Errorf("warnings: %v", got)
	}
	if got := get("?q=%2Fmnt%2FTV").messages(); len(got) != 1 || got[0] != "scan: folder unreachable; nothing in it changes" {
		t.Errorf("search: %v", got)
	}
	// Requests log at Debug, by route pattern.
	debug := get("?level=debug&q=api%3A+request")
	if len(debug.Items) == 0 || debug.Items[0].Level != "debug" {
		t.Fatalf("debug: %+v", debug)
	}
	// The newest line, then the lines after a cursor, a page at a time.
	if p := get("?limit=1"); len(p.Items) != 1 || p.Items[0].Message != "store: write failed" || *p.Total != 3 || p.Next != "" {
		t.Errorf("newest: %+v", p)
	}
	first := p.Items[0].ID
	page := get("?limit=1&after=" + first)
	if got := page.messages(); len(got) != 1 || got[0] != "scan: folder unreachable; nothing in it changes" || page.Next == "" || page.Total != nil {
		t.Errorf("after the first: %+v", page)
	}
	if got := get("?after=" + page.Next).messages(); len(got) != 1 || got[0] != "store: write failed" {
		t.Errorf("next page: %v", got)
	}

	for _, q := range []string{"?level=loud", "?after=x", "?limit=0", "?limit=5001"} {
		if w := e.do("GET", "/api/v1/logs"+q, e.adminToken, nil); w.Code != http.StatusBadRequest || problemCode(w) != codeInvalidRequest {
			t.Errorf("%s: %d %s", q, w.Code, problemCode(w))
		}
	}
	if w := e.do("GET", "/api/v1/logs", e.nobodyToken, nil); w.Code != http.StatusForbidden {
		t.Errorf("a viewer: %d", w.Code)
	}
}

// The download is the text log as the server wrote it.
func TestLogsDownload(t *testing.T) {
	e := newEnv(t, serving{})
	e.srv.log.Info("scan: finished", "folder", "/mnt/movies")
	e.do("GET", "/api/v1/home", e.adminToken, nil)
	logged := e.logs.String()
	w := e.do("GET", "/api/v1/logs/file", e.adminToken, nil)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/plain" {
		t.Fatalf("%d %s", w.Code, w.Header().Get("Content-Type"))
	}
	if cd := w.Header().Get("Content-Disposition"); !regexp.MustCompile(`^attachment; filename="kivo-log-\d{8}-\d{6}\.txt"$`).MatchString(cd) {
		t.Errorf("Content-Disposition %q", cd)
	}
	if w.Body.String() != logged {
		t.Errorf("download\n%s\ntext log\n%s", w.Body.String(), logged)
	}
	if w := e.do("GET", "/api/v1/logs/file", e.nobodyToken, nil); w.Code != http.StatusForbidden {
		t.Errorf("a viewer: %d", w.Code)
	}
}
