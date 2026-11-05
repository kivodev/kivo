-- name: Tasks :many
SELECT name, finished, summary, error FROM task;

-- name: EndRun :exec
-- A run ends: what it did, or why it failed.
INSERT INTO task (name, finished, summary, error) VALUES (@name, @finished, @summary, @error)
ON CONFLICT (name) DO UPDATE SET
  finished = excluded.finished, summary = excluded.summary, error = excluded.error;
