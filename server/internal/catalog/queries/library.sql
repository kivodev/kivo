-- name: InsertLibrary :exec
INSERT INTO library (id, name, kind, use_local_metadata, previews, markers, subtitles, refresh_days, metadata_language,
  content_rating_country, created_at)
VALUES (@id, @name, @kind, @use_local_metadata, @previews, @markers, @subtitles, @refresh_days, @metadata_language,
  @content_rating_country, @created_at);

-- name: UpdateLibrary :execrows
UPDATE library SET name = @name, use_local_metadata = @use_local_metadata, previews = @previews, markers = @markers,
  subtitles = @subtitles, refresh_days = @refresh_days, metadata_language = @metadata_language, content_rating_country = @content_rating_country
WHERE id = @id;

-- name: GetLibrary :one
SELECT * FROM library WHERE id = @id;

-- name: ListLibraries :many
-- Every library but those being deleted.
SELECT * FROM library WHERE deleting_at IS NULL ORDER BY created_at, id;

-- name: InsertFolder :exec
INSERT INTO library_folder (id, library_id, path) VALUES (@id, @library_id, @path);

-- name: ListFolders :many
-- A library's folders, but those being removed.
SELECT * FROM library_folder WHERE library_id = @library_id AND removing_at IS NULL ORDER BY path;

-- name: DeletingLibraries :many
SELECT id FROM library WHERE deleting_at IS NOT NULL ORDER BY deleting_at, id;

-- name: RemovingFolders :many
-- The folders being removed from libraries that aren't being deleted.
SELECT r.id FROM library_folder r JOIN library l ON l.id = r.library_id
WHERE r.removing_at IS NOT NULL AND l.deleting_at IS NULL
ORDER BY r.removing_at, r.id;

-- name: AllFolders :many
-- A library's folders, those being removed too.
SELECT * FROM library_folder WHERE library_id = @library_id ORDER BY path;

-- name: MarkFolderRemoving :exec
UPDATE library_folder SET removing_at = @now WHERE id = @id AND removing_at IS NULL;

-- name: MarkLibraryDeleting :execrows
-- A library being deleted, and every folder of it being removed.
UPDATE library SET deleting_at = @now WHERE id = @id AND deleting_at IS NULL;

-- name: MarkLibraryFoldersRemoving :exec
UPDATE library_folder SET removing_at = @now WHERE library_id = @library_id AND removing_at IS NULL;

-- name: RemovingFolderFiles :many
-- At most @limit files of a folder being removed: one batch of its deletion.
SELECT f.id FROM media_file f
JOIN library_folder r ON r.id = f.folder_id AND r.removing_at IS NOT NULL
WHERE f.folder_id = @folder_id
LIMIT @limit;

-- name: CountFolderFiles :one
SELECT count(*) FROM media_file WHERE folder_id = @folder_id;

-- name: DeleteRemovedFolder :execrows
-- The last step of removing a folder, once its files are deleted.
DELETE FROM library_folder AS d
WHERE d.id = @id AND d.removing_at IS NOT NULL
  AND EXISTS (SELECT 1 FROM media_file f WHERE f.folder_id = d.id) = 0;

-- name: DeleteDeletedLibrary :execrows
-- The last step of deleting a library, once its folders are removed.
DELETE FROM library AS d
WHERE d.id = @id AND d.deleting_at IS NOT NULL
  AND EXISTS (SELECT 1 FROM library_folder r WHERE r.library_id = d.id) = 0;

-- name: GetFolder :one
SELECT r.*, l.deleting_at FROM library_folder r JOIN library l ON l.id = r.library_id WHERE r.id = @id;

-- name: FolderActive :one
-- Whether a folder takes new files: it's neither being removed nor in a
-- library being deleted.
SELECT EXISTS (SELECT 1 FROM library_folder r JOIN library l ON l.id = r.library_id
  WHERE r.id = @id AND r.removing_at IS NULL AND l.deleting_at IS NULL);
