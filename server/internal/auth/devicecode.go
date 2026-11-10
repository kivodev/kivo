package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
)

const (
	codeTTL = 5 * time.Minute
	// pollInterval is how often a device polls its link.
	pollInterval = 2 * time.Second
	codeLen      = 6
	// codeAlphabet has 32 symbols. It leaves out 0, O, 1 and I, which look
	// alike on a TV.
	codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	// maxCodesPerAddr bounds the codes pending at once from one address,
	// or from one IPv6 client's /64. maxCodes bounds the codes pending in
	// all.
	maxCodesPerAddr = 5
	maxCodes        = 1000
	// pollKey is the user name the throttle counts polls under. No real
	// username contains a NUL.
	pollKey = "\x00device code"
)

// DeviceCodeStart starts signing a device in by code.
type DeviceCodeStart struct {
	// Name is the device's name. It is shown before approving, and becomes
	// the session's name.
	Name     string
	Kind     SessionKind // the kind of session it signs in to, app by default
	ClientID string      // the device's own ID, if it has one
	Device   Device      // the app and device, as the request identified them
	RemoteIP netip.Addr
}

// DeviceCode is a started device code sign-in. Code is what the device
// shows. Secret is what the device polls with every Interval, and it is
// shown to the device once. A signed-in user approves or denies the code.
// The device's next poll then gets a session as that user, or ErrDenied.
// Pending codes live in memory. Losing them on restart is fine, because the
// device starts again with a new code.
type DeviceCode struct {
	Code, Secret string
	Expires      time.Time
	Interval     time.Duration
}

// PendingDeviceCode is the device waiting on a code, as shown to the user
// asked to approve it. It holds what the device says it is, and what the
// server saw of it.
type PendingDeviceCode struct {
	Name     string
	Platform Platform
	Nearby   bool // the device started from an address on the local networks
	Started  time.Time
}

type pendingCode struct {
	code     string
	client   client // the device waiting on the code
	secret   tokenHash
	addr     string // where the device started from, as ClientAddr counts it
	nearby   bool
	started  time.Time
	expires  time.Time
	approver id.ID // the user who approved the code, or zero while it waits
	denied   bool
}

// answered reports whether the code was approved or denied.
func (pl *pendingCode) answered() bool { return pl.denied || !pl.approver.IsZero() }

// deviceCodes holds the pending codes, keyed by code and by the secret's
// hash. A code is kept for 5 more minutes after it expires. Then a late
// poll is told the code is gone or denied, and doesn't count as a guess.
type deviceCodes struct {
	mu       sync.Mutex
	byCode   map[string]*pendingCode
	bySecret map[tokenHash]*pendingCode
}

func newDeviceCodes() *deviceCodes {
	return &deviceCodes{byCode: map[string]*pendingCode{}, bySecret: map[tokenHash]*pendingCode{}}
}

func (dc *deviceCodes) removeLocked(pl *pendingCode) {
	delete(dc.byCode, pl.code)
	delete(dc.bySecret, pl.secret)
}

func (dc *deviceCodes) pruneLocked(now time.Time) {
	for _, pl := range dc.byCode {
		if now.Sub(pl.expires) > codeTTL {
			dc.removeLocked(pl)
		}
	}
}

// StartDeviceCode starts a sign-in for the named device. One address, or
// one IPv6 /64, may have a few codes pending at once. Past that it returns
// a *ThrottledError until the oldest code expires.
func (s *Service) StartDeviceCode(_ context.Context, r DeviceCodeStart) (DeviceCode, error) {
	c, err := checkClient(client{kind: r.Kind, name: r.Name, id: r.ClientID, device: r.Device}, SessionApp)
	if err != nil {
		return DeviceCode{}, err
	}
	if c.name == "" {
		return DeviceCode{}, ErrInvalidName
	}
	secret, hash := newToken()
	now := time.Now()
	pl := &pendingCode{
		client: c, secret: hash, addr: ClientAddr(r.RemoteIP), nearby: s.deps.Settings.Get().IsLocal(r.RemoteIP),
		started: now, expires: now.Add(codeTTL),
	}

	dc := s.codes
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.pruneLocked(now)
	if len(dc.byCode) >= maxCodes {
		return DeviceCode{}, fmt.Errorf("%w: %d device codes pending", ErrBusy, len(dc.byCode))
	}
	var mine int
	oldest := pl.expires
	for _, o := range dc.byCode {
		if o.addr == pl.addr && now.Before(o.expires) && !o.answered() {
			mine++
			if o.expires.Before(oldest) {
				oldest = o.expires
			}
		}
	}
	if mine >= maxCodesPerAddr {
		return DeviceCode{}, &ThrottledError{RetryAfter: oldest.Sub(now)}
	}
	for pl.code == "" || dc.byCode[pl.code] != nil {
		pl.code = newCode()
	}
	dc.byCode[pl.code] = pl
	dc.bySecret[pl.secret] = pl
	return DeviceCode{Code: pl.code, Secret: secret, Expires: pl.expires, Interval: pollInterval}, nil
}

