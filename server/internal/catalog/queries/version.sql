-- name: InsertVersion :exec
INSERT INTO media_version (id, edition, created_at) VALUES (@id, @edition, @created_at);

-- name: SetVersionEdition :exec
UPDATE media_version SET edition = @edition WHERE id = @id;

-- name: VersionEdition :one
SELECT edition FROM media_version WHERE id = @id;

-- name: SetPart :exec
UPDATE media_file SET version_id = @version_id, part = @part WHERE id = @file_id;

-- name: ClearParts :exec
-- Files leave their versions before the versions are deleted.
UPDATE media_file SET version_id = NULL, part = NULL WHERE version_id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));

-- name: InsertVersionWork :exec
INSERT INTO media_version_work (version_id, work_id, ord) VALUES (@version_id, @work_id, @ord);

-- name: DeleteVersionWorks :exec
DELETE FROM media_version_work WHERE version_id = @version_id;

-- name: DeleteVersions :exec
DELETE FROM media_version WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));

-- name: VersionParts :many
SELECT f.part, f.id AS file_id FROM media_file f WHERE f.version_id = @version_id ORDER BY f.part;

-- name: VersionWorks :many
SELECT work_id FROM media_version_work WHERE version_id = @version_id ORDER BY ord;

-- name: VersionsOfFiles :many
SELECT DISTINCT version_id FROM media_file WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids)) AND version_id IS NOT NULL;

-- name: WorksOfVersions :many
SELECT DISTINCT work_id FROM media_version_work WHERE version_id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));

-- name: WorkHasVersions :one
SELECT EXISTS (SELECT 1 FROM media_version_work WHERE work_id = @work_id);

-- name: ClearWorkLibrary :one
DELETE FROM work_library WHERE work_id = @work_id RETURNING library_id, is_root;

-- name: FillWorkLibrary :one
-- A work is available, in its root work's library, when a version with no gone
-- part covers it or one of its descendants (extras don't count for their
-- parent). Works nest at most series, season, episode, so a work's
-- descendants are those whose root or parent it is. Its last_added_at is
-- the newest added_at among the works whose versions make it available.
INSERT INTO work_library (library_id, work_id, is_root, sort_title, added_at, year, runtime_sort, resolutions, dynamic_ranges, rating_sort, creator_sort, last_added_at)
SELECT t.library_id, w.id, w.parent_id IS NULL, w.sort_title, w.added_at, k.year, k.runtime_sort, k.resolutions, k.dynamic_ranges, k.rating_sort, k.creator_sort,
  max(d.added_at)
FROM work w
JOIN work t ON t.id = w.root_id
JOIN work_browse_keys k ON k.work_id = w.id
JOIN media_version_work vw ON w.id = @work_id AND vw.work_id IN (
  SELECT @work_id
  UNION SELECT d.id FROM work d WHERE d.root_id = @work_id AND d.kind <> 'extra'
  UNION SELECT d.id FROM work d WHERE d.parent_id = @work_id AND d.kind <> 'extra')
JOIN work d ON d.id = vw.work_id
WHERE NOT EXISTS (
    SELECT 1 FROM media_file f2 WHERE f2.version_id = vw.version_id AND f2.status = 'gone')
GROUP BY w.id
RETURNING library_id, is_root;

-- name: VersionLibrary :one
-- The library a version's parts are in.
SELECT r.library_id FROM media_file f JOIN library_folder r ON r.id = f.folder_id
WHERE f.version_id = @version_id LIMIT 1;

-- name: SetRootLibrary :exec
-- The root works of works that have no library take the one a version
-- they gain is in.
UPDATE work SET library_id = @library_id
WHERE library_id IS NULL AND id IN (SELECT w.root_id FROM work w WHERE w.id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids)));

-- name: RootsElsewhere :one
-- Whether any of the root works of works is in another library than this
-- one.
SELECT EXISTS (SELECT 1 FROM work t
  WHERE t.library_id <> @library_id
    AND t.id IN (SELECT w.root_id FROM work w WHERE w.id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids))));

-- name: RootWorks :many
-- Every work of a root work.
SELECT w.id FROM work w WHERE w.root_id = @root_id;

-- name: MoveRootLibrary :exec
UPDATE work SET library_id = @library_id WHERE id = @root_id;

-- name: RootVersionsElsewhere :one
-- Whether a version of a root work has a part in another library than
-- this one.
SELECT EXISTS (SELECT 1 FROM work w
  JOIN media_version_work vw ON vw.work_id = w.id
  JOIN media_file f ON f.version_id = vw.version_id
  JOIN library_folder r ON r.id = f.folder_id
  WHERE w.root_id = @root_id
    AND r.library_id <> @library_id);

-- name: Ancestors :many
WITH RECURSIVE up (id, parent_id) AS (
  SELECT w.id, w.parent_id FROM work w WHERE w.id = @work_id
  UNION ALL
  SELECT w.id, w.parent_id FROM work w JOIN up ON w.id = up.parent_id
)
SELECT id FROM up;

-- name: VersionsOfWork :many
-- A work's versions, with what orders them: the edition label, the first
-- part's picture height and the size of all parts. A version with a gone
-- part is left out until the file returns.
SELECT vw.version_id, coalesce(mv.edition, '') AS edition,
  CAST(coalesce((
    SELECT json_extract(t.info, '$.height')
    FROM media_file p
    JOIN media_track t ON t.file_id = p.id
    WHERE p.version_id = vw.version_id AND p.part = 1 AND t.type = 'video'
      AND NOT coalesce(json_extract(t.info, '$.attached_picture'), 0)
    ORDER BY t.stream_index
    LIMIT 1
  ), 0) AS INTEGER) AS height,
  CAST((SELECT coalesce(sum(f.size), 0) FROM media_file f WHERE f.version_id = vw.version_id) AS INTEGER) AS size
FROM media_version_work vw
JOIN media_version mv ON mv.id = vw.version_id
WHERE vw.work_id = @work_id
  AND NOT EXISTS (SELECT 1 FROM media_file g WHERE g.version_id = vw.version_id AND g.status = 'gone')
ORDER BY vw.version_id;

-- name: WorkAccess :many
-- The libraries a work is available in, with its kind and its
-- content rating age: its own, else its top ancestor's.
SELECT wl.library_id, w.kind,
  coalesce(w.content_rating_age, (SELECT r.content_rating_age FROM work r WHERE r.id = w.root_id)) AS content_rating_age
FROM work_library wl
JOIN work w ON w.id = wl.work_id
WHERE wl.work_id = @work_id;

-- name: VersionlessWorks :many
SELECT id FROM work w WHERE w.id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids))
  AND NOT EXISTS (SELECT 1 FROM media_version_work vw WHERE vw.work_id = w.id);

-- name: SetAddedAt :exec
UPDATE work SET added_at = @added_at WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));
