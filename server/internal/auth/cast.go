package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mmcdole/kivo/internal/auth/dbq"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
)

// CastReceiver is what plays a cast: the Web Receiver page on any Google
// Cast device, or the Android TV app opened by Cast Connect.
type CastReceiver string

const (
	CastWeb       CastReceiver = "web"
	CastAndroidTV CastReceiver = "androidTv"
)

func (r CastReceiver) valid() bool { return r == CastWeb || r == CastAndroidTV }

// ErrInvalidReceiver means a cast token was asked for a receiver that
// doesn't exist.
var ErrInvalidReceiver = fault.New(fault.Invalid, "receiver must be web or androidTv")

// A cast token ends after castIdle unused, and lasts castLifetime at most.
const (
	castIdle     = 30 * time.Minute
	castLifetime = 24 * time.Hour
)

// NewCastToken is a cast token to issue. It holds the receiver's name, such
// as "Living Room TV", and the kind of receiver.
type NewCastToken struct {
	Name     string
	Receiver CastReceiver
}

// IssuedCastToken is a new cast token, which is shown once, and the latest
// time it can last. If it goes unused for 30 minutes it ends sooner.
type IssuedCastToken struct {
	Token   string
	Expires time.Time
}

// IssueCastToken gives by's session a cast token for a receiver. The token
// is a credential that plays as by's user. It reaches only playback, since
// api limits it to the cast routes. It ends when by's session ends. A
// caller using an API key or another cast token has no session to bind it
// to, so it gets ErrNotFound.
func (s *Service) IssueCastToken(ctx context.Context, by Caller, n NewCastToken) (IssuedCastToken, error) {
	if by.Session.IsZero() {
		return IssuedCastToken{}, fmt.Errorf("%w: issue cast token", ErrNotFound)
	}
	name, err := checkName(n.Name)
	if err != nil {
		return IssuedCastToken{}, err
	}
	if name == "" {
		return IssuedCastToken{}, ErrInvalidName
	}
	if !n.Receiver.valid() {
		return IssuedCastToken{}, fmt.Errorf("%w: %q", ErrInvalidReceiver, n.Receiver)
	}
	token, hash := newCredential(castCredential)
	now := time.Now().Truncate(time.Millisecond)
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return dbq.New(tx).InsertCastToken(ctx, dbq.InsertCastTokenParams{
			ID: id.New(), TokenHash: hash[:], SessionID: by.Session, Name: name, Receiver: string(n.Receiver), Now: now.UnixMilli(),
		})
	})
	if err != nil {
		return IssuedCastToken{}, err
	}
	return IssuedCastToken{Token: token, Expires: now.Add(castLifetime)}, nil
}

// RevokeCastToken ends the caller's own cast token. A caller that isn't
// using a cast token gets ErrNotFound.
func (s *Service) RevokeCastToken(ctx context.Context, c Caller) error {
	if c.CastToken.IsZero() {
		return fmt.Errorf("%w: revoke cast token", ErrNotFound)
	}
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		n, err := dbq.New(tx).DeleteCastToken(ctx, c.CastToken)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%w: cast token %s", ErrNotFound, c.CastToken)
		}
		return nil
	})
}

// castCaller resolves a cast token's hash to its cast token, and writes its
// last-used time when the stored one is stale.
func (s *Service) castCaller(ctx context.Context, hash tokenHash) (Caller, error) {
	row, err := s.q.CastCallerByToken(ctx, hash[:])
	if errors.Is(err, sql.ErrNoRows) {
		return Caller{}, fmt.Errorf("%w: no such token", ErrUnauthenticated)
	}
	if err != nil {
		return Caller{}, err
	}
	now := time.Now()
	stored := time.UnixMilli(row.LastUsedAt)
	if castExpired(time.UnixMilli(row.CreatedAt), stored, now) ||
		expired(SessionKind(row.SessionKind), time.UnixMilli(row.SessionUsedAt), now) {
		return Caller{}, fmt.Errorf("%w: cast token expired", ErrUnauthenticated)
	}
	s.markUsed(ctx, stored, now, func(ctx context.Context, q *dbq.Queries, used int64) error {
		return q.TouchCastToken(ctx, dbq.TouchCastTokenParams{ID: row.ID, Used: used})
	})
	return Caller{User: row.UserID, CastToken: row.ID, Name: row.Name}, nil
}

// castAlive reports whether a cast token still authenticates. The token,
// its session and its user must all still exist, and neither the token nor
// its session may have expired.
func (s *Service) castAlive(ctx context.Context, token id.ID) (bool, error) {
	row, err := s.q.CastTokenAlive(ctx, token)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	now := time.Now()
	return !castExpired(time.UnixMilli(row.CreatedAt), time.UnixMilli(row.LastUsedAt), now) &&
		!expired(SessionKind(row.SessionKind), time.UnixMilli(row.SessionUsedAt), now), nil
}

// castExpired reports whether a cast token went unused too long or
// outlived its lifetime.
func castExpired(created, used, now time.Time) bool {
	return now.Sub(used) > castIdle || now.Sub(created) > castLifetime
}

// deleteExpiredCastTokens drops the cast tokens past their idle time or
// lifetime, one batch at a time.
func (s *Service) deleteExpiredCastTokens(ctx context.Context, now time.Time) error {
	return s.db.Drain(ctx, func(ctx context.Context, tx *store.Tx) (int, error) {
		n, err := dbq.New(tx).DeleteExpiredCastTokens(ctx, dbq.DeleteExpiredCastTokensParams{
			IdleBefore: now.Add(-castIdle).UnixMilli(), MadeBefore: now.Add(-castLifetime).UnixMilli(), Limit: store.BatchSize,
		})
		return int(n), err
	})
}
