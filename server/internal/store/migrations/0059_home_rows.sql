-- users ----------------------------------------------------------------------
-- Each account's Home layout: where each of its rows goes and whether it
-- shows. A Home is arranged once any row has a position. row_id is the row's
-- id: `continueWatching`, `continueListening`, `favorites` or
-- `recentlyAdded:<library>`.
CREATE TABLE user_home_row (
  user_id  BLOB NOT NULL REFERENCES user (id) ON DELETE CASCADE,
  row_id   TEXT NOT NULL,
  position INTEGER,
  shown    INTEGER NOT NULL CHECK (shown IN (0, 1)),
  PRIMARY KEY (user_id, row_id)
) STRICT, WITHOUT ROWID;

-- An unpinned library's Recently Added row is hidden.
INSERT INTO user_home_row (user_id, row_id, position, shown)
SELECT ul.user_id,
  'recentlyAdded:' || lower(substr(hex(ul.library_id), 1, 8) || '-' || substr(hex(ul.library_id), 9, 4) || '-' ||
    substr(hex(ul.library_id), 13, 4) || '-' || substr(hex(ul.library_id), 17, 4) || '-' || substr(hex(ul.library_id), 21)),
  NULL, 0
FROM user_library ul
WHERE NOT ul.pinned;

-- Continue Listening is hidden for a user with audiobooks libraries, every one
-- of them unpinned.
INSERT INTO user_home_row (user_id, row_id, position, shown)
SELECT u.id, 'continueListening', NULL, 0
FROM user u
WHERE EXISTS (
    SELECT 1 FROM library l
    WHERE l.kind = 'audiobooks' AND l.deleting_at IS NULL
      AND (u.all_libraries OR l.id IN (SELECT g.library_id FROM library_grant g WHERE g.user_id = u.id)))
  AND NOT EXISTS (
    SELECT 1 FROM library l
    LEFT JOIN user_library ul ON ul.user_id = u.id AND ul.library_id = l.id
    WHERE l.kind = 'audiobooks' AND l.deleting_at IS NULL
      AND (u.all_libraries OR l.id IN (SELECT g.library_id FROM library_grant g WHERE g.user_id = u.id))
      AND coalesce(ul.pinned, 1));
