-- name: InsertWork :one
INSERT INTO work (id, kind, parent_id, root_id, title, sort_title, year, number, parent_number, release_date,
  extra_type, edition, provisional, added_at)
VALUES (@id, @kind, @parent_id, @root_id, @title, @sort_title, @year, @number, @parent_number, @release_date,
  @extra_type, @edition, @provisional, @added_at)
RETURNING rid;

-- name: GetWork :one
SELECT * FROM work WHERE id = @id;

-- name: EpisodeByExternalID :one
-- The episode of a series an external ID names: IDs are looked up within
-- their own root work.
SELECT x.work_id FROM work_external_id x JOIN work w ON w.id = x.work_id
WHERE x.provider = @provider AND x.kind = @kind AND x.ext_id = @ext_id AND w.root_id = @root_id AND w.kind = 'episode'
ORDER BY w.rid LIMIT 1;

-- name: InsertExternalID :exec
INSERT INTO work_external_id (work_id, provider, kind, ext_id) VALUES (@work_id, @provider, @kind, @ext_id);

-- name: ExternalIDs :many
SELECT provider, kind, ext_id FROM work_external_id WHERE work_id = @work_id ORDER BY provider, kind;

-- name: ChildSeason :one
SELECT id FROM work WHERE parent_id = @parent_id AND kind = 'season' AND number = @number;

-- name: ChildEpisode :one
-- A matched episode never adopts a provisional one: identify re-points
-- the provisional episode's versions and merges it into the matched one.
SELECT id FROM work WHERE parent_id = @parent_id AND kind = 'episode' AND number = @number
  AND provisional <= @provisional
ORDER BY provisional LIMIT 1;

-- name: UpdateWorkMetadata :exec
UPDATE work SET title = @title, sort_title = @sort_title, original_title = @original_title, original_language = @original_language,
  alt_titles = @alt_titles, year = @year, overview = @overview, release_date = @release_date,
  content_rating = @content_rating, content_rating_age = @content_rating_age, community_rating = @community_rating, community_votes = @community_votes,
  provisional = 0
WHERE id = @id;

-- name: SetLockedFields :exec
UPDATE work SET locked_fields = @locked_fields WHERE id = @id;

-- name: BumpMatchRev :exec
UPDATE work SET match_rev = match_rev + 1 WHERE id IN (SELECT unhex(value) FROM (SELECT CAST(@ids AS TEXT) AS ids), json_each(ids));

-- name: SetOrdering :execrows
UPDATE work SET ordering = @ordering, match_rev = match_rev + 1 WHERE id = @id AND kind = 'series';

-- name: SetEdition :exec
UPDATE work SET edition = @edition WHERE id = @id;

-- name: MoveEpisode :exec
UPDATE work SET parent_id = @parent_id, parent_number = @parent_number, number = @number, match_rev = match_rev + 1
WHERE id = @id;

-- name: DescribeWork :exec
-- A root work as a match describes it.
UPDATE work SET title = @title, sort_title = @sort_title, year = @year, provisional = 0, locked = locked OR @locked,
  match_rev = match_rev + 1
WHERE id = @id;

-- name: DeleteTreeExternalIDs :exec
-- The external IDs of the seasons and episodes under a series.
DELETE FROM work_external_id WHERE work_id IN (SELECT id FROM work WHERE root_id = @root_id AND id <> @root_id);

-- name: BumpTreeMatchRev :exec
UPDATE work SET match_rev = match_rev + 1 WHERE root_id = @root_id AND id <> @root_id;

-- name: UpsertGenre :one
INSERT INTO genre (name) VALUES (@name) ON CONFLICT (name) DO UPDATE SET name = excluded.name RETURNING id;

-- name: DeleteWorkGenres :exec
DELETE FROM work_genre WHERE work_id = @work_id;

-- name: InsertWorkGenre :exec
INSERT INTO work_genre (work_id, genre_id) VALUES (@work_id, @genre_id) ON CONFLICT DO NOTHING;

-- name: UpsertTag :one
INSERT INTO tag (name) VALUES (@name) ON CONFLICT (name) DO UPDATE SET name = excluded.name RETURNING id;

-- name: DeleteWorkTags :exec
DELETE FROM work_tag WHERE work_id = @work_id;

-- name: InsertWorkTag :exec
INSERT INTO work_tag (work_id, tag_id) VALUES (@work_id, @tag_id) ON CONFLICT DO NOTHING;

-- name: UpsertCompany :one
INSERT INTO company (name) VALUES (@name) ON CONFLICT (name) DO UPDATE SET name = excluded.name RETURNING id;

-- name: DeleteWorkCompanies :exec
DELETE FROM work_company WHERE work_id = @work_id;

-- name: InsertWorkCompany :exec
INSERT INTO work_company (work_id, company_id) VALUES (@work_id, @company_id) ON CONFLICT DO NOTHING;

