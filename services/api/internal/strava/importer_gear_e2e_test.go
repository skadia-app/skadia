package strava

import (
	"context"
	"fmt"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
)

// memorySource is a minimal ExportSource that serves pre-loaded CSV rows.
type memorySource struct {
	files map[string][][]string
}

func (m memorySource) BaseDir() string { return "" }

func (m memorySource) FindFiles(_ string) ([]string, error) { return nil, nil }

func (m memorySource) Exists(_ string) bool { return false }

func (m memorySource) ReadCSV(name string) ([][]string, error) {
	rows, ok := m.files[name]
	if !ok {
		return nil, fmt.Errorf("not found: %s", name)
	}
	return rows, nil
}

func (m memorySource) ReadFile(_ string) ([]byte, error) {
	return nil, fmt.Errorf("ReadFile not implemented for memorySource")
}

// insertStravaTestUser inserts a minimal user row and registers cleanup.
func insertStravaTestUser(t *testing.T, db *database.DB) string {
	t.Helper()
	ctx := context.Background()
	uid := uuid.New().String()
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		uid,
		"stravatest_"+uid[:8],
		"stravatest_"+uid[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insertStravaTestUser: %v", err)
	}
	t.Cleanup(func() {
		db.Pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", uid) //nolint:errcheck
	})
	return uid
}

func TestGearImporterE2E_ImportsBikeAndComponent(t *testing.T) {
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	ctx := context.Background()
	userID := insertStravaTestUser(t, db)

	// bikes.csv: Fahrradname, Fahrradmarke, Fahrradmodell, Fahrrad-Standardsportarten
	bikes := [][]string{
		{"Fahrradname", "Fahrradmarke", "Fahrradmodell", "Fahrrad-Standardsportarten"},
		{"Testbike", "Brand", "Model", "Ride"},
	}

	// components.csv: Fahrradname, Komponentenart, Komponentenmarke, Komponentenmodell
	components := [][]string{
		{"Fahrradname", "Komponentenart", "Komponentenmarke", "Komponentenmodell"},
		{"Testbike", "Kette", "KetteBrand", "KetteModel"},
	}

	src := memorySource{files: map[string][][]string{
		"bikes.csv":      bikes,
		"components.csv": components,
	}}

	imp := &GearImporter{}
	result, err := imp.Import(ctx, ImportParams{
		Source: src,
		UserID: userID,
		DB:     db,
	})
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if result.Imported < 2 {
		t.Fatalf("Imported = %d, want >= 2 (1 bike + 1 component); errors: %v", result.Imported, result.Errors)
	}

	// Verify the bike was stored with the correct display name.
	// buildGearName("Testbike", "Brand", "Model") → "Testbike" (name takes priority).
	gears, err := db.ListGear(ctx, userID, "")
	if err != nil {
		t.Fatalf("ListGear: %v", err)
	}
	if len(gears) != 1 {
		t.Fatalf("ListGear returned %d items, want 1", len(gears))
	}
	if gears[0].Name != "Testbike" {
		t.Fatalf("gear.Name = %q, want %q", gears[0].Name, "Testbike")
	}

	// Verify the component was attached and its type mapped correctly.
	// mapComponentType("Kette") → "chain".
	gear, err := db.GetGear(ctx, gears[0].ID)
	if err != nil {
		t.Fatalf("GetGear: %v", err)
	}
	if len(gear.Components) != 1 {
		t.Fatalf("gear.Components len = %d, want 1", len(gear.Components))
	}
	if gear.Components[0].Type != "chain" {
		t.Fatalf("component.Type = %q, want %q", gear.Components[0].Type, "chain")
	}
}
