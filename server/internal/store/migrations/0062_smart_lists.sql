-- users ----------------------------------------------------------------------
-- A smart list's rules: a WorkQuery as its JSON wire form, or NULL for a
-- manual list. A list's kind is fixed by this being set at creation.
ALTER TABLE list ADD COLUMN query TEXT;
