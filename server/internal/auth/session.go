package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/auth/dbq"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
)

// issue starts a session in the caller's transaction. It revokes the user's
// existing session on the same client.
func (s *Service) issue(ctx context.Context, tx *store.Tx, user id.ID, c client) (IssuedSession, error) {
	q := dbq.New(tx)
	token, hash := newCredential(sessionCredential)
	now := time.Now().Truncate(time.Millisecond)
	out := IssuedSession{Token: token, ClientID: c.id, Session: Session{
		ID: id.New(), User: user, Kind: c.kind, Name: c.name, Device: c.device, Created: now, LastUsed: now,
	}}
	if c.id != "" {
		if err := q.DeleteClientSession(ctx, dbq.DeleteClientSessionParams{UserID: user, ClientID: c.id}); err != nil {
			return IssuedSession{}, err
		}
	}
	err := q.InsertSession(ctx, dbq.InsertSessionParams{
		ID:          out.Session.ID,
		TokenHash:   hash[:],
		ProbeID:     probeID(hash),
		UserID:      user,
		Kind:        string(c.kind),
		Name:        c.name,
		Platform:    string(c.device.Platform),
		ClientID:    c.id,
		Now:         now.UnixMilli(),
		App:         c.device.App,
		AppVersion:  c.device.AppVersion,
		OsVersion:   c.device.OSVersion,
		DeviceModel: c.device.Model,
	})
	if err != nil {
		return IssuedSession{}, err
	}
	return out, nil
}

// Authenticate resolves a token from a cookie or an Authorization header to
// its session, API key or cast token. The token's prefix names the table,
// and one read of it also checks that the user isn't disabled. A token
// without a known prefix, or with a malformed body, is refused without a
// read. It writes the last-used time when the stored one is stale.
func (s *Service) Authenticate(ctx context.Context, token string) (Caller, error) {
	kind, hash, ok := parseCredential(token)
	if !ok {
		return Caller{}, fmt.Errorf("%w: malformed token", ErrUnauthenticated)
	}
	switch kind {
	case sessionCredential:
		return s.sessionCaller(ctx, hash)
	case apiKeyCredential:
		return s.apiKeyCaller(ctx, hash)
	case castCredential:
		return s.castCaller(ctx, hash)
	}
	return Caller{}, fmt.Errorf("%w: token kind %q", ErrUnauthenticated, kind)
}

// sessionCaller resolves a session token's hash to its session, and writes
// its last-used time when the stored one is stale.
func (s *Service) sessionCaller(ctx context.Context, hash tokenHash) (Caller, error) {
	row, err := s.q.SessionCallerByToken(ctx, hash[:])
	if errors.Is(err, sql.ErrNoRows) {
		return Caller{}, fmt.Errorf("%w: no such session", ErrUnauthenticated)
	}
	if err != nil {
		return Caller{}, err
	}
	now := time.Now()
	stored := time.UnixMilli(row.LastUsedAt)
	if expired(SessionKind(row.Kind), stored, now) {
		return Caller{}, fmt.Errorf("%w: session expired", ErrUnauthenticated)
	}
	s.markUsed(ctx, stored, now, func(ctx context.Context, q *dbq.Queries, used int64) error {
		return q.TouchSession(ctx, dbq.TouchSessionParams{ID: row.ID, Used: used})
	})
	return Caller{User: row.UserID, Session: row.ID, Name: row.Name, Device: Device{
		App: row.App, AppVersion: row.AppVersion, Platform: Platform(row.Platform), OSVersion: row.OsVersion, Model: row.DeviceModel,
	}}, nil
}

// Alive reports whether a caller's session, API key or cast token still
// authenticates. The credential must exist and its user must not be
// disabled. A session or cast token must not have expired, and a cast
// token's session must still exist. playback checks this on each media
// request of a play the caller started.
func (s *Service) Alive(ctx context.Context, c Caller) (bool, error) {
	if !c.CastToken.IsZero() {
		return s.castAlive(ctx, c.CastToken)
	}
	if c.Session.IsZero() {
		ok, err := s.q.APIKeyAlive(ctx, c.APIKey)
		if err != nil {
			return false, err
		}
		return ok == 1, nil
	}
	row, err := s.q.SessionAlive(ctx, c.Session)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !expired(SessionKind(row.Kind), time.UnixMilli(row.LastUsedAt), time.Now()), nil
}

