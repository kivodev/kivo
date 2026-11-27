package api

// level is who may call an operation, as its x-kivo-access in the spec
// states. routegen writes each level as one of these constants, so a level
// added to the spec needs one here too.
type level string

const (
	public   level = "public"   // no credentials
	signedIn level = "signedIn" // a signed-in session or an API key
	viewer   level = "viewer"   // signed in; reads filtered by its user's Viewer
	cast     level = "cast"     // a viewer, or a cast token as its user without admin
	admin    level = "admin"    // a viewer whose user is an admin
	play     level = "play"     // the play's ID in the path is the credential
)

// route is one operation of the spec and the policy it states. routegen
// writes the spec's routes to specRoutes in routes_gen.go.
type route struct {
	method, path, op string
	level            level
	// receiver means the Cast receiver calls the route from its own
	// origin, as it does every cast route.
	receiver bool
	// upload means the body is a file, not JSON.
	upload bool
}
