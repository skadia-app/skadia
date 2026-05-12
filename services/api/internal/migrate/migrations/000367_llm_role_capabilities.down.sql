DELETE FROM llm_system_config
WHERE role IN ('tool_agent', 'sql_readonly', 'write_preview', 'document_extract', 'markdown_extract', 'vision');

ALTER TABLE llm_system_config
  DROP CONSTRAINT IF EXISTS llm_system_config_eval_status_check,
  DROP COLUMN IF EXISTS eval_report,
  DROP COLUMN IF EXISTS eval_status,
  DROP COLUMN IF EXISTS capability_flags;
