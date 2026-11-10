-- playback -------------------------------------------------------------------
-- A download keeps what it delivers to its device (delivered) in place of
-- the plan the server made of it and its converted files. A stored plan
-- can't be turned into that here, so the downloads kept for devices are
-- dropped: a device asks again for one it's still waiting on or fetching,
-- and the nightly prune deletes the conversions left without a download.
DROP TABLE download;

-- A download a device asked for: the device session it's for, what it
-- asked, what it delivers, and where its preparation is. One that needs
-- work on the server (a conversion, or subtitles to prepare) is prepared
-- in the order asked (queued_at, then id). A conversion's files are in
-- cache/downloads/<id>/, kept until the device says it has them (the row is
-- deleted), its session ends, or 7 days after it settled, when the nightly
-- prune deletes both.
CREATE TABLE download (
  id         BLOB PRIMARY KEY,
  user_id    BLOB NOT NULL REFERENCES user (id) ON DELETE CASCADE,
  session_id BLOB NOT NULL REFERENCES session (id) ON DELETE CASCADE,
  work_id    BLOB NOT NULL,
  version_id BLOB NOT NULL,
  request    TEXT NOT NULL, -- JSON: what the device asked for; preparing it and Retry plan it again
  delivered  TEXT NOT NULL, -- JSON: the download as the device gets it: its parts and tracks, as planned, then as made
  state      TEXT NOT NULL CHECK (state IN ('queued', 'preparing', 'ready', 'failed')),
  reason     TEXT,          -- why it failed: a playback reason code
  created_at INTEGER NOT NULL,
  queued_at  INTEGER NOT NULL,
  settled_at INTEGER        -- when it became ready or failed
) STRICT;
CREATE INDEX download_user ON download (user_id);
CREATE INDEX download_session ON download (session_id, created_at);
CREATE INDEX download_queue ON download (queued_at, id) WHERE state IN ('queued', 'preparing');
CREATE INDEX download_settled ON download (settled_at) WHERE settled_at IS NOT NULL;
