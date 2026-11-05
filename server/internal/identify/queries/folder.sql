-- Each root work's folder: work_folder, which identify owns.

-- name: FolderWork :one
SELECT work_id FROM work_folder WHERE library_id = @library_id AND subject = @subject;

-- name: FolderOf :one
SELECT library_id, subject FROM work_folder WHERE work_id = @work_id;

-- name: LinkFolder :exec
INSERT INTO work_folder (library_id, subject, work_id) VALUES (@library_id, @subject, @work_id);

-- name: UnlinkWork :exec
DELETE FROM work_folder WHERE work_id = @work_id;
