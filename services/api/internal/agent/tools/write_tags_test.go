package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

func TestWriteTagToolsMetadata(t *testing.T) {
	reg := NewRegistry()
	for _, tool := range WriteTagTools() {
		reg.Register(tool)
	}
	for _, name := range []string{"create_tag", "update_tag", "delete_tag"} {
		tool := reg.tools[name]
		if tool == nil {
			t.Fatalf("write tool %q was not registered", name)
		}
		if tool.Permission != PermissionWriteSafe || !tool.RequiresConfirmation || !tool.IsUserOwned {
			t.Fatalf("unexpected write metadata for %s: %#v", name, tool)
		}
	}
	if err := reg.ValidateSchemas(); err != nil {
		t.Fatalf("tag tool schemas must validate: %v", err)
	}
}

func TestCreateTagWriteToolCreatesAutoLocationTag(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertActivityToolUser(t, pool)
	envelope := tagWriteEnvelope(t, pool, userID, "create_tag")
	sessionID := uuid.NewString()
	args := []byte(`{"name":"Trail Favorit","color":"#112233","text_color":"#FFFFFF","conditions":[{"field":"gear_total_distance_m","operator":">","value":1000}],"lat":47.1,"lng":7.2}`)

	prepared, err := envelope.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-create-tag", ToolName: "create_tag", Args: args,
	})
	if err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	if !strings.Contains(prepared.Summary, "Auto-Regel") {
		t.Fatalf("summary = %q, want Auto-Regel marker", prepared.Summary)
	}
	confirmEnvelopeTool(t, envelope, sessionID, "call-create-tag")
	result, err := envelope.ExecuteConfirmed(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-create-tag", ToolName: "create_tag", Args: args,
	})
	if err != nil {
		t.Fatalf("ExecuteConfirmed: %v", err)
	}
	tagID := result.(map[string]any)["tag_id"].(string)
	var name, color, textColor, category string
	var isAuto bool
	if err := pool.QueryRow(context.Background(),
		`SELECT name, color, text_color, is_auto, category FROM tags WHERE id = $1 AND user_id = $2`,
		tagID, userID,
	).Scan(&name, &color, &textColor, &isAuto, &category); err != nil {
		t.Fatalf("select tag: %v", err)
	}
	if name != "Trail Favorit" || color != "#112233" || textColor != "#FFFFFF" || !isAuto || category != "standort" {
		t.Fatalf("unexpected tag row: name=%q color=%q text=%q isAuto=%v category=%q", name, color, textColor, isAuto, category)
	}
}

