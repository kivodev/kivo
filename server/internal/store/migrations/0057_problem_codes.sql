-- activity -------------------------------------------------------------------
-- A problem's code is camelCase, as every documented value the API sends
-- is, and its ID is its component and code: problemRaised events keep
-- both as they were raised.

UPDATE activity
SET data = json_set(data, '$.code', CASE json_extract(data, '$.code')
  WHEN 'folder_unreachable' THEN 'folderUnreachable'
  WHEN 'cache_full' THEN 'cacheFull'
  WHEN 'gpu_failed' THEN 'gpuFailed'
  WHEN 'gpu_unusable' THEN 'gpuUnusable'
  WHEN 'no_accelerator' THEN 'noAccelerator'
  WHEN 'accelerator_missing' THEN 'acceleratorMissing'
  WHEN 'cpu_failed' THEN 'cpuFailed'
  WHEN 'sign_ins_failed' THEN 'signInsFailed'
  WHEN 'opensubtitles_account_rejected' THEN 'openSubtitlesAccountRejected'
  WHEN 'opensubtitles_key_rejected' THEN 'openSubtitlesKeyRejected'
  WHEN 'opensubtitles_unavailable' THEN 'openSubtitlesUnavailable'
  WHEN 'symlink_skipped' THEN 'symlinkSkipped'
  WHEN 'watch_limit' THEN 'watchLimit'
  WHEN 'database_not_local' THEN 'databaseNotLocal'
  ELSE json_extract(data, '$.code') END)
WHERE kind = 'problemRaised';

-- A metadata provider's problems are named for the provider: tmdb_key_rejected
-- is tmdbKeyRejected, and tvdb_unavailable tvdbUnavailable.
UPDATE activity
SET data = json_set(data, '$.code', CASE
  WHEN substr(json_extract(data, '$.code'), -13) = '_key_rejected'
    THEN substr(json_extract(data, '$.code'), 1, length(json_extract(data, '$.code')) - 13) || 'KeyRejected'
  ELSE substr(json_extract(data, '$.code'), 1, length(json_extract(data, '$.code')) - 12) || 'Unavailable' END)
WHERE kind = 'problemRaised' AND substr(json_extract(data, '$.problem'), 1, 9) = 'metadata.'
  AND (substr(json_extract(data, '$.code'), -13) = '_key_rejected' OR substr(json_extract(data, '$.code'), -12) = '_unavailable');

UPDATE activity
SET data = json_set(data, '$.problem',
  substr(json_extract(data, '$.problem'), 1, instr(json_extract(data, '$.problem'), '.')) || json_extract(data, '$.code'))
WHERE kind = 'problemRaised' AND instr(json_extract(data, '$.problem'), '.') > 0;
