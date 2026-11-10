-- The schema, grouped by the component that owns each table, as
-- archtest's owns map enforces. IDs are 16-byte blobs and times are Unix
-- milliseconds unless a column says otherwise. A new database starts with
-- every table empty.

-- config ---------------------------------------------------------------------

-- Settings an admin changed; defaults live in code.
CREATE TABLE setting (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL -- JSON
) STRICT, WITHOUT ROWID;

-- users ----------------------------------------------------------------------

-- An account: its access, its limits, which playback enforces, and its
-- playback preferences. The languages and subtitle mode choose tracks, and
-- remember_tracks keeps the tracks last chosen for each series. book_speed
-- is the speed it listens to books at, chosen in the audio player, and
-- autoplay_next whether up next plays its next episode by itself; both are
-- kept for every device signed in to it.
CREATE TABLE user (
  id                 BLOB PRIMARY KEY,
  username           TEXT NOT NULL COLLATE NOCASE UNIQUE,
  all_libraries      INTEGER NOT NULL DEFAULT 0 CHECK (all_libraries IN (0, 1)), -- sees every library, present and future
  stream_limit       INTEGER, -- simultaneous streams; NULL is unlimited
  remote_quality_cap INTEGER, -- bits per second for remote sessions; NULL is no cap
  downloads_allowed  INTEGER NOT NULL DEFAULT 1 CHECK (downloads_allowed IN (0, 1)),
  disabled           INTEGER NOT NULL DEFAULT 0 CHECK (disabled IN (0, 1)),
  invited_by         BLOB REFERENCES user (id) ON DELETE SET NULL,
  created_at         INTEGER NOT NULL,
  admin              INTEGER NOT NULL DEFAULT 0 CHECK (admin IN (0, 1)),
  rating_ceiling     INTEGER, -- normalized age rank; NULL is no ceiling
  block_unrated      INTEGER NOT NULL DEFAULT 0 CHECK (block_unrated IN (0, 1)), -- hides works with no rating
  audio_languages    TEXT NOT NULL DEFAULT '[]', -- JSON array of BCP 47 tags, most preferred first
  subtitle_languages TEXT NOT NULL DEFAULT '[]', -- likewise
  subtitle_mode      TEXT NOT NULL DEFAULT 'forced' CHECK (subtitle_mode IN ('none', 'forced', 'smart', 'always')),
  remember_tracks    INTEGER NOT NULL DEFAULT 1 CHECK (remember_tracks IN (0, 1)),
  book_speed         REAL NOT NULL DEFAULT 1 CHECK (book_speed BETWEEN 0.5 AND 3),
  autoplay_next      INTEGER NOT NULL DEFAULT 1 CHECK (autoplay_next IN (0, 1))
) STRICT;

-- A library an account may see, unless it sees all of them.
CREATE TABLE library_grant (
  user_id    BLOB NOT NULL REFERENCES user (id) ON DELETE CASCADE,
  library_id BLOB NOT NULL REFERENCES library (id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, library_id)
) STRICT, WITHOUT ROWID;

-- Each account's own order of the libraries it sees, and which of them are
-- pinned, which the sidebar and Home show in that order. A library with no
-- row is pinned and comes after those with rows, in the server's order: an
-- account starts with every library it sees pinned, and a library added or
-- granted later joins the end. Losing a grant takes the library's row with
-- it.
CREATE TABLE user_library (
  user_id    BLOB NOT NULL REFERENCES user (id) ON DELETE CASCADE,
  library_id BLOB NOT NULL REFERENCES library (id) ON DELETE CASCADE,
  position   INTEGER NOT NULL,
  pinned     INTEGER NOT NULL CHECK (pinned IN (0, 1)),
  PRIMARY KEY (user_id, library_id)
) STRICT, WITHOUT ROWID;

-- auth -----------------------------------------------------------------------

