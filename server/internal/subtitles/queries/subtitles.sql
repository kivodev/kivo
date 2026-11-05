-- Reads of catalog's tables, and subtitles' own subtitle_file. What it
-- extracts is in cache/, and what it downloads in data/.

-- name: FileGeneration :one
SELECT generation FROM media_file WHERE id = @id;

-- name: FileGenerations :many
SELECT id, generation FROM media_file WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));

-- name: Pending :many
-- The present files with an embedded ASS, SSA or PGS track, in libraries
-- that extract subtitles, whose subtitles aren't extracted or failed at
-- their generation, by library and path. The IN reads the few such tracks
-- from media_track_extracted first, where an EXISTS would read every file's
-- tracks.
SELECT f.id FROM library l
JOIN library_folder r ON r.library_id = l.id
JOIN media_file f ON f.folder_id = r.id
WHERE l.subtitles <> 'never' AND l.deleting_at IS NULL AND f.status = 'present'
  AND f.id IN (SELECT t.file_id FROM media_track t
    WHERE t.type = 'subtitle' AND lower(t.codec) IN ('ass', 'ssa', 'hdmv_pgs_subtitle', 'pgs', 'sup'))
  AND NOT EXISTS (SELECT 1 FROM subtitle_file x WHERE x.file_id = f.id AND x.generation = f.generation)
ORDER BY l.name, r.path, f.rel_path;

-- name: Settled :one
-- How a file's extraction ended at its generation, done or failed with the
-- reason; no row if it hasn't.
SELECT x.state, x.reason FROM subtitle_file x JOIN media_file f ON f.id = x.file_id AND f.generation = x.generation
WHERE x.file_id = @id;

-- name: Settle :exec
INSERT INTO subtitle_file (file_id, generation, state, reason, updated_at) VALUES (@file_id, @generation, @state, @reason, @updated_at)
ON CONFLICT (file_id) DO UPDATE SET
  generation = excluded.generation, state = excluded.state, reason = excluded.reason, updated_at = excluded.updated_at;

-- name: Failed :many
-- The files whose extraction failed at their generation, newest first.
SELECT x.file_id, x.reason, x.updated_at FROM subtitle_file x
JOIN media_file f ON f.id = x.file_id AND f.generation = x.generation
WHERE x.state = 'failed'
ORDER BY x.updated_at DESC;

-- name: Forget :many
-- Forgets the failures of the given files, or of every file for none.
DELETE FROM subtitle_file
WHERE state = 'failed' AND (CAST(@all AS BOOLEAN) OR file_id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids)))
RETURNING file_id;

-- name: Made :many
-- The files whose subtitles were extracted, at the generation they were
-- extracted for.
SELECT file_id, generation FROM subtitle_file WHERE state = 'done';

-- name: Unmake :exec
DELETE FROM subtitle_file WHERE file_id = @file_id AND generation = @generation;

-- name: Wanted :one
-- Whether any library extracts subtitles.
SELECT CAST(EXISTS (SELECT 1 FROM library WHERE subtitles <> 'never' AND deleting_at IS NULL) AS BOOLEAN);
