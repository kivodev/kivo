-- name: ListSettings :many
SELECT key, value FROM setting;

-- name: UpsertSetting :exec
INSERT INTO setting (key, value) VALUES (?, ?)
ON CONFLICT (key) DO UPDATE SET value = excluded.value;

-- name: DeleteSetting :exec
DELETE FROM setting WHERE key = ?;

-- name: Setting :one
SELECT value FROM setting WHERE key = ?;

-- name: InsertSettingIfAbsent :exec
INSERT INTO setting (key, value) VALUES (?, ?)
ON CONFLICT (key) DO NOTHING;