func TestCreateTagWriteToolRejectsInvalidColorAndGearCondition(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertActivityToolUser(t, pool)
	envelope := tagWriteEnvelope(t, pool, userID, "create_tag")

	_, err := envelope.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: uuid.NewString(), ToolCallID: "call-bad-color", ToolName: "create_tag", Args: []byte(`{"name":"Bad","color":"red"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "color must be #RRGGBB") {
		t.Fatalf("PrepareConfirmation err = %v, want color validation", err)
	}

	_, err = envelope.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: uuid.NewString(), ToolCallID: "call-bad-condition", ToolName: "create_tag", Args: []byte(`{"name":"Bad","condition_groups":[{"conditions":[{"field":"gear_totl_distance_m","operator":">","value":10}]}]}`),
	})
	if err == nil || !strings.Contains(err.Error(), "unknown gear condition field") {
		t.Fatalf("PrepareConfirmation err = %v, want gear condition validation", err)
	}
}

func TestUpdateTagWriteToolRejectsCrossUserTag(t *testing.T) {
	pool := testdb.Open(t)
	userA := insertActivityToolUser(t, pool)
	userB := insertActivityToolUser(t, pool)
	tagB := insertActivityToolTag(t, pool, userB, "Fremd")
	envelope := tagWriteEnvelope(t, pool, userA, "update_tag")
	args := []byte(`{"tag_id":"` + tagB + `","fields":{"name":"Nicht meins"}}`)

	_, err := envelope.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userA, SessionID: uuid.NewString(), ToolCallID: "call-cross-user-tag", ToolName: "update_tag", Args: args,
	})
	if err == nil || !strings.Contains(err.Error(), "FORBIDDEN") {
		t.Fatalf("PrepareConfirmation err = %v, want FORBIDDEN", err)
	}
}

func TestUpdateTagWriteToolUpdatesFields(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertActivityToolUser(t, pool)
	tagID := insertActivityToolTag(t, pool, userID, "Alt")
	envelope := tagWriteEnvelope(t, pool, userID, "update_tag")
	sessionID := uuid.NewString()
	args := []byte(`{"tag_id":"` + tagID + `","fields":{"name":"Neu","color":"#334455","conditions":[],"condition_groups":[],"lat":null,"lng":null}}`)

	if _, err := envelope.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-update-tag", ToolName: "update_tag", Args: args,
	}); err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	confirmEnvelopeTool(t, envelope, sessionID, "call-update-tag")
	if _, err := envelope.ExecuteConfirmed(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-update-tag", ToolName: "update_tag", Args: args,
	}); err != nil {
		t.Fatalf("ExecuteConfirmed: %v", err)
	}
	var name, color, category string
	var isAuto bool
	if err := pool.QueryRow(context.Background(),
		`SELECT name, color, is_auto, category FROM tags WHERE id = $1 AND user_id = $2`,
		tagID, userID,
	).Scan(&name, &color, &isAuto, &category); err != nil {
		t.Fatalf("select tag: %v", err)
	}
	if name != "Neu" || color != "#334455" || isAuto || category != "custom" {
		t.Fatalf("unexpected tag row: name=%q color=%q isAuto=%v category=%q", name, color, isAuto, category)
	}
}

func TestDeleteTagWriteToolSummarizesAndCascades(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertActivityToolUser(t, pool)
	activityID := insertActivityToolActivity(t, pool, userID, "Tagrunde")
	tagID := insertActivityToolTag(t, pool, userID, "Weg damit")
	routeID := uuid.NewString()
	if _, err := pool.Exec(context.Background(), `INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES ($1, $2, 'manual')`, activityID, tagID); err != nil {
		t.Fatalf("insert activity tag: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO activity_tag_removals (activity_id, tag_id) VALUES ($1, $2)`, activityID, tagID); err != nil {
		t.Fatalf("insert removal: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO tag_zones (tag_id, zone_type, center, radius_m) VALUES ($1, 'circle', ST_SetSRID(ST_MakePoint(7, 47), 4326), 100)`, tagID); err != nil {
		t.Fatalf("insert zone: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO planned_routes (id, user_id, name, sport_type, profile, geometry, waypoints, segments)
		VALUES ($1, $2, 'Testroute', 'hiking', 'hiking-mountain', ST_GeomFromText('LINESTRING(7 47, 7.1 47.1)', 4326), '[]'::jsonb, '[]'::jsonb)`,
		routeID, userID,
	); err != nil {
		t.Fatalf("insert route: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO route_tags (route_id, tag_id) VALUES ($1, $2)`, routeID, tagID); err != nil {
		t.Fatalf("insert route tag: %v", err)
	}
	envelope := tagWriteEnvelope(t, pool, userID, "delete_tag")
	sessionID := uuid.NewString()
	args := []byte(`{"tag_id":"` + tagID + `"}`)

	prepared, err := envelope.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-delete-tag", ToolName: "delete_tag", Args: args,
	})
	if err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	for _, want := range []string{"1 Aktivitäts-Zuweisungen", "eine Zone", "1 Routen-Links", "1 Entfernungs-Historien"} {
		if !strings.Contains(prepared.Summary, want) {
			t.Fatalf("summary %q missing %q", prepared.Summary, want)
		}
	}
	confirmEnvelopeTool(t, envelope, sessionID, "call-delete-tag")
	if _, err := envelope.ExecuteConfirmed(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-delete-tag", ToolName: "delete_tag", Args: args,
	}); err != nil {
		t.Fatalf("ExecuteConfirmed: %v", err)
	}
	for table, query := range map[string]string{
		"tags":                  `SELECT EXISTS (SELECT 1 FROM tags WHERE id = $1)`,
		"activity_tags":         `SELECT EXISTS (SELECT 1 FROM activity_tags WHERE tag_id = $1)`,
		"activity_tag_removals": `SELECT EXISTS (SELECT 1 FROM activity_tag_removals WHERE tag_id = $1)`,
		"tag_zones":             `SELECT EXISTS (SELECT 1 FROM tag_zones WHERE tag_id = $1)`,
		"route_tags":            `SELECT EXISTS (SELECT 1 FROM route_tags WHERE tag_id = $1)`,
	} {
		var exists bool
		if err := pool.QueryRow(context.Background(), query, tagID).Scan(&exists); err != nil {
			t.Fatalf("check %s: %v", table, err)
		}
		if exists {
			t.Fatalf("%s row still exists after tag delete", table)
		}
	}
}

func tagWriteEnvelope(t *testing.T, pool *pgxpool.Pool, userID string, allowedTools ...string) *ExecutionEnvelope {
	t.Helper()
	db := &database.DB{Pool: pool}
	if _, err := db.UpsertChatPrefs(context.Background(), userID, database.ChatUserPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: allowedTools,
	}); err != nil {
		t.Fatalf("upsert chat prefs: %v", err)
	}
	reg := NewRegistry()
	for _, tool := range WriteTagTools() {
		reg.Register(tool)
	}
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
