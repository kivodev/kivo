package main

import (
	"context"
	"path/filepath"

	"github.com/mmcdole/kivo/internal/tasks"
)

// tasks gathers every component's tasks, in the order the Tasks page lists
// them in each of its tables.
func (s *server) tasks() []tasks.Task {
	var out []tasks.Task
	for _, ts := range [][]tasks.Task{
		s.scanner.Tasks(), s.identifier.Tasks(), s.metadata.Tasks(), s.previews.Tasks(), s.markers.Tasks(),
		s.subtitles.Tasks(), s.downloads.Tasks(), s.catalog.Tasks(), s.activity.Tasks(), s.databaseTasks(),
	} {
		out = append(out, ts...)
	}
	return out
}

// databaseTasks returns the server's own tasks on its database.
func (s *server) databaseTasks() []tasks.Task {
	return []tasks.Task{
		{Name: "make-backup", Schedule: tasks.Nightly, Maintenance: true, Work: tasks.Func(func(ctx context.Context) error {
			path, err := s.db.Backup(ctx, s.boot.BackupDir, 7)
			if err == nil {
				tasks.Summarize(ctx, "Backed up to "+filepath.Base(path))
			}
			return err
		})},
		{Name: "optimize-database", Schedule: tasks.Nightly, Maintenance: true, Work: tasks.Func(s.db.Optimize)},
	}
}
