package users

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/mmcdole/kivo/internal/users/dbq"

	"github.com/mmcdole/kivo/internal/id"
)

// Viewer is what one user may see, built fresh for each request. Its
// fields are unexported so that only Service.Viewer can make one, and the
// zero Viewer sees nothing.
type Viewer struct {
	user        id.ID
	all         bool    // every library
	libs        []id.ID // sorted
	admin       bool
	ceiling     int64 // normalized age rank, meaningful only when hasCeiling
	hasCeiling  bool
	showUnrated bool
}

// Viewer builds the Viewer for a user from its current grants, so a change
// applies to the next request. Only api, playback (for a play's owner) and
// cmd/kivo call it. Everything else takes a Viewer.
func (s *Service) Viewer(ctx context.Context, user id.ID) (Viewer, error) {
	var v Viewer
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		q := dbq.New(tx)
		u, err := q.GetUser(ctx, user)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrNotFound, user)
		}
		if err != nil {
			return err
		}
		if u.Disabled {
			return fmt.Errorf("%w: %s", ErrDisabled, user)
		}
		v = Viewer{
			user:        user,
			all:         u.AllLibraries,
			admin:       u.Admin,
			ceiling:     u.RatingCeiling.Int64,
			hasCeiling:  u.RatingCeiling.Valid,
			showUnrated: !u.BlockUnrated,
		}
		if !v.all {
			if v.libs, err = q.Grants(ctx, user); err != nil {
				return err
			}
			slices.SortFunc(v.libs, compareID)
		}
		return nil
	})
	if err != nil {
		return Viewer{}, err
	}
	return v, nil
}

func compareID(a, b id.ID) int { return bytes.Compare(a[:], b[:]) }

// User returns the user the Viewer belongs to, or zero for the zero Viewer.
func (v Viewer) User() id.ID { return v.user }

// Admin reports whether the user is an admin.
func (v Viewer) Admin() bool { return v.admin }

// WithoutAdmin returns the Viewer as an ordinary user. It keeps the same
// libraries and limits, but loses everything only an admin may do.
func (v Viewer) WithoutAdmin() Viewer {
	v.admin = false
	return v
}

// AllLibraries reports whether the Viewer sees every library, including ones
// added later. Libraries is then empty, because users doesn't know which
// libraries exist.
func (v Viewer) AllLibraries() bool { return v.all }

// Libraries returns the granted libraries, sorted. It is empty when
// AllLibraries is true.
func (v Viewer) Libraries() []id.ID { return slices.Clone(v.libs) }

// CanSee reports whether the Viewer may see a library.
func (v Viewer) CanSee(lib id.ID) bool {
	if lib.IsZero() {
		return false
	}
	if v.all {
		return true
	}
	_, ok := slices.BinarySearchFunc(v.libs, lib, compareID)
	return ok
}

// RatingCeiling returns the highest normalized age rank the user may see,
// and false when it has no ceiling.
func (v Viewer) RatingCeiling() (int64, bool) { return v.ceiling, v.hasCeiling }

// ShowsUnrated reports whether the user sees video works with no rating.
func (v Viewer) ShowsUnrated() bool { return v.showUnrated }

// Access describes one library a work is available in, with the work's kind
// and content rating age. The age is the work's own, or else its series'.
// It is nil when the work is unrated.
type Access struct {
	Library id.ID
	Kind    string
	Age     *int
}

// videoKinds are the work kinds hidden when unrated from a user who blocks
// unrated works. A book carries no rating, so it always shows.
var videoKinds = []string{"movie", "series", "season", "episode", "extra", "video"}

// CanSeeWork reports whether the Viewer may see a work available as access
// describes. The work must be in a granted library. Its age there must be
// within the user's ceiling. If it is unrated, either the user shows
// unrated works or the work isn't video. This is query's viewer filter
// written in Go, for playback's checks on media requests. A test in query
// keeps the two giving the same answers.
func (v Viewer) CanSeeWork(access []Access) bool {
	for _, a := range access {
		if !v.CanSee(a.Library) {
			continue
		}
		if a.Age == nil {
			if v.showUnrated || !slices.Contains(videoKinds, a.Kind) {
				return true
			}
			continue
		}
		if !v.hasCeiling || int64(*a.Age) <= v.ceiling {
			return true
		}
	}
	return false
}
