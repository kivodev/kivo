-- name: ListDirs :many
SELECT path, mtime, listed_at, local_metadata_hash FROM scan_dir WHERE folder_id = @folder_id;

-- name: AllDirs :many
SELECT folder_id, path FROM scan_dir ORDER BY folder_id, path;

-- name: PutDir :exec
INSERT INTO scan_dir (folder_id, path, mtime, listed_at, local_metadata_hash) VALUES (@folder_id, @path, @mtime, @listed_at, @local_metadata_hash)
ON CONFLICT (folder_id, path) DO UPDATE SET
  mtime = excluded.mtime, listed_at = excluded.listed_at, local_metadata_hash = excluded.local_metadata_hash;

-- name: DeleteDir :exec
DELETE FROM scan_dir WHERE folder_id = @folder_id AND path = @path;

-- name: GetScanFolder :one
SELECT full_walk_at, full_requested FROM scan_folder WHERE folder_id = @folder_id;

-- name: RequestFullWalk :exec
INSERT INTO scan_folder (folder_id, full_requested) VALUES (@folder_id, 1)
ON CONFLICT (folder_id) DO UPDATE SET full_requested = 1;

-- name: FullWalkDone :exec
INSERT INTO scan_folder (folder_id, full_walk_at, full_requested) VALUES (@folder_id, CAST(@at AS INTEGER), 0)
ON CONFLICT (folder_id) DO UPDATE SET full_walk_at = excluded.full_walk_at, full_requested = 0;

-- name: FirstFileUnder :one
-- The first present file below a directory of a folder.
SELECT id, rel_path FROM media_file
WHERE folder_id = @folder_id AND status = 'present'
  AND rel_path > CAST(@dir AS TEXT) || '/' AND rel_path < CAST(@dir AS TEXT) || '0'
ORDER BY rel_path
LIMIT 1;

-- name: UnmatchedFiles :many
-- The folder's present files whose version is on a provisional work.
SELECT DISTINCT f.id, f.rel_path
FROM media_file f
JOIN media_version_work vw ON vw.version_id = f.version_id
JOIN work w ON w.id = vw.work_id
WHERE f.folder_id = @folder_id AND f.status = 'present' AND w.provisional
ORDER BY f.rel_path;

-- name: Unprobed :many
-- A library's present files not probed at their generation, by path.
SELECT f.id FROM media_file f JOIN library_folder r ON r.id = f.folder_id
WHERE r.library_id = @library_id AND r.removing_at IS NULL AND f.status = 'present' AND f.probed IS NOT f.generation
ORDER BY r.path, f.rel_path;

-- name: SetProbed :exec
-- Records a file probed at its generation, once catalog has its probe.
UPDATE media_file SET probed = generation, probe_error = NULL WHERE id = @id AND generation = @generation;

-- name: SetProbeFailed :exec
UPDATE media_file SET probed = generation, probe_error = @reason WHERE id = @id AND generation = @generation;

-- name: ForgetProbeFailed :exec
UPDATE media_file SET probed = NULL, probe_error = NULL WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids)) AND probe_error IS NOT NULL;

-- name: SetFolderReachable :execrows
UPDATE library_folder SET reachable = @reachable WHERE id = @id;

-- name: SetFolderScanned :exec
UPDATE library_folder SET last_scan_at = @last_scan_at WHERE id = @id;

-- name: Behind :many
-- The libraries with files a scan hasn't caught up with: not probed or not
-- identified at their generation.
SELECT DISTINCT r.library_id FROM media_file f JOIN library_folder r ON r.id = f.folder_id
JOIN library l ON l.id = r.library_id AND l.deleting_at IS NULL
WHERE f.status = 'present' AND (f.probed IS NOT f.generation OR f.identified IS NOT f.generation)
  AND (f.probed IS NOT f.generation OR f.probe_error IS NULL) AND r.removing_at IS NULL;

-- name: ProbeFailures :many
-- The present files that failed to probe at their generation, newest
-- first, by when they were last seen to change.
SELECT f.id, f.probe_error, f.mtime FROM media_file f
WHERE f.status = 'present' AND f.probed = f.generation AND f.probe_error IS NOT NULL
ORDER BY f.mtime DESC;

-- name: FileLibraries :many
SELECT DISTINCT r.library_id FROM media_file f JOIN library_folder r ON r.id = f.folder_id
WHERE f.id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));
