-- CheckInvariants: each query returns the rows that break one rule.

-- name: VersionsWithoutParts :many
SELECT v.id FROM media_version v WHERE NOT EXISTS (SELECT 1 FROM media_file p WHERE p.version_id = v.id);

-- name: VersionsWithoutWorks :many
SELECT v.id FROM media_version v WHERE NOT EXISTS (SELECT 1 FROM media_version_work w WHERE w.version_id = v.id);

-- name: VersionsAcrossLibraries :many
SELECT f.version_id FROM media_file f JOIN library_folder r ON r.id = f.folder_id
WHERE f.version_id IS NOT NULL
GROUP BY f.version_id HAVING count(DISTINCT r.library_id) > 1;

-- name: VersionsOutsideTheirRootsLibrary :many
-- A version whose files are all identified has them in its root work's
-- library. A file not identified at its generation, as a move leaves it,
-- waits to be placed.
SELECT DISTINCT f.version_id FROM media_file f
JOIN library_folder r ON r.id = f.folder_id
JOIN media_version_work vw ON vw.version_id = f.version_id
JOIN work w ON w.id = vw.work_id
JOIN work t ON t.id = w.root_id
WHERE t.library_id IS NOT r.library_id
  AND NOT EXISTS (SELECT 1 FROM media_file p WHERE p.version_id = f.version_id AND p.identified IS NOT p.generation);

-- name: RootsOutsideTheirFoldersLibrary :many
-- A root work is in its folder's library.
SELECT w.id FROM work_folder l JOIN work w ON w.id = l.work_id WHERE w.library_id <> l.library_id;

-- name: LibrariesOfNonRoots :many
-- Only a work with no parent has a library.
SELECT id FROM work WHERE parent_id IS NOT NULL AND library_id IS NOT NULL;

-- name: FoldersOfNonRoots :many
-- A folder is a root work's: a movie, series, video or book.
SELECT l.work_id FROM work_folder l JOIN work w ON w.id = l.work_id
WHERE w.parent_id IS NOT NULL;

-- name: VersionsWithGappedParts :many
SELECT version_id FROM media_file WHERE version_id IS NOT NULL
GROUP BY version_id HAVING min(part) <> 1 OR max(part) <> count(*);

-- name: WorksWithWrongRoot :many
SELECT c.id FROM work c LEFT JOIN work p ON p.id = c.parent_id
WHERE (c.parent_id IS NULL AND c.root_id <> c.id) OR (c.parent_id IS NOT NULL AND c.root_id <> p.root_id);

-- name: WorksWithWrongParent :many
SELECT c.id FROM work c JOIN work p ON p.id = c.parent_id
WHERE NOT ((c.kind = 'season' AND p.kind = 'series')
  OR (c.kind = 'episode' AND p.kind = 'season')
  OR (c.kind = 'track' AND p.kind = 'album')
  OR (c.kind = 'extra' AND p.kind IN ('movie', 'series', 'season')));

-- name: WorksWithoutNumbers :many
-- A season has its number (0 for specials) and an episode its season's; an
-- episode's own number may be missing, for one known by its air date.
SELECT id FROM work WHERE (kind = 'season' AND number IS NULL) OR (kind = 'episode' AND parent_number IS NULL);

-- name: EpisodesWithWrongSeasonNumber :many
SELECT e.id FROM work e JOIN work s ON s.id = e.parent_id
WHERE e.kind = 'episode' AND e.parent_number IS NOT s.number;

-- name: ExpectedWorkLibraries :many
WITH RECURSIVE avail (work_id, added_at) AS (
  SELECT vw.work_id, d.added_at FROM media_version_work vw
  JOIN work d ON d.id = vw.work_id
  WHERE NOT EXISTS (
    SELECT 1 FROM media_file f2 WHERE f2.version_id = vw.version_id AND f2.status = 'gone')
  UNION
  SELECT w.parent_id, a.added_at FROM avail a JOIN work w ON w.id = a.work_id
  WHERE w.parent_id IS NOT NULL AND w.kind <> 'extra'
)
SELECT a.work_id, t.library_id, w.parent_id IS NULL AS is_root, w.sort_title, w.added_at,
  k.year, k.runtime_sort, k.resolutions, k.dynamic_ranges, k.rating_sort, k.creator_sort,
  CAST(max(a.added_at) AS INTEGER) AS last_added_at
FROM avail a JOIN work w ON w.id = a.work_id JOIN work t ON t.id = w.root_id JOIN work_browse_keys k ON k.work_id = a.work_id
GROUP BY a.work_id
ORDER BY a.work_id;

-- name: ActualWorkLibraries :many
SELECT work_id, library_id, is_root, sort_title, added_at, year, runtime_sort, resolutions, dynamic_ranges, rating_sort, creator_sort, last_added_at FROM work_library ORDER BY work_id;
