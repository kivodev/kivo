package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mmcdole/kivo/internal/auth/dbq"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

// Invite is a pending invite link. Whoever accepts it gets exactly its
// grants and limits.
type Invite struct {
	ID        id.ID
	Grants    users.Grants
	Limits    users.Limits
	CreatedBy string // the admin's username
	Created   time.Time
	Expires   time.Time
}

// NewInvite describes an invite link to create.
type NewInvite struct {
	Grants  users.Grants
	Limits  users.Limits
	Expires time.Time
}

// CreateInvite creates an invite link and returns it with its token. Only
// the token's hash is stored. by is the admin creating the invite.
func (s *Service) CreateInvite(ctx context.Context, by Caller, n NewInvite) (IssuedInvite, error) {
	if !n.Grants.All && len(n.Grants.Libraries) == 0 {
		return IssuedInvite{}, users.ErrNoLibraries
	}
	if !n.Expires.After(time.Now()) {
		return IssuedInvite{}, ErrInvalidExpiry
	}
	token, hash := newBareToken()
	out := Invite{ID: id.New(), Grants: n.Grants, Limits: n.Limits, Created: time.Now().Truncate(time.Millisecond),
		Expires: n.Expires.Truncate(time.Millisecond)}
	if out.Grants.All {
		out.Grants.Libraries = nil
	}
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		err := q.InsertInvite(ctx, dbq.InsertInviteParams{
			ID: out.ID, TokenHash: hash[:], CreatedBy: by.User, AllLibraries: out.Grants.All,
			StreamLimit: nullLimit(int64(n.Limits.StreamLimit)), RemoteQualityCap: nullLimit(n.Limits.RemoteQualityCap),
			ExpiresAt: out.Expires.UnixMilli(), CreatedAt: out.Created.UnixMilli(),
		})
		if err != nil {
			return err
		}
		for _, lib := range out.Grants.Libraries {
			err := q.InsertInviteLibrary(ctx, dbq.InsertInviteLibraryParams{InviteID: out.ID, LibraryID: lib})
			if store.IsForeignKey(err) {
				return fmt.Errorf("%w: %v", users.ErrUnknownLibrary, lib)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return IssuedInvite{}, err
	}
	return IssuedInvite{Token: token, Invite: out}, nil
}

// Invites returns the pending invite links, newest first.
func (s *Service) Invites(ctx context.Context) ([]Invite, error) {
	var out []Invite
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		q := dbq.New(tx)
		rows, err := q.PendingInvites(ctx, time.Now().UnixMilli())
		if err != nil {
			return err
		}
		for _, r := range rows {
			libs, err := q.InviteLibraries(ctx, r.ID)
			if err != nil {
				return err
			}
			out = append(out, Invite{
				ID: r.ID, Grants: users.Grants{All: r.AllLibraries, Libraries: libs},
				Limits:    users.Limits{StreamLimit: int(r.StreamLimit.Int64), RemoteQualityCap: r.RemoteQualityCap.Int64},
				CreatedBy: r.CreatedBy, Created: time.UnixMilli(r.CreatedAt), Expires: time.UnixMilli(r.ExpiresAt),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RevokeInvite deletes an invite link, which stops working at once.
func (s *Service) RevokeInvite(ctx context.Context, invite id.ID) error {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		n, err := dbq.New(tx).DeleteInvite(ctx, invite)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%w: %s", ErrInviteNotFound, invite)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// InviteByToken returns the pending invite a link's token names.
func (s *Service) InviteByToken(ctx context.Context, token string) (Invite, error) {
	r, err := s.inviteByToken(ctx, s.q, token)
	if err != nil {
		return Invite{}, err
	}
	return Invite{ID: r.ID, CreatedBy: r.Username, Expires: time.UnixMilli(r.ExpiresAt)}, nil
}

func (s *Service) inviteByToken(ctx context.Context, q *dbq.Queries, token string) (dbq.InviteByTokenRow, error) {
	hash, ok := hashBareToken(token)
	if !ok {
		return dbq.InviteByTokenRow{}, ErrInviteNotFound
	}
	r, err := q.InviteByToken(ctx, dbq.InviteByTokenParams{TokenHash: hash[:], Now: time.Now().UnixMilli()})
	if errors.Is(err, sql.ErrNoRows) {
		return dbq.InviteByTokenRow{}, ErrInviteNotFound
	}
	if err != nil {
		return dbq.InviteByTokenRow{}, err
	}
	return r, nil
}

// AcceptInvite creates the user an invite link offers, and signs the user
// in. The user gets the username and password chosen in r, and exactly the
// invite's grants and limits. The link then stops working.
func (s *Service) AcceptInvite(ctx context.Context, token string, r SignInRequest) (IssuedSession, error) {
	c, err := checkRequest(r)
	if err != nil {
		return IssuedSession{}, err
	}
	if err := checkPassword(r.Password); err != nil {
		return IssuedSession{}, err
	}
	// The link is checked here first, so a dead link doesn't spend a hash.
	if _, err := s.inviteByToken(ctx, s.q, token); err != nil {
		return IssuedSession{}, err
	}
	phc, err := s.hasher.hash(r.Password)
	if err != nil {
		return IssuedSession{}, err
	}
	var out IssuedSession
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		inv, err := s.inviteByToken(ctx, q, token)
		if err != nil {
			return err
		}
		libs, err := q.InviteLibraries(ctx, inv.ID)
		if err != nil {
			return err
		}
		u, err := s.createUser(ctx, tx, users.NewUser{
			Username:  r.Username,
			Grants:    users.Grants{All: inv.AllLibraries, Libraries: libs},
			Limits:    users.Limits{StreamLimit: int(inv.StreamLimit.Int64), RemoteQualityCap: inv.RemoteQualityCap.Int64},
			InvitedBy: inv.CreatedBy,
		}, phc)
		if err != nil {
			return err
		}
		if n, err := q.UseInvite(ctx, dbq.UseInviteParams{Now: time.Now().UnixMilli(), UsedBy: u.ID, ID: inv.ID}); err != nil {
			return err
		} else if n == 0 {
			return fmt.Errorf("%w: %s", ErrInviteNotFound, inv.ID)
		}
		if out, err = s.issue(ctx, tx, u.ID, c); err != nil {
			return err
		}
		return s.deps.Activity.Joined(ctx, tx, u.ID, id.ID{})
	})
	if err != nil {
		return IssuedSession{}, err
	}
	return out, nil
}

// nullLimit returns a limit's column value. A limit of 0 or less means no
// limit and is stored as NULL.
func nullLimit(n int64) sql.NullInt64 { return sql.NullInt64{Int64: n, Valid: n > 0} }
