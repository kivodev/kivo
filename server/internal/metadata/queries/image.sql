-- name: WorkImages :many
SELECT * FROM image WHERE work_id = @work_id ORDER BY kind;

-- name: GetImage :one
SELECT * FROM image WHERE id = @id;

-- name: InsertImage :exec
INSERT INTO image (id, work_id, kind, source, origin, origin_mtime, width, height, blurhash, chosen, fetched_at)
VALUES (@id, @work_id, @kind, @source, @origin, @origin_mtime, @width, @height, @blurhash, @chosen, @fetched_at);

-- name: PersonPhoto :one
-- The photo of the person an external ID names.
SELECT i.* FROM person_external_id x
JOIN image i ON i.person_id = x.person_id AND i.kind = 'photo'
WHERE x.provider = @provider AND x.kind = @kind AND x.ext_id = @ext_id;

-- name: InsertPersonImage :exec
INSERT INTO image (id, person_id, kind, source, origin, width, height, blurhash, fetched_at)
VALUES (@id, @person_id, @kind, @source, @origin, @width, @height, @blurhash, @fetched_at);

-- name: DeletePersonImages :many
DELETE FROM image WHERE person_id = @person_id AND kind = @kind RETURNING *;

-- name: CollectionImages :many
-- The artwork of the collection a key names.
SELECT i.* FROM collection c
JOIN image i ON i.collection_id = c.id
WHERE c.key = @key
ORDER BY i.kind;

-- name: CollectionOf :one
SELECT id FROM collection WHERE key = @key;

-- name: InsertCollectionImage :exec
INSERT INTO image (id, collection_id, kind, source, origin, width, height, blurhash, fetched_at)
VALUES (@id, @collection_id, @kind, @source, @origin, @width, @height, @blurhash, @fetched_at);

-- name: DeleteCollectionImages :many
DELETE FROM image WHERE collection_id = @collection_id AND kind = @kind RETURNING *;

-- name: TrailerImages :many
-- The thumbnails of a work's trailers.
SELECT i.* FROM trailer t
JOIN image i ON i.trailer_id = t.id
WHERE t.work_id = @work_id;

-- name: TrailerThumb :one
-- The thumbnail of a work's trailer, named by its video.
SELECT i.* FROM trailer t
JOIN image i ON i.trailer_id = t.id AND i.kind = 'thumb'
WHERE t.work_id = @work_id AND t.key = @key;

-- name: TrailerOf :one
SELECT id FROM trailer WHERE work_id = @work_id AND key = @key;

-- name: InsertTrailerImage :exec
INSERT INTO image (id, trailer_id, kind, source, origin, width, height, blurhash, fetched_at)
VALUES (@id, @trailer_id, @kind, @source, @origin, @width, @height, @blurhash, @fetched_at);

-- name: DeleteTrailerImages :many
DELETE FROM image WHERE trailer_id = @trailer_id AND kind = @kind RETURNING *;

-- name: ChooseImage :exec
UPDATE image SET chosen = 1 WHERE id = @id;

-- name: DeleteImage :exec
DELETE FROM image WHERE id = @id;

-- name: UnmatchedImages :many
-- The artwork of a movie or series and the seasons and episodes under it,
-- and the thumbnails of its trailers.
SELECT i.* FROM work w CROSS JOIN image i ON i.work_id = w.id
WHERE w.root_id = @work_id AND w.kind <> 'extra'
UNION ALL
SELECT i.* FROM trailer t CROSS JOIN image i ON i.trailer_id = t.id
WHERE t.work_id = @work_id;

-- name: PutArtwork :one
-- Records an image Choose artwork lists, keeping its ID if it was listed
-- before.
INSERT INTO artwork (id, work_id, kind, source, provider, origin, preview, folder, width, height, language, ord, listed_at)
VALUES (@id, @work_id, @kind, @source, @provider, @origin, @preview, @folder, @width, @height, @language, @ord, @listed_at)
ON CONFLICT (work_id, kind, origin) WHERE work_id IS NOT NULL DO UPDATE SET source = excluded.source, provider = excluded.provider,
  preview = excluded.preview, folder = excluded.folder, width = excluded.width, height = excluded.height, language = excluded.language,
  ord = excluded.ord, listed_at = excluded.listed_at
RETURNING id;

-- name: DropArtwork :many
-- The artwork of a work's kind the latest listing no longer offers.
DELETE FROM artwork WHERE work_id = @work_id AND kind = @kind AND listed_at < @listed_at
RETURNING id, kind;

-- name: GetArtwork :one
SELECT * FROM artwork WHERE id = @id;

-- name: KnownImages :many
-- The IDs among these that are images, for the prune of the image cache.
SELECT id FROM image WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));

-- name: KnownArtwork :many
-- The IDs among these that are artwork on offer, whose previews the image
-- cache keeps.
SELECT id FROM artwork WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));
