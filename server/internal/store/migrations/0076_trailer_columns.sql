-- catalog --------------------------------------------------------------------
-- A trailer is a YouTube video, named by its key. Its language, official
-- flag and publish date only ranked it, before ord was written.
DROP INDEX trailer_work;
ALTER TABLE trailer DROP COLUMN site;
ALTER TABLE trailer DROP COLUMN language;
ALTER TABLE trailer DROP COLUMN official;
ALTER TABLE trailer DROP COLUMN published_at;
CREATE UNIQUE INDEX trailer_work ON trailer (work_id, key);

