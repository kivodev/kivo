-- auth -----------------------------------------------------------------------
-- The server proves itself to a probe with its own key, so a session has no
-- probe ID.
DROP INDEX session_probe;
ALTER TABLE session DROP COLUMN probe_id;
