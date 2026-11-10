package auth

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/auth/dbq"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

var (
	// ErrInvalidCredentials is returned for every sign-in failure. An
	// unknown user, a wrong password and a disabled user look the same.
	ErrInvalidCredentials = fault.Coded(fault.Unauthenticated, "invalidCredentials", "invalid credentials")
	// ErrWrongPassword means the current password given to change a
	// password is wrong.
	ErrWrongPassword = fault.Coded(fault.Invalid, "wrongPassword", "wrong password")
	// ErrUnauthenticated means a token is malformed, unknown, revoked or
	// expired, or its user is disabled.
	ErrUnauthenticated = fault.New(fault.Unauthenticated, "unauthenticated")
	// ErrBusy means too many passwords are being hashed. The client should
	// try again shortly.
	ErrBusy = fault.New(fault.Throttled, "busy")
	// ErrThrottled is what every *ThrottledError wraps.
	ErrThrottled = fault.Coded(fault.Throttled, "throttled", "too many attempts")
	// ErrAlreadySetUp means first-run setup has been done.
	ErrAlreadySetUp = fault.Coded(fault.Conflict, "alreadySetUp", "already set up")
	// ErrWeakPassword means a new password is too short or too long.
	ErrWeakPassword = fault.New(fault.Invalid, "password must be 8 to 1024 bytes")
	// ErrNotFound means there is no such session or API key, or the caller
	// may not see it.
	ErrNotFound = fault.New(fault.NotFound, "not found")
	// ErrInvalidExpiry means an invite would expire before it is made.
	ErrInvalidExpiry = fault.New(fault.Invalid, "expiry must be in the future")
	// ErrInvalidName means the name of a session or API key is too long, is
	// not UTF-8, or is missing where one is needed.
	ErrInvalidName = fault.New(fault.Invalid, "name must be 1 to 100 characters of UTF-8")
	// ErrInvalidClientID means a client ID is too long or not UTF-8.
	ErrInvalidClientID = fault.New(fault.Invalid, "client ID must be at most 100 characters of UTF-8")
	// ErrInvalidKind means a sign-in asked for a session kind that doesn't
	// exist.
	ErrInvalidKind = fault.New(fault.Invalid, "session kind must be browser or app")
	// ErrInviteNotFound means an invite link is unknown, used, expired or
	// revoked. All four cases look the same.
	ErrInviteNotFound = fault.New(fault.NotFound, "no such invite")
	// ErrCodeNotFound means a device code's code is unknown, expired, or
	// already approved or denied. All three cases look the same.
	ErrCodeNotFound = fault.New(fault.NotFound, "no such device code code")
	// ErrCodeGone means a poll's secret is unknown, or its code expired or
	// was used. The device starts again with a new code.
	ErrCodeGone = fault.New(fault.Gone, "device code code expired or used")
	// ErrDenied means a device code was denied. The device signs in another
	// way, or starts again.
	ErrDenied = fault.Coded(fault.Forbidden, "denied", "device code denied")
)

