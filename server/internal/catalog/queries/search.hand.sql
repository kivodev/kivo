-- Hand-written: sqlc cannot parse FTS5. Loaded by search.go. The indexes are
-- contentless, so a row is deleted and rewritten whenever what it indexes
-- changes: a work's titles or credits, a person's or collection's name.

-- name: DeleteSearch :exec
DELETE FROM work_fts WHERE rowid = @rid;

-- name: InsertSearch :exec
INSERT INTO work_fts (rowid, title, alt_titles, people) VALUES (@rid, @title, @alt_titles, @people);

-- name: DeletePersonSearch :exec
DELETE FROM person_fts WHERE rowid = (SELECT rid FROM person WHERE id = @id);

-- name: InsertPersonSearch :exec
INSERT INTO person_fts (rowid, name) SELECT rid, name FROM person WHERE id = @id;

-- name: InsertCreditedSearch :exec
-- The people credited on a work who aren't in the index yet: those just made.
INSERT INTO person_fts (rowid, name)
SELECT DISTINCT p.rid, p.name FROM credit c JOIN person p ON p.id = c.person_id
WHERE c.work_id = @work_id AND NOT EXISTS (SELECT 1 FROM person_fts f WHERE f.rowid = p.rid);

-- name: DeleteCollectionSearch :exec
DELETE FROM collection_fts WHERE rowid = (SELECT rid FROM collection WHERE id = @id);

-- name: InsertCollectionSearch :exec
INSERT INTO collection_fts (rowid, name) SELECT rid, @name FROM collection WHERE id = @id;
