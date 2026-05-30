package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestCreateRoutingProfileVariantToolMetadataAndWritePrefs(t *testing.T) {
	reg := NewRegistry()
	RegisterAll(reg)

	tool := reg.tools["create_routing_profile_variant"]
	if tool == nil {
		t.Fatal("create_routing_profile_variant was not registered")
	}
	if tool.Permission != PermissionWriteSafe || !tool.RequiresConfirmation || !tool.IsUserOwned {
		t.Fatalf("unexpected metadata: permission=%s confirmation=%v userOwned=%v", tool.Permission, tool.RequiresConfirmation, tool.IsUserOwned)
	}
	if tool.PrepareWrite == nil || tool.ExecuteWrite == nil {
		t.Fatal("create_routing_profile_variant must prepare and execute through confirmable write flow")
	}
	if err := validateSchema(tool.Name, tool.InputSchema); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}

	defs := reg.Definitions([]string{"list_routing_profiles"}, &agent.ChatPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: []string{"create_routing_profile_variant"},
	})
	if len(defs) != 1 || defs[0].Name != "list_routing_profiles" {
		t.Fatalf("Definitions should not add write tools by itself: %#v", defs)
	}
	filter := reg.AddEnabledWriteTools([]string{"list_routing_profiles"}, &agent.ChatPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: []string{"create_routing_profile_variant"},
	})
	if containsRoutingProfileWriteTestString(filter, "create_routing_profile_variant") {
		t.Fatalf("AddEnabledWriteTools = %#v, did not want implicit write tool", filter)
	}
}

func TestPrepareCreateRoutingProfileVariantNormalizesAndPreviews(t *testing.T) {
	prepared, err := prepareCreateRoutingProfileVariant(context.Background(), nil, "user-1", json.RawMessage(`{
		"base_profile_key": "gravel",
		"intent": "mehr Wald",
		"changes": {"forest_bonus": 0.8, "traffic_penalty": 0.4},
		"name_hint": "Gravel Wald",
		"activate_for": "Gravel"
	}`))
	if err != nil {
		t.Fatalf("prepareCreateRoutingProfileVariant: %v", err)
	}
	if prepared.Summary != "Routing-Profilvariante 'Gravel Wald' erstellen" {
		t.Fatalf("summary = %q", prepared.Summary)
	}
	preview := prepared.Preview.(map[string]any)
	if preview["base_profile"] != "gravel" {
		t.Fatalf("base_profile = %#v, want gravel", preview["base_profile"])
	}
	diff := preview["profile_diff"].([]map[string]any)
	if len(diff) != 3 {
		t.Fatalf("diff len = %d, want 3 including activation note", len(diff))
	}
	if !strings.Contains(string(prepared.Args), `"base_profile_key":"gravel"`) {
		t.Fatalf("prepared args not normalized to badrouter base profile: %s", prepared.Args)
	}
}

func TestPrepareCreateRoutingProfileVariantRejectsUnsupportedOverride(t *testing.T) {
	_, err := prepareCreateRoutingProfileVariant(context.Background(), nil, "user-1", json.RawMessage(`{
		"base_profile_key": "gravel",
		"changes": {"invented_weight": 0.8}
	}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported profile override") {
		t.Fatalf("err = %v, want unsupported override", err)
	}
}

func TestExecuteCreateRoutingProfileVariantCreatesNewUserProfile(t *testing.T) {
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	userID := insertRouteToolUser(t, pool, "route-profile-variant")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM routing_profiles WHERE user_id = $1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback(context.Background())

	result, err := executeCreateRoutingProfileVariant(context.Background(), &WriteExecutionDeps{
		Deps: &Deps{DB: db},
		Tx:   tx,
	}, userID, json.RawMessage(`{
		"base_profile_key": "mtb",
		"changes": {"max_mtb_scale": 2},
		"name_hint": "MTB S2 Test"
	}`))
	if err != nil {
		t.Fatalf("executeCreateRoutingProfileVariant: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	got := result.(map[string]any)
	if got["status"] != "ok" || got["base_profile"] != "mtb" {
		t.Fatalf("unexpected result: %#v", got)
	}

	var config map[string]any
	if err := pool.QueryRow(context.Background(), `SELECT config FROM routing_profiles WHERE user_id = $1 AND name = 'MTB S2 Test'`, userID).Scan(&config); err != nil {
		t.Fatalf("select profile config: %v", err)
	}
	if config["profile"] != "mtb" || config["max_mtb_scale"].(float64) != 2 {
		t.Fatalf("config = %#v", config)
	}
}

func TestExecuteCreateRoutingProfileVariantPreservesBaseProfileScopeAndConfig(t *testing.T) {
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	userID := insertRouteToolUser(t, pool, "route-profile-variant-scope")
	preds := []sport.Predicate{{Sport: "cycling", SubType: stringPtr("gravel_cycling")}}
	base, err := db.CreateRoutingProfile(context.Background(), userID, "Base Gravel "+userID[:8], "gravel", preds, sport.ModifierFilters{}, []byte(`{"profile":"gravel","name":"Base Gravel","traffic_penalty":0.2}`))
	if err != nil {
		t.Fatalf("CreateRoutingProfile base: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM routing_profiles WHERE user_id = $1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback(context.Background())

	_, err = executeCreateRoutingProfileVariant(context.Background(), &WriteExecutionDeps{
		Deps: &Deps{DB: db},
		Tx:   tx,
	}, userID, json.RawMessage(`{
		"base_profile_key": "`+base.ID+`",
		"changes": {"forest_bonus": 0.7},
		"name_hint": "Scoped Gravel Variant"
	}`))
	if err != nil {
		t.Fatalf("executeCreateRoutingProfileVariant: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	var sportTypes []string
	var config map[string]any
	if err := pool.QueryRow(context.Background(), `SELECT sport_types, config FROM routing_profiles WHERE user_id = $1 AND name = 'Scoped Gravel Variant'`, userID).Scan(&sportTypes, &config); err != nil {
		t.Fatalf("select variant: %v", err)
	}
	if len(sportTypes) != 1 || sportTypes[0] != "cycling" {
		t.Fatalf("sport_types = %#v, want inherited cycling", sportTypes)
	}
	if config["traffic_penalty"].(float64) != 0.2 || config["forest_bonus"].(float64) != 0.7 {
		t.Fatalf("config = %#v, want inherited and changed values", config)
	}
}

func stringPtr(value string) *string {
	return &value
}

func containsRoutingProfileWriteTestString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