// ThrottledError is a sign-in that the backoff refused before any password
// was checked.
type ThrottledError struct {
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string {
	return fmt.Sprintf("%v; retry after %v", ErrThrottled, e.RetryAfter.Round(time.Second))
}

func (e *ThrottledError) Unwrap() error { return ErrThrottled }

// SessionKind is which client a session is for.
type SessionKind string

const (
	SessionBrowser SessionKind = "browser" // the web client's cookie, which expires after 30 idle days
	SessionApp     SessionKind = "app"     // an app's token, which lasts until revoked
)

// Platform is what a client runs on.
type Platform string

const (
	PlatformAndroidTV    Platform = "androidTv"
	PlatformAppleTV      Platform = "appleTv"
	PlatformIPhone       Platform = "iPhone"
	PlatformIPad         Platform = "iPad"
	PlatformAndroidPhone Platform = "androidPhone"
	PlatformBrowser      Platform = "browser"
	PlatformTizen        Platform = "tizen" // the packaged app on a Samsung TV
	PlatformCast         Platform = "cast"  // the Cast receiver
)

// valid reports whether p is a known platform or empty.
func (p Platform) valid() bool {
	switch p {
	case "", PlatformAndroidTV, PlatformAppleTV, PlatformIPhone, PlatformIPad, PlatformAndroidPhone, PlatformBrowser, PlatformTizen, PlatformCast:
		return true
	}
	return false
}

// Device is the app and device a request comes from, as the app identifies
// them. Each field is empty when the app didn't say.
type Device struct {
	App        string // such as kivo-android
	AppVersion string
	Platform   Platform
	OSVersion  string
	Model      string // the device's model, coarse, such as "SHIELD Android TV" or "iPhone16,2"
}

// Caller is who a request is from. It is a user acting through one of its
// sessions, an API key it created, or a cast token that one of its sessions
// gave a Cast receiver. Exactly one of Session, APIKey and CastToken is set.
type Caller struct {
	User      id.ID
	Session   id.ID
	APIKey    id.ID
	CastToken id.ID
	// Name is the name of the session or API key, or of the cast token's
	// receiver.
	Name string
	// Device is the app and device a session last identified. It's empty
	// for an API key and a cast token.
	Device Device
}

// Session is a signed-in browser or app.
type Session struct {
	ID, User id.ID
	Kind     SessionKind
	Name     string // the client's name, empty for a browser that gave none
	Device   Device // the app and device its requests last identified; its platform is empty when the client gave none
	Created  time.Time
	LastUsed time.Time
}

// Caller returns the session as a request's caller.
func (s Session) Caller() Caller { return Caller{User: s.User, Session: s.ID, Name: s.Name} }

// IssuedSession is a new session and its token. The token is shown to the
// client once. Only its hash is stored.
type IssuedSession struct {
	Token    string
	Session  Session
	ClientID string // the client's own ID the session was issued for, or empty
}

// IssuedAPIKey is a new API key and its token. The token is shown once.
type IssuedAPIKey struct {
	Token string
	Key   APIKey
}

// IssuedInvite is a new invite link and its token. The token is shown once.
type IssuedInvite struct {
	Token  string
	Invite Invite
}

// SignInRequest is a sign-in with a password. RemoteIP is the client's
// address as api resolved it through trusted proxies.
type SignInRequest struct {
	Username, Password string
	RemoteIP           netip.Addr
	Kind               SessionKind // browser (the default) or app
	Name               string      // the client's name, shown in the list of where the user is signed in
	ClientID           string      // the client's own ID, if it has one
	Device             Device      // the app and device, as the request identified them
}

// client is the browser or app a session is for.
type client struct {
	kind   SessionKind
	name   string
	id     string // the client's own ID, or empty
	device Device // its platform defaults to browser for a browser session
}

// Service verifies credentials and issues and checks sessions.
type Service struct {
	db       *store.DB
	q        *dbq.Queries
	deps     Deps
	hasher   *hasher
	throttle *throttle
	codes    *deviceCodes
}

// Deps are auth's collaborators.
type Deps struct {
	Users *users.Service
	// Settings name the local networks, which decide whether a device code
	// is nearby.
	Settings *config.Store
	// Activity records sign-ins, failed sign-ins, and users joining.
	Activity *activity.Service
}

// New returns a Service. Call Run to drop expired sessions and cast tokens.
func New(db *store.DB, d Deps) *Service {
	return &Service{
		db:       db,
		q:        dbq.New(db.Reader()),
		deps:     d,
		hasher:   newHasher(defaultParams),
		throttle: newThrottle(),
		codes:    newDeviceCodes(),
	}
}

// dropExpiredEvery is how often Run drops expired sessions and cast tokens.
const dropExpiredEvery = 5 * time.Minute

// Run drops browser sessions idle for 30 days and expired cast tokens every
// 5 minutes, until ctx ends.
func (s *Service) Run(ctx context.Context) error {
	t := time.NewTicker(dropExpiredEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			s.dropExpired(ctx)
		}
	}
}
