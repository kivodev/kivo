package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/tasks"
)

func (h *handlers) ListTasks(ctx context.Context, _ gen.ListTasksRequestObject) (gen.ListTasksResponseObject, error) {
	all, err := h.deps.Tasks.Tasks(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.TaskList{Items: make([]gen.Task, len(all))}
	for i, t := range all {
		out.Items[i] = task(t)
	}
	return gen.ListTasks200JSONResponse(out), nil
}

// task puts a task on the wire.
func task(t tasks.Status) gen.Task {
	out := gen.Task{
		ID: gen.TaskName(t.Name), Section: gen.TaskSection(t.Section()), Nightly: t.Nightly, PendingCount: t.Left,
		Running: t.Running, Stoppable: t.Running && !t.Finish,
	}
	if t.Task.Failed != nil {
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
	if err := h.deps.Tasks.Start(ctx, req.Task); err != nil {
		return nil, err
	}
	return gen.RunTask204Response{}, nil
}

func (h *handlers) StopTask(ctx context.Context, req gen.StopTaskRequestObject) (gen.StopTaskResponseObject, error) {
	if err := h.deps.Tasks.Stop(ctx, req.Task); err != nil {
		return nil, err
	}
	return gen.StopTask204Response{}, nil
}

func (h *handlers) ListTaskFailures(ctx context.Context, req gen.ListTaskFailuresRequestObject) (gen.ListTaskFailuresResponseObject, error) {
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
	var items []id.ID
	if req.Body != nil {
		items = deref(req.Body.ItemIds)
	}
	if err := h.deps.Tasks.Retry(ctx, req.Task, items...); err != nil {
		return nil, err
	}
	return gen.RetryTask204Response{}, nil
}
