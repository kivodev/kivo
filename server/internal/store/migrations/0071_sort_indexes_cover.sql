-- catalog --------------------------------------------------------------------
-- The sorts' indexes hold is_root too. A read of a library's top-level works
-- in a sort's order names that column, so with it in the index the read
-- takes each row from the index alone, and looks none up in the table.
DROP INDEX work_library_title;
DROP INDEX work_library_release;
DROP INDEX work_library_runtime;
DROP INDEX work_library_rating;
DROP INDEX work_library_creator;
DROP INDEX work_library_last_added;
DROP INDEX work_library_added;
CREATE INDEX work_library_title ON work_library (library_id, sort_title, work_id, is_root) WHERE is_root;
CREATE INDEX work_library_release ON work_library (library_id, release_key DESC, sort_title, work_id, is_root) WHERE is_root;
CREATE INDEX work_library_runtime ON work_library (library_id, runtime_sort, sort_title, work_id, is_root) WHERE is_root;
CREATE INDEX work_library_rating ON work_library (library_id, rating_sort DESC, sort_title, work_id, is_root) WHERE is_root;
CREATE INDEX work_library_creator ON work_library (library_id, creator_sort, sort_title, work_id, is_root) WHERE is_root;
CREATE INDEX work_library_last_added ON work_library (library_id, last_added_at DESC, sort_title, work_id, is_root) WHERE is_root;
CREATE INDEX work_library_added ON work_library (library_id, added_at DESC, sort_title, work_id, is_root) WHERE is_root;
