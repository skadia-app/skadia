package database

import (
	"context"
	"encoding/json"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (db *DB) GetLLMSystemConfig(ctx context.Context) ([]model.LLMSystemConfig, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT role, backend_type, api_base, model_name,
		        api_key_enc IS NOT NULL AS has_api_key, enabled,
		        capability_flags, eval_status, eval_report, updated_at
		 FROM llm_system_config ORDER BY role`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var configs []model.LLMSystemConfig
	for rows.Next() {
		var c model.LLMSystemConfig
		var evalReport []byte
		if err := rows.Scan(&c.Role, &c.BackendType, &c.APIBase, &c.ModelName,
			&c.HasAPIKey, &c.Enabled, &c.CapabilityFlags, &c.EvalStatus, &evalReport, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if len(evalReport) > 0 {
			if err := json.Unmarshal(evalReport, &c.EvalReport); err != nil {
				return nil, err
			}
		}
		if c.EvalReport == nil {
			c.EvalReport = map[string]any{}
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

func (db *DB) GetLLMAPIKeyEnc(ctx context.Context, role string) (string, error) {
	var enc *string
	err := db.Pool.QueryRow(ctx,
		`SELECT api_key_enc FROM llm_system_config WHERE role = $1`, role).Scan(&enc)
	if err != nil || enc == nil {
		return "", err
	}
	return *enc, nil
}

func (db *DB) UpsertLLMSystemConfig(ctx context.Context, role string, update model.LLMSystemConfigUpdate, encryptedKey *string) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO llm_system_config (role, backend_type, api_base, model_name, api_key_enc, enabled, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW())
		 ON CONFLICT (role) DO UPDATE SET
		   backend_type = EXCLUDED.backend_type,
		   api_base = EXCLUDED.api_base,
		   model_name = EXCLUDED.model_name,
		   api_key_enc = COALESCE(EXCLUDED.api_key_enc, llm_system_config.api_key_enc),
		   enabled = EXCLUDED.enabled,
		   updated_at = NOW()`,
		role, update.BackendType, update.APIBase, update.ModelName, encryptedKey, update.Enabled)
	return err
}

func (db *DB) GetLLMUserConfig(ctx context.Context, userID string) ([]model.LLMUserConfig, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT user_id, role, model_name FROM llm_user_config WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var configs []model.LLMUserConfig
	for rows.Next() {
		var c model.LLMUserConfig
		if err := rows.Scan(&c.UserID, &c.Role, &c.ModelName); err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

func (db *DB) UpsertLLMUserConfig(ctx context.Context, userID, role string, modelName *string) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO llm_user_config (user_id, role, model_name)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, role) DO UPDATE SET model_name = EXCLUDED.model_name`,
		userID, role, modelName)
	return err
}

func (db *DB) GetMLUserSetting(ctx context.Context, userID, key string) ([]byte, error) {
	var value []byte
	err := db.Pool.QueryRow(ctx,
		`SELECT value_json FROM ml_user_settings WHERE user_id = $1 AND setting_key = $2`,
		userID, key,
	).Scan(&value)
	return value, err
}
