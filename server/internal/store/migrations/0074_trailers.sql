-- kivo:foreign_keys off
-- catalog --------------------------------------------------------------------
-- A movie's trailers on a video site, from its metadata, in the order ord
-- gives. key is the site's ID of the video, which the link to it is made
-- from. published_at is as the provider gives it, RFC 3339.
CREATE TABLE trailer (
  id           BLOB PRIMARY KEY,
  work_id      BLOB NOT NULL REFERENCES work (id) ON DELETE CASCADE,
  ord          INTEGER NOT NULL,
  site         TEXT NOT NULL CHECK (site IN ('youtube')),
  key          TEXT NOT NULL,
  title        TEXT NOT NULL,
  language     TEXT, -- ISO 639-1
  official     INTEGER NOT NULL DEFAULT 0 CHECK (official IN (0, 1)),
  published_at TEXT
) STRICT;
CREATE UNIQUE INDEX trailer_work ON trailer (work_id, site, key);

-- Each movie's one trailer link becomes its first trailer. The movie is made
-- due a refresh, which gives the trailer its title and thumbnail and adds
-- the movie's other trailers.
INSERT INTO trailer (id, work_id, ord, site, key, title)
SELECT unhex(printf('%012x', unixepoch('subsec') * 1000) || '7' || substr(hex(randomblob(2)), 2) || '8' ||
    substr(hex(randomblob(2)), 2) || hex(randomblob(6))),
  w.id, 0, 'youtube', substr(w.trailer_url, 33), 'Trailer'
FROM work w
WHERE w.trailer_url LIKE 'https://www.youtube.com/watch?v=_%';

UPDATE work SET refreshed_at = NULL, refresh_error = NULL
WHERE trailer_url LIKE 'https://www.youtube.com/watch?v=_%';

ALTER TABLE work DROP COLUMN trailer_url;

-- metadata -------------------------------------------------------------------
-- A trailer owns its thumbnail, as a work, person or collection owns its
-- artwork. image is rebuilt for its owner check; every row is kept.
CREATE TABLE image_new (
  id            BLOB PRIMARY KEY,
  work_id       BLOB REFERENCES work (id) ON DELETE CASCADE,
  person_id     BLOB REFERENCES person (id) ON DELETE CASCADE,
  collection_id BLOB REFERENCES collection (id) ON DELETE CASCADE,
  trailer_id    BLOB REFERENCES trailer (id) ON DELETE CASCADE,
  kind          TEXT NOT NULL CHECK (kind IN ('poster', 'backdrop', 'logo', 'banner', 'thumb', 'still', 'photo')),
  source        TEXT NOT NULL CHECK (source IN ('provider', 'local', 'upload')),
  origin        TEXT NOT NULL,
  origin_mtime  INTEGER, -- a local file's mtime, to notice it changing
  width         INTEGER NOT NULL,
  height        INTEGER NOT NULL,
  blurhash      TEXT NOT NULL,
  chosen        INTEGER NOT NULL DEFAULT 0 CHECK (chosen IN (0, 1)), -- an admin chose or uploaded it; refreshes keep it
  fetched_at    INTEGER NOT NULL,
  CHECK ((work_id IS NOT NULL) + (person_id IS NOT NULL) + (collection_id IS NOT NULL) + (trailer_id IS NOT NULL) = 1)
) STRICT;

INSERT INTO image_new (id, work_id, person_id, collection_id, kind, source, origin, origin_mtime, width, height, blurhash, chosen, fetched_at)
SELECT id, work_id, person_id, collection_id, kind, source, origin, origin_mtime, width, height, blurhash, chosen, fetched_at
FROM image;

DROP TABLE image;

ALTER TABLE image_new RENAME TO image;

CREATE UNIQUE INDEX image_work ON image (work_id, kind) WHERE work_id IS NOT NULL;
CREATE UNIQUE INDEX image_person ON image (person_id, kind) WHERE person_id IS NOT NULL;
CREATE UNIQUE INDEX image_collection ON image (collection_id, kind) WHERE collection_id IS NOT NULL;
CREATE UNIQUE INDEX image_trailer ON image (trailer_id, kind) WHERE trailer_id IS NOT NULL;