// SignOut ends the caller's own session, and its token stops working at
// once. A caller using an API key has no session, so it gets ErrNotFound.
func (s *Service) SignOut(ctx context.Context, c Caller) error {
	if c.Session.IsZero() {
		return fmt.Errorf("%w: sign out", ErrNotFound)
	}
	return s.revoke(ctx, c.Session)
}

// SignOutOthers ends every session of the caller's user except the
// caller's own. API keys stay. A caller using an API key has no session to
// keep, so it gets ErrNotFound.
func (s *Service) SignOutOthers(ctx context.Context, c Caller) error {
	if c.Session.IsZero() {
		return fmt.Errorf("%w: sign out others", ErrNotFound)
	}
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return dbq.New(tx).DeleteOtherSessions(ctx, dbq.DeleteOtherSessionsParams{UserID: c.User, Keep: c.Session})
	})
}

// revoke ends a session.
func (s *Service) revoke(ctx context.Context, session id.ID) error {
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		_, err := dbq.New(tx).DeleteSession(ctx, session)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: session %s", ErrNotFound, session)
		}
		return err
	})
}

// Sessions lists a user's sessions, most recently used first. A user sees
// their own sessions, and an admin sees anyone's. Anyone else gets
// ErrNotFound.
func (s *Service) Sessions(ctx context.Context, by Caller, user id.ID) ([]Session, error) {
	if err := s.mayManage(ctx, by, user); err != nil {
		return nil, err
	}
	rows, err := s.q.UserSessions(ctx, user)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]Session, 0, len(rows))
	for _, r := range rows {
		kind := SessionKind(r.Kind)
		if expired(kind, time.UnixMilli(r.LastUsedAt), now) {
			continue
		}
		out = append(out, s.session(dbq.SessionRow(r)))
	}
	slices.SortFunc(out, func(a, b Session) int { return b.LastUsed.Compare(a.LastUsed) })
	return out, nil
}

// Session returns one session. by may read its own user's sessions, and an
// admin may read any. Anyone else's session, or an expired one, returns
// ErrNotFound.
func (s *Service) Session(ctx context.Context, by Caller, session id.ID) (Session, error) {
	r, err := s.q.Session(ctx, session)
	if errors.Is(err, sql.ErrNoRows) || err == nil && expired(SessionKind(r.Kind), time.UnixMilli(r.LastUsedAt), time.Now()) {
		return Session{}, fmt.Errorf("%w: session %s", ErrNotFound, session)
	}
	if err != nil {
		return Session{}, err
	}
	if err := s.mayManage(ctx, by, r.UserID); err != nil {
		return Session{}, err
	}
	return s.session(r), nil
}

func (s *Service) session(r dbq.SessionRow) Session {
	return Session{
		ID: r.ID, User: r.UserID, Kind: SessionKind(r.Kind), Name: r.Name,
		Device:  Device{App: r.App, AppVersion: r.AppVersion, Platform: Platform(r.Platform), OSVersion: r.OsVersion, Model: r.DeviceModel},
		Created: time.UnixMilli(r.CreatedAt), LastUsed: time.UnixMilli(r.LastUsedAt),
	}
}

// RenameSession names a session on behalf of by. by may rename its own
// user's sessions, and an admin may rename any. Anyone else's session, or
// an expired one, returns ErrNotFound. The name is trimmed, and must be 1
// to 100 characters.
func (s *Service) RenameSession(ctx context.Context, by Caller, session id.ID, name string) (Session, error) {
	name, err := checkName(name)
	if err == nil && name == "" {
		err = ErrInvalidName
	}
	if err != nil {
		return Session{}, err
	}
	out, err := s.Session(ctx, by, session)
	if err != nil {
		return Session{}, err
	}
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return dbq.New(tx).SetSessionName(ctx, dbq.SetSessionNameParams{ID: session, Name: name})
	})
	if err != nil {
		return Session{}, err
	}
	out.Name = name
	return out, nil
}

