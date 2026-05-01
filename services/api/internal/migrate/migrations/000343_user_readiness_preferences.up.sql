CREATE TABLE IF NOT EXISTS user_readiness_preferences (
    user_id          UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    preferred_source TEXT
        CHECK (preferred_source IS NULL OR preferred_source IN ('manufacturer','skadia')),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DELETE FROM analytics_cache WHERE cache_key = 'readiness_v2';
-- Also purge legacy rows written by main-branch code with the old shape
-- ({score, level, factors{name,value,status}}, expires_at=NULL).
-- These would be served verbatim by GetReadiness after deploy — wrong schema.
DELETE FROM analytics_cache WHERE cache_key = 'readiness';

-- Note: audit-log INSERT is non-idempotent; re-running the migration appends a row.
INSERT INTO auth_audit_log (user_id, event_type, details)
SELECT NULL, 'migration', jsonb_build_object(
  'migration', '000343', 'name', 'user_readiness_preferences'
);
