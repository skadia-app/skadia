package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/gear"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// GearImporter reads gear data from DI_CONNECT/DI-Connect-Fitness/*_gear.json.
//
// Real Garmin export structure:
//
//	DI_CONNECT/DI-Connect-Fitness/<email>_gear.json
//	  → list of objects with:
//	       gearDTOS            – array of gear items
//	       gearActivityDTOs    – activity<->gear associations
//	       gearActivityTypeDTOS – sport type defaults
//
// Each gearDTO has:
//
//	gearPk, uuid, customMakeModel, gearTypeName, gearStatusName,
//	dateBegin, createDate, updateDate, maximumMeters, userProfilePk
type GearImporter struct{}

func (g *GearImporter) Name() string        { return "gear" }
func (g *GearImporter) Description() string { return "Ausrüstung (Schuhe, Fahrräder, …)" }
func (g *GearImporter) Priority() int       { return 20 }

// garminGearFile is the top-level wrapper returned by the gear JSON.
type garminGearFile struct {
	GearDTOS             []garminGearDTO   `json:"gearDTOS"`
	GearActivityTypeDTOS []json.RawMessage `json:"gearActivityTypeDTOS"`
}

type garminGearDTO struct {
	GearPk          int64   `json:"gearPk"`
	UUID            string  `json:"uuid"`
	CustomMakeModel string  `json:"customMakeModel"` // "Brand Model" or just model name
	GearTypeName    string  `json:"gearTypeName"`    // e.g. "Bike", "Shoes" (mixed case in real exports)
	GearStatusName  string  `json:"gearStatusName"`  // "active", "retired" (mixed case in real exports)
	DateBegin       string  `json:"dateBegin"`       // ISO date
	MaximumMeters   float64 `json:"maximumMeters"`
	// GearVersion is an integer in real Garmin exports (e.g. 0), not a string — omit to avoid unmarshal failure
}

type garminGearDefaults struct {
	Sports     []string
	Predicates []gear.GearDefaultPredicate
}

func (g *GearImporter) Detect(src ExportSource) (bool, int, error) {
	files, err := src.FindFiles("DI_CONNECT/DI-Connect-Fitness/*_gear.json")
	if err != nil || len(files) == 0 {
		return false, 0, nil
	}
	var wrapper []garminGearFile
	if err := src.ReadJSON(files[0], &wrapper); err != nil {
		return false, 0, nil
	}
	if len(wrapper) == 0 {
		return false, 0, nil
	}
	return true, len(wrapper[0].GearDTOS), nil
}

func (g *GearImporter) Import(ctx context.Context, params ImportParams) (*ImportResult, error) {
	result := &ImportResult{}

	files, err := params.Source.FindFiles("DI_CONNECT/DI-Connect-Fitness/*_gear.json")
	if err != nil || len(files) == 0 {
		slog.Warn("garmin gear: no gear file found")
		return result, nil
	}

	var wrapper []garminGearFile
	if err := params.Source.ReadJSON(files[0], &wrapper); err != nil {
		slog.Warn("garmin gear: cannot read gear file", "error", err)
		return result, nil
	}
	if len(wrapper) == 0 {
		return result, nil
	}
	defaultSports := garminDefaultSportsByGear(wrapper[0])

	items := wrapper[0].GearDTOS
	total := len(items)

	for i, dto := range items {
		if params.OnProgress != nil {
			params.OnProgress(i+1, total, "gear: "+dto.CustomMakeModel)
		}

		create := gearCreateFromGarminDTO(dto, defaultSports)
		gearName := create.Name

		status := "active"
		if strings.EqualFold(dto.GearStatusName, "RETIRED") || strings.EqualFold(dto.GearStatusName, "NOT_ACTIVE") {
			status = "retired"
		}

		garminID := dto.UUID
		if garminID == "" {
			garminID = strings.TrimSpace(strings.Join([]string{dto.GearTypeName, dto.CustomMakeModel}, "_"))
		}

		// Check if already imported (by garmin_gear_id)
		exists, err := params.DB.GearExistsByGarminID(ctx, params.UserID, garminID)
		if err != nil {
			slog.Warn("garmin gear: check exists failed", "error", err)
		}
		if exists {
			result.Skipped++
			continue
		}

		gear, err := params.DB.CreateGear(ctx, params.UserID, create)
		if err != nil {
			slog.Warn("garmin gear: create gear failed", "name", gearName, "error", err)
			result.Errors = append(result.Errors, "create gear "+gearName+": "+err.Error())
			continue
		}

		// Set garmin_gear_id
		if err := params.DB.SetGearGarminID(ctx, gear.ID, garminID); err != nil {
			slog.Warn("garmin gear: set garmin_gear_id failed", "error", err)
		}

		// Update status if retired
		if status == "retired" {
			if err := params.DB.UpdateGearStatus(ctx, gear.ID, params.UserID, status); err != nil {
				slog.Warn("garmin gear: update status failed", "error", err)
			}
		}

		result.Imported++
	}

	slog.Info("garmin gear import done", "imported", result.Imported, "skipped", result.Skipped)
	return result, nil
}

