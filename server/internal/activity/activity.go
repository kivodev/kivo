package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"time"

	"github.com/mmcdole/kivo/internal/activity/dbq"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
)

// Kind is what an event records.
type Kind string

// The kinds of event.
const (
	PlayStarted     Kind = "playStarted"
	PlayFinished    Kind = "playFinished" // a play ended past the played threshold
	Added           Kind = "added"        // a folder's new files, under one root work
	SignedIn        Kind = "signedIn"
	SignInFailed    Kind = "signInFailed" // a run of failed sign-ins for one user from one address
	UserJoined      Kind = "userJoined"
	PasswordChanged Kind = "passwordChanged"
	TaskEnded       Kind = "taskEnded" // a night run or a Run now run ended
	ProblemRaised   Kind = "problemRaised"
)

// Group is a set of kinds, as offered by the Activity page's Show filter.
type Group string

// The groups. The empty Group means every kind.
const (
	GroupPlays   Group = "plays"
	GroupAdded   Group = "added"
	GroupSignIns Group = "signIns"
	GroupServer  Group = "server"
)

// groups are the kinds of each group.
var groups = map[Group][]Kind{
	GroupPlays:   {PlayStarted, PlayFinished},
	GroupAdded:   {Added},
	GroupSignIns: {SignedIn, SignInFailed, UserJoined, PasswordChanged},
	GroupServer:  {TaskEnded, ProblemRaised},
}

// Event is one row of activity. Only the fields its kind uses are set, with
// values as they were when it happened. The rest are zero.
type Event struct {
	ID   id.ID     `json:"-"`
	At   time.Time `json:"-"`
	Kind Kind      `json:"-"`

	User     id.ID  `json:"user,omitzero"`
	Username string `json:"username,omitempty"`
	// Work is the work that was played, or the root work that files were added
	// under. Title is the work's name. For an episode it is the series'
	// name.
	Work     id.ID  `json:"work,omitzero"`
	WorkKind string `json:"workKind,omitempty"`
	Title    string `json:"title,omitempty"`
	Season   int    `json:"season,omitempty"`
	Episode  int    `json:"episode,omitempty"`
	Library  string `json:"library,omitempty"` // the name of the library files were added to
	Count    int    `json:"count,omitempty"`   // the number of files added or failed sign-ins
	Client   string `json:"client,omitempty"`  // the name of the session or API key
	Address  string `json:"address,omitempty"`
	Local    bool   `json:"local,omitempty"`
	By       string `json:"by,omitempty"` // the admin who added a user or changed their password, or empty
	Task     string `json:"task,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Error    string `json:"error,omitempty"`
	Failed   int    `json:"failed,omitempty"`  // the number of the task's failed items
	Problem  string `json:"problem,omitempty"` // the problem's ID
	Code     string `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
	Severity string `json:"severity,omitempty"`

	CommandLine bool `json:"commandLine,omitempty"` // a password was reset from the command line
}

const (
	keep = 90 * 24 * time.Hour // how long events are kept
	// failRun is how long after a failed sign-in another failure for the
	// same user from the same address still joins its row. It matches how
	// long the backoff remembers a failure.
	failRun = time.Hour
	// failedSignIns failed sign-ins within failWindow raise a problem. Five
	// is where the backoff starts, past a few typos.
	failedSignIns = 5
	failWindow    = 24 * time.Hour
)

// TaskPrune is the nightly cleanup of events older than 90 days.
const TaskPrune = "prune-activity"

// Service records and lists activity.
type Service struct {
	db  *store.DB
	q   *dbq.Queries
	bus *events.Bus
}

// New returns the service, which publishes activityChanged on bus once an
// event's transaction commits.
func New(db *store.DB, bus *events.Bus) *Service {
	return &Service{db: db, q: dbq.New(db.Reader()), bus: bus}
}

// Tasks returns the service's only task, the nightly cleanup.
func (s *Service) Tasks() []tasks.Task {
	return []tasks.Task{{Name: TaskPrune, Schedule: tasks.Nightly, Maintenance: true, Work: tasks.Func(s.prune)}}
}

func (s *Service) prune(ctx context.Context) error {
	var n int64
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		var err error
		n, err = dbq.New(tx).Prune(ctx, time.Now().Add(-keep).UnixMilli())
		return err
	})
	if err != nil {
		return err
	}
	tasks.Summarize(ctx, fmt.Sprintf("Removed %d events", n))
	return nil
}

// PlayActivity is what a play's events record about it.
type PlayActivity struct {
	User, Work id.ID
	Client     string
	Local      bool
}

