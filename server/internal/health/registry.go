package health

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"
)

// Registry collects the problems components report, and passes newly
// raised problems to RegistryOptions.Raised. It rechecks the problems
// whenever they're read, and every 30 seconds while Run runs.
type Registry struct {
	reporters []Reporter
	raised    func(ctx context.Context, raised []OpenProblem) // RegistryOptions.Raised

	mu   sync.Mutex             // held through a check, so two checks never run at once
	open map[string]OpenProblem // the open problems by ID, as the last check saw them
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

// NewRegistry returns a registry of the problems reporters report.
func NewRegistry(reporters []Reporter, opt RegistryOptions) *Registry {
	if opt.Raised == nil {
		opt.Raised = func(context.Context, []OpenProblem) {}
	}
	return &Registry{reporters: reporters, raised: opt.Raised, open: map[string]OpenProblem{}}
}

// OpenProblem is a reported problem with its ID, which is made from its
// component and code.
type OpenProblem struct {
	ID string
	Problem
}

// Open rechecks the problems and returns them newest first.
func (r *Registry) Open(ctx context.Context) []OpenProblem {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.check(ctx)
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

// check reads the open problems and passes on those raised since the last
// check. The caller holds r.mu.
func (r *Registry) check(ctx context.Context) []OpenProblem {
	open := r.read(ctx)
	r.tell(ctx, open)
	return open
}

// read returns every component's problems newest first. Each component
// raises and resolves its own problems as it works.
func (r *Registry) read(ctx context.Context) []OpenProblem {
	var open []OpenProblem
	for _, rep := range r.reporters {
		for _, p := range rep.Health(ctx) {
			open = append(open, OpenProblem{ID: p.Component + "." + p.Code, Problem: p})
		}
	}
	slices.SortStableFunc(open, func(a, b OpenProblem) int {
		return cmp.Or(b.RaisedAt.Compare(a.RaisedAt), cmp.Compare(a.ID, b.ID))
	})
	return open
}

// tell passes the problems the last check didn't find, or found raised at a
// different time, to RegistryOptions.Raised. The caller holds r.mu.
func (r *Registry) tell(ctx context.Context, open []OpenProblem) {
	var raised []OpenProblem
	now := make(map[string]OpenProblem, len(open))
	for _, p := range open {
		if last, ok := r.open[p.ID]; !ok || !last.RaisedAt.Equal(p.RaisedAt) {
			raised = append(raised, p)
		}
		now[p.ID] = p
	}
	r.open = now
	if len(raised) > 0 {
		r.raised(context.WithoutCancel(ctx), raised)
	}
}