func gearCreateFromGarminDTO(dto garminGearDTO, defaultSports map[string]garminGearDefaults) *model.GearCreate {
	gearName := dto.CustomMakeModel
	if gearName == "" {
		gearName = dto.GearTypeName
	}
	var purchasedAt *string
	if dto.DateBegin != "" {
		purchasedAt = &dto.DateBegin
	}
	// garminDefaultPredicatesForDTO returns nil when no defaults are known for
	// this device — nil maps to "field not sent, compute from taxonomy".
	preds := garminDefaultPredicatesForDTO(dto, defaultSports)
	var applicablePredicates *[]gear.GearDefaultPredicate
	if preds != nil {
		applicablePredicates = &preds
	}
	return &model.GearCreate{
		Name:                     gearName,
		Type:                     mapGearType(dto.GearTypeName),
		PurchasedAt:              purchasedAt,
		ApplicableSportPredicates: applicablePredicates,
	}
}

func garminDefaultSportsForDTO(dto garminGearDTO, defaults map[string]garminGearDefaults) []string {
	for _, key := range garminGearDefaultKeys(dto.GearPk, dto.UUID) {
		if defaultSet, ok := defaults[key]; ok {
			return append([]string(nil), defaultSet.Sports...)
		}
	}
	return nil
}

func garminDefaultPredicatesForDTO(dto garminGearDTO, defaults map[string]garminGearDefaults) []gear.GearDefaultPredicate {
	for _, key := range garminGearDefaultKeys(dto.GearPk, dto.UUID) {
		if defaultSet, ok := defaults[key]; ok {
			return append([]gear.GearDefaultPredicate(nil), defaultSet.Predicates...)
		}
	}
	return nil
}

func garminDefaultSportsByGear(file garminGearFile) map[string]garminGearDefaults {
	defaults := map[string]garminGearDefaults{}
	seenSports := map[string]map[string]struct{}{}
	seenPredicates := map[string]map[string]struct{}{}
	for _, raw := range file.GearActivityTypeDTOS {
		gearKeys, rawSport := parseGarminGearActivityTypeDefault(raw)
		if len(gearKeys) == 0 || rawSport == "" {
			continue
		}
		predicate, ok := garminGearDefaultPredicate(rawSport)
		if !ok {
			continue
		}
		for _, key := range gearKeys {
			defaultSet := defaults[key]
			if seenSports[key] == nil {
				seenSports[key] = map[string]struct{}{}
			}
			if _, ok := seenSports[key][predicate.SportType]; !ok {
				seenSports[key][predicate.SportType] = struct{}{}
				defaultSet.Sports = append(defaultSet.Sports, predicate.SportType)
			}
			if seenPredicates[key] == nil {
				seenPredicates[key] = map[string]struct{}{}
			}
			predicateKey := garminGearDefaultPredicateKey(predicate)
			if _, ok := seenPredicates[key][predicateKey]; !ok {
				seenPredicates[key][predicateKey] = struct{}{}
				defaultSet.Predicates = append(defaultSet.Predicates, predicate)
			}
			defaults[key] = defaultSet
		}
	}
	return defaults
}

