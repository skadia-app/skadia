package config

import "testing"

func TestLoadAgentDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://skadia:secret@localhost:5432/skadia_test_abcdef123456?sslmode=disable")
	t.Setenv("READONLY_DATABASE_URL", "postgres://skadia_readonly:secret@localhost:5432/skadia_test_abcdef123456?sslmode=disable")
	t.Setenv("LLM_MODEL", "")
	t.Setenv("LLM_CONTEXT_WINDOW", "")
	t.Setenv("LLM_ENCRYPTION_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.ReadonlyDatabaseURL == "" {
		t.Fatal("ReadonlyDatabaseURL is empty")
	}
	if cfg.LLMModel != "skadia-default" {
		t.Fatalf("LLMModel = %q, want skadia-default", cfg.LLMModel)
	}
	if cfg.LLMContextWindow != 8000 {
		t.Fatalf("LLMContextWindow = %d, want 8000", cfg.LLMContextWindow)
	}
}