func newCode() string {
	b := make([]byte, codeLen)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = codeAlphabet[b[i]%byte(len(codeAlphabet))] // 256 is a multiple of 32, so every symbol is as likely
	}
	return string(b)
}

// LookUpDeviceCode returns the device waiting on code. It is shown before
// the user approves or denies the code.
func (s *Service) LookUpDeviceCode(_ context.Context, p Caller, code string, ip netip.Addr) (PendingDeviceCode, error) {
	var out PendingDeviceCode
	err := s.withWaitingCode(p, code, ip, func(pl *pendingCode) {
		out = PendingDeviceCode{Name: pl.client.name, Platform: pl.client.device.Platform, Nearby: pl.nearby, Started: pl.started}
	})
	if err != nil {
		return PendingDeviceCode{}, err
	}
	return out, nil
}

// ApproveDeviceCode lets the device waiting on code sign in as p's user.
// A code can be answered only once.
func (s *Service) ApproveDeviceCode(_ context.Context, p Caller, code string, ip netip.Addr) error {
	if err := s.withWaitingCode(p, code, ip, func(pl *pendingCode) { pl.approver = p.User }); err != nil {
		return err
	}
	return nil
}

// DenyDeviceCode refuses the device waiting on code. Its next poll gets
// ErrDenied. A code can be answered only once.
func (s *Service) DenyDeviceCode(_ context.Context, p Caller, code string, ip netip.Addr) error {
	if err := s.withWaitingCode(p, code, ip, func(pl *pendingCode) { pl.denied = true }); err != nil {
		return err
	}
	return nil
}

// withWaitingCode runs fn on the code if it's waiting for an answer. Codes
// are short, so a wrong code counts against the user and address the way a
// wrong password does. Enough wrong codes return a *ThrottledError.
func (s *Service) withWaitingCode(p Caller, code string, ip netip.Addr, fn func(*pendingCode)) error {
	try, d := s.throttle.begin(ip, "\x00"+p.User.String())
	if d > 0 {
		return &ThrottledError{RetryAfter: d}
	}
	code = strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(code))
	now := time.Now()
	dc := s.codes
	dc.mu.Lock()
	pl := dc.byCode[code]
	ok := pl != nil && now.Before(pl.expires) && !pl.answered()
	if ok {
		fn(pl)
	}
	dc.mu.Unlock()
	if !ok {
		try.fail()
		return ErrCodeNotFound
	}
	try.succeed()
	return nil
}

// PollDeviceCode is the device asking whether its code was answered. It
// returns false while the code waits. Once approved, it returns a session
// for the approving user, one time only, replacing that user's session on
// the same client. A denied code returns ErrDenied. An unknown secret, or a
// code that expired or was used, returns ErrCodeGone. Unknown secrets count
// against the address, as wrong passwords do.
func (s *Service) PollDeviceCode(ctx context.Context, secret string, ip netip.Addr) (IssuedSession, bool, error) {
	try, d := s.throttle.begin(ip, pollKey)
	if d > 0 {
		return IssuedSession{}, false, &ThrottledError{RetryAfter: d}
	}
	defer try.end()
	hash, _ := hashToken(secret) // a malformed secret gives the zero hash, which no code has
	now := time.Now()
	dc := s.codes
	dc.mu.Lock()
	dc.pruneLocked(now)
	pl := dc.bySecret[hash]
	var err error
	switch {
	case pl == nil:
		try.fail()
		err = ErrCodeGone
	case pl.denied:
		err = ErrDenied
	case !now.Before(pl.expires):
		err = ErrCodeGone
	case pl.approver.IsZero():
		dc.mu.Unlock()
		return IssuedSession{}, false, nil
	default:
		dc.removeLocked(pl)
	}
	dc.mu.Unlock()
	if err != nil {
		return IssuedSession{}, false, err
	}

	var out IssuedSession
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		var err error
		if out, err = s.issue(ctx, tx, pl.approver, pl.client); err != nil {
			return err
		}
		return s.deps.Activity.SignedIn(ctx, tx, s.signIn(pl.approver, pl.client.name, ip))
	})
	if err != nil {
		return IssuedSession{}, false, err
	}
	return out, true, nil
}