func parseGarminGearActivityTypeDefault(raw json.RawMessage) ([]string, string) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, ""
	}
	keys := garminGearDefaultKeys(numberAsInt64(obj["gearPk"], obj["gearId"], obj["gearID"]), stringValue(obj["gearUuid"], obj["gearUUID"], obj["uuid"]))
	rawSport := stringValue(obj["activityTypeKey"], obj["typeKey"], obj["sportType"])
	if rawSport == "" {
		rawSport = stringFromNested(obj["activityType"], "typeKey", "activityTypeKey", "key", "sportType")
	}
	if rawSport == "" {
		rawSport = stringFromNested(obj["activityTypeDTO"], "typeKey", "activityTypeKey", "key", "sportType")
	}
	return keys, rawSport
}

func garminGearDefaultKeys(gearPk int64, uuid string) []string {
	var keys []string
	if gearPk != 0 {
		keys = append(keys, fmt.Sprintf("pk:%d", gearPk))
	}
	if strings.TrimSpace(uuid) != "" {
		keys = append(keys, "uuid:"+strings.TrimSpace(uuid))
	}
	return keys
}

func garminGearDefaultPredicate(rawSport string) (gear.GearDefaultPredicate, bool) {
	for _, source := range []sport.Source{sport.SourceGarminCourse, sport.SourceFITFile} {
		result, err := sport.ResolveSport(sport.ResolveInput{Source: source, RawSport: strings.TrimSpace(rawSport)})
		if err == nil && result.Resolved && result.Tuple.SportType != "" {
			if _, err := gear.NormalizeGearDefaultSports([]string{result.Tuple.SportType}); err == nil {
				predicate, ok := gear.GearDefaultPredicateForSource(source, rawSport)
				if ok {
					return predicate, true
				}
			}
		}
	}
	return gear.GearDefaultPredicate{}, false
}

func garminGearDefaultPredicateKey(predicate gear.GearDefaultPredicate) string {
	key := predicate.SportType
	if predicate.SubType != nil {
		key += ":" + *predicate.SubType
	}
	if predicate.Modifiers != nil {
		if predicate.Modifiers.IsElectric != nil {
			key += "|is_electric=" + garminBoolString(*predicate.Modifiers.IsElectric)
		}
		if predicate.Modifiers.IsIndoor != nil {
			key += "|is_indoor=" + garminBoolString(*predicate.Modifiers.IsIndoor)
		}
		if predicate.Modifiers.IsVirtual != nil {
			key += "|is_virtual=" + garminBoolString(*predicate.Modifiers.IsVirtual)
		}
	}
	return key
}

func garminBoolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func numberAsInt64(values ...any) int64 {
	for _, value := range values {
		switch v := value.(type) {
		case float64:
			return int64(v)
		case int64:
			return v
		case json.Number:
			n, _ := v.Int64()
			return n
		}
	}
	return 0
}

func stringValue(values ...any) string {
	for _, value := range values {
		if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func stringFromNested(value any, keys ...string) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		values := make([]any, 0, len(keys))
		for _, key := range keys {
			values = append(values, v[key])
		}
		return stringValue(values...)
	default:
		return ""
	}
}

// mapGearType converts Garmin gear type names to Skadia gear types.
func mapGearType(garminType string) string {
	switch strings.ToUpper(garminType) {
	case "SHOES", "RUNNING_SHOES", "TRAIL_RUNNING_SHOES":
		return "shoes"
	case "BIKE", "ROAD_BIKE", "MOUNTAIN_BIKE", "GRAVEL_BIKE", "E_BIKE":
		return "bike"
	case "WETSUIT":
		return "wetsuit"
	case "SKIS", "SKI":
		return "skis"
	case "SNOWBOARD":
		return "snowboard"
	case "RACKET":
		return "racket"
	default:
		return "other"
	}
}
