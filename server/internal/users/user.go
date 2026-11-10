package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users/dbq"
)

var (
	// ErrNotFound means there is no such user.
	ErrNotFound = fault.New(fault.NotFound, "user not found")
	// ErrDisabled means the user is disabled, so it gets no Viewer.
	ErrDisabled = fault.New(fault.Unauthenticated, "user disabled")
	// ErrNameTaken means the username is in use. Usernames compare
	// case-insensitively.
	ErrNameTaken = fault.Coded(fault.Conflict, "nameTaken", "name taken")
	// ErrInvalidName means a username is empty, too long, padded with spaces
	// or contains control characters.
	ErrInvalidName = fault.New(fault.Invalid, "invalid name")
	// ErrLastAdmin means the change would leave no enabled admin.
	ErrLastAdmin = fault.Coded(fault.Conflict, "lastAdmin", "last admin")
	// ErrOwnUser means an admin would delete, disable or remove admin from
	// their own user.
	ErrOwnUser = fault.Coded(fault.Conflict, "ownUser", "own user")
	// ErrNoLibraries means a user was given neither every library nor
	// any one of them.
	ErrNoLibraries = fault.New(fault.Invalid, "no libraries chosen")
	// ErrUnknownLibrary means a library to grant doesn't exist.
	ErrUnknownLibrary = fault.New(fault.Invalid, "unknown library")
	// ErrInvalidPlayback means a playback preference has a value it can't
	// take, such as a language that isn't a BCP 47 tag.
	ErrInvalidPlayback = fault.New(fault.Invalid, "invalid playback preference")
)

// Service manages users and their grants.
type Service struct {
	db  *store.DB
	q   *dbq.Queries
	bus *events.Bus
}

// New returns a Service over db that publishes its events on bus.
func New(db *store.DB, bus *events.Bus) *Service {
	return &Service{db: db, q: dbq.New(db.Reader()), bus: bus}
}

// User is a person who signs in.
type User struct {
	ID       id.ID
	Username string
	Admin    bool
	Grants   Grants
	Limits   Limits
	Parental Parental
	Playback Playback
	// Downloads is whether the user may download. Currently every user may.
	Downloads bool
	Disabled  bool
	// OnlyAdmin is whether the user is the only enabled admin. ErrLastAdmin
	// keeps such a user an enabled admin.
	OnlyAdmin bool
	// InvitedBy is the admin whose invite the user came from. It is zero
	// when the user was created directly or the inviter is gone.
	InvitedBy id.ID
	Created   time.Time
}

// Grants are the libraries a user may see.
type Grants struct {
	// All grants every library, including ones added later. The first
	// admin gets it, because only catalog knows which libraries exist.
	All       bool
	Libraries []id.ID // sorted, and empty when All is set
}

// Limits are a user's stream limits, which playback enforces.
type Limits struct {
	StreamLimit      int   // simultaneous streams; 0 means unlimited
	RemoteQualityCap int64 // bits per second for remote sessions; 0 means no cap
}

// Parental is a user's rating rules, which query and playback enforce.
type Parental struct {
	// Ceiling is the highest normalized age rank the user sees. Nil means
	// no ceiling.
	Ceiling      *int
	BlockUnrated bool // hides video works with no rating
}

// NewUser describes a user to create.
type NewUser struct {
	Username  string
	Admin     bool
	Grants    Grants
	Limits    Limits
	Parental  Parental
	InvitedBy id.ID
}

// Change is an edit of a user. A nil field is left as it is.
type Change struct {
	Username *string
	Admin    *bool
	Disabled *bool
	Grants   *Grants
	Limits   *Limits
	Parental *Parental
}

