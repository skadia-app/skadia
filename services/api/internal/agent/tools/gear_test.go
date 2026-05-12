package tools

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestGearToolsSchemaValidAndUserOwned(t *testing.T) {
	reg := NewRegistry()
	for _, tool := range GearTools() {
		reg.Register(tool)
		if !tool.IsUserOwned {
			t.Fatalf("%s IsUserOwned = false, want true", tool.Name)
		}
		if strings.HasPrefix(tool.Name, "create_") || strings.HasPrefix(tool.Name, "update_") || strings.HasPrefix(tool.Name, "retire_") || strings.HasPrefix(tool.Name, "set_") {
			if tool.Permission != PermissionWriteSafe || !tool.RequiresConfirmation {
				t.Fatalf("%s write flags permission=%s confirmation=%v", tool.Name, tool.Permission, tool.RequiresConfirmation)
			}
			continue
		}
		if tool.Permission != PermissionRead {
			t.Fatalf("%s Permission = %s, want read", tool.Name, tool.Permission)
		}
	}
	if err := reg.ValidateSchemas(); err != nil {
		t.Fatalf("ValidateSchemas: %v", err)
	}
}

func TestEscapeLikePrefixEscapesWildcardsBeforeAppendingPrefixWildcard(t *testing.T) {
	got := escapeLikePrefix(`bike\_%`)
	if got != `bike\\\_\%%` {
		t.Fatalf("escapeLikePrefix = %q, want escaped literal prefix with trailing wildcard", got)
	}
}

func TestGetGearDocumentsIncludesExtractedDataAndCostSummaryWhenOptedIn(t *testing.T) {
	fx := newGearToolFixture(t)
	if _, err := fx.db.UpsertChatPrefs(context.Background(), fx.userID, database.ChatUserPreferences{
		ExposeDocumentExtractedData: true,
	}); err != nil {
		t.Fatalf("upsert chat prefs: %v", err)
	}
	docID := uuid.NewString()
	_, err := fx.pool.Exec(context.Background(), `
		INSERT INTO gear_documents
			(id, user_id, gear_id, doc_type, original_filename, content_type, file_size_bytes,
			 document_amount, document_currency, extracted_data, processing_status)
		VALUES
			($1, $2, $3, 'receipt', 'receipt.pdf', 'application/pdf', 123,
			 12.34, 'EUR', '{"vendor":"Shop","total_amount":12.34,"currency":"EUR"}'::jsonb, 'processed')`,
		docID, fx.userID, fx.gearID,
	)
	if err != nil {
		t.Fatalf("insert gear document: %v", err)
	}

	got, err := executeGetGearDocuments(context.Background(), &Deps{DB: fx.db}, fx.userID, json.RawMessage(`{"gear_id":"`+fx.gearID+`"}`))
	if err != nil {
		t.Fatalf("executeGetGearDocuments: %v", err)
	}
	result := got.(map[string]any)
	if result["count"] != 1 {
		t.Fatalf("count = %v, want 1", result["count"])
	}
	if result["total_cost"] != 12.34 {
		t.Fatalf("total_cost = %#v, want 12.34", result["total_cost"])
	}
	docs := result["documents"].([]map[string]any)
	extracted, ok := docs[0]["extracted_data"].(map[string]any)
	if !ok {
		t.Fatalf("extracted_data = %#v, want object", docs[0]["extracted_data"])
	}
	if extracted["vendor"] != "Shop" {
		t.Fatalf("vendor = %#v, want Shop", extracted["vendor"])
	}
}

func TestGetGearDocumentsHidesExtractedDataByDefault(t *testing.T) {
	fx := newGearToolFixture(t)
	_, err := fx.pool.Exec(context.Background(), `
		INSERT INTO gear_documents
			(id, user_id, gear_id, doc_type, original_filename, content_type, file_size_bytes,
			 document_amount, document_currency, extracted_data, processing_status)
		VALUES
			($1, $2, $3, 'receipt', 'receipt.pdf', 'application/pdf', 123,
			 12.34, 'EUR', '{"vendor":"Shop","total_amount":12.34,"currency":"EUR"}'::jsonb, 'processed')`,
		uuid.NewString(), fx.userID, fx.gearID,
	)
	if err != nil {
		t.Fatalf("insert gear document: %v", err)
	}

	got, err := executeGetGearDocuments(context.Background(), &Deps{DB: fx.db}, fx.userID, json.RawMessage(`{"gear_id":"`+fx.gearID+`"}`))
	if err != nil {
		t.Fatalf("executeGetGearDocuments: %v", err)
	}
	docs := got.(map[string]any)["documents"].([]map[string]any)
	if docs[0]["extracted_data"] != nil {
		t.Fatalf("extracted_data = %#v, want nil by default", docs[0]["extracted_data"])
	}
	if got.(map[string]any)["total_cost"] != 12.34 {
		t.Fatalf("total_cost = %#v, want 12.34", got.(map[string]any)["total_cost"])
	}
}

