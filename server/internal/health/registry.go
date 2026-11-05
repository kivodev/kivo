package health

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/events"
)

// Registry collects the problems components report. It keeps the problems
// an admin dismissed hidden. It publishes problemsChanged when the open
// problems change, and passes newly raised problems to
// RegistryOptions.Raised. It rechecks the problems whenever they're read,
// and every 30 seconds while Run runs.
type Registry struct {
	reporters []Reporter
	bus       *events.Bus
	raised    func(ctx context.Context, raised []OpenProblem) // RegistryOptions.Raised

	mu     sync.Mutex             // held through a check, so two checks never run at once
	hidden map[string]time.Time   // dismissed problems by ID, with when each was raised
	open   map[string]OpenProblem // the open problems by ID, as the last check saw them
}

// RegistryOptions are the registry's optional hooks.
type RegistryOptions struct {
	// Raised receives the open problems that a check finds and the last
	// check didn't, or found raised at a different time. It runs during the
	// check, so it must not read the registry. ctx is the reader's context,
	// without its cancellation.
	Raised func(ctx context.Context, raised []OpenProblem)
}

// recheckEvery is how often Run rechecks the problems. A problem raised
// while nobody reads them is still reported within this time.
const recheckEvery = 30 * time.Second

// NewRegistry returns a registry of the problems reporters report. It
// publishes on bus.
func NewRegistry(reporters []Reporter, bus *events.Bus, opt RegistryOptions) *Registry {
	if opt.Raised == nil {
		opt.Raised = func(context.Context, []OpenProblem) {}
	}
	return &Registry{
		reporters: reporters, bus: bus, raised: opt.Raised,
		hidden: map[string]time.Time{}, open: map[string]OpenProblem{},
	}
}

// OpenProblem is a reported problem with its ID, which is made from its
// component and code.
type OpenProblem struct {
	ID string
	Problem
}

// Open rechecks the problems and returns them newest first, leaving out the
// dismissed ones. A dismissal is forgotten once its problem resolves, so a
// problem raised again later shows again.
func (r *Registry) Open(ctx context.Context) []OpenProblem {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.check(ctx)
}

// Dismiss hides the open problem with id until it's raised again, and
// rechecks. It reports whether a problem with id was open.
func (r *Registry) Dismiss(ctx context.Context, id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.check(ctx) {
		if p.ID == id {
			r.hidden[p.ID] = p.RaisedAt
			r.check(ctx)
			return true
		}
	}
	return false
}

// Run rechecks the problems every 30 seconds until ctx ends.
func (r *Registry) Run(ctx context.Context) error {
	t := time.NewTicker(recheckEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			r.Open(ctx)
		}
	}
}

// check reads the open problems and reports what changed since the last
// check. The caller holds r.mu.
func (r *Registry) check(ctx context.Context) []OpenProblem {
	open := r.read(ctx)
	r.tell(ctx, open)
	return open
}

// read returns every component's problems newest first, leaving out the
// dismissed ones. It forgets the dismissals of problems that resolved.
// Each component raises and resolves its own problems as it works. The
// caller holds r.mu.
func (r *Registry) read(ctx context.Context) []OpenProblem {
	var all []OpenProblem
	for _, rep := range r.reporters {
		for _, p := range rep.Health(ctx) {
			all = append(all, OpenProblem{ID: p.Component + "." + p.Code, Problem: p})
		}
	}
	slices.SortStableFunc(all, func(a, b OpenProblem) int {
		return cmp.Or(b.RaisedAt.Compare(a.RaisedAt), cmp.Compare(a.ID, b.ID))
	})
	live := map[string]time.Time{}
	open := all[:0]
	for _, p := range all {
		live[p.ID] = p.RaisedAt
		if since, ok := r.hidden[p.ID]; ok && since.Equal(p.RaisedAt) {
			continue
		}
		open = append(open, p)
	}
	for id, since := range r.hidden {
		if s, ok := live[id]; !ok || !s.Equal(since) {
			delete(r.hidden, id)
		}
	}
	return open
}

// tell publishes problemsChanged, with the number open, when the open
// problems differ from the last check. It passes the problems raised since
// then to RegistryOptions.Raised. The caller holds r.mu.
func (r *Registry) tell(ctx context.Context, open []OpenProblem) {
	var raised []OpenProblem
	var changed bool
	now := make(map[string]OpenProblem, len(open))
	for _, p := range open {
		last, ok := r.open[p.ID]
		fresh := !ok || !last.RaisedAt.Equal(p.RaisedAt)
		if fresh {
			raised = append(raised, p)
		}
		changed = changed || fresh || last.Message != p.Message
		now[p.ID] = p
	}
	changed = changed || len(now) != len(r.open)
	r.open = now
	if changed {
		r.bus.Publish(events.Event{Kind: events.ProblemsChanged, Scope: events.Scope{AdminOnly: true}, Data: len(open)})
	}
	if len(raised) > 0 {
		r.raised(context.WithoutCancel(ctx), raised)
	}
}
