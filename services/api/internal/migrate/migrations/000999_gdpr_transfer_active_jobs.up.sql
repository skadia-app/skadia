CREATE UNIQUE INDEX IF NOT EXISTS idx_bg_jobs_one_gdpr_transfer_active
    ON background_jobs (user_id)
    WHERE job_type IN ('gdpr_export', 'data_import')
      AND status IN ('uploading', 'queued', 'running');
