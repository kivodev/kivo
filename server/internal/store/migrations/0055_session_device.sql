-- auth -----------------------------------------------------------------------
-- A session keeps the app and device it was signed in from, as the app's
-- requests identify them, and takes a changed one from a later request.
ALTER TABLE session ADD COLUMN app TEXT NOT NULL DEFAULT '';
ALTER TABLE session ADD COLUMN app_version TEXT NOT NULL DEFAULT '';
ALTER TABLE session ADD COLUMN os_version TEXT NOT NULL DEFAULT '';
ALTER TABLE session ADD COLUMN device_model TEXT NOT NULL DEFAULT '';
