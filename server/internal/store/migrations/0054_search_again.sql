-- identify -------------------------------------------------------------------
-- A folder found by search is matched again under the matching rules: a
-- release year more than two years from the folder's, or no release year
-- when the folder gives one, isn't accepted, and a different sequel number
-- disqualifies. Only a decision those rules could change is made again: one
-- that scored under 100, or whose match has no year or another year than
-- the folder's. Its files are identified again by the next scan, which
-- searches afresh.

CREATE TEMP TABLE search_again AS
SELECT library_id, subject FROM match
WHERE source = 'search' AND status = 'matched'
  AND (score < 100 OR year IS NULL
    OR (substr(query, instr(query, '|') + 1) <> '0'
      AND year <> CAST(substr(query, instr(query, '|') + 1) AS INTEGER)));

-- A subject is a folder, or for a loose movie its folder, its title key and
-- its year joined by '|', with an empty folder for a library folder's top.
UPDATE media_file SET identified = NULL
WHERE id IN (
  SELECT f.id FROM search_again a
  JOIN library_folder r ON r.library_id = a.library_id
  JOIN media_file f ON f.folder_id = r.id
  WHERE CASE WHEN instr(a.subject, '|') = 0
    THEN f.rel_path LIKE a.subject || '/%'
    ELSE CASE WHEN substr(a.subject, 1, instr(a.subject, '|') - 1) = ''
      THEN f.rel_path NOT LIKE '%/%'
      ELSE f.rel_path LIKE substr(a.subject, 1, instr(a.subject, '|') - 1) || '/%'
        AND f.rel_path NOT LIKE substr(a.subject, 1, instr(a.subject, '|') - 1) || '/%/%'
    END
  END);

DELETE FROM match WHERE (library_id, subject) IN (SELECT library_id, subject FROM search_again);

DROP TABLE search_again;
