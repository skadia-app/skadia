package garmin

import (
	"context"
	"log/slog"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/model"
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
	GearDTOS []garminGearDTO `json:"gearDTOS"`
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

	items := wrapper[0].GearDTOS
	total := len(items)

	for i, dto := range items {
		if params.OnProgress != nil {
			params.OnProgress(i+1, total, "gear: "+dto.CustomMakeModel)
		}

		// Map Garmin gear type to Skadia gear type
		gearType := mapGearType(dto.GearTypeName)
		gearName := dto.CustomMakeModel
		if gearName == "" {
			gearName = dto.GearTypeName
		}

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

		var purchasedAt *string
		if dto.DateBegin != "" {
			purchasedAt = &dto.DateBegin
		}

		create := &model.GearCreate{
			Name:        gearName,
			Type:        gearType,
			PurchasedAt: purchasedAt,
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
