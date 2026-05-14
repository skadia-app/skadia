package config

import (
	"os"
	"testing"
)

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
	if cfg.LLMModel != "" {
		t.Fatalf("LLMModel = %q, want empty when LLM_MODEL is unset", cfg.LLMModel)
	}
	if cfg.LLMContextWindow != 8000 {
		t.Fatalf("LLMContextWindow = %d, want 8000", cfg.LLMContextWindow)
	}
}

func TestLoadMLCallbackSecretFile(t *testing.T) {
	secretFile := t.TempDir() + "/ml_callback_secret"
	if err := os.WriteFile(secretFile, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
	t.Setenv("DATABASE_URL", "postgres://skadia:secret@localhost:5432/skadia_test_abcdef123456?sslmode=disable")
	t.Setenv("ML_CALLBACK_SECRET_FILE", secretFile)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MLCallbackSecret != "file-secret" {
		t.Fatalf("MLCallbackSecret = %q, want file-secret", cfg.MLCallbackSecret)
	}
}

func TestLoadMLLLMBrokerTokenOverridesCallbackSecret(t *testing.T) {
	brokerFile := t.TempDir() + "/ml_llm_broker_token"
	if err := os.WriteFile(brokerFile, []byte("broker-token\n"), 0o600); err != nil {
		t.Fatalf("write broker token file: %v", err)
	}
	t.Setenv("DATABASE_URL", "postgres://skadia:secret@localhost:5432/skadia_test_abcdef123456?sslmode=disable")
	t.Setenv("ML_CALLBACK_SECRET", "callback-secret")
	t.Setenv("ML_LLM_BROKER_TOKEN_FILE", brokerFile)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MLLLMBrokerToken != "broker-token" {
		t.Fatalf("MLLLMBrokerToken = %q, want broker-token", cfg.MLLLMBrokerToken)
	}
}

func TestLoadS3CredentialsFromDeprecatedGarageFiles(t *testing.T) {
	accessFile := t.TempDir() + "/garage_access_key"
	secretFile := t.TempDir() + "/garage_secret_key"
	if err := os.WriteFile(accessFile, []byte("garage-access\n"), 0o600); err != nil {
		t.Fatalf("write access file: %v", err)
	}
	if err := os.WriteFile(secretFile, []byte("garage-secret\n"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
	t.Setenv("DATABASE_URL", "postgres://skadia:secret@localhost:5432/skadia_test_abcdef123456?sslmode=disable")
	t.Setenv("STORAGE_BACKEND", "s3")
	t.Setenv("GARAGE_ACCESS_KEY_FILE", accessFile)
	t.Setenv("GARAGE_SECRET_KEY_FILE", secretFile)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.S3AccessKey != "garage-access" {
		t.Fatalf("S3AccessKey = %q, want garage-access", cfg.S3AccessKey)
	}
	if cfg.S3SecretKey != "garage-secret" {
		t.Fatalf("S3SecretKey = %q, want garage-secret", cfg.S3SecretKey)
	}
}
