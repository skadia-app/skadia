package auth

// Tests that the three gear tables added in Mig-354 are exported with the
// correct user_id = $1 filter — i.e. User-A's export never contains User-B's
// rows and vice versa.
//
// Pattern mirrors gdpr_fusion_roundtrip_test.go: uses executeExportJobOnce
// directly to avoid the async HTTP layer.

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func openGearExportDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Open(t)
}

// createUserForGearTest inserts a minimal user and registers a t.Cleanup delete.
func createUserForGearTest(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.New().String()
	email := fmt.Sprintf("gear-export-%s@example.com", id[:8])
	uname := fmt.Sprintf("gearexp_%s", id[:8])
	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, username, email, display_name, timezone, email_verified, registration_source)
		VALUES ($1, $2, $3, 'Gear Export Test', 'UTC', true, 'test')
	`, id, uname, email)
	if err != nil {
		t.Fatalf("createUserForGearTest: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id)
	})
	return id
}

// insertGearRow inserts a minimal gear row for the given user and returns its UUID.
func insertGearRow(t *testing.T, pool *pgxpool.Pool, userID, name string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO gear (id, user_id, name, type, status, scope)
		VALUES ($1, $2, $3, 'bike', 'active', 'user')
	`, id, userID, name)
	if err != nil {
		t.Fatalf("insertGearRow %q: %v", name, err)
	}
	return id
}

// insertActivityForGear inserts a minimal activity row and returns its UUID.
func insertActivityForGear(t *testing.T, pool *pgxpool.Pool, userID string) string {
	t.Helper()
	id := uuid.New().String()
	now := time.Now().UTC()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO activities (id, user_id, title, sport_type, started_at, ended_at,
		                        visibility, is_manual_entry, created_at, updated_at)
		VALUES ($1, $2, 'Gear Export Activity', 'cycling', $3, $4, 'private', false, $3, $3)
	`, id, userID, now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("insertActivityForGear: %v", err)
	}
	return id
}

// insertAssignment inserts one row in activity_gear_assignments.
func insertAssignment(t *testing.T, pool *pgxpool.Pool, userID, activityID, gearID string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO activity_gear_assignments
		    (id, user_id, activity_id, gear_id, role_key, source, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'primary', 'test', NOW(), NOW())
	`, id, userID, activityID, gearID)
	if err != nil {
		t.Fatalf("insertAssignment: %v", err)
	}
	return id
}

// insertSwapEvent inserts one row in gear_component_swap_events.
// Uses gear_component_swap_events which is validated by user_id = gear.user_id.
// Both retired_component_id and new_component_id are optional (set to NULL here
// so no gear_components rows are required).
func insertSwapEvent(t *testing.T, pool *pgxpool.Pool, userID, gearID string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO gear_component_swap_events
		    (id, user_id, gear_id, slot_key, swap_date, created_at)
		VALUES ($1, $2, $3, 'rear_derailleur', NOW(), NOW())
	`, id, userID, gearID)
	if err != nil {
		t.Fatalf("insertSwapEvent: %v", err)
	}
	return id
}

// insertSchemaDefinition inserts one row in gear_schema_definitions.
func insertSchemaDefinition(t *testing.T, pool *pgxpool.Pool, userID, typeKey string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO gear_schema_definitions (id, user_id, type_key, definition, created_at, updated_at)
		VALUES ($1, $2, $3, '{"label":"Custom"}'::jsonb, NOW(), NOW())
	`, id, userID, typeKey)
	if err != nil {
		t.Fatalf("insertSchemaDefinition: %v", err)
	}
	return id
}

// runExportForUser executes an export job for the given user and returns the
// path to the resulting ZIP file. The caller is responsible for deleting it.
func runExportForUser(t *testing.T, pool *pgxpool.Pool, userID string) string {
	t.Helper()
	ctx := context.Background()

	db := &database.DB{Pool: pool}
	cfg := &config.Config{JWTSecret: strings.Repeat("ab", 32)}
	st := settings.NewStoreForTest(map[string]string{})
	h := NewHandler(db, cfg, st, &Mailer{}, middleware.NewRateLimiter(ctx), ctx)
	h.ShadowFS = newRoundtripShadowFS()
	store, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("runExportForUser: NewLocalStore: %v", err)
	}
	h.ArtifactStore = store

	jobID := uuid.New().String()
	_, err = pool.Exec(ctx, `
		INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at)
		VALUES ($1, 'gdpr_export', $2, 'running', NOW(), NOW())
	`, jobID, userID)
	if err != nil {
		t.Fatalf("runExportForUser: insert job: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM background_jobs WHERE id = $1", jobID)
	})

	if err := h.executeExportJobOnce(ctx, jobID, userID); err != nil {
		t.Fatalf("runExportForUser: executeExportJobOnce: %v", err)
	}

	return materializeExportArtifactZip(t, ctx, pool, store, jobID)
}

