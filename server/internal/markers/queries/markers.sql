-- Skip markers. markers writes media_marker and marker_file; the rest
-- are reads of catalog's and userstate's tables.

-- name: Season :one
-- A season and its series' original language.
SELECT s.number AS season_number, CAST(coalesce(r.original_language, '') AS TEXT) AS original_language
FROM work s JOIN work r ON r.id = s.root_id
WHERE s.id = @id AND s.kind = 'season';

-- name: SeasonFiles :many
-- The present, probed files of a season's episodes, with each episode's
-- number, the file's place in its version, how many parts that version has
-- and how many works it covers, and the file's library, by episode.
SELECT e.id AS episode_id, e.number AS episode_number, f.id, f.generation, f.part,
  (SELECT count(*) FROM media_file p WHERE p.version_id = f.version_id) AS parts,
  (SELECT count(*) FROM media_version_work c WHERE c.version_id = f.version_id) AS works,
  r.library_id
FROM work e
JOIN media_version_work vw ON vw.work_id = e.id
JOIN media_file f ON f.version_id = vw.version_id
JOIN library_folder r ON r.id = f.folder_id
WHERE e.parent_id = @season AND e.kind = 'episode' AND f.status = 'present' AND f.duration_ms IS NOT NULL
ORDER BY e.number, e.id, f.version_id, f.part;

-- name: EpisodeFiles :many
-- The present, probed files of episodes.
SELECT DISTINCT f.id
FROM media_version_work vw
JOIN work e ON e.id = vw.work_id AND e.kind = 'episode'
JOIN media_file f ON f.version_id = vw.version_id
WHERE vw.work_id IN (SELECT unhex(value) FROM (SELECT CAST(@episodes AS TEXT) AS episodes), json_each(episodes)) AND f.status = 'present' AND f.duration_ms > 0
ORDER BY f.id;

-- name: Settled :one
-- Whether a file's markers are decided at its generation, by these rules,
-- or failed at it.
SELECT CAST(EXISTS (SELECT 1 FROM marker_file x JOIN media_file f ON f.id = x.file_id AND f.generation = x.generation
  WHERE x.file_id = @id AND (x.rules = @rules OR x.state = 'failed')) AS BOOLEAN);

-- name: FileSeason :one
-- A file's season, by its episode of lowest number, its series' original
-- language, its part of its version and how many parts that version has.
SELECT e.parent_id AS season_id, CAST(coalesce(r.original_language, '') AS TEXT) AS original_language,
  f.part, (SELECT count(*) FROM media_file p WHERE p.version_id = f.version_id) AS parts
FROM media_file f
JOIN media_version_work vw ON vw.version_id = f.version_id
JOIN work e ON e.id = vw.work_id AND e.kind = 'episode'
JOIN work r ON r.id = e.root_id
WHERE f.id = @id AND e.parent_id IS NOT NULL
ORDER BY e.number, e.id
LIMIT 1;

-- name: MarkerSeasons :many
-- The seasons, in libraries that find markers, with present, probed
-- files, and those files,
-- with whether their markers are decided at their generation by the rules
-- or failed at it, when their series was last played and when an episode
-- of theirs was last added.
SELECT s.id AS season_id, f.id,
  CAST(EXISTS (SELECT 1 FROM marker_file x WHERE x.file_id = f.id AND x.generation = f.generation
    AND (x.rules = @rules OR x.state = 'failed')) AS BOOLEAN) AS settled,
  CAST(coalesce((SELECT max(us.last_played_at) FROM work p JOIN user_state us ON us.work_id = p.id
    WHERE p.root_id = s.root_id), 0) AS INTEGER) AS played_at,
  CAST(coalesce((SELECT max(p.added_at) FROM work p WHERE p.parent_id = s.id), 0) AS INTEGER) AS added_at
FROM library l
JOIN library_folder r ON r.library_id = l.id
JOIN media_file f ON f.folder_id = r.id AND f.status = 'present' AND f.duration_ms > 0
JOIN media_version_work vw ON vw.version_id = f.version_id
JOIN work e ON e.id = vw.work_id AND e.kind = 'episode'
JOIN work s ON s.id = e.parent_id
WHERE l.markers <> 'never' AND l.kind = 'shows' AND l.deleting_at IS NULL
ORDER BY s.id, f.id;

