-- Nicht-default LLM-Rollen entfernen (vlm/vlm_fallback/analysis werden nicht mehr benoetigt)
-- Qwen3.5-9B ist nativ multimodal und uebernimmt alle Aufgaben ueber die 'default'-Rolle
DELETE FROM llm_system_config WHERE role IN ('vlm', 'vlm_fallback', 'analysis');
DELETE FROM llm_user_config WHERE role IN ('vlm', 'vlm_fallback', 'analysis');