-- An account's password is its one credential.
CREATE TABLE credential (
  id         BLOB PRIMARY KEY,
  user_id    BLOB NOT NULL REFERENCES user (id) ON DELETE CASCADE,
  kind       TEXT NOT NULL CHECK (kind IN ('password')),
  secret     TEXT NOT NULL, -- argon2id PHC string
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX credential_user ON credential (user_id, kind);

-- A signed-in browser or app. The token itself is never stored.
--   client_id  the random ID the client keeps: a user signing in again from
--              the same client replaces that client's session; NULL for a
--              session made without one
--   platform   what the client runs on, such as androidTv or browser; empty
--              when the client gave none. Signing a device in by code takes
--              its device code's.
--   probe_id   the first 16 bytes of SHA-256("kivo-probe-id" ‖ token_hash).
--              An app sends it with the unauthenticated server probe, so the
--              server can prove, keyed by the session's token hash, that it's
--              the server the app signed in to, without the app sending its
--              token. NULL for a session made without one.
CREATE TABLE session (
  id           BLOB PRIMARY KEY,
  token_hash   BLOB NOT NULL UNIQUE, -- SHA-256 of the token
  user_id      BLOB NOT NULL REFERENCES user (id) ON DELETE CASCADE,
  kind         TEXT NOT NULL CHECK (kind IN ('browser', 'app')),
  name         TEXT NOT NULL DEFAULT '', -- the client's name
  created_at   INTEGER NOT NULL,
  last_used_at INTEGER NOT NULL, -- written when used, at most a minute behind
  client_id    TEXT,
  platform     TEXT NOT NULL DEFAULT '',
  probe_id     BLOB
) STRICT;
CREATE INDEX session_user ON session (user_id);
CREATE INDEX session_browser_used ON session (last_used_at) WHERE kind = 'browser';
CREATE UNIQUE INDEX session_client ON session (user_id, client_id) WHERE client_id IS NOT NULL;
CREATE UNIQUE INDEX session_probe ON session (probe_id);

-- A key for scripts and other tools, with the admin access of the account
-- that created it. The token itself is never stored.
CREATE TABLE api_key (
  id           BLOB PRIMARY KEY,
  token_hash   BLOB NOT NULL UNIQUE, -- SHA-256 of the token
  name         TEXT NOT NULL,
  created_by   BLOB NOT NULL REFERENCES user (id) ON DELETE CASCADE,
  created_at   INTEGER NOT NULL,
  last_used_at INTEGER -- NULL until it's used, then at most a minute behind
) STRICT;
CREATE INDEX api_key_created_by ON api_key (created_by);

-- A cast token: the credential a signed-in session gives a Google Cast
-- receiver, so the receiver plays as that session's user. It reaches only
-- playback, ends when its session does, and expires 30 minutes after its
-- last use and 24 hours after it was made. The token itself is never
-- stored.
CREATE TABLE cast_token (
  id           BLOB PRIMARY KEY,
  token_hash   BLOB NOT NULL UNIQUE, -- SHA-256 of the token
  session_id   BLOB NOT NULL REFERENCES session (id) ON DELETE CASCADE,
  name         TEXT NOT NULL, -- the receiver's name, such as "Living Room TV"
  receiver     TEXT NOT NULL CHECK (receiver IN ('web', 'androidTv')),
  created_at   INTEGER NOT NULL,
  last_used_at INTEGER NOT NULL -- written when used, at most a minute behind
) STRICT;
CREATE INDEX cast_token_session ON cast_token (session_id);
CREATE INDEX cast_token_used ON cast_token (last_used_at);
CREATE INDEX cast_token_created ON cast_token (created_at);

-- Security-relevant events. No foreign keys: the log outlives what it names.
CREATE TABLE audit (
  id         BLOB PRIMARY KEY,
  at         INTEGER NOT NULL,
  action     TEXT NOT NULL,
  user_id    BLOB,
  session_id BLOB,
  remote_ip  TEXT NOT NULL DEFAULT '',
  api_key_id BLOB
) STRICT;
CREATE INDEX audit_at ON audit (at);

-- An invite link: whoever opens it before it expires chooses a username and
-- password, and the account gets exactly the access it carries. It works
-- once. The token itself is never stored.
CREATE TABLE invite (
  id                 BLOB PRIMARY KEY,
  token_hash         BLOB NOT NULL UNIQUE, -- SHA-256 of the token
  created_by         BLOB NOT NULL REFERENCES user (id) ON DELETE CASCADE,
  all_libraries      INTEGER NOT NULL CHECK (all_libraries IN (0, 1)),
  stream_limit       INTEGER, -- as on user
  remote_quality_cap INTEGER, -- as on user
  expires_at         INTEGER NOT NULL,
  used_at            INTEGER,
  used_by            BLOB REFERENCES user (id) ON DELETE SET NULL,
  created_at         INTEGER NOT NULL
) STRICT;

-- The libraries an invite grants, unless it grants all of them.
CREATE TABLE invite_library (
  invite_id  BLOB NOT NULL REFERENCES invite (id) ON DELETE CASCADE,
  library_id BLOB NOT NULL REFERENCES library (id) ON DELETE CASCADE,
  PRIMARY KEY (invite_id, library_id)
) STRICT, WITHOUT ROWID;

-- catalog --------------------------------------------------------------------
-- What exists: libraries and their folders; files with their tracks,
-- chapters and attachments; the versions files make up; works; people and
-- lookups; collections; books; and the search indexes.

-- A library. Removing a folder from it, or deleting it, purges the files a
-- batch at a time; meanwhile the library is marked (deleting_at, and a
-- folder's removing_at), with when it was asked for, and is hidden
-- everywhere at once. previews, markers and subtitles are when the library
-- makes preview thumbnails, finds its episodes' intro and credits markers,
-- and extracts its files' embedded subtitles ahead of a play: nightly, and
-- also as media is added, or never. refresh_days is how often its works
-- have their metadata refreshed, in days, or 0 for never.
CREATE TABLE library (
  id                     BLOB PRIMARY KEY,
  name                   TEXT NOT NULL,
  kind                   TEXT NOT NULL CHECK (kind IN ('movies', 'shows', 'videos', 'audiobooks', 'music')),
  use_local_metadata     INTEGER NOT NULL DEFAULT 1,
  previews               TEXT NOT NULL DEFAULT 'nightly' CHECK (previews IN ('never', 'nightly', 'nightly_and_added')),
  created_at             INTEGER NOT NULL,
  deleting_at            INTEGER,
  metadata_language      TEXT, -- BCP 47; NULL is the server's
  content_rating_country TEXT, -- ISO 3166-1 alpha-2; NULL is the server's
  markers                TEXT NOT NULL DEFAULT 'nightly_and_added'
    CHECK (markers IN ('never', 'nightly', 'nightly_and_added')),
  refresh_days           INTEGER NOT NULL DEFAULT 30 CHECK (refresh_days IN (0, 30, 60, 90)),
  subtitles              TEXT NOT NULL DEFAULT 'nightly_and_added'
    CHECK (subtitles IN ('never', 'nightly', 'nightly_and_added'))
) STRICT;

-- A library's folder on disk. reachable and last_scan_at are scan's.
CREATE TABLE library_folder (
  id           BLOB PRIMARY KEY,
  library_id   BLOB NOT NULL REFERENCES library (id),
  path         TEXT NOT NULL UNIQUE,
  reachable    INTEGER NOT NULL DEFAULT 1,
  last_scan_at INTEGER,
  removing_at  INTEGER
) STRICT;
CREATE INDEX library_folder_library ON library_folder (library_id);

-- A file in a library folder. version_id and part are the version it's a
-- part of and its place there, numbered from 1; both are NULL while it's
-- unmatched, and catalog clears both before it deletes a version.
--   start_ms     its first timestamp, as probing reads it: zero for most
--                files, often not for MPEG-TS and M2TS. ffmpeg's copied
--                timestamps start there, so cues from an in-stream text
--                track are cut into segments from there.
--   tags         the tags of its container that books are described from,
--                as probing reads them: a JSON object of album, artist,
--                album_artist, composer, grouping, series, part, asin, date,
--                description, publisher, genre, track and disc, each present
--                only when the file has it. NULL until probed.
--   probed       the generation it was last probed at, and probe_error why
--                that probe failed for good (scan's)
--   identified   the generation its folder was last identified at
--                (identify's). A scan probes and identifies what's behind,
--                so a file whose probe or identification was cut off is done
--                by the next one.
CREATE TABLE media_file (
  id          BLOB PRIMARY KEY,
  folder_id   BLOB NOT NULL REFERENCES library_folder (id),
  rel_path    TEXT NOT NULL,
  size        INTEGER NOT NULL,
  mtime       INTEGER NOT NULL,
  edge_hash   BLOB,    -- first and last 64 KiB, for move detection
  os_hash     INTEGER, -- OpenSubtitles' file hash
  generation  INTEGER NOT NULL DEFAULT 1,
  status      TEXT NOT NULL CHECK (status IN ('present', 'gone')),
  gone_at     INTEGER,
  container   TEXT,
  duration_ms INTEGER,
  bitrate     INTEGER,
  version_id  BLOB REFERENCES media_version (id),
  part        INTEGER CHECK ((version_id IS NULL) = (part IS NULL)),
  start_ms    INTEGER NOT NULL DEFAULT 0,
  tags        TEXT,
  probed      INTEGER,
  probe_error TEXT,
  identified  INTEGER,
  UNIQUE (folder_id, rel_path)
) STRICT;
CREATE INDEX media_file_hash ON media_file (size, edge_hash) WHERE edge_hash IS NOT NULL;
CREATE UNIQUE INDEX media_file_version ON media_file (version_id, part) WHERE version_id IS NOT NULL;
-- The Missing files list pages newest missing first by (gone_at, id): a
-- scan marks a whole batch gone at one time, so the ID breaks the ties in
-- the index rather than in a sort.
CREATE INDEX media_file_gone ON media_file (gone_at, id) WHERE status = 'gone';
-- The files a scan hasn't caught up with, and those whose probe failed.
CREATE INDEX media_file_behind ON media_file (folder_id)
  WHERE status = 'present' AND (probed IS NOT generation OR identified IS NOT generation);
CREATE INDEX media_file_probe_failed ON media_file (mtime) WHERE probe_error IS NOT NULL;

-- A file's tracks. id is what clients choose a track by. An embedded track
-- is the container's stream stream_index, and keeps its id while a re-probe
-- finds the same type and codec there. A subtitle file next to the video
-- (sidecar, rel_path relative to the library folder) or downloaded from
-- OpenSubtitles (rel_path relative to data/subtitles/) is keyed by its path,
-- so adding or removing one leaves every other track's id alone. codec is
-- ffprobe's name, for subtitle files too. info is the JSON probing wrote for
-- an embedded track (media.Track), NULL for the others.
CREATE TABLE media_track (
  id               BLOB PRIMARY KEY,
  file_id          BLOB NOT NULL REFERENCES media_file (id) ON DELETE CASCADE,
  source           TEXT NOT NULL CHECK (source IN ('embedded', 'sidecar', 'downloaded')),
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
-- The files with an embedded ASS, SSA or PGS track, which subtitles'
-- Pending reads, without reading every file's tracks.
CREATE INDEX media_track_extracted ON media_track (file_id)
WHERE type = 'subtitle' AND lower(codec) IN ('ass', 'ssa', 'hdmv_pgs_subtitle', 'pgs', 'sup');

CREATE TABLE media_chapter (
  file_id  BLOB NOT NULL REFERENCES media_file (id) ON DELETE CASCADE,
  idx      INTEGER NOT NULL,
  start_ms INTEGER NOT NULL,
  end_ms   INTEGER NOT NULL,
  title    TEXT,
  PRIMARY KEY (file_id, idx)
) STRICT, WITHOUT ROWID;

-- The files a container carries besides its tracks: fonts for styled
-- subtitles, cover images. Probing records every one, in the container's
-- order (idx), with the sha256 of its bytes as lowercase hex. kind is what
-- the bytes are (font, image or other), whatever mime_type, the type the
-- container declares, says.
CREATE TABLE media_attachment (
  file_id   BLOB NOT NULL REFERENCES media_file (id) ON DELETE CASCADE,
  idx       INTEGER NOT NULL,
  kind      TEXT NOT NULL CHECK (kind IN ('font', 'image', 'other')),
  filename  TEXT NOT NULL,
  mime_type TEXT,
  size      INTEGER NOT NULL,
  sha256    TEXT NOT NULL,
  PRIMARY KEY (file_id, idx)
) STRICT, WITHOUT ROWID;
CREATE INDEX media_attachment_sha256 ON media_attachment (sha256);

-- A playable copy of one or more works. Its parts are its files.
CREATE TABLE media_version (
  id         BLOB PRIMARY KEY,
  created_at INTEGER NOT NULL,
  edition    TEXT -- the cut its files' edition tag names, such as "Final Cut"
) STRICT, WITHOUT ROWID;

-- The works a version covers, in order.
CREATE TABLE media_version_work (
  version_id BLOB NOT NULL REFERENCES media_version (id) ON DELETE CASCADE,
  work_id    BLOB NOT NULL REFERENCES work (id),
  ord        INTEGER NOT NULL,
  PRIMARY KEY (version_id, work_id)
) STRICT, WITHOUT ROWID;
CREATE INDEX media_version_work_work ON media_version_work (work_id);

-- What you watch, read or listen to. Where a field goes: work holds what
-- the shared machinery reads and what any kind can have; a kind's side
-- table (book) what only that kind's pages read; a join table anything with
-- several values. No column means something else for another kind.
--   number             its place among its siblings: a season's number, an
--                      episode's in its season, a track's on its disc
--   parent_number      the group above it: an episode's season, a track's
--                      disc
--   release_date       YYYY, YYYY-MM or YYYY-MM-DD: a movie's or album's
--                      release, an episode's air date, a book's publication
--   content_rating     the content rating for the content rating country as
--                      the provider gives it ("PG-13", "TV-MA"), beside the
--                      minimum age it means (content_rating_age); metadata
--                      writes both, or neither when the work is unrated
--   community_rating   TMDb's average vote (0 to 10) and community_votes how
--                      many votes it averages, both NULL with no votes
--   original_language  the language it was made in, as the provider gives it
--                      ("en", "ja"); a season's and episode's are their
--                      series'. Clients mark the audio in it as the original.
--   refreshed_at       when its metadata was last refreshed, and
--                      refresh_error why that refresh failed (metadata's). A
--                      failed refresh still sets refreshed_at, so it waits
--                      out its library's refresh_days like any other, unless
--                      it's retried.
--   library_id         a root work's library, its folder's, which it takes
--                      with its first version; NULL on every other work
CREATE TABLE work (
  rid                INTEGER PRIMARY KEY, -- the search index's rowid only
  id                 BLOB NOT NULL UNIQUE,
  kind               TEXT NOT NULL CHECK (kind IN ('movie', 'series', 'season', 'episode', 'extra', 'video', 'book', 'album', 'track')),
  parent_id          BLOB REFERENCES work (id),
  root_id            BLOB NOT NULL, -- the top ancestor (itself for a movie, series, video, book or album)
  number             INTEGER,
  parent_number      INTEGER,
  title              TEXT NOT NULL,
  sort_title         TEXT NOT NULL,
  original_title     TEXT,
  alt_titles         TEXT, -- JSON array, searched
  year               INTEGER,
  release_date       TEXT,
  overview           TEXT,
  content_rating     TEXT,
  content_rating_age INTEGER,
  community_rating   REAL,
  community_votes    INTEGER,
  ordering           TEXT, -- series: aired, dvd, absolute or an episode group ID
  extra_type         TEXT, -- extra: trailer, featurette, ...
  edition            TEXT, -- movie: the label its folder's edition tag gives it
  provisional        INTEGER NOT NULL DEFAULT 0,
  locked             INTEGER NOT NULL DEFAULT 0, -- a manual match; a root work's covers the works under it
  match_rev          INTEGER NOT NULL DEFAULT 0,
  locked_fields      TEXT, -- JSON array of fields an admin edited
  added_at           INTEGER NOT NULL,
  original_language  TEXT,
  refreshed_at       INTEGER,
  refresh_error      TEXT,
  library_id         BLOB REFERENCES library (id) ON DELETE SET NULL,
  CHECK ((kind IN ('movie', 'series', 'video', 'book', 'album')) = (parent_id IS NULL)),
  CHECK (parent_number IS NULL OR kind IN ('episode', 'track')),
  CHECK (ordering IS NULL OR kind = 'series'),
  CHECK (extra_type IS NULL OR kind = 'extra'),
  CHECK (edition IS NULL OR kind = 'movie')
) STRICT;
-- A parent's children in order, or one by its number.
CREATE INDEX work_children ON work (parent_id, number);
-- A series' episodes in order, for Next Up, unplayed counts and the
-- series' last played time.
CREATE INDEX work_root ON work (root_id, kind, parent_number, number);
-- Search's title-prefix fallback, a case-insensitive range.
CREATE INDEX work_sort_title ON work (sort_title COLLATE NOCASE);
CREATE INDEX work_refresh_failed ON work (refreshed_at) WHERE refresh_error IS NOT NULL;

-- An external ID describes a root work and may describe several: the key
-- holds the work, and lookups by ID read the index.
CREATE TABLE work_external_id (
  work_id  BLOB NOT NULL REFERENCES work (id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  kind     TEXT NOT NULL,
  ext_id   TEXT NOT NULL,
  PRIMARY KEY (work_id, provider, kind, ext_id)
) STRICT, WITHOUT ROWID;
CREATE INDEX work_external_id_ext ON work_external_id (provider, kind, ext_id);

-- The library each work is available in, its root work's: a version there
-- with every part present, on the work or a descendant. Rebuilt at every
-- write that changes it, with copies of the work's sort title and browse
-- keys (work_browse_keys below), so a sort or a video filter reads one
-- index. last_added_at is the Last episode added sort's key: the newest
-- added_at among the work and the descendants whose versions make it
-- available (extras don't count for their parent), so a series' is its
-- newest episode's and a movie's its own.
CREATE TABLE work_library (
  library_id     BLOB NOT NULL REFERENCES library (id),
  work_id        BLOB NOT NULL REFERENCES work (id) ON DELETE CASCADE,
  is_root        INTEGER NOT NULL,
  sort_title     TEXT NOT NULL,
  added_at       INTEGER NOT NULL,
  year           INTEGER NOT NULL DEFAULT 0,
  runtime_sort   INTEGER NOT NULL DEFAULT 9223372036854775807,
  resolutions    INTEGER NOT NULL DEFAULT 0,
  dynamic_ranges INTEGER NOT NULL DEFAULT 0,
  rating_sort    INTEGER NOT NULL DEFAULT -1,
  creator_sort   TEXT NOT NULL DEFAULT '',
  last_added_at  INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (library_id, work_id)
) STRICT, WITHOUT ROWID;
-- The grids' sorts, each breaking ties by title and then by ID, read in the
-- index's order.
CREATE INDEX work_library_title ON work_library (library_id, sort_title, work_id) WHERE is_root;
CREATE INDEX work_library_release ON work_library (library_id, year DESC, sort_title, work_id) WHERE is_root;
CREATE INDEX work_library_runtime ON work_library (library_id, runtime_sort, sort_title, work_id) WHERE is_root;
CREATE INDEX work_library_rating ON work_library (library_id, rating_sort DESC, sort_title, work_id) WHERE is_root;
-- Recently Added, which collapses new episodes into their series, so it
-- reads every row of a library rather than only the top-level ones.
CREATE INDEX work_library_recent ON work_library (library_id, added_at DESC);
CREATE INDEX work_library_creator ON work_library (library_id, creator_sort, sort_title, work_id) WHERE is_root;
CREATE INDEX work_library_last_added ON work_library (library_id, last_added_at DESC, sort_title, work_id) WHERE is_root;
CREATE INDEX work_library_added ON work_library (library_id, added_at DESC, sort_title, work_id) WHERE is_root;
CREATE UNIQUE INDEX work_library_work ON work_library (work_id);

-- A work's browse keys, which catalog copies into work_library. Each comes
-- from the versions with no gone part.
--   year            the release year, 0 when unknown
--   runtime_sort    the first version's running time in ms, or the largest
--                   integer when it has none, so those sort last
--   resolutions     a bit per class among its versions' video: 1 sd, 2 720p,
--                   4 1080p, 8 4k
--   dynamic_ranges  1 for HDR10 or HLG, 2 for Dolby Vision
--   rating_sort     the community rating weighted toward a prior of 6.5 by
--                   50 votes, (votes * rating + 50 * 6.5) / (votes + 50) *
--                   1000, truncated; -1 when there is no rating, so those
--                   sort last. query computes the same key from a card for
--                   its cursor.
--   creator_sort    the first author's sort name, '' with none
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

-- A person or a group (a band, an orchestra). People are told apart by
-- their external IDs, then by name, so two people may share a name.
CREATE TABLE person (
  rid       INTEGER PRIMARY KEY,
  id        BLOB NOT NULL UNIQUE,
  name      TEXT NOT NULL,
  sort_name TEXT NOT NULL,
  overview  TEXT -- biography
) STRICT;
CREATE INDEX person_name ON person (name);

-- An ID that identifies exactly one person: (tmdb, person, 17419), later an
-- author's ASIN or an artist's MusicBrainz ID.
CREATE TABLE person_external_id (
  provider  TEXT NOT NULL,
  kind      TEXT NOT NULL,
  ext_id    TEXT NOT NULL,
  person_id BLOB NOT NULL REFERENCES person (id) ON DELETE CASCADE,
  PRIMARY KEY (provider, kind, ext_id)
) STRICT, WITHOUT ROWID;
CREATE INDEX person_external_id_person ON person_external_id (person_id);

CREATE TABLE credit (
  work_id   BLOB NOT NULL REFERENCES work (id) ON DELETE CASCADE,
  person_id BLOB NOT NULL REFERENCES person (id),
  role      TEXT NOT NULL, -- free text: actor, director; later album artist, author, narrator
  ord       INTEGER NOT NULL,
  character TEXT,
  PRIMARY KEY (work_id, role, ord)
) STRICT, WITHOUT ROWID;
CREATE INDEX credit_person ON credit (person_id);

CREATE TABLE genre (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE) STRICT;
CREATE TABLE work_genre (
  work_id  BLOB NOT NULL REFERENCES work (id) ON DELETE CASCADE,
  genre_id INTEGER NOT NULL REFERENCES genre (id),
  PRIMARY KEY (work_id, genre_id)
) STRICT, WITHOUT ROWID;

CREATE TABLE tag (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE) STRICT;
CREATE TABLE work_tag (
  work_id BLOB NOT NULL REFERENCES work (id) ON DELETE CASCADE,
  tag_id  INTEGER NOT NULL REFERENCES tag (id),
  PRIMARY KEY (work_id, tag_id)
) STRICT, WITHOUT ROWID;

-- Companies: studios, networks, publishers and labels. A book's publisher
-- is one of its companies.
CREATE TABLE company (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE) STRICT;
CREATE TABLE work_company (
  work_id    BLOB NOT NULL REFERENCES work (id) ON DELETE CASCADE,
  company_id INTEGER NOT NULL REFERENCES company (id),
  PRIMARY KEY (work_id, company_id)
) STRICT, WITHOUT ROWID;

-- TMDb movie collections and the folders of a videos library. sort_name
-- follows catalog.SortTitle, as a work's sort title does.
CREATE TABLE collection (
  rid        INTEGER PRIMARY KEY, -- the search index's rowid only
  id         BLOB NOT NULL UNIQUE,
  key        TEXT NOT NULL UNIQUE, -- tmdb:<id>, or folder:<library id>/<path>
  name       TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  sort_name  TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE TABLE collection_work (
  collection_id BLOB NOT NULL REFERENCES collection (id) ON DELETE CASCADE,
  work_id       BLOB NOT NULL REFERENCES work (id) ON DELETE CASCADE,
  ord           INTEGER NOT NULL,
  PRIMARY KEY (collection_id, work_id)
) STRICT, WITHOUT ROWID;
CREATE INDEX collection_work_work ON collection_work (work_id);

-- Search matches words: unicode61 splits text at anything but letters and
-- digits and folds case and diacritics, and the prefix index keeps a
-- query's last word, a prefix, fast at one to three characters. The indexes
-- are contentless, so they store no text twice; catalog rewrites a row at
-- every write that changes what's indexed.

-- A work's title, with its hyphenated words joined ("SpiderMan") and its
-- sort title when the title doesn't hold it; its alternative and original
-- titles; the names of the people credited on it.
CREATE VIRTUAL TABLE work_fts USING fts5 (
  title, alt_titles, people,
  tokenize = 'unicode61 remove_diacritics 2', prefix = '1 2 3', content = '', contentless_delete = 1
);

-- People by name, for search's People.
CREATE VIRTUAL TABLE person_fts USING fts5 (
  name,
  tokenize = 'unicode61 remove_diacritics 2', prefix = '1 2 3', content = '', contentless_delete = 1
);

-- A collection's name, with its hyphenated words joined and its sort name
-- when the name doesn't hold it, as work_fts has a title.
CREATE VIRTUAL TABLE collection_fts USING fts5 (
  name,
  tokenize = 'unicode61 remove_diacritics 2', prefix = '1 2 3', content = '', contentless_delete = 1
);

-- What only a book has. Its authors and narrators are credits (roles author
-- and narrator, in order), its publisher a company, its ASIN and ISBN
-- external IDs, its description the work's overview and its publication
-- the work's release date.
CREATE TABLE book (
  work_id  BLOB PRIMARY KEY REFERENCES work (id) ON DELETE CASCADE,
  subtitle TEXT,
  language TEXT, -- as the source gives it
  abridged INTEGER CHECK (abridged IN (0, 1)) -- NULL when unknown
) STRICT, WITHOUT ROWID;

-- A series of books, never a collection. key is where it came from: a
-- folder or tag (local:<author>/<series>), later a provider's ID; renaming
-- one changes its name, not its key. sort_name follows catalog.SortTitle.
CREATE TABLE book_series (
  id         BLOB PRIMARY KEY,
  key        TEXT NOT NULL UNIQUE,
  name       TEXT NOT NULL,
  sort_name  TEXT NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX book_series_name ON book_series (name);

-- A book's place in a series: position_label as written ("1", "1.5",
-- "1-3"), position_num its number for ordering (1.5; 1 for "1-3"), NULL
-- when it has none. A book in several series has one primary.
CREATE TABLE book_series_book (
  series_id      BLOB NOT NULL REFERENCES book_series (id) ON DELETE CASCADE,
  work_id        BLOB NOT NULL REFERENCES work (id) ON DELETE CASCADE,
  position_label TEXT,
  position_num   REAL,
  is_primary     INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
  PRIMARY KEY (series_id, work_id)
) STRICT, WITHOUT ROWID;
CREATE INDEX book_series_book_work ON book_series_book (work_id);

-- scan -----------------------------------------------------------------------

-- What scan last saw of each directory, so unchanged directories are never
-- listed again.
CREATE TABLE scan_dir (
  folder_id           BLOB NOT NULL REFERENCES library_folder (id) ON DELETE CASCADE,
  path                TEXT NOT NULL, -- relative to the library folder; "." for the folder itself
  mtime               INTEGER NOT NULL,
  listed_at           INTEGER NOT NULL,
  local_metadata_hash TEXT NOT NULL DEFAULT '', -- names and times of hint, NFO and artwork files, to notice changes
  PRIMARY KEY (folder_id, path)
) STRICT, WITHOUT ROWID;

-- When each library folder last had a full walk, and whether an admin asked
-- for one.
CREATE TABLE scan_folder (
  folder_id      BLOB PRIMARY KEY REFERENCES library_folder (id) ON DELETE CASCADE,
  full_walk_at   INTEGER,
  full_requested INTEGER NOT NULL DEFAULT 0
) STRICT, WITHOUT ROWID;

-- identify -------------------------------------------------------------------

-- The decision for one subject of a library: a series or movie folder, or a
-- loose movie sharing its folder with others. It records what was looked up
-- (query) and what came back, so a matched folder is searched once while
-- its source and query stay the same; an unmatched one is looked up again.
CREATE TABLE match (
  library_id BLOB NOT NULL REFERENCES library (id) ON DELETE CASCADE,
  subject    TEXT NOT NULL, -- a folder path, or folder|title key|year for a loose movie
  source     TEXT NOT NULL CHECK (source IN ('manual', 'path', 'nfo', 'search')),
  query      TEXT NOT NULL, -- the ID or title and year looked up; empty for manual
  status     TEXT NOT NULL CHECK (status IN ('matched', 'unmatched')),
  provider   TEXT,          -- the accepted candidate, when matched
  kind       TEXT,
  ext_id     TEXT,
  title      TEXT,
  year       INTEGER,
  score      INTEGER,       -- 0 to 100, for a search
  ordering   TEXT,          -- the episode ordering the folder's episodes were placed in
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (library_id, subject),
  CHECK ((status = 'matched') = (ext_id IS NOT NULL))
) STRICT, WITHOUT ROWID;

-- Each root work's folder: the library and the subject its files are
-- identified as, which its match row is keyed by too. identify links them
-- as it identifies a folder.
CREATE TABLE work_folder (
  library_id BLOB NOT NULL REFERENCES library (id) ON DELETE CASCADE,
  subject    TEXT NOT NULL,
  work_id    BLOB NOT NULL UNIQUE REFERENCES work (id) ON DELETE CASCADE,
  PRIMARY KEY (library_id, subject)
) STRICT, WITHOUT ROWID;

-- metadata -------------------------------------------------------------------

-- A cached provider response. Rows outlive expires_at so a stale answer can
-- be served while the provider is down. key is the request without any API
-- key; body is NULL for a cached not-found.
CREATE TABLE provider_cache (
  provider   TEXT NOT NULL,
  key        TEXT NOT NULL,
  body       BLOB,
  fetched_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  PRIMARY KEY (provider, key)
) STRICT;
CREATE INDEX provider_cache_expires ON provider_cache (expires_at);

-- A work's, person's or collection's artwork, one per kind, re-encoded to
-- WebP at fixed widths under cache/images/. origin is where it came from (a
-- provider URL or a file next to the media) and never reaches a client.
CREATE TABLE image (
  id            BLOB PRIMARY KEY,
  work_id       BLOB REFERENCES work (id) ON DELETE CASCADE,
  person_id     BLOB REFERENCES person (id) ON DELETE CASCADE,
  collection_id BLOB REFERENCES collection (id) ON DELETE CASCADE,
  kind          TEXT NOT NULL CHECK (kind IN ('poster', 'backdrop', 'logo', 'banner', 'thumb', 'still', 'photo')),
  source        TEXT NOT NULL CHECK (source IN ('provider', 'local', 'upload')),
  origin        TEXT NOT NULL,
  origin_mtime  INTEGER, -- a local file's mtime, to notice it changing
  width         INTEGER NOT NULL,
  height        INTEGER NOT NULL,
  blurhash      TEXT NOT NULL,
  chosen        INTEGER NOT NULL DEFAULT 0 CHECK (chosen IN (0, 1)), -- an admin chose or uploaded it; refreshes keep it
  fetched_at    INTEGER NOT NULL,
  CHECK ((work_id IS NOT NULL) + (person_id IS NOT NULL) + (collection_id IS NOT NULL) = 1)
) STRICT;
CREATE UNIQUE INDEX image_work ON image (work_id, kind) WHERE work_id IS NOT NULL;
CREATE UNIQUE INDEX image_person ON image (person_id, kind) WHERE person_id IS NOT NULL;
CREATE UNIQUE INDEX image_collection ON image (collection_id, kind) WHERE collection_id IS NOT NULL;

-- Artwork an admin can choose for a work or person: the providers' images
-- and the files next to the media, recorded as Choose artwork lists them so
-- each has an ID to show and choose it by. origin is a provider URL or a
-- file's path and never reaches a client; folder is a local file's library
-- folder, which it's read through. A preview is made the first time one is
-- shown, in cache/images/ under its ID.
CREATE TABLE artwork (
  id        BLOB PRIMARY KEY,
  work_id   BLOB REFERENCES work (id) ON DELETE CASCADE,
  person_id BLOB REFERENCES person (id) ON DELETE CASCADE,
  kind      TEXT NOT NULL CHECK (kind IN ('poster', 'backdrop', 'logo', 'still', 'photo')),
  source    TEXT NOT NULL CHECK (source IN ('provider', 'local')),
  provider  TEXT,
  origin    TEXT NOT NULL,
  folder    TEXT,
  width     INTEGER,
  height    INTEGER,
  language  TEXT, -- ISO 639-1; NULL for textless or unknown
  ord       INTEGER NOT NULL,
  listed_at INTEGER NOT NULL,
  CHECK ((work_id IS NULL) <> (person_id IS NULL))
) STRICT;
CREATE UNIQUE INDEX artwork_work ON artwork (work_id, kind, origin) WHERE work_id IS NOT NULL;
CREATE UNIQUE INDEX artwork_person ON artwork (person_id, kind, origin) WHERE person_id IS NOT NULL;

-- userstate ------------------------------------------------------------------
-- Each account's watch state, play history and track choices. Positions
-- are on the version's timeline. A work's watch state, plays and track
-- choices go with it when it's deleted.

-- favorited_at is when the work was marked a favorite, which the Favorites
-- lists sort by, newest first; NULL when it isn't one. version_id is the
-- version last played, which the position is on.
CREATE TABLE user_state (
  user_id        BLOB NOT NULL REFERENCES user (id) ON DELETE CASCADE,
  work_id        BLOB NOT NULL REFERENCES work (id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  played         INTEGER NOT NULL DEFAULT 0,
  play_count     INTEGER NOT NULL DEFAULT 0,
  position_ms    INTEGER NOT NULL DEFAULT 0,
  resumable      INTEGER NOT NULL DEFAULT 0, -- past the Continue Watching floor and not finished
  favorite       INTEGER NOT NULL DEFAULT 0,
  rating         INTEGER,
  last_played_at INTEGER,
  updated_at     INTEGER NOT NULL,
  favorited_at   INTEGER,
  version_id     BLOB, -- the version may be gone
  PRIMARY KEY (user_id, work_id)
) STRICT, WITHOUT ROWID;
CREATE INDEX user_state_resume ON user_state (user_id, last_played_at DESC) WHERE resumable;
CREATE INDEX user_state_work ON user_state (work_id);
-- The Favorites row.
CREATE INDEX user_state_favorite ON user_state (user_id, favorited_at DESC) WHERE favorite;

CREATE TABLE play_event (
  id          BLOB PRIMARY KEY,
  user_id     BLOB NOT NULL REFERENCES user (id) ON DELETE CASCADE,
  work_id     BLOB NOT NULL REFERENCES work (id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  version_id  BLOB NOT NULL, -- the version may be gone; history stays
  play_id     BLOB NOT NULL, -- the play it came from
  started_at  INTEGER NOT NULL,
  ended_at    INTEGER NOT NULL,
  position_ms INTEGER NOT NULL,
  completed   INTEGER NOT NULL DEFAULT 0 -- reached the played threshold
) STRICT;
CREATE INDEX play_event_user ON play_event (user_id, started_at DESC);
CREATE INDEX play_event_work ON play_event (work_id);
-- The server-wide watch history, every account's plays newest first.
CREATE INDEX play_event_started ON play_event (started_at DESC);
CREATE UNIQUE INDEX play_event_play ON play_event (play_id, work_id);

-- The audio and subtitle tracks an account last chose for a series.
CREATE TABLE track_choice (
  user_id           BLOB NOT NULL REFERENCES user (id) ON DELETE CASCADE,
  series_id         BLOB NOT NULL REFERENCES work (id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  audio_language    TEXT,
  subtitle_language TEXT,
  subtitle_kind     TEXT, -- off, forced, full or sdh
  updated_at        INTEGER NOT NULL,
  PRIMARY KEY (user_id, series_id)
) STRICT, WITHOUT ROWID;

-- markers --------------------------------------------------------------------

-- A file's skip markers, found at one generation of the file: the intro
-- or credits a season's detection decided on, from the season's audio
-- fingerprints (and, for credits, its pictures), and those an admin set.
-- Times are the file's own, in milliseconds. A file has at most one found
-- marker of a kind; an admin's sits beside it, and an admin's with no end
-- says the file has none of that kind. final is set on credits that run to
-- the end of the file, or that only a next episode's preview follows.
-- reason says, for admins, why the marker is what it is. Chapters' markers
-- aren't stored: they're read from the chapters as a play starts.
CREATE TABLE media_marker (
  file_id    BLOB NOT NULL REFERENCES media_file (id) ON DELETE CASCADE,
  generation INTEGER NOT NULL,
  kind       TEXT NOT NULL CHECK (kind IN ('intro', 'recap', 'credits')),
  source     TEXT NOT NULL CHECK (source IN ('fingerprint', 'manual')),
  start_ms   INTEGER NOT NULL,
  end_ms     INTEGER,
  final      INTEGER NOT NULL DEFAULT 0 CHECK (final IN (0, 1)),
  reason     TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (file_id, kind, source),
  CHECK (end_ms IS NOT NULL OR source = 'manual'),
  CHECK (final = 0 OR kind = 'credits')
) STRICT, WITHOUT ROWID;

-- What detection last decided for each file: the generation and the version
-- of the rules (detect.RulesVersion) its markers were decided at, whether
-- they found any (done) or not (none), and when. Kept whether or not they
-- found any, so a file decided by older rules is decided again. A file
-- whose fingerprint failed for good is decided as failed, with why, so it
-- isn't read again until it changes or is retried; a failed row stands
-- whatever the rules.
CREATE TABLE marker_file (
  file_id    BLOB PRIMARY KEY REFERENCES media_file (id) ON DELETE CASCADE,
  generation INTEGER NOT NULL,
  rules      INTEGER NOT NULL,
  state      TEXT NOT NULL CHECK (state IN ('done', 'none', 'failed')),
  reason     TEXT CHECK ((state = 'failed') = (reason IS NOT NULL)),
  updated_at INTEGER NOT NULL
) STRICT, WITHOUT ROWID;

-- previews -------------------------------------------------------------------

-- Each file's preview thumbnails: made at a generation, or failed at it and
-- why, so a broken file isn't read again until it changes or is retried.
-- The thumbnails themselves are in cache/previews/.
CREATE TABLE preview_file (
  file_id    BLOB PRIMARY KEY REFERENCES media_file (id) ON DELETE CASCADE,
  generation INTEGER NOT NULL,
  state      TEXT NOT NULL CHECK (state IN ('done', 'failed')),
  reason     TEXT CHECK ((state = 'failed') = (reason IS NOT NULL)),
  updated_at INTEGER NOT NULL
) STRICT, WITHOUT ROWID;

-- subtitles ------------------------------------------------------------------

-- The same for each file's extracted subtitles, in cache/subtitles/.
CREATE TABLE subtitle_file (
  file_id    BLOB PRIMARY KEY REFERENCES media_file (id) ON DELETE CASCADE,
  generation INTEGER NOT NULL,
  state      TEXT NOT NULL CHECK (state IN ('done', 'failed')),
  reason     TEXT CHECK ((state = 'failed') = (reason IS NOT NULL)),
  updated_at INTEGER NOT NULL
) STRICT, WITHOUT ROWID;

-- playback -------------------------------------------------------------------

-- A download a device asked for: the device session it's for, what it
-- asked, the plan the server made of it (the version's files, the decision,
-- the tracks), and where its preparation is. One that needs work on the
-- server (a conversion, or subtitles to prepare) is prepared one at a time,
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
  request    TEXT NOT NULL, -- JSON: what the device asked for; Retry plans it again
  plan       TEXT NOT NULL, -- JSON
  state      TEXT NOT NULL CHECK (state IN ('queued', 'preparing', 'ready', 'failed')),
  reason     TEXT,          -- why it failed: a playback reason code
  files      TEXT,          -- JSON: each converted part's size and SHA-256, once ready
  created_at INTEGER NOT NULL,
  queued_at  INTEGER NOT NULL,
  settled_at INTEGER        -- when it became ready or failed
) STRICT;
CREATE INDEX download_user ON download (user_id);
CREATE INDEX download_session ON download (session_id, created_at);
CREATE INDEX download_queue ON download (queued_at, id) WHERE state IN ('queued', 'preparing');
CREATE INDEX download_settled ON download (settled_at) WHERE settled_at IS NOT NULL;

-- tasks ----------------------------------------------------------------------

-- Each task's last run: when it ended, with what it did or why it failed.
-- A run that's open is held in memory only.
CREATE TABLE task (
  name     TEXT PRIMARY KEY,
  finished INTEGER NOT NULL, -- when the last run ended
  summary  TEXT,             -- what the last run did, or that it was stopped
  error    TEXT              -- why the last run failed
) STRICT, WITHOUT ROWID;

-- activity -------------------------------------------------------------------

-- What happened on the server, for admins: plays started and finished,
-- root works added, sign-ins and failed ones, users joining, task runs ending
-- and problems raised. Each row is written in the transaction of the change
-- it tells of, and kept 90 days. data is what the row says, as things were
-- then (names as well as IDs), as JSON. A run of failed sign-ins for one
-- user from one address is one row, its count and time the latest.
CREATE TABLE activity (
  id   BLOB PRIMARY KEY,
  at   INTEGER NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('playStarted', 'playFinished', 'added', 'signedIn', 'signInFailed',
    'userJoined', 'taskEnded', 'problemRaised')),
  data TEXT NOT NULL
) STRICT;
CREATE INDEX activity_at ON activity (at, id);
CREATE INDEX activity_kind ON activity (kind, at, id);
