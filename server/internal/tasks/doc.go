// Package tasks runs the server's background work. Each task is one row of
// the Tasks page, or a step shown on another task's row. Its work is one function, or a list of items read from
// the tables of the feature that owns it each time it looks for work. So
// the only record a task keeps of its own is how its last run ended.
package tasks
