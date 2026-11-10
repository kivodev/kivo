package api

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/id"
)

func (h *handlers) ListActivity(ctx context.Context, req gen.ListActivityRequestObject) (gen.ListActivityResponseObject, error) {
	p := req.Params
	var g activity.Group
	if p.Group != nil {
		if !p.Group.Valid() {
			return nil, invalid("unknown group", field("group", "invalid"))
		}
		g = activity.Group(*p.Group)
	}
	pg, err := ownPagingOf(p.After, p.Limit, defaultLimit, maxLimit)
	if err != nil {
		return nil, err
	}
	pos, err := cursor(pg, parsePosition, "the cursor is invalid")
	if err != nil {
		return nil, err
	}
	events, more, err := h.deps.Activity.List(ctx, g, pos, pg.limit)
	if err != nil {
		return nil, err
	}
	out := gen.ActivityFeed{Items: make([]gen.ActivityEvent, len(events))}
	for i, e := range events {
		out.Items[i] = activityEvent(e)
	}
	if more {
		last := events[len(events)-1]
		out.Next = ptr(strconv.FormatInt(last.At.UnixMilli(), 10) + "." + last.ID.String())
	}
	return gen.ListActivity200JSONResponse(out), nil
}

// parsePosition reads an activity cursor, which is an event's time in
// milliseconds and its ID.
func parsePosition(s string) (activity.Position, error) {
	at, rest, _ := strings.Cut(s, ".")
	ms, err := strconv.ParseInt(at, 10, 64)
	if err != nil {
		return activity.Position{}, err
	}
	u, err := id.Parse(rest)
	if err != nil {
		return activity.Position{}, err
	}
	return activity.Position{At: time.UnixMilli(ms), ID: u}, nil
}

// activityEvent is an event on the wire. Its fields go under the group for
// its kind.
func activityEvent(e activity.Event) gen.ActivityEvent {
	out := gen.ActivityEvent{ID: e.ID, OccurredAt: e.At, Kind: gen.ActivityKind(e.Kind)}
	work := gen.ActivityWork{WorkID: e.Work, Kind: gen.WorkKind(e.WorkKind), Title: e.Title, SeasonNumber: opt(e.Season), EpisodeNumber: opt(e.Episode)}
	switch e.Kind {
	case activity.PlayStarted, activity.PlayFinished:
		out.Play = &gen.ActivityPlay{UserID: e.User, Username: e.Username, Work: work, ClientName: opt(e.Client), Local: e.Local}
	case activity.Added:
		out.Added = &gen.ActivityAdded{LibraryName: e.Library, Work: work, Count: e.Count}
	case activity.SignedIn, activity.SignInFailed:
		out.SignIn = &gen.ActivitySignIn{
			UserID: opt(e.User), Username: opt(e.Username), ClientName: opt(e.Client), Address: e.Address, Local: e.Local,
		}
		if e.Kind == activity.SignInFailed {
			out.SignIn.Count = &e.Count
		}
	case activity.UserJoined:
		out.Joined = &gen.ActivityJoined{UserID: e.User, Username: e.Username, AddedBy: opt(e.By)}
	case activity.PasswordChanged:
		out.Password = &gen.ActivityPassword{UserID: e.User, Username: e.Username, ChangedBy: opt(e.By), Address: opt(e.Address)}
	case activity.TaskEnded:
		out.Task = &gen.ActivityTask{Task: gen.TaskName(e.Task), Summary: opt(e.Summary), Error: opt(e.Error), FailedCount: e.Failed}
	case activity.ProblemRaised:
		out.Problem = &gen.ActivityProblem{ID: e.Problem, Code: e.Code, Message: e.Message, Severity: gen.ProblemSeverity(e.Severity)}
	}
	return out
}
