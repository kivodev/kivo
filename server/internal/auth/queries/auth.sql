-- name: AnyUser :one
SELECT EXISTS (SELECT 1 FROM user);

-- name: SignInUser :one
-- The user and its password hash, by username (compared case-insensitively).
SELECT u.id, u.disabled, c.secret
FROM user u LEFT JOIN credential c ON c.user_id = u.id AND c.kind = 'password'
WHERE u.username = @username;

-- name: UserPassword :one
-- A user's username and password hash, by ID.
SELECT u.username, c.secret
FROM user u LEFT JOIN credential c ON c.user_id = u.id AND c.kind = 'password'
WHERE u.id = @id;

-- name: InsertPassword :exec
INSERT INTO credential (id, user_id, kind, secret, created_at, updated_at)
VALUES (@id, @user_id, 'password', @secret, @now, @now);

-- name: SetPassword :execrows
INSERT INTO credential (id, user_id, kind, secret, created_at, updated_at)
SELECT @id, u.id, 'password', @secret, @now, @now FROM user u WHERE u.id = @user_id
ON CONFLICT (user_id, kind) DO UPDATE SET secret = excluded.secret, updated_at = excluded.updated_at;

-- name: InsertSession :exec
-- An empty @client_id is none.
INSERT INTO session (id, token_hash, probe_id, user_id, kind, name, platform, client_id, created_at, last_used_at,
  app, app_version, os_version, device_model)
VALUES (@id, @token_hash, @probe_id, @user_id, @kind, @name, @platform, NULLIF(CAST(@client_id AS TEXT), ''), @now, @now,
  @app, @app_version, @os_version, @device_model);

-- name: DeleteClientSession :many
-- The user's session on @client_id, which a new sign-in from that client
-- replaces.
DELETE FROM session WHERE user_id = @user_id AND client_id = CAST(@client_id AS TEXT) RETURNING id;

-- name: CallerByToken :one
-- The session or API key with this token, joined to its user in the same
-- read, so disabling the user takes effect at once. An API key's kind is
-- empty, and so is its device.
SELECT s.id, s.user_id, s.kind, s.name, s.last_used_at,
  s.app, s.app_version, s.platform, s.os_version, s.device_model
FROM session s JOIN user u ON u.id = s.user_id
WHERE s.token_hash = @token_hash AND u.disabled = 0
UNION ALL
SELECT k.id, k.created_by, '', k.name, coalesce(k.last_used_at, k.created_at), '', '', '', '', ''
FROM api_key k JOIN user u ON u.id = k.created_by
WHERE k.token_hash = @token_hash AND u.disabled = 0;

-- name: ProbeKey :one
-- The token hash of the session with this probe ID, which keys the
-- server's proof to a probe from its app.
SELECT token_hash FROM session WHERE probe_id = @probe_id;

-- name: SessionAlive :one
-- A session whose user isn't disabled.
SELECT s.kind, s.last_used_at
FROM session s JOIN user u ON u.id = s.user_id
WHERE s.id = @id AND u.disabled = 0;

-- name: Session :one
SELECT id, user_id, kind, name, platform, created_at, last_used_at, app, app_version, os_version, device_model
FROM session WHERE id = @id;

-- name: SetSessionDevice :exec
-- The app and device a session's requests now identify. An empty
-- @platform keeps the session's own.
UPDATE session SET app = @app, app_version = @app_version, os_version = @os_version, device_model = @device_model,
  platform = coalesce(nullif(CAST(@platform AS TEXT), ''), platform)
WHERE id = @id;

-- name: SetSessionName :exec
UPDATE session SET name = @name WHERE id = @id;

-- name: APIKeyAlive :one
SELECT EXISTS (
  SELECT 1 FROM api_key k JOIN user u ON u.id = k.created_by
  WHERE k.id = @id AND u.disabled = 0);

-- name: DeleteSession :one
DELETE FROM session WHERE id = @id RETURNING user_id;

-- name: DeleteOtherSessions :many
-- A user's sessions but @keep; every one when @keep is NULL.
DELETE FROM session WHERE user_id = @user_id AND (id != @keep OR @keep IS NULL) RETURNING id;

-- name: TouchSession :exec
UPDATE session SET last_used_at = max(last_used_at, CAST(@used AS INTEGER)) WHERE id = @id;

-- name: TouchAPIKey :exec
UPDATE api_key SET last_used_at = max(coalesce(last_used_at, 0), CAST(@used AS INTEGER)) WHERE id = @id;

