package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHealthToolsRegisterAsUserOwnedReadTools(t *testing.T) {
	reg := NewRegistry()
	RegisterHealthTools(reg)

	for _, name := range []string{
		"get_health_status",
		"get_training_load",
		"get_analytics",
		"get_hrv_trend",
		"get_readiness",
	} {
		tool := reg.tools[name]
		if tool == nil {
			t.Fatalf("missing tool %s", name)
		}
		if tool.Permission != PermissionRead {
			t.Fatalf("%s permission = %s, want read", name, tool.Permission)
		}
		if !tool.IsUserOwned {
			t.Fatalf("%s must be user-owned", name)
		}
		if tool.ExecuteRead == nil {
			t.Fatalf("%s has no read executor", name)
		}
	}
	writeTool := reg.tools["record_health_metric"]
	if writeTool == nil || writeTool.Permission != PermissionWriteSafe || !writeTool.RequiresConfirmation || !writeTool.IsUserOwned {
		t.Fatalf("record_health_metric metadata = %#v", writeTool)
	}
	if err := reg.ValidateSchemas(); err != nil {
		t.Fatalf("ValidateSchemas: %v", err)
	}
}

func TestRecordHealthMetricWeightWriteTool(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := "00000000-0000-0000-0000-0000000aa201"
	insertToolTestUser(t, pool, userID, "agent-health-write")
	envelope := healthWriteEnvelope(t, pool, userID)
	sessionID := "00000000-0000-0000-0000-0000000bb201"
	args := json.RawMessage(`{"metric_kind":"weight_kg","date_iso":"2026-05-12","kg":82.4,"body_fat_pct":18.5}`)

	if _, err := envelope.PrepareConfirmation(ctx, ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-weight", ToolName: "record_health_metric", Args: args,
	}); err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	if _, err := envelope.ExecuteConfirmed(ctx, ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-weight", ToolName: "record_health_metric", Args: args,
	}); err != nil {
		t.Fatalf("ExecuteConfirmed: %v", err)
	}
	var weightG int
	if err := pool.QueryRow(ctx,
		`SELECT weight_g FROM health_weight_entries WHERE user_id = $1 AND measured_at_utc = '2026-05-12T12:00:00Z'`,
		userID,
	).Scan(&weightG); err != nil {
		t.Fatalf("select weight: %v", err)
	}
	if weightG != 82400 {
		t.Fatalf("weight_g = %d, want 82400", weightG)
	}
}

func TestRecordHealthMetricRejectsInvalidHRV(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := "00000000-0000-0000-0000-0000000aa202"
	insertToolTestUser(t, pool, userID, "agent-health-invalid")
	envelope := healthWriteEnvelope(t, pool, userID)

	_, err := envelope.PrepareConfirmation(ctx, ToolExecutionRequest{
		UserID: userID, SessionID: "00000000-0000-0000-0000-0000000bb202", ToolCallID: "call-invalid",
		ToolName: "record_health_metric", Args: json.RawMessage(`{"metric_kind":"hrv_rmssd","recorded_at_iso":"2026-05-12T08:00:00Z","value_ms":0}`),
	})
	if err == nil {
		t.Fatal("PrepareConfirmation accepted invalid HRV value")
	}
}

