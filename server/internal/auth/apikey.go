package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/auth/dbq"
	"github.com/mmcdole/kivo/internal/id"
)

// NewAPIKey describes an API key to create.
type NewAPIKey struct {
	Name string // 1 to 100 characters
}

// APIKey is a key for scripts and other tools. It has the admin access of
// the user that created it, and setting that user's password doesn't revoke
// it.
type APIKey struct {
	ID       id.ID
	Name     string
	Created  time.Time
	LastUsed time.Time // zero until the key is used
}

// CreateAPIKey creates an API key for by and returns it with its token. by
// must be an admin, which api checks. Only the token's hash is stored.
func (s *Service) CreateAPIKey(ctx context.Context, by Caller, k NewAPIKey) (IssuedAPIKey, error) {
	name, err := checkName(k.Name)
	if err != nil {
		return IssuedAPIKey{}, err
	}
	if name == "" {
		return IssuedAPIKey{}, ErrInvalidName
	}
	token, hash := newToken()
	key := APIKey{ID: id.New(), Name: name, Created: time.Now().Truncate(time.Millisecond)}
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return dbq.New(tx).InsertAPIKey(ctx, dbq.InsertAPIKeyParams{
			ID: key.ID, TokenHash: hash[:], Name: name, CreatedBy: by.User, CreatedAt: key.Created.UnixMilli(),
		})
	})
	if err != nil {
		return IssuedAPIKey{}, err
	}
	return IssuedAPIKey{Token: token, Key: key}, nil
}

// APIKeys returns every API key, newest first.
func (s *Service) APIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.q.APIKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]APIKey, len(rows))
	for i, r := range rows {
		out[i] = s.apiKey(dbq.APIKeyRow(r))
	}
	return out, nil
}

// APIKey returns one API key.
func (s *Service) APIKey(ctx context.Context, key id.ID) (APIKey, error) {
	r, err := s.q.APIKey(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return APIKey{}, fmt.Errorf("%w: API key %s", ErrNotFound, key)
	}
	if err != nil {
		return APIKey{}, err
	}
	return s.apiKey(r), nil
}

func (s *Service) apiKey(r dbq.APIKeyRow) APIKey {
	var stored time.Time
	if r.LastUsedAt.Valid {
		stored = time.UnixMilli(r.LastUsedAt.Int64)
	}
	return APIKey{ID: r.ID, Name: r.Name, Created: time.UnixMilli(r.CreatedAt), LastUsed: s.lastUsed(r.ID, stored)}
}

// RevokeAPIKey deletes an API key, and its token stops working at once.
// Only an admin may, which api checks.
func (s *Service) RevokeAPIKey(ctx context.Context, key id.ID) error {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		_, err := dbq.New(tx).DeleteAPIKey(ctx, key)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: API key %s", ErrNotFound, key)
		}
		if err != nil {
			return err
		}
		s.revokeAtCommit(tx, key)
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