-- People are told apart by their external IDs, then by name (catalog's
-- resolvePerson).

-- name: PersonByExternalID :one
SELECT person_id FROM person_external_id WHERE provider = @provider AND kind = @kind AND ext_id = @ext_id;

-- name: PeopleNamed :many
-- Everyone with this name, oldest first.
SELECT id FROM person WHERE name = @name ORDER BY rid;

-- name: PersonExternalIDs :many
SELECT provider, kind, ext_id FROM person_external_id WHERE person_id = @person_id ORDER BY provider, kind, ext_id;

-- name: InsertPerson :exec
INSERT INTO person (id, name, sort_name) VALUES (@id, @name, @sort_name);

-- name: InsertPersonExternalID :exec
-- An ID another person has stays theirs.
INSERT INTO person_external_id (provider, kind, ext_id, person_id) VALUES (@provider, @kind, @ext_id, @person_id)
ON CONFLICT DO NOTHING;

-- name: DeleteCredits :exec
DELETE FROM credit WHERE work_id = @work_id;

-- name: InsertCredit :exec
INSERT INTO credit (work_id, person_id, role, ord, character) VALUES (@work_id, @person_id, @role, @ord, @character);

-- name: TrailerKeys :many
SELECT site, key FROM trailer WHERE work_id = @work_id;

-- name: PutTrailer :exec
-- Records a work's trailer. A video the work already lists keeps its row,
-- and so its thumbnail.
INSERT INTO trailer (id, work_id, ord, site, key, title, language, official, published_at)
VALUES (@id, @work_id, @ord, @site, @key, @title, @language, @official, @published_at)
ON CONFLICT (work_id, site, key) DO UPDATE SET ord = excluded.ord, title = excluded.title, language = excluded.language,
  official = excluded.official, published_at = excluded.published_at;

-- name: DeleteTrailer :exec
DELETE FROM trailer WHERE work_id = @work_id AND site = @site AND key = @key;

-- name: DeleteTrailers :exec
DELETE FROM trailer WHERE work_id = @work_id;

-- name: CreditNames :many
SELECT p.name FROM credit c JOIN person p ON p.id = c.person_id WHERE c.work_id = @work_id ORDER BY c.role, c.ord;

-- name: UpsertCollection :one
INSERT INTO collection (id, key, name, sort_name, created_at) VALUES (@id, @key, @name, @sort_name, @created_at)
ON CONFLICT (key) DO UPDATE SET name = excluded.name, sort_name = excluded.sort_name RETURNING id;

-- name: LeaveCollections :exec
-- Remove a work from every collection whose key has the prefix (tmdb:).
DELETE FROM collection_work WHERE work_id = @work_id
  AND collection_id IN (SELECT id FROM collection WHERE key LIKE @prefix || '%');

-- name: JoinCollection :exec
INSERT INTO collection_work (collection_id, work_id, ord) VALUES (@collection_id, @work_id, @ord)
ON CONFLICT DO UPDATE SET ord = excluded.ord;

-- Unmatching works.

-- name: UnmatchTargets :many
-- A movie or series, and the seasons and episodes under it.
SELECT w.id, w.kind, w.title, w.number, w.parent_number, w.release_date FROM work w
WHERE w.root_id = @work_id AND w.kind <> 'extra';

-- name: ResetWork :exec
-- Leaves a work with only what its files' names say.
UPDATE work SET title = @title, sort_title = @sort_title, year = @year, release_date = @release_date,
  original_title = NULL, original_language = NULL, alt_titles = NULL, overview = NULL, content_rating = NULL, content_rating_age = NULL,
  community_rating = NULL, community_votes = NULL, locked_fields = NULL,
  provisional = @provisional, locked = @locked, match_rev = match_rev + 1
WHERE id = @id;

-- name: DeleteExternalIDs :exec
DELETE FROM work_external_id WHERE work_id = @work_id;

-- name: LeaveAllCollections :exec
DELETE FROM collection_work WHERE work_id = @work_id;

-- Merging an episode into the one that replaced it.

-- name: MoveVersionWorks :exec
UPDATE OR IGNORE media_version_work SET work_id = @into WHERE work_id = @from;

-- name: DropVersionWorks :exec
DELETE FROM media_version_work WHERE work_id = @work_id;

-- name: DeleteWork :exec
DELETE FROM work WHERE id = @id;

-- name: EmptyWorks :many
-- Works with nothing left: no version and no child. A parent
-- becomes empty once its last child is deleted.
SELECT w.id, w.rid FROM work w
WHERE NOT EXISTS (SELECT 1 FROM media_version_work vw WHERE vw.work_id = w.id)
  AND NOT EXISTS (SELECT 1 FROM work c WHERE c.parent_id = w.id)
LIMIT @limit;
