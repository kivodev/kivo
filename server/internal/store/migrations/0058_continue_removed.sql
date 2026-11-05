-- userstate ------------------------------------------------------------------
-- When the user removed a work from Continue Watching or Continue Listening:
-- a series for its episodes, the work itself otherwise. A later play shows
-- it again.
ALTER TABLE user_state ADD COLUMN continue_removed_at INTEGER;