func TestGetGearDocumentsHidesExtractedDataWhenOptedOut(t *testing.T) {
	fx := newGearToolFixture(t)
	if _, err := fx.db.UpsertChatPrefs(context.Background(), fx.userID, database.ChatUserPreferences{
		ExposeDocumentExtractedData: false,
	}); err != nil {
		t.Fatalf("upsert chat prefs: %v", err)
	}
	_, err := fx.pool.Exec(context.Background(), `
		INSERT INTO gear_documents
			(id, user_id, gear_id, doc_type, original_filename, content_type, file_size_bytes,
			 extracted_data, processing_status)
		VALUES
			($1, $2, $3, 'manual', 'manual.pdf', 'application/pdf', 123,
			 '{"serial":"ABC-123"}'::jsonb, 'processed')`,
		uuid.NewString(), fx.userID, fx.gearID,
	)
	if err != nil {
		t.Fatalf("insert gear document: %v", err)
	}

	got, err := executeGetGearDocuments(context.Background(), &Deps{DB: fx.db}, fx.userID, json.RawMessage(`{"gear_id":"`+fx.gearID+`"}`))
	if err != nil {
		t.Fatalf("executeGetGearDocuments: %v", err)
	}
	docs := got.(map[string]any)["documents"].([]map[string]any)
	if docs[0]["extracted_data"] != nil {
		t.Fatalf("extracted_data = %#v, want nil when opted out", docs[0]["extracted_data"])
	}
}

func TestGearDocumentModelScanIncludesExtractedData(t *testing.T) {
	fx := newGearToolFixture(t)
	_, err := fx.pool.Exec(context.Background(), `
		INSERT INTO gear_documents
			(id, user_id, gear_id, doc_type, original_filename, content_type, file_size_bytes,
			 extracted_data, processing_status)
		VALUES
			($1, $2, $3, 'manual', 'manual.pdf', 'application/pdf', 123,
			 '{"serial":"ABC-123"}'::jsonb, 'processed')`,
		uuid.NewString(), fx.userID, fx.gearID,
	)
	if err != nil {
		t.Fatalf("insert gear document: %v", err)
	}

	docs, err := fx.db.ListGearDocuments(context.Background(), fx.gearID)
	if err != nil {
		t.Fatalf("ListGearDocuments: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs len = %d, want 1", len(docs))
	}
	var extracted map[string]any
	if err := json.Unmarshal(docs[0].ExtractedData, &extracted); err != nil {
		t.Fatalf("unmarshal ExtractedData: %v", err)
	}
	if extracted["serial"] != "ABC-123" {
		t.Fatalf("serial = %#v, want ABC-123", extracted["serial"])
	}
}

func TestGearServiceEventsDoesNotReturnEventType(t *testing.T) {
	fx := newGearToolFixture(t)
	componentID := fx.insertComponent(t, "Chain", "chain")
	eventID := uuid.NewString()
	_, err := fx.pool.Exec(context.Background(), `
		INSERT INTO gear_service_events (id, component_id, performed_at, description, notes)
		VALUES ($1, $2, $3, 'Cleaned', 'No event_type column here')`,
		eventID, componentID, time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("insert service event: %v", err)
	}

	got, err := executeGearServiceEvents(context.Background(), &Deps{DB: fx.db}, fx.userID, json.RawMessage(`{"gear_id":"`+fx.gearID+`"}`))
	if err != nil {
		t.Fatalf("executeGearServiceEvents: %v", err)
	}
	events := got.(map[string]any)["events"].([]map[string]any)
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if _, ok := events[0]["event_type"]; ok {
		t.Fatalf("event_type returned in event row: %#v", events[0])
	}
}

func TestGearListTypePrefixTreatsWildcardsLiterally(t *testing.T) {
	fx := newGearToolFixture(t)
	fx.insertGear(t, "Literal Percent", "bike%literal", "bike", false)
	fx.insertGear(t, "Wildcard Target", "bikeXliteral", "bike", false)

	got, err := executeGearList(context.Background(), &Deps{DB: fx.db}, fx.userID, json.RawMessage(`{"type":"bike%"}`))
	if err != nil {
		t.Fatalf("executeGearList: %v", err)
	}
	rows := got.(map[string]any)["gear"].([]map[string]any)
	var names []string
	for _, row := range rows {
		names = append(names, row["name"].(string))
	}
	if strings.Contains(strings.Join(names, ","), "Wildcard Target") {
		t.Fatalf("LIKE wildcard was not escaped, got names %v", names)
	}
	if !strings.Contains(strings.Join(names, ","), "Literal Percent") {
		t.Fatalf("literal percent gear missing, got names %v", names)
	}
}

