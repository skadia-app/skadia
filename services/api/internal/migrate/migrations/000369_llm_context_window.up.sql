ALTER TABLE llm_system_config
  ADD COLUMN IF NOT EXISTS context_window INTEGER;

ALTER TABLE llm_system_config
  DROP CONSTRAINT IF EXISTS llm_system_config_context_window_check;

ALTER TABLE llm_system_config
  ADD CONSTRAINT llm_system_config_context_window_check
  CHECK (context_window IS NULL OR (context_window >= 1024 AND context_window <= 2000000));
