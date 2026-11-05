-- name: GetCached :one
SELECT body, fetched_at, expires_at FROM provider_cache WHERE provider = @provider AND key = @key;

-- name: PutCached :exec
INSERT INTO provider_cache (provider, key, body, fetched_at, expires_at)
VALUES (@provider, @key, @body, @fetched_at, @expires_at)
ON CONFLICT (provider, key) DO UPDATE SET
  body = excluded.body, fetched_at = excluded.fetched_at, expires_at = excluded.expires_at;

-- name: PruneCached :execrows
-- At most @limit of the entries long past expiry (anything younger may
-- still stand in for a provider that is down): one batch of the prune.
DELETE FROM provider_cache WHERE rowid IN (
  SELECT p.rowid FROM provider_cache p WHERE p.expires_at < @before LIMIT @limit);