-- name: CountPending :one
-- How many episode files of libraries that find markers aren't settled:
-- what MarkerSeasons lists for every library, less the settled ones.
SELECT count(DISTINCT f.id) FROM library l
JOIN library_folder r ON r.library_id = l.id
JOIN media_file f ON f.folder_id = r.id AND f.status = 'present' AND f.duration_ms > 0
JOIN media_version_work vw ON vw.version_id = f.version_id
JOIN work e ON e.id = vw.work_id AND e.kind = 'episode' AND e.parent_id IS NOT NULL
LEFT JOIN marker_file x ON x.file_id = f.id AND x.generation = f.generation
  AND (x.rules = @rules OR x.state = 'failed')
WHERE l.markers <> 'never' AND l.kind = 'shows' AND l.deleting_at IS NULL AND x.file_id IS NULL;

-- name: DeleteFoundMarkers :exec
-- A file's markers found by detection, keeping an admin's.
DELETE FROM media_marker WHERE file_id = @file_id AND source <> 'manual';

-- name: InsertMarker :exec
INSERT INTO media_marker (file_id, generation, kind, source, start_ms, end_ms, final, reason)
VALUES (@file_id, @generation, @kind, @source, @start_ms, @end_ms, @final, @reason);

-- name: SetDecided :exec
-- What detection decided for a file: at its generation, by the rules, and
-- whether it found markers.
INSERT INTO marker_file (file_id, generation, rules, state, updated_at)
VALUES (@file_id, @generation, @rules, @state, @updated_at)
ON CONFLICT (file_id) DO UPDATE SET generation = excluded.generation, rules = excluded.rules,
  state = excluded.state, reason = NULL, updated_at = excluded.updated_at;

-- name: SetFailed :exec
-- A file whose fingerprint failed for good at its generation, and why.
INSERT INTO marker_file (file_id, generation, rules, state, reason, updated_at)
VALUES (@file_id, @generation, @rules, 'failed', @reason, @updated_at)
ON CONFLICT (file_id) DO UPDATE SET generation = excluded.generation, rules = excluded.rules,
  state = 'failed', reason = excluded.reason, updated_at = excluded.updated_at;

-- name: Failed :many
-- The files whose fingerprint failed at their generation, newest first.
SELECT x.file_id, x.reason, x.updated_at FROM marker_file x
JOIN media_file f ON f.id = x.file_id AND f.generation = x.generation
WHERE x.state = 'failed'
ORDER BY x.updated_at DESC;

-- name: Forget :many
-- Forgets the failures of the given files, or of every file for none.
DELETE FROM marker_file
WHERE state = 'failed' AND (CAST(@all AS BOOLEAN) OR file_id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids)))
RETURNING file_id;

-- name: PruneMarkers :execrows
-- Markers of files at an earlier generation than they are now.
DELETE FROM media_marker
WHERE file_id IN (
    SELECT m.file_id FROM media_marker m JOIN media_file f ON f.id = m.file_id
    WHERE m.generation <> f.generation LIMIT @limit)
  AND generation <> (SELECT f.generation FROM media_file f WHERE f.id = media_marker.file_id);

-- name: PruneDecided :execrows
-- What detection decided for files at an earlier generation than they are
-- now.
DELETE FROM marker_file
WHERE file_id IN (
    SELECT m.file_id FROM marker_file m JOIN media_file f ON f.id = m.file_id
    WHERE m.generation <> f.generation LIMIT @limit);
-- name: StoredMarkers :many
-- The markers found or set for files, at the generation each file is now.
SELECT m.file_id, m.kind, m.source, m.start_ms, m.end_ms, m.final
FROM media_marker m JOIN media_file f ON f.id = m.file_id AND f.generation = m.generation
WHERE m.file_id IN (SELECT unhex(value) FROM (SELECT CAST(@files AS TEXT) AS files), json_each(files));

-- name: Wanted :one
-- Whether any shows library finds markers.
SELECT CAST(EXISTS (SELECT 1 FROM library WHERE markers <> 'never' AND kind = 'shows' AND deleting_at IS NULL) AS BOOLEAN);
