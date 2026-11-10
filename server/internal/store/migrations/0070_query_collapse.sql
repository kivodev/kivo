-- users ----------------------------------------------------------------------
-- A smart list's query has no collapse key; the rest of it stays as saved.
UPDATE list SET query = json_remove(query, '$.collapse')
WHERE json_valid(query) AND json_type(query, '$.collapse') IS NOT NULL;
