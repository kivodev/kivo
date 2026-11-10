-- Reads of catalog's tables, and previews' own preview_file.

-- name: Pending :many
-- The present, probed files with a picture, in libraries that make
-- previews, whose previews aren't made or failed at their generation, by
-- library and path.
SELECT f.id FROM library l
JOIN library_folder r ON r.library_id = l.id
JOIN media_file f ON f.folder_id = r.id
WHERE l.previews <> 'never' AND l.deleting_at IS NULL AND f.status = 'present' AND f.duration_ms > 0
  AND EXISTS (SELECT 1 FROM media_track t WHERE t.file_id = f.id AND t.type = 'video'
    AND coalesce(json_extract(t.info, '$.attached_picture'), 0) = 0)
  AND NOT EXISTS (SELECT 1 FROM preview_file x WHERE x.file_id = f.id AND x.generation = f.generation)
ORDER BY l.name, r.path, f.rel_path;

-- name: FailedNow :one
-- Whether a file failed at its generation.
SELECT CAST(EXISTS (SELECT 1 FROM preview_file x JOIN media_file f ON f.id = x.file_id AND f.generation = x.generation
  WHERE x.file_id = @id AND x.state = 'failed') AS BOOLEAN);

-- name: Settle :exec
INSERT INTO preview_file (file_id, generation, state, reason, updated_at) VALUES (@file_id, @generation, @state, @reason, @updated_at)
ON CONFLICT (file_id) DO UPDATE SET
  generation = excluded.generation, state = excluded.state, reason = excluded.reason, updated_at = excluded.updated_at;

-- name: Failed :many
-- The files whose previews failed at their generation, newest first.
SELECT x.file_id, x.reason, x.updated_at FROM preview_file x
JOIN media_file f ON f.id = x.file_id AND f.generation = x.generation
WHERE x.state = 'failed'
ORDER BY x.updated_at DESC;

-- name: Forget :many
-- Forgets the failures of the given files, or of every file for none.
DELETE FROM preview_file
WHERE state = 'failed' AND (CAST(@all AS BOOLEAN) OR file_id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids)))
RETURNING file_id;

-- name: Made :many
-- The files whose previews were made, at the generation they were made for.
SELECT file_id, generation FROM preview_file WHERE state = 'done';

-- name: Unmake :exec
DELETE FROM preview_file WHERE file_id = @file_id AND generation = @generation;

-- name: Wanted :one
-- Whether any library makes previews.
SELECT CAST(EXISTS (SELECT 1 FROM library WHERE previews <> 'never' AND deleting_at IS NULL) AS BOOLEAN);
