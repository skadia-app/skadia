-- Add metadata column for GDPR export job results (sha256, size, file_path)
ALTER TABLE background_jobs ADD COLUMN IF NOT EXISTS metadata JSONB;

-- Add updated_at for stale-job detection (heartbeat); nullable — NULL means no heartbeat set yet
ALTER TABLE background_jobs ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;

-- Set updated_at for existing completed/failed jobs to their finished_at
UPDATE background_jobs SET updated_at = COALESCE(finished_at, started_at) WHERE updated_at IS NULL;

-- Mark any stale running/queued jobs as failed (pod crash during migration)
UPDATE background_jobs SET status = 'failed', error = 'stale: cleared during migration 000289', updated_at = NOW(), finished_at = NOW()
WHERE status IN ('queued', 'running');
