DELETE FROM llm_system_config
WHERE role IN ('tool_agent', 'document_extract', 'markdown_extract', 'vision')
  AND backend_type = 'disabled'
  AND enabled = false
  AND api_base IS NULL
  AND model_name IS NULL
  AND api_key_enc IS NULL
  AND eval_status = 'untested'
  AND eval_report = '{}'::jsonb
  AND (
    (role = 'tool_agent' AND capability_flags = ARRAY['streaming_chat', 'tool_calls', 'json_args', 'german_answers', 'sql_readonly', 'write_preview']::text[])
    OR (role = 'document_extract' AND capability_flags = ARRAY['document_extract', 'json_extract', 'long_context']::text[])
    OR (role = 'markdown_extract' AND capability_flags = ARRAY['markdown_extract', 'table_preservation']::text[])
    OR (role = 'vision' AND capability_flags = ARRAY['vision']::text[])
  );

UPDATE llm_system_config
SET enabled = true,
    updated_at = NOW()
WHERE api_key_enc IS NULL
  AND (
    role = 'default' AND backend_type = 'ollama' AND api_base = 'http://ollama:11434' AND model_name = 'qwen3:8b'
  );

ALTER TABLE llm_system_config
  DROP CONSTRAINT IF EXISTS llm_system_config_eval_status_check,
  DROP COLUMN IF EXISTS eval_report,
  DROP COLUMN IF EXISTS eval_status,
  DROP COLUMN IF EXISTS capability_flags;
