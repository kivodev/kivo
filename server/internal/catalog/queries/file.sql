-- name: FileByPath :one
SELECT id, size, mtime, generation, status FROM media_file
WHERE folder_id = @folder_id AND rel_path = @rel_path;

-- name: InsertFile :exec
INSERT INTO media_file (id, folder_id, rel_path, size, mtime, status)
VALUES (@id, @folder_id, @rel_path, @size, @mtime, 'present');

-- name: UpdateFileStat :one
-- A file seen again: present, and a new generation when its content changed.
UPDATE media_file SET size = @size, mtime = @mtime, status = 'present', gone_at = NULL,
  generation = generation + CAST(@bump AS BOOLEAN)
WHERE id = @id
RETURNING generation;

-- name: MarkGone :execrows
-- A file found missing from a path is gone only while it's still present at
-- that path: a probe that moved it to a new path meanwhile wins.
UPDATE media_file SET status = 'gone', gone_at = @now
WHERE id = @id AND folder_id = @folder_id AND rel_path = @rel_path AND status = 'present';

-- name: FileGeneration :one
SELECT generation FROM media_file WHERE id = @id;

-- name: FileGenerations :many
SELECT id, generation FROM media_file WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));

-- name: GetFile :one
SELECT sqlc.embed(f), r.path AS folder_path, r.library_id FROM media_file f
JOIN library_folder r ON r.id = f.folder_id
WHERE f.id = @id;

-- name: FilesByHash :many
SELECT sqlc.embed(f), r.path AS folder_path, r.library_id FROM media_file f
JOIN library_folder r ON r.id = f.folder_id
WHERE f.size = @size AND f.edge_hash = @edge_hash;

-- name: SetProbe :execrows
UPDATE media_file SET container = @container, duration_ms = @duration_ms, bitrate = @bitrate,
  start_ms = @start_ms, edge_hash = @edge_hash, os_hash = @os_hash, tags = @tags
WHERE id = @id AND generation = @generation;

-- name: EmbeddedTracks :many
SELECT id, stream_index, type, codec FROM media_track
WHERE file_id = @file_id AND source = 'embedded';

-- name: DeleteTracks :exec
DELETE FROM media_track WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));

-- name: InsertEmbeddedTrack :exec
INSERT INTO media_track (id, file_id, source, stream_index, type, codec, language, title, is_default, forced, hearing_impaired, info)
VALUES (@id, @file_id, 'embedded', @stream_index, @type, @codec, @language, @title, @is_default, @forced, @hearing_impaired, @info);

-- name: UpdateEmbeddedTrack :exec
-- A re-probe that finds the same type and codec at a stream index keeps
-- the track's ID and refreshes the rest.
UPDATE media_track SET language = @language, title = @title, is_default = @is_default, forced = @forced,
  hearing_impaired = @hearing_impaired, info = @info
WHERE id = @id;

-- name: ListTracks :many
-- Embedded tracks by stream index, then subtitle files next to the video,
-- then downloaded ones, each by path.
SELECT * FROM media_track WHERE file_id = @file_id
ORDER BY source <> 'embedded', source = 'downloaded', stream_index, rel_path;

-- name: DeleteChapters :exec
DELETE FROM media_chapter WHERE file_id = @file_id;

-- name: InsertChapter :exec
INSERT INTO media_chapter (file_id, idx, start_ms, end_ms, title) VALUES (@file_id, @idx, @start_ms, @end_ms, @title);

-- name: ListChapters :many
SELECT * FROM media_chapter WHERE file_id = @file_id ORDER BY idx;

-- name: DeleteAttachments :exec
DELETE FROM media_attachment WHERE file_id = @file_id;

-- name: InsertAttachment :exec
INSERT INTO media_attachment (file_id, idx, kind, filename, mime_type, size, sha256)
VALUES (@file_id, @idx, @kind, @filename, @mime_type, @size, @sha256);

