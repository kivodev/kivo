-- Books: the book side table, authors and narrators, and series.

-- name: GetBook :one
SELECT * FROM book WHERE work_id = @work_id;

-- name: PutBook :exec
INSERT INTO book (work_id, subtitle, language)
VALUES (@work_id, @subtitle, @language)
ON CONFLICT (work_id) DO UPDATE SET subtitle = excluded.subtitle, language = excluded.language;

-- name: DeleteRoleCredits :exec
DELETE FROM credit WHERE work_id = @work_id AND role = @role;

-- name: FirstAuthor :one
SELECT p.name FROM credit c JOIN person p ON p.id = c.person_id
WHERE c.work_id = @work_id AND c.role = 'author' ORDER BY c.ord LIMIT 1;

-- name: DeleteBookExternalIDs :exec
DELETE FROM work_external_id WHERE work_id = @work_id AND provider = @provider AND kind = @kind;

-- name: WorkSeries :many
SELECT series_id, position_label, is_primary FROM book_series_book WHERE work_id = @work_id ORDER BY is_primary DESC, series_id;

-- name: LeaveSeries :exec
DELETE FROM book_series_book WHERE work_id = @work_id;

-- name: UpsertSeries :one
-- A series by its key, made if it's new. An existing one keeps its name, so
-- a series renamed by an admin stays renamed when its folder is scanned.
INSERT INTO book_series (id, key, name, sort_name, created_at) VALUES (@id, @key, @name, @sort_name, @created_at)
ON CONFLICT (key) DO UPDATE SET key = excluded.key RETURNING id;

-- name: SeriesByName :one
-- The oldest series with this name, other than one (none when NULL).
SELECT id FROM book_series WHERE name = @name AND coalesce(id <> @other, 1) ORDER BY created_at, id LIMIT 1;

-- name: GetSeries :one
SELECT * FROM book_series WHERE id = @id;

-- name: JoinSeries :exec
INSERT INTO book_series_book (series_id, work_id, position_label, position_num, is_primary)
VALUES (@series_id, @work_id, @position_label, @position_num, @is_primary)
ON CONFLICT DO NOTHING;

-- name: DeleteEmptySeries :exec
DELETE FROM book_series WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids))
  AND NOT EXISTS (SELECT 1 FROM book_series_book m WHERE m.series_id = book_series.id);

-- Renaming and merging authors, narrators and series.

-- name: GetPerson :one
SELECT * FROM person WHERE id = @id;

-- name: PersonByName :one
-- The oldest person with this name, other than one.
SELECT id FROM person WHERE name = @name AND id <> @other ORDER BY rid LIMIT 1;

-- name: MovePersonExternalIDs :exec
-- A merged person's IDs move to the person they're merged into, unless
-- that person has the same one.
UPDATE OR IGNORE person_external_id SET person_id = @into WHERE person_id = @from;

-- name: RenamePerson :exec
UPDATE person SET name = @name, sort_name = @sort_name WHERE id = @id;

-- name: SetPersonOverview :exec
UPDATE person SET overview = @overview WHERE id = @id;

-- name: PersonWorks :many
-- The works a person is credited on, with each role and the work's kind,
-- for a rename.
SELECT DISTINCT c.work_id, c.role, w.kind FROM credit c JOIN work w ON w.id = c.work_id
WHERE c.person_id = @person_id ORDER BY c.work_id, c.role;

-- name: PersonRoleCredits :many
SELECT ord FROM credit WHERE work_id = @work_id AND role = @role AND person_id = @person_id ORDER BY ord;

-- name: CreditedAs :one
SELECT EXISTS (SELECT 1 FROM credit WHERE work_id = @work_id AND role = @role AND person_id = @person_id);

-- name: DeleteCredit :exec
DELETE FROM credit WHERE work_id = @work_id AND role = @role AND ord = @ord;

-- name: MoveCredit :exec
UPDATE credit SET person_id = @person_id WHERE work_id = @work_id AND role = @role AND ord = @ord;

-- name: DeletePerson :exec
-- Only once nothing credits them.
DELETE FROM person WHERE id = @id AND NOT EXISTS (SELECT 1 FROM credit c WHERE c.person_id = person.id);

-- name: RenameSeries :exec
UPDATE book_series SET name = @name, sort_name = @sort_name WHERE id = @id;

-- name: SeriesMemberships :many
SELECT work_id, position_label, position_num, is_primary FROM book_series_book WHERE series_id = @series_id ORDER BY work_id;

-- name: InSeries :one
SELECT EXISTS (SELECT 1 FROM book_series_book WHERE series_id = @series_id AND work_id = @work_id);

-- name: LeaveOneSeries :exec
DELETE FROM book_series_book WHERE series_id = @series_id AND work_id = @work_id;

