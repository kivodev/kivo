-- catalog --------------------------------------------------------------------
-- The titles TMDb recommends for a movie or series, best first. ext_id is
-- TMDb's ID of a title of the work's own kind, which may not be in any
-- library yet, so one added later matches at once.
CREATE TABLE work_recommendation (
  work_id BLOB NOT NULL REFERENCES work (id) ON DELETE CASCADE,
  ord     INTEGER NOT NULL,
  ext_id  TEXT NOT NULL,
  PRIMARY KEY (work_id, ord)
) STRICT, WITHOUT ROWID;

-- A work's similar titles are found from its genres, tags and companies.
CREATE INDEX work_genre_genre ON work_genre (genre_id, work_id);
CREATE INDEX work_tag_tag ON work_tag (tag_id, work_id);
CREATE INDEX work_company_company ON work_company (company_id, work_id);

-- metadata -------------------------------------------------------------------
-- Matched movies and series are made due a refresh, which fills in their
-- recommendations.
UPDATE work SET refreshed_at = NULL, refresh_error = NULL
WHERE kind IN ('movie', 'series') AND NOT provisional
  AND EXISTS (SELECT 1 FROM work_external_id x WHERE x.work_id = work.id AND x.provider = 'tmdb');
