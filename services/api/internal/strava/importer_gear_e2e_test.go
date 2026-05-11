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
		{"Testbike", "Brand", "Model", "E-Mountain Bike Ride"},
	}

	// components.csv: Fahrradname, Komponentenart, Komponentenmarke, Komponentenmodell
	components := [][]string{
		{"Fahrradname", "Komponentenart", "Komponentenmarke", "Komponentenmodell"},
		{"Testbike", "Kette", "KetteBrand", "KetteModel"},
		{"Testbike", "Federgabel", "ForkBrand", "ForkModel"},
		{"Testbike", "Bremsbeläge", "BrakeBrand", "BrakeModel"},
		{"Testbike", "Zahnkranzpaket", "CassetteBrand", "CassetteModel"},
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
	if result.Imported < 5 {
		t.Fatalf("Imported = %d, want >= 5 (1 bike + 4 components); errors: %v", result.Imported, result.Errors)
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
	if gears[0].Type != "bike" {
		t.Fatalf("gear.Type = %q, want %q", gears[0].Type, "bike")
	}
	if got, ok := gears[0].Modifiers["is_electric"].(bool); !ok || !got {
		t.Fatalf("gear.Modifiers[is_electric] = %#v, want true", gears[0].Modifiers["is_electric"])
	}

	// Verify components were attached and mapped to schema component keys.
	gear, err := db.GetGear(ctx, gears[0].ID)
	if err != nil {
		t.Fatalf("GetGear: %v", err)
	}
	if len(gear.Components) != 4 {
		t.Fatalf("gear.Components len = %d, want 4", len(gear.Components))
	}
	componentTypes := map[string]string{}
	for _, component := range gear.Components {
		componentTypes[component.Name] = component.Type
	}
	wantTypes := map[string]string{
		"Kette KetteModel":             "bike.chain",
		"Federgabel ForkModel":         "bike.fork",
		"Bremsbeläge BrakeModel":       "bike.brake_pads",
		"Zahnkranzpaket CassetteModel": "bike.cassette",
	}
	for name, want := range wantTypes {
		if componentTypes[name] != want {
			t.Fatalf("component %q type = %q, want %q", name, componentTypes[name], want)
		}
	}

	result, err = imp.Import(ctx, ImportParams{
		Source: src,
		UserID: userID,
		DB:     db,
	})
	if err != nil {
		t.Fatalf("second Import returned error: %v", err)
	}
	if result.Imported != 0 {
		t.Fatalf("second Import imported %d items, want 0 duplicate creates", result.Imported)
	}
	gear, err = db.GetGear(ctx, gears[0].ID)
	if err != nil {
		t.Fatalf("GetGear after second import: %v", err)
	}
	if len(gear.Components) != 4 {
		t.Fatalf("gear.Components len after second import = %d, want 4", len(gear.Components))
	}
}
