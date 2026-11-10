-- Reads of catalog's tables. Readings live in cache/; the tables markers
-- writes are in markers.sql.

-- name: FileGeneration :one
-- Read in the transaction that stores a file's markers, so they're stored
-- only for the generation they were found at.
SELECT generation FROM media_file WHERE id = @id;
