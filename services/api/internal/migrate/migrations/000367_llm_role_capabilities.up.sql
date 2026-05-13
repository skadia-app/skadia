ALTER TABLE llm_system_config
  ADD COLUMN IF NOT EXISTS capability_flags TEXT[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS eval_status TEXT NOT NULL DEFAULT 'untested',
  ADD COLUMN IF NOT EXISTS eval_report JSONB NOT NULL DEFAULT '{}'::jsonb;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'llm_system_config_eval_status_check'
      AND conrelid = 'llm_system_config'::regclass
  ) THEN
    ALTER TABLE llm_system_config
      ADD CONSTRAINT llm_system_config_eval_status_check
      CHECK (eval_status IN ('untested', 'passed', 'failed', 'waived'));
  END IF;
END $$;

INSERT INTO llm_system_config (
  role, backend_type, api_base, model_name, api_key_enc, enabled,
  capability_flags, eval_status, eval_report
)
SELECT
  'tool_agent', backend_type, api_base, model_name, api_key_enc, enabled,
  ARRAY['streaming_chat', 'tool_calls', 'json_args', 'german_answers', 'sql_readonly', 'write_preview']::text[],
  'untested',
  '{}'::jsonb
FROM llm_system_config
WHERE role = 'default'
  AND enabled
  AND (
    api_key_enc IS NOT NULL
    OR backend_type IS DISTINCT FROM 'ollama'
    OR api_base IS DISTINCT FROM 'http://ollama:11434'
    OR model_name IS DISTINCT FROM 'qwen3:8b'
  )
ON CONFLICT (role) DO UPDATE SET
  backend_type = EXCLUDED.backend_type,
  api_base = EXCLUDED.api_base,
  model_name = EXCLUDED.model_name,
  api_key_enc = EXCLUDED.api_key_enc,
  enabled = EXCLUDED.enabled,
  capability_flags = EXCLUDED.capability_flags,
  updated_at = NOW()
WHERE NOT llm_system_config.enabled;

UPDATE llm_system_config
SET enabled = false,
    eval_status = 'untested',
    eval_report = '{}'::jsonb,
    updated_at = NOW()
WHERE api_key_enc IS NULL
  AND (
    role = 'default' AND backend_type = 'ollama' AND api_base = 'http://ollama:11434' AND model_name = 'qwen3:8b'
  );

INSERT INTO llm_system_config (role, backend_type, enabled, capability_flags, eval_status, eval_report)
VALUES
  ('tool_agent', 'disabled', false, ARRAY['streaming_chat', 'tool_calls', 'json_args', 'german_answers', 'sql_readonly', 'write_preview']::text[], 'untested', '{}'::jsonb),
  ('document_extract', 'disabled', false, ARRAY['document_extract', 'json_extract', 'long_context']::text[], 'untested', '{}'::jsonb),
  ('markdown_extract', 'disabled', false, ARRAY['markdown_extract', 'table_preservation']::text[], 'untested', '{}'::jsonb),
  ('vision', 'disabled', false, ARRAY['vision']::text[], 'untested', '{}'::jsonb)
ON CONFLICT (role) DO NOTHING;
