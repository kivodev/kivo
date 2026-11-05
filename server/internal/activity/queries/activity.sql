-- name: Insert :exec
INSERT INTO activity (id, at, kind, data) VALUES (@id, @at, @kind, @data);

-- name: Failures :many
-- The failed sign-ins since @since, the latest first.
SELECT id, at, data FROM activity WHERE kind = 'signInFailed' AND at >= @since ORDER BY at DESC, id DESC;

-- name: UpdateFailures :exec
-- A run of failed sign-ins goes on: its count and time are the latest.
UPDATE activity SET at = @at, data = @data WHERE id = @id;

-- name: Latest :many
-- A page of every kind, the newest first, before (@before_at, @before_id).
SELECT id, at, kind, data FROM activity
WHERE (at, id) < (@before_at, CAST(@before_id AS BLOB))
ORDER BY at DESC, id DESC
LIMIT @limit;

-- name: LatestOf :many
-- A page of the kinds in the JSON array @kinds, the newest first, before
-- (@before_at, @before_id).
SELECT id, at, kind, data FROM activity
WHERE kind IN (SELECT value FROM (SELECT CAST(@kinds AS TEXT) AS kinds), json_each(kinds)) AND (at, id) < (@before_at, CAST(@before_id AS BLOB))
ORDER BY at DESC, id DESC
LIMIT @limit;

-- name: Prune :execrows
DELETE FROM activity WHERE at < @before;

-- name: WorkName :one
-- How a work is named: an episode by its series, season and number.
SELECT w.kind, w.title, coalesce(r.title, w.title) AS root_title, w.number, w.parent_number
FROM work w LEFT JOIN work r ON r.id = w.root_id
WHERE w.id = @id;

-- name: Username :one
SELECT username FROM user WHERE id = @id;

-- name: LibraryName :one
SELECT name FROM library WHERE id = @id;
