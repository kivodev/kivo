-- name: InsertDownload :exec
INSERT INTO download (id, user_id, session_id, work_id, version_id, request, delivered, state, created_at, queued_at, settled_at)
VALUES (@id, @user_id, @session_id, @work_id, @version_id, @request, @delivered, @state, @created_at, @queued_at, @settled_at);

-- name: GetDownload :one
SELECT * FROM download WHERE id = @id;

-- name: SessionDownloads :many
-- A device's downloads, newest first.
SELECT * FROM download WHERE session_id = @session_id ORDER BY created_at DESC;

-- name: SameDownload :many
-- The device's downloads of a work that haven't failed, newest first. One
-- asked for the same way is what a device asking again gets back.
SELECT * FROM download
WHERE session_id = @session_id AND work_id = @work_id AND state <> 'failed'
ORDER BY created_at DESC;

-- name: QueueAhead :one
-- How many downloads waiting to be prepared were asked for before this one.
SELECT count(*) FROM download
WHERE state IN ('queued', 'preparing')
  AND (queued_at < @queued_at OR queued_at = @queued_at AND id < @id);

-- name: OpenDownloads :many
-- The downloads waiting to be prepared, in the order they were asked for.
SELECT id FROM download WHERE state IN ('queued', 'preparing') ORDER BY queued_at, id;

-- name: QueuedUsers :many
-- The users with downloads waiting to be prepared, whose places in the
-- queue move as one is prepared.
SELECT DISTINCT user_id FROM download WHERE state IN ('queued', 'preparing');

-- name: StartDownload :execrows
UPDATE download SET state = 'preparing' WHERE id = @id AND state IN ('queued', 'preparing');

-- name: RequeueDownload :exec
UPDATE download SET state = 'queued' WHERE id = @id AND state = 'preparing';

-- name: SettleDownload :execrows
-- A ready download takes what it delivers from how it was made. A failed
-- one keeps what it had.
UPDATE download SET state = @state, reason = @reason, delivered = coalesce(sqlc.narg('delivered'), delivered),
  settled_at = @settled_at
WHERE id = @id AND state IN ('queued', 'preparing');

-- name: ReplanDownload :execrows
-- Retry: a failed download planned again, at the back of the queue, while
-- it's in the state Retry read it in.
UPDATE download SET version_id = @version_id, delivered = @delivered, state = @state, reason = NULL,
  queued_at = @queued_at, settled_at = @settled_at
WHERE id = @id AND state = @was;

-- name: DeleteDownload :execrows
DELETE FROM download WHERE id = @id;

-- name: ExpiredDownloads :many
SELECT id FROM download WHERE settled_at < @before ORDER BY settled_at LIMIT @limit;

-- name: MoveDownloadSubtitle :execrows
-- Downloads that name a subtitle track, in what was asked and what is
-- delivered, name another instead. Track IDs are unique, so the text replaces.
UPDATE download SET request = replace(request, CAST(@from_track AS TEXT), CAST(@to_track AS TEXT)),
  delivered = replace(delivered, CAST(@from_track AS TEXT), CAST(@to_track AS TEXT))
WHERE instr(request, CAST(@from_track AS TEXT)) > 0 OR instr(delivered, CAST(@from_track AS TEXT)) > 0;

-- name: DownloadsAmong :many
SELECT id FROM download WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));
