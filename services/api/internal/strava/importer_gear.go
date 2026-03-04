package strava

import (
	"context"
	"log/slog"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// GearImporter reads bikes, shoes, and components from a Strava export.
//
// bikes.csv columns (German):
//
//	Fahrradname, Fahrradmarke, Fahrradmodell, Fahrrad-Standardsportarten
//
// shoes.csv columns (German):
//
//	Schuhname, Schuhmarke, Schuhmodell, Schuh-Standardsportarten
//
// components.csv columns (German):
//
//	Fahrradname, Komponentenart, Komponentenmarke, Komponentenmodell
type GearImporter struct{}

func (g *GearImporter) Name() string        { return "gear" }
func (g *GearImporter) Description() string { return "Ausrustung (Fahrräder, Schuhe, Komponenten)" }
func (g *GearImporter) Priority() int       { return 20 }

func (g *GearImporter) Detect(src ExportSource) (bool, int, error) {
	count := 0
	if rows, err := src.ReadCSV("bikes.csv"); err == nil && len(rows) > 1 {
		count += len(rows) - 1
	}
	if rows, err := src.ReadCSV("shoes.csv"); err == nil && len(rows) > 1 {
		count += len(rows) - 1
	}
	return count > 0, count, nil
}

func (g *GearImporter) Import(ctx context.Context, params ImportParams) (*ImportResult, error) {
	result := &ImportResult{}

	// Maps Strava bike name → skadia gear ID (for component assignment)
	bikeNameToID := make(map[string]string)

	// --- Bikes ---
	if rows, err := params.Source.ReadCSV("bikes.csv"); err == nil && len(rows) >= 2 {
		hdr := csvHeader(rows[0])
		total := len(rows) - 1
		for i, row := range rows[1:] {
			if params.OnProgress != nil {
				params.OnProgress(i+1, total, "gear (bikes): "+colVal(row, hdr["Fahrradname"]))
			}

			name := colVal(row, hdr["Fahrradname"])
			brand := colVal(row, hdr["Fahrradmarke"])
			modelStr := colVal(row, hdr["Fahrradmodell"])
			sports := colVal(row, hdr["Fahrrad-Standardsportarten"])

			// Derive a meaningful display name
			displayName := buildGearName(name, brand, modelStr)

			// Detect e-bike
			gearType := "bike"
			if strings.Contains(strings.ToLower(sports), "e-bike") || strings.Contains(strings.ToLower(displayName), "e-bike") {
				gearType = "e_bike"
			}

			// Check for duplicate by name
			exists, gearID, err := params.DB.GearExistsByNameForUser(ctx, params.UserID, displayName)
			if err != nil {
				slog.Warn("strava gear: check exists failed", "name", displayName, "error", err)
			}
			if exists {
				result.Skipped++
				bikeNameToID[name] = gearID
				continue
			}

			var brandPtr, modelPtr *string
			if brand != "" {
				brandPtr = &brand
			}
			if modelStr != "" {
				modelPtr = &modelStr
			}

			var defaultSports []string
			if sports != "" {
				defaultSports = mapStravaDefaultSports(sports)
			}

			create := &model.GearCreate{
				Name:             displayName,
				Type:             gearType,
				Brand:            brandPtr,
				Model:            modelPtr,
				DefaultForSports: defaultSports,
			}

			gear, err := params.DB.CreateGear(ctx, params.UserID, create)
			if err != nil {
				slog.Warn("strava gear: create bike failed", "name", displayName, "error", err)
				result.Errors = append(result.Errors, "create bike "+displayName+": "+err.Error())
				continue
			}
			bikeNameToID[name] = gear.ID
			result.Imported++
			slog.Info("strava gear: imported bike", "name", displayName, "id", gear.ID)
		}
	}

	// --- Shoes ---
	if rows, err := params.Source.ReadCSV("shoes.csv"); err == nil && len(rows) >= 2 {
		hdr := csvHeader(rows[0])
		total := len(rows) - 1
		for i, row := range rows[1:] {
			if params.OnProgress != nil {
				params.OnProgress(i+1, total, "gear (shoes): "+colVal(row, hdr["Schuhname"]))
			}

			name := colVal(row, hdr["Schuhname"])
			brand := colVal(row, hdr["Schuhmarke"])
			modelStr := colVal(row, hdr["Schuhmodell"])
			sports := colVal(row, hdr["Schuh-Standardsportarten"])

			displayName := buildGearName(name, brand, modelStr)

			// Check for duplicate by name
			exists, _, err := params.DB.GearExistsByNameForUser(ctx, params.UserID, displayName)
			if err != nil {
				slog.Warn("strava gear: check exists failed", "name", displayName, "error", err)
			}
			if exists {
				result.Skipped++
				continue
			}

			var brandPtr, modelPtr *string
			if brand != "" {
				brandPtr = &brand
			}
			if modelStr != "" {
				modelPtr = &modelStr
			}

			var defaultSports []string
			if sports != "" {
				defaultSports = mapStravaDefaultSports(sports)
			}

			create := &model.GearCreate{
				Name:             displayName,
				Type:             "shoes",
				Brand:            brandPtr,
				Model:            modelPtr,
				DefaultForSports: defaultSports,
			}

			gear, err := params.DB.CreateGear(ctx, params.UserID, create)
			if err != nil {
				slog.Warn("strava gear: create shoes failed", "name", displayName, "error", err)
				result.Errors = append(result.Errors, "create shoes "+displayName+": "+err.Error())
				continue
			}
			result.Imported++
			slog.Info("strava gear: imported shoes", "name", displayName, "id", gear.ID)
		}
	}

	// --- Components ---
	if rows, err := params.Source.ReadCSV("components.csv"); err == nil && len(rows) >= 2 {
		hdr := csvHeader(rows[0])
		total := len(rows) - 1
		for i, row := range rows[1:] {
			bikeName := colVal(row, hdr["Fahrradname"])
			compType := colVal(row, hdr["Komponentenart"])
			compBrand := colVal(row, hdr["Komponentenmarke"])
			compModel := colVal(row, hdr["Komponentenmodell"])

			gearID, ok := bikeNameToID[bikeName]
			if !ok || gearID == "" {
				// Try to look up gear by name
				exists, id, _ := params.DB.GearExistsByNameForUser(ctx, params.UserID, bikeName)
				if !exists {
					result.Skipped++
					continue
				}
				gearID = id
				bikeNameToID[bikeName] = gearID
			}

			if params.OnProgress != nil {
				params.OnProgress(i+1, total, "components: "+compType)
			}

			compName := compType
			if compModel != "" {
				compName = compType + " " + compModel
			}

			var brandPtr, modelPtr *string
			if compBrand != "" {
				b := strings.TrimSpace(compBrand)
				brandPtr = &b
			}
			if compModel != "" {
				m := strings.TrimSpace(compModel)
				modelPtr = &m
			}

			create := &model.GearComponentCreate{
				Name:  compName,
				Type:  mapComponentType(compType),
				Brand: brandPtr,
				Model: modelPtr,
			}

			if _, err := params.DB.CreateComponent(ctx, gearID, create); err != nil {
				slog.Warn("strava gear: create component failed", "type", compType, "error", err)
				// Don't count as error — components are optional
				continue
			}
			result.Imported++
		}
	}

	slog.Info("strava gear import done", "imported", result.Imported, "skipped", result.Skipped)
	return result, nil
}

// buildGearName constructs a display name from name, brand, and model fields.
// Priority: explicit name > brand+model > model > brand > "Unbekannt"
func buildGearName(name, brand, model string) string {
	if name != "" {
		return name
	}
	if brand != "" && model != "" {
		return brand + " " + model
	}
	if model != "" {
		return model
	}
	if brand != "" {
		return brand
	}
	return "Unbekannte Ausrüstung"
}

// mapStravaDefaultSports converts Strava sport strings to Skadia sport types.
func mapStravaDefaultSports(sports string) []string {
	var result []string
	for _, s := range strings.Split(sports, ",") {
		s = strings.TrimSpace(s)
		mapped := mapStravaSportType(s)
		if mapped != "" {
			result = append(result, mapped)
		}
	}
	return result
}

// mapComponentType normalises German component type names.
func mapComponentType(compType string) string {
	compType = strings.TrimSpace(compType)
	lower := strings.ToLower(compType)
	switch {
	case strings.Contains(lower, "kette"):
		return "chain"
	case strings.Contains(lower, "rahmen"):
		return "frame"
	case strings.Contains(lower, "reifen"), strings.Contains(lower, "tire"):
		return "tire"
	case strings.Contains(lower, "schlauch"), strings.Contains(lower, "tube"):
		return "inner_tube"
	case strings.Contains(lower, "kassette"), strings.Contains(lower, "cassette"):
		return "cassette"
	case strings.Contains(lower, "bremse"), strings.Contains(lower, "brake"):
		return "brake"
	case strings.Contains(lower, "belag"), strings.Contains(lower, "pad"):
		return "brake_pad"
	case strings.Contains(lower, "pedale"), strings.Contains(lower, "pedal"):
		return "pedal"
	case strings.Contains(lower, "sattel"), strings.Contains(lower, "saddle"):
		return "saddle"
	case strings.Contains(lower, "lenker"), strings.Contains(lower, "handlebar"):
		return "handlebar"
	case strings.Contains(lower, "vorbau"), strings.Contains(lower, "stem"):
		return "stem"
	case strings.Contains(lower, "kurbel"), strings.Contains(lower, "crank"):
		return "crank"
	case strings.Contains(lower, "umwerfer"), strings.Contains(lower, "derailleur"):
		return "derailleur"
	case strings.Contains(lower, "schaltwerk"):
		return "derailleur"
	case strings.Contains(lower, "laufrad"), strings.Contains(lower, "wheel"):
		return "wheel"
	case strings.Contains(lower, "felge"), strings.Contains(lower, "rim"):
		return "rim"
	case strings.Contains(lower, "nabe"), strings.Contains(lower, "hub"):
		return "hub"
	default:
		return "other"
	}
}