// Played records, in the caller's transaction, that a play started or
// finished past the played threshold.
func (s *Service) Played(ctx context.Context, tx *store.Tx, kind Kind, p PlayActivity) error {
	if err := s.record(ctx, tx, Event{Kind: kind, User: p.User, Work: p.Work, Client: p.Client, Local: p.Local}); err != nil {
		return fmt.Errorf("record %s activity: %w", kind, err)
	}
	return nil
}

// FilesAdded records, in the caller's transaction, that count new files
// from one folder were added under one root work in library.
func (s *Service) FilesAdded(ctx context.Context, tx *store.Tx, library, work id.ID, count int) error {
	lib, err := dbq.New(tx).LibraryName(ctx, library)
	if err == nil {
		err = s.record(ctx, tx, Event{Kind: Added, Work: work, Library: lib, Count: count})
	}
	if err != nil {
		return fmt.Errorf("record %s activity: %w", Added, err)
	}
	return nil
}

// SignIn is what a sign-in's events record about it.
type SignIn struct {
	User    id.ID // zero for a username that no user has
	Client  string
	Address netip.Addr
	Local   bool
}

// SignedIn records a new session, in the caller's transaction.
func (s *Service) SignedIn(ctx context.Context, tx *store.Tx, in SignIn) error {
	e := Event{Kind: SignedIn, User: in.User, Client: in.Client, Address: address(in.Address), Local: in.Local}
	if err := s.record(ctx, tx, e); err != nil {
		return fmt.Errorf("record %s activity: %w", SignedIn, err)
	}
	return nil
}

// Failed records a failed sign-in in the caller's transaction. If the last
// failure for the same user from the same address was within the hour,
// this one joins that row.
func (s *Service) Failed(ctx context.Context, tx *store.Tx, in SignIn) error {
	if err := s.failed(ctx, tx, in); err != nil {
		return fmt.Errorf("record %s activity: %w", SignInFailed, err)
	}
	return nil
}

func (s *Service) failed(ctx context.Context, tx *store.Tx, in SignIn) error {
	now := time.Now()
	q := dbq.New(tx)
	rows, err := q.Failures(ctx, now.Add(-failRun).UnixMilli())
	if err != nil {
		return err
	}
	addr := address(in.Address)
	for _, r := range rows {
		var e Event
		if json.Unmarshal([]byte(r.Data), &e) != nil || e.User != in.User || e.Address != addr {
			continue
		}
		e.Count++
		e.Local = in.Local
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if err := q.UpdateFailures(ctx, dbq.UpdateFailuresParams{ID: r.ID, At: now.UnixMilli(), Data: string(data)}); err != nil {
			return err
		}
		tx.AfterCommit(s.changed)
		return nil
	}
	return s.record(ctx, tx, Event{Kind: SignInFailed, At: now, User: in.User, Address: addr, Local: in.Local, Count: 1})
}

// Joined records a new user in the caller's transaction. by is the admin
// who added the user, or zero when the user came from an invite.
func (s *Service) Joined(ctx context.Context, tx *store.Tx, user, by id.ID) error {
	e := Event{Kind: UserJoined, User: user}
	var err error
	if !by.IsZero() {
		e.By, err = dbq.New(tx).Username(ctx, by)
	}
	if err == nil {
		err = s.record(ctx, tx, e)
	}
	if err != nil {
		return fmt.Errorf("record %s activity: %w", UserJoined, err)
	}
	return nil
}

// PasswordSet records a user's new password in the caller's transaction.
// by is the admin who set it, or zero when the user changed their own or
// commandLine says it was reset from the command line. address is where
// the user changed it from, or the zero Addr.
func (s *Service) PasswordSet(ctx context.Context, tx *store.Tx, user, by id.ID, addr netip.Addr, commandLine bool) error {
	e := Event{Kind: PasswordChanged, User: user, Address: address(addr), CommandLine: commandLine}
	var err error
	if !by.IsZero() && by != user {
		e.By, err = dbq.New(tx).Username(ctx, by)
	}
	if err == nil {
		err = s.record(ctx, tx, e)
	}
	if err != nil {
		return fmt.Errorf("record %s activity: %w", PasswordChanged, err)
	}
	return nil
}

// TaskRunEnded records the end of a task's run, in the transaction that
// records the run's end. It serves as the tasks runner's Options.Ended.
func (s *Service) TaskRunEnded(ctx context.Context, tx *store.Tx, r tasks.RunEnd) error {
	e := Event{Kind: TaskEnded, At: r.At, Task: r.Task, Summary: r.Summary, Error: r.Error, Failed: r.Failed}
	if err := s.record(ctx, tx, e); err != nil {
		return fmt.Errorf("record %s activity: %w", TaskEnded, err)
	}
	return nil
}

