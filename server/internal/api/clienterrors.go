package api

import (
	"context"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/auth"
)

// The limits on a client error report. The client cuts to them too, but
// the server doesn't trust it to.
const (
	maxErrorMessage = 500
	maxStackFrames  = 10
	maxStackFrame   = 300
	maxErrorRoute   = 200
	maxErrorVersion = 64
	maxEngineField  = 32
)

// The reports a minute a session may send, and an address, which a
// household's TVs and browsers share. A client in a loop of errors is held
// to a few lines in the log, and the rest are dropped.
const (
	callerErrorReports = 10
	addrErrorReports   = 30
	errorReportWindow  = time.Minute
)

func (h *handlers) ReportClientError(ctx context.Context, req gen.ReportClientErrorRequestObject) (gen.ReportClientErrorResponseObject, error) {
	c := callerOf(ctx)
	if wait := h.errorReports.take(c, requestOf(ctx).ip, time.Now()); wait > 0 {
		// Dropped without a log line, so a flood can't fill the log.
		return nil, problem{status: http.StatusTooManyRequests, code: codeRateLimited, detail: "too many client error reports", retryAfter: wait}
	}
	if !req.Body.Kind.Valid() {
		return nil, invalid("unknown kind", field("kind", "invalid"))
	}
	attrs := append([]any{"request", requestID(ctx), "user", c.User}, clientErrorAttrs(req.Body)...)
	h.log.WarnContext(ctx, "client: error reported", attrs...)
	return gen.ReportClientError204Response{}, nil
}

// clientErrorAttrs returns a report's log attributes, each made safe to
// log and cut to its limit. A field that isn't safe is left out.
func clientErrorAttrs(b *gen.ClientErrorReport) []any {
	attrs := []any{"kind", string(b.Kind), "message", clip(b.Message, maxErrorMessage)}
	if s := stackTrace(deref(b.Stack)); s != "" {
		attrs = append(attrs, "stack", s)
	}
	if r := routeTemplate(deref(b.Route)); r != "" {
		attrs = append(attrs, "route", r)
	}
	if v := reportedWord(deref(b.Version), maxErrorVersion); v != "" {
		attrs = append(attrs, "client_version", v)
	}
	if e := b.Engine; e != nil {
		for _, f := range []struct {
			key string
			v   *string
		}{{"platform", e.Platform}, {"tizen", e.TizenVersion}, {"chromium", e.ChromiumVersion}} {
			if v := reportedWord(deref(f.v), maxEngineField); v != "" {
				attrs = append(attrs, f.key, v)
			}
		}
	}
	return attrs
}

// stackTrace joins a report's first frames, each clipped, one a line.
func stackTrace(frames []string) string {
	frames = frames[:min(len(frames), maxStackFrames)]
	out := make([]string, 0, len(frames))
	for _, f := range frames {
		if f = clip(f, maxStackFrame); f != "" {
			out = append(out, f)
		}
	}
	return strings.Join(out, "\n")
}

var (
	// A URL's query or fragment, which can hold a search the user typed or
	// a token, is cut from anything a client reports.
	urlQuery = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^\s?#]*)[?#][^\s)]*`)
	// An ID in a route, which the template should have named by its
	// parameter but a client may not have.
	routeID      = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}|\d{3,}`)
	routePattern = regexp.MustCompile(`^/[A-Za-z0-9/_$.-]*$`)
	tokenPattern = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)
)

// clip makes a reported text safe to log: URLs lose their queries,
// control characters become spaces, and it's cut to n characters.
func clip(s string, n int) string {
	s = strings.ToValidUTF8(s, "")
	s = urlQuery.ReplaceAllString(s, "$1")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > n {
		s = string([]rune(s)[:n])
	}
	return s
}

// routeTemplate returns a reported route if it is a template, with any ID
// left in it named $id, or "" when it isn't one.
func routeTemplate(s string) string {
	if len(s) > maxErrorRoute || !routePattern.MatchString(s) {
		return ""
	}
	return routeID.ReplaceAllString(s, "$$id")
}

// reportedWord returns a reported version or engine field if it is a short word
// of letters, digits and ._+-, or "".
func reportedWord(s string, n int) string {
	if len(s) > n || !tokenPattern.MatchString(s) {
		return ""
	}
	return s
}

// errorReports counts each session's and address's reports in fixed
// one-minute windows. Only signed-in callers reach it, so its keys are
// bounded by the sessions there are, and a window's keys go when it ends.
// The zero value is ready to use.
type errorReports struct {
	mu     sync.Mutex
	window time.Time
	counts map[string]int
}

// take counts a report from the caller at ip, or returns how long until it
// may send one.
func (e *errorReports) take(c caller, ip netip.Addr, now time.Time) time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()
	if now.Sub(e.window) >= errorReportWindow {
		e.window, e.counts = now, map[string]int{}
	}
	who := "session " + c.Session.String()
	if c.Session.IsZero() {
		who = "key " + c.APIKey.String() + " " + c.User.String()
	}
	addr := "addr " + auth.ClientAddr(ip)
	if e.counts[who] >= callerErrorReports || e.counts[addr] >= addrErrorReports {
		return e.window.Add(errorReportWindow).Sub(now)
	}
	e.counts[who]++
	e.counts[addr]++
	return 0
}
