-- Reads of catalog's tables, and writes of media_file.identified, which
-- identify owns.

-- name: FolderFiles :many
-- The present files under a directory of a library folder, as a range of paths: '0'
-- follows '/'.
SELECT id, rel_path, size, generation, tags FROM media_file
WHERE folder_id = @folder_id AND status = 'present'
  AND rel_path >= CAST(@dir AS TEXT) || '/' AND rel_path < CAST(@dir AS TEXT) || '0'
ORDER BY rel_path;

-- name: FileGenerations :many
SELECT id, generation FROM media_file WHERE status = 'present' AND id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));

-- name: Placements :many
-- The versions files are parts of, with their edition labels, and the
-- works each covers, in order.
SELECT f.id AS file_id, vw.version_id, v.edition, vw.work_id, w.kind, w.root_id, w.provisional, w.match_rev
FROM media_file f
JOIN media_version v ON v.id = f.version_id
JOIN media_version_work vw ON vw.version_id = f.version_id
JOIN work w ON w.id = vw.work_id
WHERE f.id IN (SELECT unhex(value) FROM (SELECT CAST(@files AS TEXT) AS files), json_each(files))
ORDER BY vw.version_id, vw.ord;

-- name: VersionParts :many
SELECT version_id, id AS file_id FROM media_file WHERE version_id IN (SELECT unhex(value) FROM (SELECT CAST(@versions AS TEXT) AS versions), json_each(versions)) ORDER BY version_id, part;

-- name: MatchRevs :many
SELECT id, match_rev FROM work WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));

-- name: Extra :one
SELECT id FROM work
WHERE parent_id = @parent_id AND kind = 'extra' AND title = @title AND extra_type = @extra_type
ORDER BY rid LIMIT 1;

-- name: WorkState :one
SELECT kind, root_id, edition, provisional, locked, library_id FROM work WHERE id = @id;

-- name: ExternalIDsOf :many
SELECT provider, kind, ext_id FROM work_external_id WHERE work_id = @work_id ORDER BY provider, kind, ext_id;

-- name: PresentElsewhere :one
-- Whether a root work has present files other than these: of its versions
-- and of the works under it.
SELECT EXISTS (
  SELECT 1 FROM work w
  CROSS JOIN media_version_work vw ON vw.work_id = w.id
  CROSS JOIN media_file f ON f.version_id = vw.version_id
  WHERE w.root_id = @root_id AND f.status = 'present'
    AND f.id NOT IN (SELECT unhex(value) FROM (SELECT CAST(@files AS TEXT) AS files), json_each(files)));

-- name: GoneRootFiles :many
-- The files of a root work's versions with no part present: of its
-- versions and of the works under it.
SELECT DISTINCT f.id FROM work w
CROSS JOIN media_version_work vw ON vw.work_id = w.id
CROSS JOIN media_file f ON f.version_id = vw.version_id
WHERE w.root_id = @root_id
  AND NOT EXISTS (SELECT 1 FROM media_file p WHERE p.version_id = vw.version_id AND p.status = 'present');

-- name: EpisodePlace :one
SELECT parent_id, parent_number, number FROM work WHERE id = @id;

-- name: Ordering :one
SELECT ordering FROM work WHERE id = @id;

-- name: FilesOfWork :many
-- The present files of the versions on a work and the works under it,
-- with their versions, in its root work's library: a file in another was moved
-- there, and its identify, pending, places it.
WITH RECURSIVE sub (id) AS (
  SELECT w.id FROM work w WHERE w.id = @work_id
  UNION ALL
  SELECT w.id FROM work w JOIN sub ON w.parent_id = sub.id
)
SELECT DISTINCT f.id, r.library_id, f.rel_path, f.generation, vw.version_id
FROM sub
CROSS JOIN media_version_work vw ON vw.work_id = sub.id
CROSS JOIN media_file f ON f.version_id = vw.version_id AND f.status = 'present'
JOIN library_folder r ON r.id = f.folder_id
WHERE r.library_id = coalesce((SELECT t.library_id FROM work x JOIN work t ON t.id = x.root_id WHERE x.id = @work_id), r.library_id)
ORDER BY f.id;

-- name: LibraryRefreshWorks :many
-- What Refresh all metadata goes through: each matched work available in
-- the library, and each unmatched movie, series or video, whose folders are
-- identified again, with what's under it. Extras are left as a movie's own
-- refresh leaves them.
SELECT w.id, w.provisional FROM work_library wl
JOIN work w ON w.id = wl.work_id
JOIN work r ON r.id = w.root_id
WHERE wl.library_id = @library_id AND w.kind <> 'extra'
  AND (w.parent_id IS NULL OR w.provisional = 0 AND r.provisional = 0)
ORDER BY w.id;

-- name: Unidentified :many
-- A library's present files probed but not identified at their
-- generation, by path.
SELECT f.id, f.rel_path FROM media_file f JOIN library_folder r ON r.id = f.folder_id
WHERE r.library_id = @library_id AND r.removing_at IS NULL AND f.status = 'present'
  AND f.probed = f.generation AND f.probe_error IS NULL AND f.identified IS NOT f.generation
ORDER BY r.path, f.rel_path;

-- name: UnidentifiedBetween :many
-- A library's files that Unidentified lists, within a range of paths.
SELECT f.id, f.rel_path, f.generation FROM media_file f JOIN library_folder r ON r.id = f.folder_id
WHERE r.library_id = @library_id AND r.removing_at IS NULL AND f.status = 'present'
  AND f.probed = f.generation AND f.probe_error IS NULL AND f.identified IS NOT f.generation
  AND f.rel_path >= CAST(@from AS TEXT) AND f.rel_path < CAST(@before AS TEXT)
ORDER BY r.path, f.rel_path;

-- name: CountUnidentified :one
-- How many files Unidentified lists across the libraries.
SELECT count(*) FROM media_file f JOIN library_folder r ON r.id = f.folder_id
JOIN library l ON l.id = r.library_id AND l.deleting_at IS NULL
WHERE r.removing_at IS NULL AND f.status = 'present'
  AND f.probed = f.generation AND f.probe_error IS NULL AND f.identified IS NOT f.generation;

-- name: SetIdentified :exec
UPDATE media_file SET identified = generation WHERE id = @id AND generation = @generation;

-- name: Reidentify :exec
UPDATE media_file SET identified = NULL WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));
