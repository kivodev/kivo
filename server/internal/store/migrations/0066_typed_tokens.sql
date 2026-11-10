-- auth -----------------------------------------------------------------------
-- A session, API key or cast token made before tokens carried their kind
-- can never authenticate, and app sessions and API keys are never pruned,
-- so all three are cleared. Every device signs in again.
DELETE FROM cast_token;
DELETE FROM api_key;
DELETE FROM session;
