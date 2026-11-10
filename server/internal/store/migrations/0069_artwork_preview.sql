-- metadata -------------------------------------------------------------------
-- The URL of a smaller copy of offered provider artwork, which its preview
-- is made from, or NULL to make it from origin.
ALTER TABLE artwork ADD COLUMN preview TEXT;