// ProblemsRaised records raised problems. It serves as the problems
// registry's Options.Raised. Problems raised by this package are left out,
// because they describe events already listed.
func (s *Service) ProblemsRaised(ctx context.Context, raised []health.OpenProblem) error {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		for _, p := range raised {
			if p.Component == component {
				continue
			}
			e := Event{Kind: ProblemRaised, At: p.RaisedAt, Problem: p.ID, Code: p.Code, Message: p.Message, Severity: p.Severity.String()}
			if err := s.record(ctx, tx, e); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("record %s activity: %w", ProblemRaised, err)
	}
	return nil
}

// record writes an event in the caller's transaction. It stores the
// current names of the event's user and work.
func (s *Service) record(ctx context.Context, tx *store.Tx, e Event) error {
	q := dbq.New(tx)
	if !e.User.IsZero() {
		u, err := q.Username(ctx, e.User)
		if err != nil {
			return err
		}
		e.Username = u
	}
	if !e.Work.IsZero() {
		w, err := q.WorkName(ctx, e.Work)
		if err != nil {
			return err
		}
		e.WorkKind, e.Title = w.Kind, w.RootTitle
		if w.Kind == "episode" {
			e.Season, e.Episode = int(w.ParentNumber.Int64), int(w.Number.Int64)
		}
	}
	if e.At.IsZero() {
		e.At = time.Now()
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := q.Insert(ctx, dbq.InsertParams{ID: id.New(), At: e.At.UnixMilli(), Kind: string(e.Kind), Data: string(data)}); err != nil {
		return err
	}
	tx.AfterCommit(s.changed)
	return nil
}

// changed tells admins an event was recorded.
func (s *Service) changed() {
	s.bus.Publish(events.Event{Kind: events.ActivityChanged, Scope: events.Scope{AdminOnly: true}})
}

// address is how an event names a client's address.
func address(ip netip.Addr) string {
	if !ip.IsValid() {
		return ""
	}
	return ip.Unmap().String()
}

// component is the package's name on its problems.
const component = "activity"

// Health raises a problem while there have been five or more failed
// sign-ins in the last day. The problem's time is that of the latest
// failure.
func (s *Service) Health(ctx context.Context) []health.Problem {
	rows, err := s.q.Failures(ctx, time.Now().Add(-failWindow).UnixMilli())
	if err != nil || len(rows) == 0 {
		return nil
	}
	n := 0
	for _, r := range rows {
		var e Event
		if json.Unmarshal([]byte(r.Data), &e) == nil {
			n += e.Count
		}
	}
	if n < failedSignIns {
		return nil
	}
	return []health.Problem{{
		Component: component, Code: "signInsFailed", Severity: health.Warning,
		Message: fmt.Sprintf("%d failed sign-ins in the last day", n), RaisedAt: time.UnixMilli(rows[0].At),
	}}
}

// Position is where a page of events begins. The page holds the events
// before it.
type Position struct {
	At time.Time
	ID id.ID
}

// ErrUnknownGroup means a list asked for a group that doesn't exist.
var ErrUnknownGroup = fault.New(fault.Invalid, "unknown group")

// List returns up to limit events before p, newest first, and whether more
// follow. It lists the group's kinds, or every kind for the empty group.
// The zero Position starts at the newest event.
func (s *Service) List(ctx context.Context, g Group, p Position, limit int) ([]Event, bool, error) {
	if p.At.IsZero() {
		p = Position{At: time.UnixMilli(1<<62 - 1), ID: id.ID{}}
		for i := range p.ID {
			p.ID[i] = 0xff
		}
	}
	var rows []dbq.Activity
	var err error
	if g == "" {
		rows, err = s.q.Latest(ctx, dbq.LatestParams{BeforeAt: p.At.UnixMilli(), BeforeID: p.ID[:], Limit: int64(limit + 1)})
	} else {
		kinds, ok := groups[g]
		if !ok {
			return nil, false, fmt.Errorf("%w: %q", ErrUnknownGroup, g)
		}
		var set []byte
		if set, err = json.Marshal(kinds); err != nil {
			return nil, false, err
		}
		rows, err = s.q.LatestOf(ctx, dbq.LatestOfParams{
			Kinds: string(set), BeforeAt: p.At.UnixMilli(), BeforeID: p.ID[:], Limit: int64(limit + 1),
		})
	}
	if err != nil {
		return nil, false, err
	}
	more := len(rows) > limit
	rows = rows[:min(len(rows), limit)]
	out := make([]Event, 0, len(rows))
	for _, r := range rows {
		var e Event
		if err := json.Unmarshal([]byte(r.Data), &e); err != nil {
			return nil, false, err
		}
		e.ID, e.At, e.Kind = r.ID, time.UnixMilli(r.At).UTC(), Kind(r.Kind)
		out = append(out, e)
	}
	return out, more, nil
}
