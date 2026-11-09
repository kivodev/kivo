-- userstate ------------------------------------------------------------------
DROP INDEX user_state_favorite;
ALTER TABLE user_state DROP COLUMN favorite;
ALTER TABLE user_state DROP COLUMN favorited_at;

-- users ----------------------------------------------------------------------
-- A list of works a user keeps. Every user has My List, built_in, which
-- can't be deleted. A name is unique per user, ignoring case.
CREATE TABLE list (
  id         BLOB PRIMARY KEY,
  user_id    BLOB NOT NULL REFERENCES user (id) ON DELETE CASCADE,
  name       TEXT NOT NULL COLLATE NOCASE,
  built_in   INTEGER NOT NULL DEFAULT 0 CHECK (built_in IN (0, 1)),
  created_at INTEGER NOT NULL,
  UNIQUE (user_id, name)
) STRICT;
CREATE UNIQUE INDEX list_built_in ON list (user_id) WHERE built_in;

INSERT INTO list (id, user_id, name, built_in, created_at)
SELECT unhex(printf('%012x', unixepoch('subsec') * 1000) || '7' || substr(hex(randomblob(2)), 2) || '8' ||
    substr(hex(randomblob(2)), 2) || hex(randomblob(6))),
  u.id, 'My List', 1, unixepoch('subsec') * 1000
FROM user u;

-- userstate ------------------------------------------------------------------
-- A work in a list, most recently added first.
CREATE TABLE list_work (
  list_id  BLOB NOT NULL REFERENCES list (id) ON DELETE CASCADE,
  work_id  BLOB NOT NULL REFERENCES work (id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  added_at INTEGER NOT NULL,
  PRIMARY KEY (list_id, work_id)
) STRICT, WITHOUT ROWID;
CREATE INDEX list_work_list ON list_work (list_id, added_at DESC);
CREATE INDEX list_work_work ON list_work (work_id);

-- users ----------------------------------------------------------------------
DELETE FROM user_home_row WHERE row_id = 'favorites';