func TestGearSummaryUsesFusionAwareAggregation(t *testing.T) {
	fx := newGearToolFixture(t)
	activityID := fx.insertActivity(t, 10000, 1800, 900, 600, 2000, 250, 120)
	_, err := fx.pool.Exec(context.Background(), `
		INSERT INTO activity_gear_assignments
			(user_id, activity_id, gear_id, role_key, source, distance_m_override, moving_time_s_override)
		VALUES ($1, $2, $3, 'primary', 'test', 7000, 1200)`,
		fx.userID, activityID, fx.gearID,
	)
	if err != nil {
		t.Fatalf("insert aga: %v", err)
	}

	got, err := executeGearSummary(context.Background(), &Deps{DB: fx.db}, fx.userID, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("executeGearSummary: %v", err)
	}
	result := got.(map[string]any)
	if result["total_distance_km"] != 7.0 {
		t.Fatalf("total_distance_km = %#v, want 7.0", result["total_distance_km"])
	}
	if result["total_time_s"] != int64(1800) {
		t.Fatalf("total_time_s = %#v, want 1800", result["total_time_s"])
	}
	if result["total_moving_time_s"] != int64(1200) {
		t.Fatalf("total_moving_time_s = %#v, want 1200", result["total_moving_time_s"])
	}
	if math.Round(result["total_elevation_gain_m"].(float64)) != 650 {
		t.Fatalf("total_elevation_gain_m = %#v, want 650", result["total_elevation_gain_m"])
	}
}

func TestGearStatsCrossUserIsolationReturnsEmpty(t *testing.T) {
	fx := newGearToolFixture(t)
	foreignUserID := fx.insertUser(t)
	foreignGearID := fx.insertGearForUser(t, foreignUserID, "Foreign Bike", "bike", "bike", false)

	got, err := executeGetGearStats(context.Background(), &Deps{DB: fx.db}, fx.userID, json.RawMessage(`{"gear_id":"`+foreignGearID+`"}`))
	if err != nil {
		t.Fatalf("executeGetGearStats: %v", err)
	}
	rows := got.(map[string]any)["gear"].([]map[string]any)
	if len(rows) != 0 {
		t.Fatalf("foreign gear leaked: %#v", rows)
	}
}

type gearToolFixture struct {
	db     *database.DB
	pool   *pgxpool.Pool
	userID string
	gearID string
}

func newGearToolFixture(t *testing.T) gearToolFixture {
	t.Helper()
	pool := testdb.Open(t)
	fx := gearToolFixture{db: &database.DB{Pool: pool}, pool: pool}
	fx.userID = fx.insertUser(t)
	fx.gearID = fx.insertGear(t, "Fixture Bike", "bike", "bike", false)
	return fx
}

func (fx gearToolFixture) insertUser(t *testing.T) string {
	t.Helper()
	userID := uuid.NewString()
	_, err := fx.pool.Exec(context.Background(),
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "gear-tool-"+userID[:8], "gear-tool-"+userID[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = fx.pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return userID
}

func (fx gearToolFixture) insertGear(t *testing.T, name, typeKey, scope string, retired bool) string {
	t.Helper()
	return fx.insertGearForUser(t, fx.userID, name, typeKey, scope, retired)
}

func (fx gearToolFixture) insertGearForUser(t *testing.T, userID, name, typeKey, scope string, retired bool) string {
	t.Helper()
	gearID := uuid.NewString()
	status := "active"
	if retired {
		status = "retired"
	}
	_, err := fx.pool.Exec(context.Background(), `
		INSERT INTO gear (id, user_id, name, type, scope, is_retired, status, total_distance_m, total_activities, total_time_s)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 12345, 3, 456)`,
		gearID, userID, name, typeKey, scope, retired, status,
	)
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	return gearID
}

func (fx gearToolFixture) insertComponent(t *testing.T, name, typeKey string) string {
	t.Helper()
	componentID := uuid.NewString()
	_, err := fx.pool.Exec(context.Background(), `
		INSERT INTO gear_components (id, gear_id, name, type, status, is_retired)
		VALUES ($1, $2, $3, $4, 'active', false)`,
		componentID, fx.gearID, name, typeKey,
	)
	if err != nil {
		t.Fatalf("insert component: %v", err)
	}
	return componentID
}

func (fx gearToolFixture) insertActivity(t *testing.T, distanceM float64, elapsedS int, ascentM, descentM, liftDistanceM, liftAscentM, liftDescentM float64) string {
	t.Helper()
	activityID := uuid.NewString()
	startedAt := time.Now().UTC().Truncate(time.Second)
	_, err := fx.pool.Exec(context.Background(), `
		INSERT INTO activities
			(id, user_id, sport_type, started_at, ended_at, file_hash,
			 distance_m, elapsed_time_s, moving_time_s, elevation_gain_m, elevation_loss_m,
			 lift_distance_m, lift_ascent_m, lift_descent_m,
			 exclude_lift_subtraction, is_group_secondary)
		VALUES ($1, $2, 'cycling', $3, $4, $5, $6, $7, $7, $8, $9, $10, $11, $12, false, false)`,
		activityID, fx.userID, startedAt, startedAt.Add(time.Duration(elapsedS)*time.Second),
		"gear-tool-"+activityID[:8], distanceM, elapsedS, ascentM, descentM, liftDistanceM, liftAscentM, liftDescentM,
	)
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	return activityID
}
