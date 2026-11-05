package api

import (
	_ "embed"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// level is who may call an operation, as its x-kivo-access in the spec
// states.
type level string

const (
	public   level = "public"   // no credentials
	signedIn level = "signedIn" // a signed-in session or an API key
	viewer   level = "viewer"   // signed in; reads filtered by its user's Viewer
	cast     level = "cast"     // a viewer, or a cast token as its user without admin
	admin    level = "admin"    // a viewer whose user is an admin
	play     level = "play"     // the play's ID in the path is the credential
)

// known reports whether l is one of the levels above.
func (l level) known() bool {
	switch l {
	case public, signedIn, viewer, cast, admin, play:
		return true
	}
	return false
}

// route is one operation of the spec and the policy it states.
type route struct {
	method, path, op string
	level            level
	// receiver means the Cast receiver calls the route from its own
	// origin, as it does every cast route.
	receiver bool
	// upload means the body is a file, not JSON.
	upload bool
}

// pattern is the route as the mux knows it.
func (r route) pattern() string { return r.method + " " + r.path }

//go:embed openapi.yaml
var spec []byte

var errSpec = errors.New("spec")

// methods are the keys of a path item that are operations.
var methods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

// operation is what the server reads of one operation of the spec.
type operation struct {
	ID       string `yaml:"operationId"`
	Access   level  `yaml:"x-kivo-access"`
	Receiver bool   `yaml:"x-kivo-receiver"`
	Upload   bool   `yaml:"x-kivo-upload"`
}

// embeddedRoutes holds the embedded spec's routes, read once. Servers
// share the map and never change it.
var embeddedRoutes = sync.OnceValues(func() (map[string]route, error) { return loadRoutes(spec) })

// loadRoutes reads every operation of the spec with its x-kivo-access,
// x-kivo-receiver and x-kivo-upload, keyed by mux pattern. Access is denied
// by default. An operation without a known level is an error, so
// the server never serves it.
func loadRoutes(b []byte) (map[string]route, error) {
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("read the API spec: %w", err)
	}
	out := map[string]route{}
	var errs []error
	for _, path := range slices.Sorted(maps.Keys(doc.Paths)) {
		item := doc.Paths[path]
		for _, method := range methods {
			node, ok := item[method]
			if !ok {
				continue
			}
			var op operation
			if err := node.Decode(&op); err != nil {
				return nil, fmt.Errorf("read the API spec's %s %s: %w", strings.ToUpper(method), path, err)
			}
			r := route{method: strings.ToUpper(method), path: path, op: op.ID, level: op.Access,
				receiver: op.Receiver || op.Access == cast, upload: op.Upload}
			switch {
			case op.ID == "":
				errs = append(errs, fmt.Errorf("%w: %s has no operationId", errSpec, r.pattern()))
			case op.Access == "":
				errs = append(errs, fmt.Errorf("%w: operation %s (%s) has no x-kivo-access", errSpec, r.op, r.pattern()))
			case !op.Access.known():
				errs = append(errs, fmt.Errorf("%w: operation %s (%s) has unknown x-kivo-access %q", errSpec, r.op, r.pattern(), op.Access))
			default:
				out[r.pattern()] = r
			}
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return out, nil
}
