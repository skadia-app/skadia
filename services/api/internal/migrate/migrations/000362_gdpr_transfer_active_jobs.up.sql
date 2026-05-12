WITH ranked_active_transfers AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY user_id
            ORDER BY
                CASE status
                    WHEN 'running' THEN 0
                    WHEN 'uploading' THEN 1
                    WHEN 'queued' THEN 2
                    ELSE 3
                END,
                updated_at DESC,
                started_at DESC,
                id
        ) AS rn
    FROM background_jobs
    WHERE job_type IN ('gdpr_export', 'data_import')
      AND status IN ('uploading', 'queued', 'running')
)
UPDATE background_jobs AS j
SET status = 'failed',
    error = COALESCE(NULLIF(j.error, ''), 'superseded_by_active_gdpr_transfer_migration'),
    finished_at = COALESCE(j.finished_at, NOW()),
    updated_at = NOW()
FROM ranked_active_transfers AS r
WHERE j.id = r.id
  AND r.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_bg_jobs_one_gdpr_transfer_active
    ON background_jobs (user_id)
    WHERE job_type IN ('gdpr_export', 'data_import')
      AND status IN ('uploading', 'queued', 'running');
