package strava

import (
	"context"
	"log/slog"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/gear"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
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

	var batchIDPtr *string
	if params.JobID != "" {
		batchIDPtr = &params.JobID
	}

	// --- Bikes ---
	if rows, err := params.Source.ReadCSV("bikes.csv"); err == nil && len(rows) >= 2 {
		hdr := CSVHeader(rows[0])
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

			tracker := params.DB.NewImportTracker(ctx, params.UserID, "strava", displayName, nil, batchIDPtr, model.ImportTypeGear)
			tracker.Processing(ctx)

			modifiers := map[string]any{}
			if isElectricBikeGear(sports, displayName) {
				modifiers["is_electric"] = true
			}

			// Check for duplicate by name
			exists, gearID, err := params.DB.GearExistsByNameForUser(ctx, params.UserID, displayName)
			if err != nil {
				slog.Warn("strava gear: check exists failed", "name", displayName, "error", err)
			}
			if exists {
				result.Skipped++
				bikeNameToID[name] = gearID
				tracker.Skipped(ctx, "duplicate", nil)
				continue
			}

			var brandPtr, modelPtr *string
			if brand != "" {
				brandPtr = &brand
			}
			if modelStr != "" {
				modelPtr = &modelStr
			}

			var applicablePredicates *[]gear.GearDefaultPredicate
			if sports != "" {
				_, preds := mapStravaDefaultSportDefaults(sports)
				applicablePredicates = &preds
			}

			create := &model.GearCreate{
				Name:                      displayName,
				Type:                      "bike",
				Brand:                     brandPtr,
				Model:                     modelPtr,
				ApplicableSportPredicates: applicablePredicates,
				Modifiers:                 modifiers,
			}

			gear, err := params.DB.CreateGear(ctx, params.UserID, create)
			if err != nil {
				slog.Warn("strava gear: create bike failed", "name", displayName, "error", err)
				result.Errors = append(result.Errors, "create bike "+displayName+": "+err.Error())
				tracker.Error(ctx, err)
				continue
			}
			bikeNameToID[name] = gear.ID
			tracker.Completed(ctx, "")
			result.Imported++
			slog.Info("strava gear: imported bike", "name", displayName, "id", gear.ID)
		}
	}

	// --- Shoes ---
	if rows, err := params.Source.ReadCSV("shoes.csv"); err == nil && len(rows) >= 2 {
		hdr := CSVHeader(rows[0])
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

			tracker := params.DB.NewImportTracker(ctx, params.UserID, "strava", displayName, nil, batchIDPtr, model.ImportTypeGear)
			tracker.Processing(ctx)

			// Check for duplicate by name
			exists, _, err := params.DB.GearExistsByNameForUser(ctx, params.UserID, displayName)
			if err != nil {
				slog.Warn("strava gear: check exists failed", "name", displayName, "error", err)
			}
			if exists {
				result.Skipped++
				tracker.Skipped(ctx, "duplicate", nil)
				continue
			}

			var brandPtr, modelPtr *string
			if brand != "" {
				brandPtr = &brand
			}
			if modelStr != "" {
				modelPtr = &modelStr
			}

			var applicablePredicates *[]gear.GearDefaultPredicate
			if sports != "" {
				_, preds := mapStravaDefaultSportDefaults(sports)
				applicablePredicates = &preds
			}

			create := &model.GearCreate{
				Name:                      displayName,
				Type:                      mapStravaShoeType(sports),
				Brand:                     brandPtr,
				Model:                     modelPtr,
				ApplicableSportPredicates: applicablePredicates,
			}

			gear, err := params.DB.CreateGear(ctx, params.UserID, create)
			if err != nil {
				slog.Warn("strava gear: create shoes failed", "name", displayName, "error", err)
				result.Errors = append(result.Errors, "create shoes "+displayName+": "+err.Error())
				tracker.Error(ctx, err)
				continue
			}
			tracker.Completed(ctx, "")
			result.Imported++
			slog.Info("strava gear: imported shoes", "name", displayName, "id", gear.ID)
		}
	}

	// --- Components ---
	if rows, err := params.Source.ReadCSV("components.csv"); err == nil && len(rows) >= 2 {
		hdr := CSVHeader(rows[0])
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

			mappedType := mapComponentType(compType)
			if g.skipOrUpdateExistingComponent(ctx, params, gearID, compName, mappedType) {
				result.Skipped++
				continue
			}

			create := &model.GearComponentCreate{
				Name:  compName,
				Type:  mappedType,
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

func isElectricBikeGear(sports, displayName string) bool {
	return containsAnyFolded(sports, "e-bike", "ebike", "e-mountain") ||
		containsAnyFolded(displayName, "e-bike", "ebike", "e-mountain")
}

func mapStravaShoeType(sports string) string {
	_, predicates := mapStravaDefaultSportDefaults(sports)
	for _, predicate := range predicates {
		switch predicate.SportType {
		case "cycling":
			return "shoes.cycling"
		case "running":
			if predicate.SubType != nil && *predicate.SubType == "trail" {
				return "shoes.running.trail"
			}
			return "shoes.running"
		case "hiking", "walking":
			return "shoes.hiking"
		case "rock_climbing":
			return "shoes.climbing"
		case "strength_training", "training", "fitness_equipment":
			return "shoes.training"
		}
	}
	return "shoes"
}

func mapStravaDefaultSportPredicates(sports string) []gear.GearDefaultPredicate {
	_, predicates := mapStravaDefaultSportDefaults(sports)
	return predicates
}

func mapStravaDefaultSportDefaults(sports string) ([]string, []gear.GearDefaultPredicate) {
	// Detect separator: pipe takes precedence (English export).
	sep := ","
	if strings.Contains(sports, "|") {
		sep = "|"
	}
	var defaultSports []string
	var predicates []gear.GearDefaultPredicate
	seenSports := make(map[string]bool)
	seenPredicates := make(map[string]bool)
	for s := range strings.SplitSeq(sports, sep) {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		predicate, ok := gear.GearDefaultPredicateForSource(sport.SourceStrava, s)
		if !ok || predicate.SportType == "" {
			continue
		}
		if !seenSports[predicate.SportType] {
			defaultSports = append(defaultSports, predicate.SportType)
			seenSports[predicate.SportType] = true
		}
		key := gearDefaultPredicateKey(predicate)
		if !seenPredicates[key] {
			predicates = append(predicates, predicate)
			seenPredicates[key] = true
		}
	}
	return defaultSports, predicates
}

func gearDefaultPredicateKey(predicate gear.GearDefaultPredicate) string {
	key := predicate.SportType
	if predicate.SubType != nil {
		key += ":" + *predicate.SubType
	}
	if predicate.Modifiers != nil {
		if predicate.Modifiers.IsElectric != nil {
			key += "|is_electric=" + boolString(*predicate.Modifiers.IsElectric)
		}
		if predicate.Modifiers.IsIndoor != nil {
			key += "|is_indoor=" + boolString(*predicate.Modifiers.IsIndoor)
		}
		if predicate.Modifiers.IsVirtual != nil {
			key += "|is_virtual=" + boolString(*predicate.Modifiers.IsVirtual)
		}
	}
	return key
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// mapComponentType normalises German component type names.
func mapComponentType(compType string) string {
	compType = strings.TrimSpace(compType)
	lower := strings.ToLower(compType)
	switch {
	case containsAnyFolded(lower, "kettenblatt", "chainring"):
		return "bike.chainring"
	case containsAnyFolded(lower, "kette", "chain"):
		return "bike.chain"
	case strings.Contains(lower, "rahmen"):
		return "bike.frame"
	case containsAnyFolded(lower, "gabel", "fork"):
		return "bike.fork"
	case containsAnyFolded(lower, "daempfer", "dämpfer", "shock"):
		return "bike.shock"
	case containsAnyFolded(lower, "reifen", "tire"):
		return "bike.tire"
	case containsAnyFolded(lower, "schlauch", "tube"):
		return "bike.tube"
	case containsAnyFolded(lower, "kassette", "cassette", "zahnkranz"):
		return "bike.cassette"
	case containsAnyFolded(lower, "bremsbelag", "bremsbeläg", "brake pad", "pad"):
		return "bike.brake_pads"
	case containsAnyFolded(lower, "bremse", "brake"):
		return "bike.brake"
	case strings.Contains(lower, "pedale"), strings.Contains(lower, "pedal"):
		return "bike.pedals"
	case strings.Contains(lower, "sattelstütze"), strings.Contains(lower, "sattelstuetze"), strings.Contains(lower, "seatpost"):
		return "bike.seatpost"
	case strings.Contains(lower, "sattel"), strings.Contains(lower, "saddle"):
		return "bike.saddle"
	case strings.Contains(lower, "lenker"), strings.Contains(lower, "handlebar"):
		return "bike.handlebar"
	case strings.Contains(lower, "vorbau"), strings.Contains(lower, "stem"):
		return "bike.stem"
	case strings.Contains(lower, "kurbel"), strings.Contains(lower, "crank"):
		return "bike.crank_arms"
	case strings.Contains(lower, "umwerfer"), strings.Contains(lower, "front derailleur"):
		return "bike.front_derailleur"
	case strings.Contains(lower, "schaltwerk"), strings.Contains(lower, "derailleur"):
		return "bike.rear_derailleur"
	case strings.Contains(lower, "laufrad"), strings.Contains(lower, "wheel"):
		return "bike.wheel"
	case strings.Contains(lower, "felge"), strings.Contains(lower, "rim"):
		return "bike.rim"
	case strings.Contains(lower, "nabe"), strings.Contains(lower, "hub"):
		return "bike.hub"
	default:
		return "other"
	}
}

func containsAnyFolded(value string, needles ...string) bool {
	lower := strings.ToLower(value)
	for _, needle := range needles {
		if strings.Contains(lower, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func (g *GearImporter) skipOrUpdateExistingComponent(ctx context.Context, params ImportParams, gearID, name, mappedType string) bool {
	candidates, err := params.DB.FindComponentsByName(ctx, gearID, name)
	if err != nil {
		slog.Warn("strava gear: find existing component failed", "name", name, "error", err)
		return false
	}
	if len(candidates) == 0 {
		return false
	}
	if mappedType != "" && candidates[0].Type != mappedType {
		if _, err := params.DB.UpdateComponent(ctx, candidates[0].ID, nil, &mappedType, nil, nil, nil, nil, nil, nil, nil, nil, nil); err != nil {
			slog.Warn("strava gear: update existing component type failed", "name", name, "type", mappedType, "error", err)
		}
	}
	return true
}
