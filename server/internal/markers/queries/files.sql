-- Reads of catalog's tables. Readings live in cache/; the tables markers
-- writes are in markers.sql.

-- name: FileGeneration :one
SELECT generation FROM media_file WHERE id = @id;

-- name: FileGenerations :many
SELECT id, generation FROM media_file WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));
