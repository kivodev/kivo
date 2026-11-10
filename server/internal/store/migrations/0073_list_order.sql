-- users ----------------------------------------------------------------------
-- The order a manual list is shown in: the user's own, 'custom', or by one
-- of its works' fields. A smart list's rules order it, so its sort is unused.
ALTER TABLE list ADD COLUMN sort TEXT NOT NULL DEFAULT 'custom'
  CHECK (sort IN ('custom', 'title', 'releaseDate', 'rating', 'dateAdded'));

-- userstate ------------------------------------------------------------------
-- list_work is rebuilt with each work's place. Every list starts in the
-- order it was shown in: most recently added first, then by title.

-- A work in a list. position is its place in the list's custom order,
-- lowest first. Places are spaced 2^32 apart, so a work goes to the top, or
-- between two others, without the rest moving.
CREATE TABLE list_work_new (
  list_id  BLOB NOT NULL REFERENCES list (id) ON DELETE CASCADE,
  work_id  BLOB NOT NULL REFERENCES work (id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  added_at INTEGER NOT NULL,
  position INTEGER NOT NULL,
  PRIMARY KEY (list_id, work_id)
) STRICT, WITHOUT ROWID;

INSERT INTO list_work_new (list_id, work_id, added_at, position)
SELECT lw.list_id, lw.work_id, lw.added_at,
  4294967296 * row_number() OVER (PARTITION BY lw.list_id ORDER BY lw.added_at DESC, w.sort_title, w.id)
FROM list_work lw
JOIN work w ON w.id = lw.work_id;

DROP TABLE list_work;

ALTER TABLE list_work_new RENAME TO list_work;

CREATE INDEX list_work_position ON list_work (list_id, position);
CREATE INDEX list_work_work ON list_work (work_id);
