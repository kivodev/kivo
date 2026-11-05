-- name: GetState :one
SELECT * FROM user_state WHERE user_id = @user_id AND work_id = @work_id;

-- name: StatesOfWork :many
SELECT * FROM user_state WHERE work_id = @work_id;

-- name: SaveProgress :exec
-- A position from playback, on the version played; an older report never
-- overwrites a newer one.
INSERT INTO user_state (user_id, work_id, position_ms, resumable, version_id, last_played_at, updated_at)
VALUES (@user_id, @work_id, @position_ms, @resumable, @version_id, CAST(@at AS INTEGER), @at)
ON CONFLICT (user_id, work_id) DO UPDATE SET
  position_ms = excluded.position_ms, resumable = excluded.resumable, version_id = excluded.version_id,
  last_played_at = excluded.last_played_at, updated_at = excluded.updated_at
WHERE excluded.updated_at >= user_state.updated_at;

-- name: SaveFinished :exec
-- Playback reached the played threshold: one more play, and back to the
-- start. The version stays the last played unless a later play has written
-- since.
INSERT INTO user_state (user_id, work_id, played, play_count, position_ms, resumable, version_id, last_played_at, updated_at)
VALUES (@user_id, @work_id, 1, 1, 0, 0, @version_id, CAST(@at AS INTEGER), @at)
ON CONFLICT (user_id, work_id) DO UPDATE SET
  played = 1, play_count = play_count + 1, position_ms = 0, resumable = 0,
  version_id = CASE WHEN excluded.last_played_at >= coalesce(last_played_at, 0) THEN excluded.version_id ELSE version_id END,
  last_played_at = max(coalesce(last_played_at, 0), excluded.last_played_at),
  updated_at = max(updated_at, excluded.updated_at);

-- name: SetPlayed :exec
-- Mark played or unplayed by hand. Marking played counts a play if there was none.
INSERT INTO user_state (user_id, work_id, played, play_count, position_ms, resumable, updated_at)
VALUES (@user_id, @work_id, @played, @played, 0, 0, @at)
ON CONFLICT (user_id, work_id) DO UPDATE SET
  played = excluded.played, position_ms = 0, resumable = 0, updated_at = excluded.updated_at,
  play_count = CASE WHEN excluded.played AND play_count = 0 THEN 1 ELSE play_count END;

-- name: AddFavorite :exec
-- Mark a favorite by hand. One already marked keeps when it was marked.
INSERT INTO user_state (user_id, work_id, favorite, favorited_at, updated_at) VALUES (@user_id, @work_id, 1, CAST(@at AS INTEGER), @at)
ON CONFLICT (user_id, work_id) DO UPDATE SET
  favorite = 1, favorited_at = CASE WHEN favorite THEN favorited_at ELSE excluded.favorited_at END,
  updated_at = excluded.updated_at;

-- name: RemoveFavorite :exec
INSERT INTO user_state (user_id, work_id, favorite, updated_at) VALUES (@user_id, @work_id, 0, @at)
ON CONFLICT (user_id, work_id) DO UPDATE SET favorite = 0, favorited_at = NULL, updated_at = excluded.updated_at;

-- name: RemoveFromContinue :exec
-- Leaves updated_at alone, so an offline position from before the removal
-- still applies.
INSERT INTO user_state (user_id, work_id, continue_removed_at, updated_at) VALUES (@user_id, @work_id, CAST(@at AS INTEGER), @at)
ON CONFLICT (user_id, work_id) DO UPDATE SET continue_removed_at = excluded.continue_removed_at;

-- name: SetRating :exec
INSERT INTO user_state (user_id, work_id, rating, updated_at) VALUES (@user_id, @work_id, @rating, @at)
ON CONFLICT (user_id, work_id) DO UPDATE SET rating = excluded.rating, updated_at = excluded.updated_at;

-- name: PutState :exec
-- Write a whole row, for merging works.
INSERT INTO user_state (user_id, work_id, played, play_count, position_ms, resumable, version_id, favorite, favorited_at, rating, last_played_at, updated_at)
VALUES (@user_id, @work_id, @played, @play_count, @position_ms, @resumable, @version_id, @favorite, @favorited_at, @rating, @last_played_at, @updated_at)
ON CONFLICT (user_id, work_id) DO UPDATE SET
  played = excluded.played, play_count = excluded.play_count, position_ms = excluded.position_ms,
  resumable = excluded.resumable, version_id = excluded.version_id, favorite = excluded.favorite, favorited_at = excluded.favorited_at, rating = excluded.rating,
  last_played_at = excluded.last_played_at, updated_at = excluded.updated_at;

-- name: DeleteStates :exec
DELETE FROM user_state WHERE work_id = @work_id;

-- name: Episodes :many
-- The episodes under a series or season, or the work itself otherwise.
WITH RECURSIVE sub (id, kind) AS (
  SELECT w.id, w.kind FROM work w WHERE w.id = @work_id
  UNION ALL
  SELECT w.id, w.kind FROM work w JOIN sub ON w.parent_id = sub.id WHERE w.kind IN ('season', 'episode')
)
SELECT id FROM sub WHERE kind NOT IN ('series', 'season');

-- name: InsertEvent :exec
INSERT INTO play_event (id, user_id, work_id, version_id, play_id, started_at, ended_at, position_ms, completed)
VALUES (@id, @user_id, @work_id, @version_id, @play_id, @started_at, @ended_at, @position_ms, @completed)
-- A play's later reports continue its event.
ON CONFLICT (play_id, work_id) DO UPDATE SET
  ended_at = excluded.ended_at, position_ms = excluded.position_ms, completed = completed OR excluded.completed;

-- name: PlayEvent :one
-- A play's event for a work: when its latest report written was, and whether
-- it finished the work.
SELECT ended_at, completed FROM play_event WHERE play_id = @play_id AND work_id = @work_id;

-- name: MoveEvents :exec
UPDATE play_event SET work_id = @into WHERE work_id = @from;

-- name: MoveVersionEvents :many
-- A version's own plays follow it to the episode it now covers.
UPDATE play_event SET work_id = @into WHERE work_id = @from AND version_id = @version_id
RETURNING user_id, completed;

-- name: CompletedPlays :one
SELECT count(*) FROM play_event WHERE user_id = @user_id AND work_id = @work_id AND completed;

-- name: GetTrackChoice :one
SELECT * FROM track_choice WHERE user_id = @user_id AND series_id = @series_id;

-- name: SetTrackChoice :exec
INSERT INTO track_choice (user_id, series_id, audio_language, subtitle_language, subtitle_kind, updated_at)
VALUES (@user_id, @series_id, @audio_language, @subtitle_language, @subtitle_kind, @updated_at)
ON CONFLICT (user_id, series_id) DO UPDATE SET
  audio_language = excluded.audio_language, subtitle_language = excluded.subtitle_language,
  subtitle_kind = excluded.subtitle_kind, updated_at = excluded.updated_at;
