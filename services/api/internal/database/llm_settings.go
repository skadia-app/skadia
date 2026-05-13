package database

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
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
		if !model.IsValidLLMRole(c.Role) {
			continue
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
	keyState := update.APIKeyState
	if keyState == "" {
		keyState = model.LLMAPIKeyStateUnchanged
	}
	if !model.IsValidLLMBackendType(update.BackendType) {
		return fmt.Errorf("invalid LLM backend_type %q", update.BackendType)
	}
	if !model.IsValidLLMAPIKeyState(keyState) {
		return fmt.Errorf("invalid LLM api_key_state %q", keyState)
	}
	if keyState == model.LLMAPIKeyStateSet && encryptedKey == nil {
		return fmt.Errorf("api_key is required when api_key_state is set")
	}
	if update.BackendType == model.LLMBackendDisabled {
		encryptedKey = nil
		keyState = model.LLMAPIKeyStateClear
	}
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO llm_system_config (role, backend_type, api_base, model_name, api_key_enc, enabled, eval_status, eval_report, updated_at)
	 VALUES ($1, $2, $3, $4, $5, $6, 'untested', '{}'::jsonb, NOW())
		 ON CONFLICT (role) DO UPDATE SET
		   eval_status = CASE
		     WHEN llm_system_config.backend_type IS DISTINCT FROM EXCLUDED.backend_type
			       OR llm_system_config.api_base IS DISTINCT FROM EXCLUDED.api_base
			       OR llm_system_config.model_name IS DISTINCT FROM EXCLUDED.model_name
			       OR llm_system_config.enabled IS DISTINCT FROM EXCLUDED.enabled
			       OR $7 IN ('set', 'clear')
			     THEN 'untested'
			     ELSE llm_system_config.eval_status
			   END,
		   eval_report = CASE
		     WHEN llm_system_config.backend_type IS DISTINCT FROM EXCLUDED.backend_type
			       OR llm_system_config.api_base IS DISTINCT FROM EXCLUDED.api_base
			       OR llm_system_config.model_name IS DISTINCT FROM EXCLUDED.model_name
			       OR llm_system_config.enabled IS DISTINCT FROM EXCLUDED.enabled
			       OR $7 IN ('set', 'clear')
			     THEN '{}'::jsonb
			     ELSE llm_system_config.eval_report
			   END,
			   backend_type = EXCLUDED.backend_type,
			   api_base = EXCLUDED.api_base,
			   model_name = EXCLUDED.model_name,
			   api_key_enc = CASE
			     WHEN $7 = 'set' THEN EXCLUDED.api_key_enc
			     WHEN $7 = 'clear' THEN NULL
			     ELSE llm_system_config.api_key_enc
			   END,
			   enabled = EXCLUDED.enabled,
			   updated_at = NOW()`,
		role, update.BackendType, update.APIBase, update.ModelName, encryptedKey, update.Enabled, keyState)
	return err
}

func (db *DB) UpdateLLMEvalResult(ctx context.Context, role, status string, report any) error {
	raw, err := json.Marshal(report)
	if err != nil {
		return err
	}
	tag, err := db.Pool.Exec(ctx,
		`UPDATE llm_system_config
		 SET eval_status = $2,
		     eval_report = $3::jsonb,
		     updated_at = NOW()
		 WHERE role = $1`,
		role, status, string(raw))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (db *DB) UpdateLLMModelEvalResult(ctx context.Context, role string, report any) error {
	raw, err := json.Marshal(report)
	if err != nil {
		return err
	}
	tag, err := db.Pool.Exec(ctx,
		`WITH filtered AS (
		   SELECT COALESCE(jsonb_agg(existing.item) FILTER (WHERE existing.item IS NOT NULL), '[]'::jsonb) AS model_evals
		   FROM llm_system_config AS cfg
		   LEFT JOIN LATERAL jsonb_array_elements(COALESCE(cfg.eval_report->'model_evals', '[]'::jsonb)) AS existing(item) ON true
		   WHERE cfg.role = $1
		     AND (
		       existing.item IS NULL
		       OR NOT (
		         COALESCE(existing.item->>'role', existing.item #>> '{identity,role}') = ($2::jsonb #>> '{identity,role}')
		         AND COALESCE(existing.item->>'backend_type', existing.item #>> '{identity,backend_type}') = ($2::jsonb #>> '{identity,backend_type}')
		         AND COALESCE(existing.item->>'api_base_host', existing.item #>> '{identity,api_base_host}') = ($2::jsonb #>> '{identity,api_base_host}')
		         AND COALESCE(existing.item->>'model_name', existing.item #>> '{identity,model_name}') = ($2::jsonb #>> '{identity,model_name}')
		         AND COALESCE(existing.item->>'suite_id', existing.item #>> '{identity,suite_id}') = ($2::jsonb #>> '{identity,suite_id}')
		         AND COALESCE(existing.item->>'suite_hash', existing.item #>> '{identity,suite_hash}') = ($2::jsonb #>> '{identity,suite_hash}')
		       )
		     )
		 )
		 UPDATE llm_system_config
		 SET eval_report = jsonb_set(
		       COALESCE(eval_report, '{}'::jsonb),
		       '{model_evals}',
		       filtered.model_evals || jsonb_build_array($2::jsonb),
		       true
		     ),
		     updated_at = NOW()
		 FROM filtered
		 WHERE role = $1`,
		role, string(raw))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
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
