-- catalog --------------------------------------------------------------------
-- A movie's trailer on YouTube, from its metadata, or NULL with none.
ALTER TABLE work ADD COLUMN trailer_url TEXT;