func TestGetAnalyticsFiltersByUser(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	ownerID := "00000000-0000-0000-0000-0000000aa101"
	otherID := "00000000-0000-0000-0000-0000000aa102"
	insertToolTestUser(t, pool, ownerID, "agent-health-owner")
	insertToolTestUser(t, pool, otherID, "agent-health-other")
	cleanupAnalytics := func(userID string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `DELETE FROM analytics_cache WHERE user_id = $1::uuid`, userID); err != nil {
			t.Fatalf("cleanup analytics_cache for %s: %v", userID, err)
		}
	}
	cleanupAnalytics(ownerID)
	cleanupAnalytics(otherID)
	t.Cleanup(func() {
		cleanupAnalytics(ownerID)
		cleanupAnalytics(otherID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO analytics_cache (user_id, cache_key, computed_at, data)
		VALUES
			($1::uuid, 'readiness', $3, '{"score": 81, "label": "owner"}'::jsonb),
			($2::uuid, 'readiness', $3, '{"score": 12, "label": "other"}'::jsonb)
	`, ownerID, otherID, time.Now().UTC()); err != nil {
		t.Fatalf("insert analytics_cache: %v", err)
	}

	got, err := getAnalytics(ctx, pool, ownerID, getAnalyticsArgs{Analysis: "readiness"})
	if err != nil {
		t.Fatalf("getAnalytics: %v", err)
	}
	data := got["data"].(map[string]any)
	if data["label"] != "owner" {
		t.Fatalf("data label = %#v, want owner", data["label"])
	}
}

func TestGetHealthStatusReadsRequestedAspectOnly(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := "00000000-0000-0000-0000-0000000aa103"
	insertToolTestUser(t, pool, userID, "agent-health-status")
	if _, err := pool.Exec(ctx, `DELETE FROM health_snapshots WHERE user_id = $1::uuid`, userID); err != nil {
		t.Fatalf("cleanup health_snapshots: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM health_snapshots WHERE user_id = $1::uuid`, userID)
	})
	if _, err := pool.Exec(ctx, `
		INSERT INTO health_snapshots (user_id, measured_at_utc, rmssd_ms, sdrr_ms, extra_data)
		VALUES ($1::uuid, $2, 50, 70, '{"weekly_avg": 48, "status": "balanced"}'::jsonb)
	`, userID, time.Now().UTC()); err != nil {
		t.Fatalf("insert health_snapshots: %v", err)
	}

	got, err := getHealthStatus(ctx, pool, userID, getHealthStatusArgs{Aspect: "hrv", Days: 30})
	if err != nil {
		t.Fatalf("getHealthStatus: %v", err)
	}
	if _, ok := got["sleep"]; ok {
		t.Fatalf("aspect hrv returned sleep: %#v", got)
	}
	hrv := got["hrv"].(map[string]any)
	if hrv["avg_rmssd"] != 50.0 {
		t.Fatalf("avg_rmssd = %#v, want 50", hrv["avg_rmssd"])
	}
	latest := hrv["latest"].(map[string]any)
	if latest["status"] != "balanced" {
		t.Fatalf("latest status = %#v, want balanced", latest["status"])
	}
}

func TestLegacyHealthToolsWrapHealthStatus(t *testing.T) {
	reg := NewRegistry()
	RegisterHealthTools(reg)
	for _, item := range []struct {
		name string
		args json.RawMessage
	}{
		{"get_hrv_trend", json.RawMessage(`{"days": 7}`)},
		{"get_readiness", json.RawMessage(`{}`)},
	} {
		tool := reg.tools[item.name]
		if tool == nil || !tool.IsUserOwned {
			t.Fatalf("legacy tool %s not registered as user-owned", item.name)
		}
	}
}

func healthWriteEnvelope(t *testing.T, pool *pgxpool.Pool, userID string) *ExecutionEnvelope {
	t.Helper()
	db := &database.DB{Pool: pool}
	if _, err := db.UpsertChatPrefs(context.Background(), userID, database.ChatUserPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: []string{"record_health_metric"},
	}); err != nil {
		t.Fatalf("upsert chat prefs: %v", err)
	}
	reg := NewRegistry()
	reg.Register(RecordHealthMetricTool())
	return NewExecutionEnvelope(reg, &Deps{
		DB:      db,
		Audit:   agent.NewAuditWriter(db),
		Confirm: agent.NewConfirmationManager(5 * time.Minute),
		Preferences: &agent.PreferenceLoader{LoadFunc: func(ctx context.Context, userID string) (*agent.ChatPreferences, error) {
			prefs, err := db.GetChatPrefs(ctx, userID)
			if err != nil {
				return nil, err
			}
			return &agent.ChatPreferences{
				ChatWriteToolsEnabled: prefs.ChatWriteToolsEnabled,
				ChatAllowedWriteTools: prefs.ChatAllowedWriteTools,
			}, nil
		}},
	})
}
