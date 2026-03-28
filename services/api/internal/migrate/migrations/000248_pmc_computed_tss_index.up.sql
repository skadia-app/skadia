CREATE INDEX IF NOT EXISTS idx_activities_computed_tss_daily
  ON activities (user_id, started_at)
  WHERE computed_tss IS NOT NULL;