-- name: DeleteIdleBrowserSessions :execrows
-- At most @limit of the browser sessions idle since before @before: one batch.
DELETE FROM session WHERE rowid IN (
  SELECT s.rowid FROM session s WHERE s.kind = 'browser' AND s.last_used_at < @before LIMIT @limit);

-- name: InsertInvite :exec
INSERT INTO invite (id, token_hash, created_by, all_libraries, stream_limit, remote_quality_cap, expires_at, created_at)
VALUES (@id, @token_hash, @created_by, @all_libraries, @stream_limit, @remote_quality_cap, @expires_at, @created_at);

-- name: InsertInviteLibrary :exec
INSERT INTO invite_library (invite_id, library_id) VALUES (@invite_id, @library_id)
ON CONFLICT DO NOTHING;

-- name: PendingInvites :many
-- Invites not yet used or expired, newest first, with their creator's name.
SELECT i.id, i.all_libraries, i.stream_limit, i.remote_quality_cap, i.expires_at, i.created_at, u.username AS created_by
FROM invite i JOIN user u ON u.id = i.created_by
WHERE i.used_at IS NULL AND i.expires_at > @now
ORDER BY i.created_at DESC;

-- name: InviteLibraries :many
SELECT library_id FROM invite_library WHERE invite_id = @invite_id;

-- name: InviteByToken :one
-- A pending invite, with its creator's name.
SELECT i.id, i.all_libraries, i.stream_limit, i.remote_quality_cap, i.expires_at, i.created_by, u.username
FROM invite i JOIN user u ON u.id = i.created_by
WHERE i.token_hash = @token_hash AND i.used_at IS NULL AND i.expires_at > @now;

-- name: UseInvite :execrows
UPDATE invite SET used_at = CAST(@now AS INTEGER), used_by = @used_by WHERE id = @id AND used_at IS NULL;

-- name: DeleteInvite :execrows
DELETE FROM invite WHERE id = @id;

-- name: UserSessions :many
SELECT id, user_id, kind, name, platform, created_at, last_used_at, app, app_version, os_version, device_model
FROM session WHERE user_id = @user_id;

-- name: InsertAPIKey :exec
INSERT INTO api_key (id, token_hash, name, created_by, created_at)
VALUES (@id, @token_hash, @name, @created_by, @created_at);

-- name: APIKeys :many
-- Every API key, newest first.
SELECT id, name, created_at, last_used_at FROM api_key ORDER BY created_at DESC;

-- name: APIKey :one
SELECT id, name, created_at, last_used_at FROM api_key WHERE id = @id;

-- name: DeleteAPIKey :one
DELETE FROM api_key WHERE id = @id RETURNING created_by;

-- name: InsertCastToken :exec
INSERT INTO cast_token (id, token_hash, session_id, name, receiver, created_at, last_used_at)
VALUES (@id, @token_hash, @session_id, @name, @receiver, @now, @now);

-- name: CastCallerByToken :one
-- The cast token with this token, with its session's user, kind and last
-- use, in one read, so a disabled user or a signed-out session ends it at
-- once.
SELECT c.id, c.session_id, c.name, c.created_at, c.last_used_at, s.user_id, s.kind AS session_kind,
  s.last_used_at AS session_used_at
FROM cast_token c JOIN session s ON s.id = c.session_id JOIN user u ON u.id = s.user_id
WHERE c.token_hash = @token_hash AND u.disabled = 0;

-- name: CastTokenAlive :one
-- A cast token whose session and user still stand, with what decides
-- whether it or its session expired.
SELECT c.created_at, c.last_used_at, c.session_id, s.kind AS session_kind, s.last_used_at AS session_used_at
FROM cast_token c JOIN session s ON s.id = c.session_id JOIN user u ON u.id = s.user_id
WHERE c.id = @id AND u.disabled = 0;

-- name: DeleteCastToken :execrows
DELETE FROM cast_token WHERE id = @id;

-- name: TouchCastToken :exec
UPDATE cast_token SET last_used_at = max(last_used_at, CAST(@used AS INTEGER)) WHERE id = @id;

-- name: DeleteExpiredCastTokens :execrows
-- At most @limit of the cast tokens unused since before @idle_before or
-- made before @made_before: one batch.
DELETE FROM cast_token WHERE rowid IN (
  SELECT c.rowid FROM cast_token c WHERE c.last_used_at < @idle_before OR c.created_at < @made_before LIMIT @limit);