-- name: ListAttachments :many
SELECT * FROM media_attachment WHERE file_id = @file_id ORDER BY idx;

-- name: AttachmentLibraries :many
-- The libraries holding a file that carries an attachment, by its SHA-256.
SELECT DISTINCT r.library_id FROM media_attachment a
JOIN media_file f ON f.id = a.file_id
JOIN library_folder r ON r.id = f.folder_id
WHERE a.sha256 = @sha256;

-- name: MoveFile :exec
-- Relink: the gone row takes the found file's place.
UPDATE media_file SET folder_id = @folder_id, rel_path = @rel_path, size = @size, mtime = @mtime,
  status = 'present', gone_at = NULL
WHERE id = @id;

-- name: DeleteFiles :exec
DELETE FROM media_file WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));

-- name: FileLibraries :many
SELECT DISTINCT r.library_id FROM media_file f JOIN library_folder r ON r.id = f.folder_id
WHERE f.id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));

-- name: FilesUnder :many
-- Files at or below a directory of a folder; dir is "" for the whole folder.
SELECT id, rel_path, status FROM media_file
WHERE folder_id = @folder_id
  AND (CAST(@dir AS TEXT) = '' OR (rel_path > @dir || '/' AND rel_path < @dir || '0'))
ORDER BY rel_path;

-- name: GoneBatch :many
-- At most @limit of the files gone at or before @before: one batch of the
-- prune task, or of deleting every missing file.
SELECT id FROM media_file WHERE status = 'gone' AND gone_at <= CAST(@before AS INTEGER) LIMIT @limit;

-- name: PutSubtitleFile :one
-- A subtitle file keeps its ID while it stays at its path.
INSERT INTO media_track (id, file_id, source, rel_path, type, codec, language, title, is_default, forced, hearing_impaired)
VALUES (@id, @file_id, @source, @rel_path, 'subtitle', @codec, @language, @title, @is_default, @forced, @hearing_impaired)
ON CONFLICT (file_id, source, rel_path) DO UPDATE SET codec = excluded.codec, language = excluded.language,
  title = excluded.title, is_default = excluded.is_default, forced = excluded.forced,
  hearing_impaired = excluded.hearing_impaired
RETURNING id;

-- name: FilesIn :many
-- Files directly in a directory of a folder; dir is "" for the folder itself.
SELECT id, rel_path, status FROM media_file
WHERE folder_id = @folder_id
  AND CASE WHEN CAST(@dir AS TEXT) = '' THEN instr(rel_path, '/') = 0
      ELSE rel_path > @dir || '/' AND rel_path < @dir || '0' AND instr(substr(rel_path, length(@dir) + 2), '/') = 0 END
ORDER BY rel_path;

-- name: Subjects :many
-- What jobs work on, by ID: files, with the first work each one's version
-- covers, works, library folders and libraries. A season or episode comes
-- with its series.
SELECT f.id, f.rel_path AS path, coalesce(w.kind, '') AS kind, coalesce(w.title, '') AS title,
  w.number, w.parent_number, coalesce(s.title, '') AS series
FROM media_file f
LEFT JOIN work w ON w.id = (SELECT vw.work_id FROM media_version_work vw WHERE vw.version_id = f.version_id ORDER BY vw.ord LIMIT 1)
LEFT JOIN work s ON s.id = w.root_id AND w.kind IN ('season', 'episode')
WHERE f.id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids))
UNION ALL
SELECT w.id, '', w.kind, w.title, w.number, w.parent_number, coalesce(s.title, '')
FROM work w
LEFT JOIN work s ON s.id = w.root_id AND w.kind IN ('season', 'episode')
WHERE w.id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids))
UNION ALL
SELECT r.id, r.path, '', '', NULL, NULL, ''
FROM library_folder r
WHERE r.id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids))
UNION ALL
SELECT l.id, '', '', l.name, NULL, NULL, ''
FROM library l
WHERE l.id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));
