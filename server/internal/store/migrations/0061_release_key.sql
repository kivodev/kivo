-- catalog --------------------------------------------------------------------
-- A work's release date as its earliest day, YYYY-MM-DD, or '' when it has
-- none: a partial date counts from its start, and a work with only a year
-- from that year. The grid's release sort reads it; a series takes its first
-- air date.
DROP VIEW work_browse_keys;
CREATE VIEW work_browse_keys AS
SELECT w.id AS work_id,
  coalesce(w.year, 0) AS year,
  CAST(coalesce((SELECT nullif(sum(f.duration_ms), 0) FROM media_file f
    WHERE f.version_id = (SELECT min(vw.version_id) FROM media_version_work vw WHERE vw.work_id = w.id
      AND NOT EXISTS (SELECT 1 FROM media_file g WHERE g.version_id = vw.version_id AND g.status = 'gone'))),
    9223372036854775807) AS INTEGER) AS runtime_sort,
  CAST(coalesce((SELECT
      max(v.width >= 3200) * 8 + max(v.width >= 1700 AND v.width < 3200) * 4
      + max(v.width >= 1200 AND v.width < 1700) * 2 + max(v.width < 1200)
    FROM (SELECT coalesce(json_extract(t.info, '$.width'), 0) AS width
      FROM media_version_work vw
      JOIN media_file p ON p.version_id = vw.version_id AND p.part = 1
      JOIN media_track t ON t.file_id = p.id AND t.type = 'video'
      WHERE vw.work_id = w.id AND NOT coalesce(json_extract(t.info, '$.attached_picture'), 0)
        AND NOT EXISTS (SELECT 1 FROM media_file g WHERE g.version_id = vw.version_id AND g.status = 'gone')) v), 0) AS INTEGER) AS resolutions,
  CAST(coalesce((SELECT
      max(json_extract(t.info, '$.range') IN ('hdr10', 'hlg'))
      + max(coalesce(json_extract(t.info, '$.dolby_vision_profile'), 0) > 0) * 2
    FROM media_version_work vw
    JOIN media_file p ON p.version_id = vw.version_id AND p.part = 1
    JOIN media_track t ON t.file_id = p.id AND t.type = 'video'
    WHERE vw.work_id = w.id AND NOT coalesce(json_extract(t.info, '$.attached_picture'), 0)
      AND NOT EXISTS (SELECT 1 FROM media_file g WHERE g.version_id = vw.version_id AND g.status = 'gone')), 0) AS INTEGER) AS dynamic_ranges,
  CAST(CASE WHEN w.release_date IS NOT NULL THEN substr(w.release_date || '-01-01', 1, 10)
    WHEN w.year > 0 THEN printf('%04d-01-01', w.year) ELSE '' END AS TEXT) AS release_key,
  CAST(coalesce((w.community_votes * w.community_rating + 325.0) / (w.community_votes + 50) * 1000, -1) AS INTEGER) AS rating_sort,
  CAST(coalesce((SELECT p.sort_name FROM credit c JOIN person p ON p.id = c.person_id
    WHERE c.work_id = w.id AND c.role = 'author' ORDER BY c.ord LIMIT 1), '') AS TEXT) AS creator_sort
FROM work w;

ALTER TABLE work_library ADD COLUMN release_key TEXT NOT NULL DEFAULT '';
UPDATE work_library SET release_key = (
  SELECT CASE WHEN w.release_date IS NOT NULL THEN substr(w.release_date || '-01-01', 1, 10)
    WHEN w.year > 0 THEN printf('%04d-01-01', w.year) ELSE '' END
  FROM work w WHERE w.id = work_library.work_id);
DROP INDEX work_library_release;
CREATE INDEX work_library_release ON work_library (library_id, release_key DESC, sort_title, work_id) WHERE is_root;
