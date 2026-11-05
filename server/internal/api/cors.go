package api

import (
	"net/http"
	"slices"
	"strings"
)

// castReceiverOrigin is the origin the Cast receiver page is served from.
// The receiver's routes answer it across origins. Moving the receiver to
// another host changes this and nothing else on the server.
const castReceiverOrigin = "https://cast.kivo.watch"

// isReceiverOrigin reports whether origin serves the Cast receiver.
func isReceiverOrigin(origin string) bool {
	return origin == castReceiverOrigin
}

// preflightMaxAge is how long, in seconds, a browser may keep a preflight's
// answer.
const preflightMaxAge = "600"

// allowReceiver lets the Cast receiver's origin read a receiver route's
// answer without credentials. The receiver's token goes in the
// Authorization header, never in a cookie.
func allowReceiver(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Add("Vary", "Origin")
	origin := r.Header.Get("Origin")
	if !isReceiverOrigin(origin) {
		return
	}
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Expose-Headers", "Retry-After, X-Request-Id")
}

// allowAnyOrigin lets any origin read a media route's answer without
// credentials. The play's ID in the path is the credential.
func allowAnyOrigin(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")
}

// isPreflight reports whether r is a CORS preflight.
func isPreflight(r *http.Request) bool {
	return r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
}

// mediaHeaders are the request headers a media route's preflight allows,
// for range requests.
var mediaHeaders = []string{"range"}

// receiverHeaders are the request headers a receiver route's preflight
// allows. They carry the token, a JSON body's content type and the
// receiver's identity.
var receiverHeaders = append([]string{"authorization", "content-type"}, deviceHeaders...)

// preflight answers a CORS preflight for the route the request would
// reach. A media route is allowed from any origin. A receiver route is
// allowed from the Cast receiver's origin, with Chrome's Private Network
// Access allowed too, because the server is usually on the local network
// and the receiver's page isn't. Anything else is refused.
func (s *Server) preflight(w http.ResponseWriter, r *http.Request, mux *http.ServeMux) {
	method := r.Header.Get("Access-Control-Request-Method")
	probe := r.Clone(r.Context())
	probe.Method = method
	_, pattern := mux.Handler(probe)
	rt, known := s.routes[pattern]
	if known {
		requestOf(r.Context()).route = pattern
	}
	h := w.Header()
	h.Add("Vary", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers")
	asked := requestedHeaders(r)
	switch {
	case known && rt.level == play && (method == http.MethodGet || method == http.MethodHead) && allowed(asked, mediaHeaders):
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, HEAD")
		if len(asked) > 0 {
			h.Set("Access-Control-Allow-Headers", strings.Join(asked, ", "))
		}
	case known && rt.receiver && isReceiverOrigin(r.Header.Get("Origin")) && allowed(asked, receiverHeaders):
		h.Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		h.Set("Access-Control-Allow-Methods", rt.method)
		if len(asked) > 0 {
			h.Set("Access-Control-Allow-Headers", strings.Join(asked, ", "))
		}
	default:
		s.writeProblem(w, r, problem{status: http.StatusForbidden, code: codeCrossOrigin})
		return
	}
	if strings.EqualFold(r.Header.Get("Access-Control-Request-Private-Network"), "true") {
		h.Set("Access-Control-Allow-Private-Network", "true")
	}
	h.Set("Access-Control-Max-Age", preflightMaxAge)
	w.WriteHeader(http.StatusNoContent)
}

// requestedHeaders are the headers a preflight asks for, lowercased.
func requestedHeaders(r *http.Request) []string {
	var out []string
	for _, v := range r.Header.Values("Access-Control-Request-Headers") {
		for h := range strings.SplitSeq(v, ",") {
			if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
				out = append(out, h)
			}
		}
	}
	return out
}

// allowed reports whether every header asked for is one of ok.
func allowed(asked, ok []string) bool {
	for _, h := range asked {
		if !slices.Contains(ok, h) {
			return false
		}
	}
	return true
}
