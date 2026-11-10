-- activity -------------------------------------------------------------------
-- A password change is recorded as passwordChanged. activity is rebuilt for
-- its check; every row and rowid is kept, and so are both its indexes.

-- What happened on the server, for admins: plays started and finished,
-- root works added, sign-ins and failed ones, users joining, passwords
-- changed, task runs ending and problems raised. Each row is written in the
-- transaction of the change it tells of, and kept 90 days. data is what the
-- row says, as things were then (names as well as IDs), as JSON. A run of
-- failed sign-ins for one user from one address is one row, its count and
-- time the latest.
CREATE TABLE activity_new (
  id   BLOB PRIMARY KEY,
  at   INTEGER NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('playStarted', 'playFinished', 'added', 'signedIn', 'signInFailed',
    'userJoined', 'passwordChanged', 'taskEnded', 'problemRaised')),
  data TEXT NOT NULL
) STRICT;

INSERT INTO activity_new (rowid, id, at, kind, data)
SELECT rowid, id, at, kind, data FROM activity;

DROP TABLE activity;

ALTER TABLE activity_new RENAME TO activity;

CREATE INDEX activity_at ON activity (at, id);
CREATE INDEX activity_kind ON activity (kind, at, id);
