package api

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/playback"
	"github.com/mmcdole/kivo/internal/subtitles"
)

// statusClientClosed is nginx's status for a request whose client hung up
// before the answer. It is recorded for metrics and logs, and no client
// sees it.
const statusClientClosed = 499

// The problem codes that api sends itself, as the spec's Problem.code. A
// fault carries its own code.
const (
	codeInvalidRequest      = "invalidRequest"
	codeUnauthenticated     = "unauthenticated"
	codeForbidden           = "forbidden"
	codeCrossOrigin         = "crossOrigin"
	codeNotFound            = "notFound"
	codeRangeNotSatisfiable = "rangeNotSatisfiable"
	codeOwnUser             = "ownUser"
	codeRateLimited         = "rateLimited"
	codeInternal            = "internal"
)

// problem is an error a handler returns to be sent as a problem document.
// Its detail is English text for logs. It never holds internal error text.
type problem struct {
	status     int
	code       string
	detail     string
	fields     []gen.FieldError
	retryAfter time.Duration
	reason     string
	mismatch   *gen.CapabilityMismatch
	plays      []gen.PlaySummary
	message    string    // an admin's words to a stopped play's viewer
	resetsAt   time.Time // when a used-up quota resets
}

func (p problem) Error() string { return p.code + ": " + p.detail }

func invalid(detail string, fields ...gen.FieldError) problem {
	return problem{status: http.StatusBadRequest, code: codeInvalidRequest, detail: detail, fields: fields}
}

func field(name, code string) gen.FieldError {
	return gen.FieldError{Field: name, Code: gen.FieldErrorCode(code)}
}

var errNotFound = problem{status: http.StatusNotFound, code: codeNotFound}

// problemFor maps an error from a service to its problem. Anything it
// doesn't know is a 500 whose text stays in the log.
func problemFor(err error) (problem, bool) {
	var p problem
	if errors.As(err, &p) {
		return p, true
	}
	f, ok := fault.Of(err)
	if !ok && errors.Is(err, context.DeadlineExceeded) {
		// Requests to providers are the ones with deadlines.
		f, ok = metadata.ErrUnavailable, true
	}
	if ok {
		return faultProblem(err, f)
	}
	return problem{status: http.StatusInternalServerError, code: codeInternal}, false
}

// faultProblem builds the problem for err, whose first fault is f. The
// problem takes a status from the fault's kind, and the fault's code,
// Detail and field. An internal fault is known only when it has its own
// code, which its package reports where it happens.
func faultProblem(err error, f *fault.Error) (problem, bool) {
	p := problem{status: http.StatusInternalServerError, code: f.Code()}
	switch f.Kind() {
	case fault.Internal:
		return p, p.code != codeInternal
	case fault.Invalid:
		p.status = http.StatusBadRequest
	case fault.Unauthenticated:
		p.status = http.StatusUnauthorized
	case fault.Forbidden:
		p.status = http.StatusForbidden
	case fault.NotFound:
		p.status = http.StatusNotFound
	case fault.Conflict:
		p.status = http.StatusConflict
	case fault.Gone:
		p.status = http.StatusGone
	case fault.Throttled:
		p.status, p.retryAfter = http.StatusTooManyRequests, time.Second
		if t, ok := errors.AsType[*auth.ThrottledError](err); ok {
			p.retryAfter = t.RetryAfter
		}
	case fault.Unavailable:
		p.status = http.StatusServiceUnavailable
	}
	p.detail = f.Error()
	if fe, ok := fault.FieldOf(err); ok {
		p.fields = []gen.FieldError{field(fe.Name, fe.Code)}
	}
	if r, ok := errors.AsType[*playback.RefusedError](err); ok {
		p.reason, p.mismatch = string(r.Reason), capabilityMismatch(r.Unmet)
	}
	if st, ok := errors.AsType[*playback.StoppedError](err); ok {
		p.message = st.Message
	}
	if q, ok := errors.AsType[*subtitles.QuotaError](err); ok {
		p.resetsAt = q.Reset
	}
	return p, true
}

// writeError sends err as a problem document, logging what it doesn't map.
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(r.Context().Err(), context.Canceled) {
		// The client went away. A player does this with segments it no
		// longer wants after a seek, and the web client does it with a
		// refetch it replaced. There is no one to answer and nothing to
		// report. err is whatever the cancellation surfaced as, such as
		// context.Canceled or the interrupt SQLite stops a query with.
		s.log.DebugContext(r.Context(), "api: request cancelled", "request", requestID(r.Context()), "route", routeOf(r), "err", err)
		w.WriteHeader(statusClientClosed)
		return
	}
	p, known := problemFor(err)
	if !known {
		s.log.ErrorContext(r.Context(), "api: request failed", "request", requestID(r.Context()), "operation", requestOf(r.Context()).op,
			"route", routeOf(r), "err", err)
	}
	s.writeProblem(w, r, p)
}

// requestError answers a request the generated code couldn't parse.
func (s *Server) requestError(w http.ResponseWriter, r *http.Request, err error) {
	p := problem{status: http.StatusBadRequest, code: codeInvalidRequest, detail: "the request is malformed"}
	var format *gen.InvalidParamFormatError
	var required *gen.RequiredParamError
	var many *gen.TooManyValuesForParamError
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		p.status, p.detail = http.StatusRequestEntityTooLarge, "the request body is too large"
	case errors.As(err, &format):
		p.fields = []gen.FieldError{field(format.ParamName, "invalid")}
	case errors.As(err, &required):
		p.fields = []gen.FieldError{field(required.ParamName, "required")}
	case errors.As(err, &many):
		p.fields = []gen.FieldError{field(many.ParamName, "invalid")}
	}
	s.writeProblem(w, r, p)
}

// document is the problem as sent.
func (s *Server) document(r *http.Request, p problem) gen.Problem {
	doc := gen.Problem{
		Type: "about:blank", Title: http.StatusText(p.status), Status: p.status,
		Code: gen.ProblemCode(p.code), RequestID: requestID(r.Context()),
	}
	if p.detail != "" {
		doc.Detail = &p.detail
	}
	if len(p.fields) > 0 {
		doc.Errors = &p.fields
	}
	if p.reason != "" {
		doc.Reason = optAs[gen.RefusalReason](p.reason)
	}
	doc.Mismatch = p.mismatch
	if len(p.plays) > 0 {
		doc.Plays = &p.plays
	}
	if p.message != "" {
		doc.Message = &p.message
	}
	if !p.resetsAt.IsZero() {
		doc.ResetsAt = ptr(p.resetsAt.UTC())
	}
	if p.retryAfter > 0 {
		secs := int(math.Ceil(p.retryAfter.Seconds()))
		doc.RetryAfterSeconds = &secs
	}
	return doc
}

func (s *Server) writeProblem(w http.ResponseWriter, r *http.Request, p problem) {
	doc := s.document(r, p)
	if doc.RetryAfterSeconds != nil {
		w.Header().Set("Retry-After", strconv.Itoa(*doc.RetryAfterSeconds))
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(p.status)
	_ = json.NewEncoder(w).Encode(doc)
}
