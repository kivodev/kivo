-- name: InsertUser :exec
INSERT INTO user (id, username, admin, all_libraries, stream_limit, remote_quality_cap,
  rating_ceiling, block_unrated, subtitle_mode, invited_by, created_at)
VALUES (@id, @username, @admin, @all_libraries, @stream_limit, @remote_quality_cap,
  @rating_ceiling, @block_unrated, 'forced', @invited_by, @created_at);

-- name: SetUsername :execrows
UPDATE user SET username = @username WHERE id = @id;

-- name: SetAdmin :execrows
UPDATE user SET admin = @admin WHERE id = @id;

-- name: SetDisabled :execrows
UPDATE user SET disabled = @disabled WHERE id = @id;

-- name: SetAllLibraries :execrows
UPDATE user SET all_libraries = @all_libraries WHERE id = @id;

-- name: SetLimits :execrows
UPDATE user SET stream_limit = @stream_limit, remote_quality_cap = @remote_quality_cap WHERE id = @id;

-- name: SetParental :execrows
UPDATE user SET rating_ceiling = @rating_ceiling, block_unrated = @block_unrated WHERE id = @id;

-- name: DeleteUser :execrows
DELETE FROM user WHERE id = @id;

-- name: EnabledAdmins :one
SELECT count(*) FROM user WHERE admin = 1 AND disabled = 0;

-- name: DeleteGrants :exec
DELETE FROM library_grant WHERE user_id = @user_id;

-- name: InsertGrant :exec
INSERT INTO library_grant (user_id, library_id) VALUES (@user_id, @library_id)
ON CONFLICT DO NOTHING;

-- name: Grants :many
SELECT library_id FROM library_grant WHERE user_id = @user_id;

-- name: AllGrants :many
SELECT user_id, library_id FROM library_grant;

-- name: DeleteUngrantedLibraries :exec
-- A user forgets the place of a library it no longer sees, so the
-- library is pinned again, at the end, if it's granted back.
DELETE FROM user_library
WHERE user_library.user_id = @user_id
  AND library_id NOT IN (SELECT g.library_id FROM library_grant g WHERE g.user_id = @user_id);

-- name: InsertList :exec
INSERT INTO list (id, user_id, name, built_in, query, created_at) VALUES (@id, @user_id, @name, @built_in, @query, @created_at);

-- name: CountLists :one
SELECT count(*) FROM list WHERE user_id = @user_id;

-- name: GetList :one
SELECT id, built_in, query IS NOT NULL AS smart FROM list WHERE id = @id AND user_id = @user_id;

-- name: MovePersonInListQueries :exec
-- A person merged into another keeps the rules that name them.
UPDATE list SET query = replace(query, CAST(@from AS TEXT), CAST(@into AS TEXT))
WHERE query LIKE '%' || CAST(@from AS TEXT) || '%';

-- name: SetListQuery :exec
UPDATE list SET query = @query WHERE id = @id;

-- name: RenameList :exec
UPDATE list SET name = @name WHERE id = @id;

-- name: DeleteList :exec
DELETE FROM list WHERE id = @id;

-- name: DeleteHomeRows :exec
DELETE FROM user_home_row WHERE user_id = @user_id;

-- name: InsertHomeRow :exec
INSERT INTO user_home_row (user_id, row_id, position, shown) VALUES (@user_id, @row_id, @position, @shown);

-- name: DeleteLibraryOrder :exec
DELETE FROM user_library WHERE user_id = @user_id;

-- name: InsertLibraryOrder :exec
INSERT INTO user_library (user_id, library_id, position, pinned) VALUES (@user_id, @library_id, @position, @pinned);

-- name: SetPlayback :execrows
UPDATE user SET audio_languages = @audio_languages, subtitle_languages = @subtitle_languages,
  subtitle_mode = @subtitle_mode, remember_tracks = @remember_tracks, autoplay_next = @autoplay_next,
  book_speed = @book_speed
WHERE id = @id;

-- name: GetUser :one
SELECT * FROM user WHERE id = @id;

-- name: SavedListQueries :many
SELECT id, query FROM list WHERE query IS NOT NULL;

-- name: ListUsers :many
SELECT * FROM user ORDER BY username;
