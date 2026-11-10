package api

import (
	"context"
	"slices"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/scan"
	"github.com/mmcdole/kivo/internal/tasks"
)

func (h *handlers) ListTasks(ctx context.Context, _ gen.ListTasksRequestObject) (gen.ListTasksResponseObject, error) {
	all, err := h.deps.Tasks.Tasks(ctx)
	if err != nil {
		return nil, err
	}
	shown := withMatching(all)
	out := gen.TaskList{Items: make([]gen.Task, len(shown))}
	for i, t := range shown {
		out.Items[i] = task(t)
	}
	return gen.ListTasks200JSONResponse(out), nil
}

// withMatching shows identification as the second step of a library scan.
// Identification is its own task so that one title can be identified
// without scanning its library, but admins see matching as part of the
// scan. So the identify task isn't listed, and while it works and no scan
// does, the scan's row runs on the step identify with the files left to
// identify as its pending count. Its last and next runs stay the scan's.
func withMatching(all []tasks.Status) []tasks.Status {
	i := slices.IndexFunc(all, func(s tasks.Status) bool { return s.Name == identify.TaskIdentifyMedia })
	if i < 0 {
		return all
	}
	matching := all[i]
	shown := slices.Delete(slices.Clone(all), i, i+1)
	if !matching.Running {
		return shown
	}
	for j, s := range shown {
		if s.Name == scan.TaskScanLibraries && !s.Running {
			s.Running, s.Step, s.Done, s.Total = true, string(gen.TaskStepIdentify), 0, 0
			s.PendingCount = matching.PendingCount
			shown[j] = s
		}
	}
	return shown
}

// listed returns ErrNotFound for the identify task, which the tasks list
// leaves out. Stop on the scan's row stops it.
func listed(name string) error {
	if name == identify.TaskIdentifyMedia {
		return tasks.ErrNotFound
	}
	return nil
}

// task puts a task on the wire.
func task(t tasks.Status) gen.Task {
	out := gen.Task{
		ID: gen.TaskName(t.Name), Section: gen.TaskSection(t.Section()), Nightly: t.Schedule == tasks.Nightly, PendingCount: t.PendingCount,
		Running: t.Running, Stoppable: t.Running && !t.Unstoppable,
	}
	if items, ok := t.Work.(tasks.Items); ok && items.Failed != nil {
		out.FailedCount = ptr(t.Failures)
	}
	if t.Last != nil {
		out.LastRun = &gen.TaskRun{FinishedAt: t.Last.At.UTC(), Summary: opt(t.Last.Summary), Error: opt(t.Last.Error)}
	}
	if !t.Next.IsZero() {
		out.NextRunAt = ptr(t.Next.UTC())
	}
	if t.Running {
		out.Progress = &gen.TaskProgress{Step: optAs[gen.TaskStep](t.Step), Done: t.Done, Total: t.Total}
	}
	return out
}

func (h *handlers) RunTask(ctx context.Context, req gen.RunTaskRequestObject) (gen.RunTaskResponseObject, error) {
	if err := listed(req.Task); err != nil {
		return nil, err
	}
	if err := h.deps.Tasks.Start(ctx, req.Task); err != nil {
		return nil, err
	}
	return gen.RunTask204Response{}, nil
}

func (h *handlers) StopTask(ctx context.Context, req gen.StopTaskRequestObject) (gen.StopTaskResponseObject, error) {
	if err := listed(req.Task); err != nil {
		return nil, err
	}
	if err := h.deps.Tasks.Stop(ctx, req.Task); err != nil {
		return nil, err
	}
	// The scan's row also shows its matching. Stopped files stay waiting
	// for the next scan or the identify task's hourly look.
	if req.Task == scan.TaskScanLibraries {
		if err := h.deps.Tasks.Stop(ctx, identify.TaskIdentifyMedia); err != nil {
			return nil, err
		}
	}
	return gen.StopTask204Response{}, nil
}

func (h *handlers) ListTaskFailures(ctx context.Context, req gen.ListTaskFailuresRequestObject) (gen.ListTaskFailuresResponseObject, error) {
	if err := listed(req.Task); err != nil {
		return nil, err
	}
	failed, err := h.deps.Tasks.Failed(ctx, req.Task)
	if err != nil {
		return nil, err
	}
	ids := make([]id.ID, len(failed))
	for i, f := range failed {
		ids[i] = f.Item
	}
	names, err := h.deps.Catalog.Subjects(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := gen.TaskFailureList{Items: make([]gen.TaskFailure, len(failed))}
	for i, f := range failed {
		out.Items[i] = gen.TaskFailure{ItemID: f.Item, Title: opt(names[f.Item]), Reason: f.Reason, FailedAt: f.At.UTC()}
	}
	return gen.ListTaskFailures200JSONResponse(out), nil
}

func (h *handlers) RetryTask(ctx context.Context, req gen.RetryTaskRequestObject) (gen.RetryTaskResponseObject, error) {
	if err := listed(req.Task); err != nil {
		return nil, err
	}
	var items []id.ID
	if req.Body != nil {
		items = deref(req.Body.ItemIds)
	}
	if err := h.deps.Tasks.Retry(ctx, req.Task, items...); err != nil {
		return nil, err
	}
	return gen.RetryTask204Response{}, nil
}
