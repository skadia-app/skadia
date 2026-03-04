-- System-Konfiguration (eine Zeile pro Modell-Rolle)
CREATE TABLE IF NOT EXISTS llm_system_config (
    role         TEXT PRIMARY KEY,
    -- 'default' | 'vlm' | 'vlm_fallback' | 'analysis'
    backend_type TEXT NOT NULL DEFAULT 'disabled',
    -- 'ollama' | 'llama_server' | 'openai' | 'anthropic' | 'disabled'
    api_base     TEXT,
    model_name   TEXT,
    api_key_enc  TEXT,   -- AES-256-GCM verschluesselt, Base64
    enabled      BOOLEAN NOT NULL DEFAULT false,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Initiale Defaults aus aktueller config.yaml
INSERT INTO llm_system_config (role, backend_type, api_base, model_name, enabled) VALUES
    ('default',      'ollama', 'http://ollama:11434', 'qwen3:8b',    true),
    ('vlm',          'ollama', 'http://ollama:11434', 'qwen3-vl:8b', true),
    ('vlm_fallback', 'ollama', 'http://ollama:11434', 'qwen3-vl:4b', true),
    ('analysis',     'disabled', NULL, NULL,                         false)
ON CONFLICT DO NOTHING;

-- User-Overrides (nur Modell-Auswahl)
CREATE TABLE IF NOT EXISTS llm_user_config (
    user_id    TEXT NOT NULL,
    role       TEXT NOT NULL,
    model_name TEXT,
    PRIMARY KEY (user_id, role)
);