// CreateUser creates a user and its grants in the caller's transaction.
func (s *Service) CreateUser(ctx context.Context, tx *store.Tx, u NewUser) (User, error) {
	if err := checkName(u.Username); err != nil {
		return User{}, err
	}
	if !u.Grants.All && len(u.Grants.Libraries) == 0 {
		return User{}, ErrNoLibraries
	}
	out := User{
		ID:        id.New(),
		Username:  u.Username,
		Admin:     u.Admin,
		Grants:    sorted(u.Grants),
		Limits:    u.Limits,
		Parental:  u.Parental,
		Playback:  DefaultPlayback,
		Downloads: true,
		InvitedBy: u.InvitedBy,
		Created:   time.Now().Truncate(time.Millisecond),
	}
	q := dbq.New(tx)
	err := q.InsertUser(ctx, dbq.InsertUserParams{
		ID:               out.ID,
		Username:         u.Username,
		Admin:            u.Admin,
		AllLibraries:     u.Grants.All,
		StreamLimit:      nullLimit(int64(u.Limits.StreamLimit)),
		RemoteQualityCap: nullLimit(u.Limits.RemoteQualityCap),
		RatingCeiling:    ceiling(u.Parental.Ceiling),
		BlockUnrated:     u.Parental.BlockUnrated,
		InvitedBy:        u.InvitedBy,
		CreatedAt:        out.Created.UnixMilli(),
	})
	if store.IsUnique(err) {
		return User{}, fmt.Errorf("%w: %q", ErrNameTaken, u.Username)
	}
	if err != nil {
		return User{}, err
	}
	if err := insertGrants(ctx, q, out.ID, out.Grants); err != nil {
		return User{}, err
	}
	if err := insertMyList(ctx, q, out.ID, out.Created); err != nil {
		return User{}, err
	}
	// TODO: no webhook is sent for the user-created event.
	return out, nil
}

// Update applies a change to a user in one write, made by the user by. It
// refuses with ErrOwnUser a change by a user to their own that disables
// them or removes their admin, and with ErrLastAdmin a change that would
// leave no enabled admin. A new username is checked the same way as at
// creation.
func (s *Service) Update(ctx context.Context, by, user id.ID, c Change) error {
	if by == user && (c.Admin != nil && !*c.Admin || c.Disabled != nil && *c.Disabled) {
		return ErrOwnUser
	}
	if c.Grants != nil && !c.Grants.All && len(c.Grants.Libraries) == 0 {
		return ErrNoLibraries
	}
	if c.Username != nil {
		if err := checkName(*c.Username); err != nil {
			return err
		}
	}
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if _, err := q.GetUser(ctx, user); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrNotFound, user)
		} else if err != nil {
			return err
		}
		if c.Username != nil {
			_, err := q.SetUsername(ctx, dbq.SetUsernameParams{Username: *c.Username, ID: user})
			if store.IsUnique(err) {
				return fmt.Errorf("%w: %q", ErrNameTaken, *c.Username)
			}
			if err != nil {
				return err
			}
		}
		if c.Admin != nil {
			if _, err := q.SetAdmin(ctx, dbq.SetAdminParams{Admin: *c.Admin, ID: user}); err != nil {
				return err
			}
		}
		if c.Disabled != nil {
			if _, err := q.SetDisabled(ctx, dbq.SetDisabledParams{Disabled: *c.Disabled, ID: user}); err != nil {
				return err
			}
		}
		if a := c.Grants; a != nil {
			if _, err := q.SetAllLibraries(ctx, dbq.SetAllLibrariesParams{AllLibraries: a.All, ID: user}); err != nil {
				return err
			}
			if err := q.DeleteGrants(ctx, user); err != nil {
				return err
			}
			if err := insertGrants(ctx, q, user, *a); err != nil {
				return err
			}
			if !a.All {
				if err := q.DeleteUngrantedLibraries(ctx, user); err != nil {
					return err
				}
			}
		}
		if l := c.Limits; l != nil {
			_, err := q.SetLimits(ctx, dbq.SetLimitsParams{
				StreamLimit: nullLimit(int64(l.StreamLimit)), RemoteQualityCap: nullLimit(l.RemoteQualityCap), ID: user,
			})
			if err != nil {
				return err
			}
		}
		if p := c.Parental; p != nil {
			_, err := q.SetParental(ctx, dbq.SetParentalParams{
				RatingCeiling: ceiling(p.Ceiling), BlockUnrated: p.BlockUnrated, ID: user,
			})
			if err != nil {
				return err
			}
		}
		if c.Admin == nil && c.Disabled == nil {
			return nil
		}
		return checkAdmin(ctx, q)
	})
}

// Delete deletes a user with its grants, sessions, watch state and
// history, as the user by asks. It refuses with ErrOwnUser to delete by
// itself, and with ErrLastAdmin to delete the last enabled admin.
func (s *Service) Delete(ctx context.Context, by, user id.ID) error {
	if by == user {
		return ErrOwnUser
	}
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if n, err := q.DeleteUser(ctx, user); err != nil {
			return err
		} else if n == 0 {
			return fmt.Errorf("%w: %s", ErrNotFound, user)
		}
		return checkAdmin(ctx, q)
	})
	if err != nil {
		return err
	}
	return nil
}

