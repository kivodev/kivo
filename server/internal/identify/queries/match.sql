-- name: GetMatch :one
SELECT * FROM match WHERE library_id = @library_id AND subject = @subject;

-- name: PutMatch :exec
INSERT INTO match (library_id, subject, source, query, status, provider, kind, ext_id, title, year, score, ordering, updated_at)
VALUES (@library_id, @subject, @source, @query, @status, @provider, @kind, @ext_id, @title, @year, @score, @ordering, @updated_at)
ON CONFLICT (library_id, subject) DO UPDATE SET
  source = excluded.source, query = excluded.query, status = excluded.status, provider = excluded.provider,
  kind = excluded.kind, ext_id = excluded.ext_id, title = excluded.title, year = excluded.year,
  score = excluded.score, ordering = excluded.ordering, updated_at = excluded.updated_at;

-- name: MoveMatch :exec
-- A folder's decision follows it when it's renamed or moved.
UPDATE OR REPLACE match SET library_id = @library_id, subject = @subject
WHERE library_id = @old_library_id AND subject = @old_subject;
