-- kivo:foreign_keys off
-- media ----------------------------------------------------------------------
-- A subtitle file next to the video has the source 'external', the name the
-- API gives it. media_track is rebuilt for its check; every row and rowid is
-- kept, and so are its index and the view that reads it.

DROP VIEW work_browse_keys;

-- A file's tracks. id is what clients choose a track by. An embedded track
-- is the container's stream stream_index, and keeps its id while a re-probe
-- finds the same type and codec there. A subtitle file next to the video
-- (external, rel_path relative to the library folder) or downloaded from
-- OpenSubtitles (rel_path relative to data/subtitles/) is keyed by its path,
-- so adding or removing one leaves every other track's id alone. codec is
-- ffprobe's name, for subtitle files too. info is the JSON probing wrote for
-- an embedded track (media.Track), NULL for the others.
CREATE TABLE media_track_new (
  id               BLOB PRIMARY KEY,
  file_id          BLOB NOT NULL REFERENCES media_file (id) ON DELETE CASCADE,
  source           TEXT NOT NULL CHECK (source IN ('embedded', 'external', 'downloaded')),
  stream_index     INTEGER,
  rel_path         TEXT,
  type             TEXT NOT NULL CHECK (type IN ('video', 'audio', 'subtitle')),
  codec            TEXT NOT NULL,
  language         TEXT,
  title            TEXT,
  is_default       INTEGER NOT NULL DEFAULT 0,
  forced           INTEGER NOT NULL DEFAULT 0,
  hearing_impaired INTEGER NOT NULL DEFAULT 0,
  info             TEXT,
  UNIQUE (file_id, stream_index),
  UNIQUE (file_id, source, rel_path),
  CHECK ((source = 'embedded') = (stream_index IS NOT NULL)),
  CHECK ((source = 'embedded') = (rel_path IS NULL)),
  CHECK (source = 'embedded' OR type = 'subtitle')
) STRICT;

INSERT INTO media_track_new (rowid, id, file_id, source, stream_index, rel_path, type, codec, language, title,
  is_default, forced, hearing_impaired, info)
SELECT rowid, id, file_id,
  CASE WHEN source IN ('embedded', 'downloaded') THEN source ELSE 'external' END,
  stream_index, rel_path, type, codec, language, title, is_default, forced, hearing_impaired, info
FROM media_track;

DROP TABLE media_track;

ALTER TABLE media_track_new RENAME TO media_track;

-- The files with an embedded ASS, SSA or PGS track, which subtitles'
-- Pending reads, without reading every file's tracks.
CREATE INDEX media_track_extracted ON media_track (file_id)
WHERE type = 'subtitle' AND lower(codec) IN ('ass', 'ssa', 'hdmv_pgs_subtitle', 'pgs', 'sup');

-- The browse keys, unchanged from the baseline.
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
  CAST(coalesce((w.community_votes * w.community_rating + 325.0) / (w.community_votes + 50) * 1000, -1) AS INTEGER) AS rating_sort,
  CAST(coalesce((SELECT p.sort_name FROM credit c JOIN person p ON p.id = c.person_id
    WHERE c.work_id = w.id AND c.role = 'author' ORDER BY c.ord LIMIT 1), '') AS TEXT) AS creator_sort
FROM work w;