// User returns a user.
func (s *Service) User(ctx context.Context, user id.ID) (User, error) {
	var out User
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		q := dbq.New(tx)
		u, err := q.GetUser(ctx, user)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrNotFound, user)
		}
		if err != nil {
			return err
		}
		libs, err := q.Grants(ctx, user)
		if err != nil {
			return err
		}
		admins, err := q.EnabledAdmins(ctx)
		if err != nil {
			return err
		}
		out = fromRow(u, libs, admins)
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return out, nil
}

// Users returns every user, by username.
func (s *Service) Users(ctx context.Context) ([]User, error) {
	var out []User
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		q := dbq.New(tx)
		rows, err := q.ListUsers(ctx)
		if err != nil {
			return err
		}
		grants, err := q.AllGrants(ctx)
		if err != nil {
			return err
		}
		admins, err := q.EnabledAdmins(ctx)
		if err != nil {
			return err
		}
		libs := map[id.ID][]id.ID{}
		for _, g := range grants {
			libs[g.UserID] = append(libs[g.UserID], g.LibraryID)
		}
		out = make([]User, len(rows))
		for i, u := range rows {
			out[i] = fromRow(u, libs[u.ID], admins)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// checkAdmin fails with ErrLastAdmin when no enabled admin is left, so the
// write it is part of rolls back.
func checkAdmin(ctx context.Context, q *dbq.Queries) error {
	n, err := q.EnabledAdmins(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrLastAdmin
	}
	return nil
}

func insertGrants(ctx context.Context, q *dbq.Queries, user id.ID, a Grants) error {
	if a.All {
		return nil
	}
	for _, lib := range a.Libraries {
		err := q.InsertGrant(ctx, dbq.InsertGrantParams{UserID: user, LibraryID: lib})
		if store.IsForeignKey(err) {
			return fmt.Errorf("%w: %v", ErrUnknownLibrary, lib)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// fromRow builds a User from its row, the libraries it is granted, and the
// number of enabled admins.
func fromRow(u dbq.User, libs []id.ID, admins int64) User {
	out := User{
		ID: u.ID, Username: u.Username, Admin: u.Admin,
		Grants: sorted(Grants{All: u.AllLibraries, Libraries: libs}),
		Limits: Limits{
			StreamLimit: int(u.StreamLimit.Int64), RemoteQualityCap: u.RemoteQualityCap.Int64,
		},
		Parental: Parental{BlockUnrated: u.BlockUnrated},
		Playback: Playback{
			AudioLanguage: firstLanguage(u.AudioLanguages), SubtitleLanguage: firstLanguage(u.SubtitleLanguages),
			SubtitleMode: u.SubtitleMode, RememberTracks: u.RememberTracks, AutoplayNext: u.AutoplayNext, BookSpeed: u.BookSpeed,
		},
		Downloads: u.DownloadsAllowed, Disabled: u.Disabled, InvitedBy: u.InvitedBy, Created: time.UnixMilli(u.CreatedAt),
		OnlyAdmin: u.Admin && !u.Disabled && admins == 1,
	}
	if u.RatingCeiling.Valid {
		c := int(u.RatingCeiling.Int64)
		out.Parental.Ceiling = &c
	}
	return out
}

// sorted returns a with its libraries sorted, or with no libraries when it
// grants every library.
func sorted(a Grants) Grants {
	if a.All {
		return Grants{All: true}
	}
	libs := slices.Clone(a.Libraries)
	slices.SortFunc(libs, compareID)
	return Grants{Libraries: slices.Compact(libs)}
}

// maxNameLen is the most runes a username may have, both when it is
// created and when it is given at sign-in.
const maxNameLen = 64

func checkName(s string) error {
	n := utf8.RuneCountInString(s)
	if n == 0 || n > maxNameLen || !utf8.ValidString(s) || strings.TrimSpace(s) != s ||
		strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return fmt.Errorf("%w: %q", ErrInvalidName, s)
	}
	return nil
}

// nullLimit returns a limit's column value. A limit of 0 or less means no
// limit and is stored as NULL.
func nullLimit(n int64) sql.NullInt64 { return sql.NullInt64{Int64: n, Valid: n > 0} }

func ceiling(c *int) sql.NullInt64 {
	if c == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*c), Valid: true}
}