// SeeDevice records the app and device a session's request identified,
// when they differ from the ones the session holds (c.Device). A request
// that identifies nothing, an API key and a cast token change nothing.
func (s *Service) SeeDevice(ctx context.Context, c Caller, d Device) error {
	if !d.Platform.valid() {
		d.Platform = ""
	}
	if c.Session.IsZero() || d == (Device{}) {
		return nil
	}
	if d.Platform == "" {
		d.Platform = c.Device.Platform
	}
	if d == c.Device {
		return nil
	}
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return dbq.New(tx).SetSessionDevice(ctx, dbq.SetSessionDeviceParams{
			ID: c.Session, App: d.App, AppVersion: d.AppVersion, OsVersion: d.OSVersion, DeviceModel: d.Model,
			Platform: string(d.Platform),
		})
	})
}

// RevokeSession signs a session out on behalf of by. by may revoke its own
// user's sessions, and an admin may revoke any. Anyone else's session
// returns ErrNotFound.
func (s *Service) RevokeSession(ctx context.Context, by Caller, session id.ID) error {
	r, err := s.q.Session(ctx, session)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: session %s", ErrNotFound, session)
	}
	if err != nil {
		return err
	}
	if err := s.mayManage(ctx, by, r.UserID); err != nil {
		return err
	}
	return s.revoke(ctx, session)
}

// mayManage checks that by may see and sign out user's sessions. That holds
// when user is by's own user, or when by is an admin.
func (s *Service) mayManage(ctx context.Context, by Caller, user id.ID) error {
	if by.User == user {
		return nil
	}
	u, err := s.deps.Users.User(ctx, by.User)
	if err != nil {
		return err
	}
	if !u.Admin {
		return fmt.Errorf("%w: user %s", ErrNotFound, user)
	}
	if _, err := s.deps.Users.User(ctx, user); err != nil {
		return err
	}
	return nil
}

// browserIdle is how long a browser session lasts unused.
const browserIdle = 30 * 24 * time.Hour

// staleAfter is how old a stored last-used time gets before a request
// writes a new one, so the stored time lags real use by at most a minute,
// which is small next to castIdle.
const staleAfter = time.Minute

// expired reports whether a browser session has been idle too long.
func expired(kind SessionKind, used, now time.Time) bool {
	return kind == SessionBrowser && now.Sub(used) > browserIdle
}

// markUsed writes now as a credential's last-used time with touch, when the
// stored time is more than staleAfter old. A failed write is logged, and the
// request carries on.
func (s *Service) markUsed(ctx context.Context, stored, now time.Time, touch func(ctx context.Context, q *dbq.Queries, used int64) error) {
	if now.Sub(stored) <= staleAfter {
		return
	}
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return touch(ctx, dbq.New(tx), now.UnixMilli())
	})
	if err != nil {
		slog.WarnContext(ctx, "auth: write last-used time", "err", err)
	}
}

// dropExpired drops browser sessions that have been idle too long and expired
// cast tokens, one batch at a time.
func (s *Service) dropExpired(ctx context.Context) {
	now := time.Now()
	before := now.Add(-browserIdle).UnixMilli()
	err := s.db.Drain(ctx, func(ctx context.Context, tx *store.Tx) (int, error) {
		n, err := dbq.New(tx).DeleteIdleBrowserSessions(ctx, dbq.DeleteIdleBrowserSessionsParams{Before: before, Limit: store.BatchSize})
		return int(n), err
	})
	if err == nil {
		err = s.deleteExpiredCastTokens(ctx, now)
	}
	if err != nil {
		slog.ErrorContext(ctx, "auth: drop expired sessions", "err", err)
	}
}
