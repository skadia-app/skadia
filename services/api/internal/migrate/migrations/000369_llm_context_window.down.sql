ALTER TABLE llm_system_config
  DROP CONSTRAINT IF EXISTS llm_system_config_context_window_check;

ALTER TABLE llm_system_config
  DROP COLUMN IF EXISTS context_window;
