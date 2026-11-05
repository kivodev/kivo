-- Reads of catalog's tables, and writes of the refresh columns metadata
-- owns in work.

-- name: Subtree :many
-- A work and the seasons and episodes under it.
WITH RECURSIVE sub (id) AS (
  SELECT w.id FROM work w WHERE w.id = @work_id
  UNION ALL
  SELECT w.id FROM work w JOIN sub ON w.parent_id = sub.id WHERE w.kind IN ('season', 'episode')
)
SELECT w.id, w.provisional FROM sub CROSS JOIN work w ON w.id = sub.id;

-- name: Unmatched :one
-- Whether a work, or the root work it's under, is provisional: nothing
-- describes it.
SELECT CAST(w.provisional OR r.provisional AS BOOLEAN) FROM work w JOIN work r ON r.id = w.root_id WHERE w.id = @id;

-- name: LocalFiles :many
-- The present files of a work and the works under it, in libraries that use
-- local metadata.
WITH RECURSIVE sub (id) AS (
  SELECT w.id FROM work w WHERE w.id = @work_id
  UNION ALL
  SELECT w.id FROM work w JOIN sub ON w.parent_id = sub.id WHERE w.kind IN ('season', 'episode')
)
SELECT DISTINCT r.path AS folder_path, f.rel_path
FROM sub
CROSS JOIN media_version_work vw ON vw.work_id = sub.id
CROSS JOIN media_file f ON f.version_id = vw.version_id AND f.status = 'present'
JOIN library_folder r ON r.id = f.folder_id
JOIN library l ON l.id = r.library_id AND l.use_local_metadata
ORDER BY r.path, f.rel_path;

-- name: BookFiles :many
-- A book's present files, its first version's parts in order, with their
-- tags, running times and the size of the cover picture each carries (zero
-- for none).
SELECT r.path AS folder_path, f.rel_path, f.mtime, f.tags, coalesce(f.duration_ms, 0) AS duration_ms,
  CAST(coalesce((SELECT json_extract(t.info, '$.width') FROM media_track t WHERE t.file_id = f.id AND t.type = 'video'
    AND json_extract(t.info, '$.attached_picture') ORDER BY t.stream_index LIMIT 1), 0) AS INTEGER) AS cover_width,
  CAST(coalesce((SELECT json_extract(t.info, '$.height') FROM media_track t WHERE t.file_id = f.id AND t.type = 'video'
    AND json_extract(t.info, '$.attached_picture') ORDER BY t.stream_index LIMIT 1), 0) AS INTEGER) AS cover_height
FROM media_file f
JOIN library_folder r ON r.id = f.folder_id
WHERE f.version_id = (SELECT min(vw.version_id) FROM media_version_work vw WHERE vw.work_id = @work_id)
  AND f.status = 'present'
ORDER BY f.part;

-- name: WorkLibrary :one
SELECT library_id FROM work_library WHERE work_id = @work_id;

-- name: BookText :one
-- What a book's refresh keeps of its description while its provider is
-- unavailable.
SELECT overview, release_date FROM work WHERE id = @id;

-- name: DueRefreshes :many
-- The works due a metadata refresh: every work but extras, provisional
-- works and those under one, never refreshed, or in a library that
-- refreshes its metadata last refreshed longer ago than that library's
-- refresh_days before @now; the oldest first.
SELECT w.id
FROM library l
CROSS JOIN work_library wl ON wl.library_id = l.id
CROSS JOIN work w ON w.id = wl.work_id
JOIN work r ON r.id = w.root_id
WHERE l.deleting_at IS NULL AND w.kind <> 'extra' AND w.provisional = 0 AND r.provisional = 0
  AND (w.refreshed_at IS NULL OR l.refresh_days > 0 AND w.refreshed_at < @now - l.refresh_days * 86400000)
GROUP BY w.id
ORDER BY coalesce(min(w.refreshed_at), 0), w.id;

-- name: FailedRefreshes :many
-- The works whose last refresh failed, newest first.
SELECT id, refresh_error, refreshed_at FROM work WHERE refresh_error IS NOT NULL
ORDER BY refreshed_at DESC;

-- name: RefreshWanted :one
-- Whether any library refreshes its metadata.
SELECT CAST(EXISTS (SELECT 1 FROM library WHERE refresh_days > 0 AND deleting_at IS NULL) AS BOOLEAN);

-- name: MarkRefreshed :exec
-- Records a work's metadata refreshed.
UPDATE work SET refreshed_at = @now, refresh_error = NULL WHERE id = @id;

-- name: MarkRefreshFailed :exec
-- Records a work's refresh failed now, and why: it waits for its library's
-- next refresh, as one that worked does.
UPDATE work SET refreshed_at = @now, refresh_error = @reason WHERE id = @id;

-- name: MarkDue :exec
-- Makes works due a metadata refresh at once.
UPDATE work SET refreshed_at = NULL, refresh_error = NULL WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));