func materializeExportArtifactZip(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store gdprartifacts.Store, jobID string) string {
	t.Helper()

	var metadata []byte
	if err := pool.QueryRow(ctx,
		`SELECT metadata FROM background_jobs WHERE id = $1`, jobID,
	).Scan(&metadata); err != nil {
		t.Fatalf("materializeExportArtifactZip: read metadata: %v", err)
	}
	meta, err := gdprartifacts.DecodeJobMetadata(metadata)
	if err != nil {
		t.Fatalf("materializeExportArtifactZip: DecodeJobMetadata: %v", err)
	}
	if meta.Artifact == nil || len(meta.Artifact.Objects) != 1 {
		t.Fatalf("materializeExportArtifactZip: artifact metadata = %#v", meta.Artifact)
	}
	reader, err := store.Open(ctx, meta.Artifact.Objects[0], nil)
	if err != nil {
		t.Fatalf("materializeExportArtifactZip: open artifact: %v", err)
	}
	defer reader.Close()

	tmp, err := os.CreateTemp(t.TempDir(), "skadia-export-*.zip")
	if err != nil {
		t.Fatalf("materializeExportArtifactZip: create temp zip: %v", err)
	}
	if _, err := io.Copy(tmp, reader); err != nil {
		tmp.Close()
		t.Fatalf("materializeExportArtifactZip: copy artifact: %v", err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatalf("materializeExportArtifactZip: close temp zip: %v", err)
	}
	return tmp.Name()
}

// readZIPEntry decodes a JSON-array entry from the ZIP into a slice of maps.
// Returns nil if the entry is absent.
func readZIPEntry(t *testing.T, zipPath, entryName string) []map[string]any {
	t.Helper()
	rc, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("readZIPEntry: open %q: %v", zipPath, err)
	}
	defer rc.Close()

	for _, f := range rc.File {
		if f.Name == entryName {
			r, err := f.Open()
			if err != nil {
				t.Fatalf("readZIPEntry: open entry %q: %v", entryName, err)
			}
			defer r.Close()
			var rows []map[string]any
			if err := json.NewDecoder(r).Decode(&rows); err != nil {
				t.Fatalf("readZIPEntry: decode %q: %v", entryName, err)
			}
			return rows
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Task 11.1b — Tests
// ---------------------------------------------------------------------------

// TestGearSchemaDefinitionsExportFiltersByUserId verifies that
// gear_schema_definitions rows are scoped to the requesting user.
func TestGearSchemaDefinitionsExportFiltersByUserId(t *testing.T) {
	pool := openGearExportDB(t)

	userA := createUserForGearTest(t, pool)
	userB := createUserForGearTest(t, pool)

	rowAID := insertSchemaDefinition(t, pool, userA, "custom_bike_a")
	rowBID := insertSchemaDefinition(t, pool, userB, "custom_bike_b")

	zipPath := runExportForUser(t, pool, userA)
	t.Cleanup(func() { os.Remove(zipPath) })

	rows := readZIPEntry(t, zipPath, "data/gear_schema_definitions.json")
	if rows == nil {
		t.Fatal("data/gear_schema_definitions.json missing from export ZIP")
	}

	ids := make(map[string]bool, len(rows))
	for _, row := range rows {
		if id, ok := row["id"].(string); ok {
			ids[id] = true
		}
	}

	if !ids[rowAID] {
		t.Errorf("User-A's gear_schema_definition %s not found in export", rowAID)
	}
	if ids[rowBID] {
		t.Errorf("User-B's gear_schema_definition %s leaked into User-A's export", rowBID)
	}
}

// TestActivityGearAssignmentsExportFiltersByUserId verifies that
// activity_gear_assignments rows are scoped to the requesting user.
func TestActivityGearAssignmentsExportFiltersByUserId(t *testing.T) {
	pool := openGearExportDB(t)

	userA := createUserForGearTest(t, pool)
	userB := createUserForGearTest(t, pool)

	gearA := insertGearRow(t, pool, userA, "Gear A")
	gearB := insertGearRow(t, pool, userB, "Gear B")
	actA := insertActivityForGear(t, pool, userA)
	actB := insertActivityForGear(t, pool, userB)
	rowAID := insertAssignment(t, pool, userA, actA, gearA)
	rowBID := insertAssignment(t, pool, userB, actB, gearB)

	zipPath := runExportForUser(t, pool, userA)
	t.Cleanup(func() { os.Remove(zipPath) })

	rows := readZIPEntry(t, zipPath, "data/activity_gear_assignments.json")
	if rows == nil {
		t.Fatal("data/activity_gear_assignments.json missing from export ZIP")
	}

	ids := make(map[string]bool, len(rows))
	for _, row := range rows {
		if id, ok := row["id"].(string); ok {
			ids[id] = true
		}
	}

	if !ids[rowAID] {
		t.Errorf("User-A's assignment %s not found in export", rowAID)
	}
	if ids[rowBID] {
		t.Errorf("User-B's assignment %s leaked into User-A's export", rowBID)
	}
}

// TestGearComponentSwapEventsExportFiltersByUserId verifies that
// gear_component_swap_events rows are scoped to the requesting user.
func TestGearComponentSwapEventsExportFiltersByUserId(t *testing.T) {
	pool := openGearExportDB(t)

	userA := createUserForGearTest(t, pool)
	userB := createUserForGearTest(t, pool)

	gearA := insertGearRow(t, pool, userA, "Gear Swap A")
	gearB := insertGearRow(t, pool, userB, "Gear Swap B")
	rowAID := insertSwapEvent(t, pool, userA, gearA)
	rowBID := insertSwapEvent(t, pool, userB, gearB)

	zipPath := runExportForUser(t, pool, userA)
	t.Cleanup(func() { os.Remove(zipPath) })

	rows := readZIPEntry(t, zipPath, "data/gear_component_swap_events.json")
	if rows == nil {
		t.Fatal("data/gear_component_swap_events.json missing from export ZIP")
	}

	ids := make(map[string]bool, len(rows))
	for _, row := range rows {
		if id, ok := row["id"].(string); ok {
			ids[id] = true
		}
	}

	if !ids[rowAID] {
		t.Errorf("User-A's swap event %s not found in export", rowAID)
	}
	if ids[rowBID] {
		t.Errorf("User-B's swap event %s leaked into User-A's export", rowBID)
	}
}
