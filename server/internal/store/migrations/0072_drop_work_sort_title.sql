-- catalog --------------------------------------------------------------------
-- No read orders or finds works by their sort title alone, ignoring case:
-- a library's works are read in title order from work_library_title.
DROP INDEX work_sort_title;
