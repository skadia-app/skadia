package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// Zone type constants shared with other packages.
const (
	ZoneTypeHR    = "hr"
	ZoneTypePower = "power"
	ZoneTypeSpeed = "speed"
)

// TrainingZonesResponse is the top-level response for GET /api/v1/training-zones.
type TrainingZonesResponse struct {
	Sports []SportZones `json:"sports"`
}

// SportZones groups all zone details for a single sport (keyed by fit_sport_id).
type SportZones struct {
	Sport      int16                  `json:"sport"`
	SportLabel string                 `json:"sport_label"`
	Zones      map[string]*ZoneDetail `json:"zones"` // keys: "hr", "power", "speed"
}

// ZoneDetail holds the configuration for one zone type.
type ZoneDetail struct {
	ZoneNames                []string  `json:"zone_names"`
	ZoneHighs                []float64 `json:"zone_highs"`
	ThresholdHeartRate       *int16    `json:"threshold_heart_rate,omitempty"`
	FunctionalThresholdPower *int32    `json:"functional_threshold_power,omitempty"`
	ThresholdSpeedMps        *float64  `json:"threshold_speed_mps,omitempty"`
	SourceType               string    `json:"source_type"`
}

// sportLabelFromFitID converts a FIT sport integer to a German display label.
func sportLabelFromFitID(id int16) string {
	sportType := fitimport.SportToString(typedef.Sport(id))
	return sport.SportLabel(sportType, "")
}

// modelZoneToDetail converts a model.TrainingZone into the API ZoneDetail DTO.
func modelZoneToDetail(z *model.TrainingZone) *ZoneDetail {
	d := &ZoneDetail{
		ZoneNames:          z.ZoneNames,
		ZoneHighs:          z.ZoneHighs,
		ThresholdHeartRate: z.ThresholdHeartRate,
		SourceType:         z.SourceType,
	}
	if z.FunctionalThresholdPower != nil {
		d.FunctionalThresholdPower = z.FunctionalThresholdPower
	}
	if z.ThresholdSpeedMps != nil {
		v := float64(*z.ThresholdSpeedMps)
		d.ThresholdSpeedMps = &v
	}
	return d
}

// GetTrainingZonesAll returns all training zones for the authenticated user,
// grouped by sport.  The sport list is the union of sports that already have
// stored zones and distinct fit_sport_id values present in the user's activities.
//
// GET /api/v1/training-zones
func (h *Handler) GetTrainingZonesAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	// Single query for all zone rows (avoids N+1).
	allZones, err := h.DB.GetTrainingZones(r.Context(), userID, nil, nil)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", fmt.Sprintf("failed to list zones: %v", err), r.URL.Path)
		return
	}

	// Build a map from sport ID → SportZones, populated with the actual zone data.
	bySport := make(map[int16]*SportZones)
	for i := range allZones {
		z := &allZones[i]
		sz, exists := bySport[z.Sport]
		if !exists {
			sz = &SportZones{
				Sport:      z.Sport,
				SportLabel: sportLabelFromFitID(z.Sport),
				Zones:      make(map[string]*ZoneDetail),
			}
			bySport[z.Sport] = sz
		}
		sz.Zones[z.ZoneType] = modelZoneToDetail(z)
	}

	// Also include sports from activities that have no zones yet (empty Zones map).
	sportIDs, err := h.DB.GetAllTrainingZoneSports(r.Context(), userID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", fmt.Sprintf("failed to list sports: %v", err), r.URL.Path)
		return
	}

	resp := TrainingZonesResponse{
		Sports: make([]SportZones, 0, len(sportIDs)),
	}
	for _, sid := range sportIDs {
		if sz, exists := bySport[sid]; exists {
			resp.Sports = append(resp.Sports, *sz)
		} else {
			resp.Sports = append(resp.Sports, SportZones{
				Sport:      sid,
				SportLabel: sportLabelFromFitID(sid),
				Zones:      make(map[string]*ZoneDetail),
			})
		}
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// updateTrainingZonesRequest is the body accepted by PUT /api/v1/training-zones.
type updateTrainingZonesRequest struct {
	Sport                    int16     `json:"sport"`
	SubSport                 int16     `json:"sub_sport"`
	ZoneType                 string    `json:"zone_type"`
	ZoneNames                []string  `json:"zone_names"`
	ZoneHighs                []float64 `json:"zone_highs"`
	ThresholdHeartRate       *int16    `json:"threshold_heart_rate,omitempty"`
	FunctionalThresholdPower *int32    `json:"functional_threshold_power,omitempty"`
	ThresholdSpeedMps        *float64  `json:"threshold_speed_mps,omitempty"`
	MaxHeartRate             *int16    `json:"max_heart_rate,omitempty"`
	HrCalcType               *int16    `json:"hr_calc_type,omitempty"`
	PwrCalcType              *int16    `json:"pwr_calc_type,omitempty"`
}

// UpdateTrainingZones saves training zone data for a single sport/sub_sport/zone_type
// with source_type = 'manual_override' (highest priority, always wins).
//
// PUT /api/v1/training-zones
func (h *Handler) UpdateTrainingZones(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	req, ok := httputil.DecodeJSON[updateTrainingZonesRequest](w, r)
	if !ok {
		return
	}

	// --- Validation ---

	if len(req.ZoneNames) == 0 || len(req.ZoneHighs) == 0 {
		httputil.WriteError(w, http.StatusUnprocessableEntity, "Validation Error", "zone_names and zone_highs must be non-empty", r.URL.Path)
		return
	}
	if len(req.ZoneNames) != len(req.ZoneHighs) {
		httputil.WriteError(w, http.StatusUnprocessableEntity, "Validation Error", "zone_names and zone_highs must have the same length", r.URL.Path)
		return
	}
	if len(req.ZoneNames) < 2 || len(req.ZoneNames) > 10 {
		httputil.WriteError(w, http.StatusUnprocessableEntity, "Validation Error", "zone count must be between 2 and 10", r.URL.Path)
		return
	}

	// zone_highs must be strictly ascending
	for i := range len(req.ZoneHighs) - 1 {
		if req.ZoneHighs[i] >= req.ZoneHighs[i+1] {
			httputil.WriteError(w, http.StatusUnprocessableEntity, "Validation Error", "zone_highs must be strictly ascending", r.URL.Path)
			return
		}
	}

	switch req.ZoneType {
	case ZoneTypeHR:
		if req.ThresholdHeartRate == nil || *req.ThresholdHeartRate <= 0 {
			httputil.WriteError(w, http.StatusUnprocessableEntity, "Validation Error", "threshold_heart_rate must be > 0 for HR zones", r.URL.Path)
			return
		}
	case ZoneTypePower:
		if req.FunctionalThresholdPower == nil || *req.FunctionalThresholdPower <= 0 {
			httputil.WriteError(w, http.StatusUnprocessableEntity, "Validation Error", "functional_threshold_power must be > 0 for power zones", r.URL.Path)
			return
		}
	case ZoneTypeSpeed:
		// threshold_speed_mps is optional for speed zones; no extra check required
	default:
		httputil.WriteError(w, http.StatusUnprocessableEntity, "Validation Error", fmt.Sprintf("unknown zone_type %q (expected: hr, power, speed)", req.ZoneType), r.URL.Path)
		return
	}

	// --- Build model and upsert ---

	var tsMps *float32
	if req.ThresholdSpeedMps != nil {
		v := float32(*req.ThresholdSpeedMps)
		tsMps = &v
	}

	zone := &model.TrainingZone{
		UserID:                   userID,
		Sport:                    req.Sport,
		SubSport:                 req.SubSport,
		ZoneType:                 req.ZoneType,
		ZoneNames:                req.ZoneNames,
		ZoneHighs:                req.ZoneHighs,
		ThresholdHeartRate:       req.ThresholdHeartRate,
		FunctionalThresholdPower: req.FunctionalThresholdPower,
		ThresholdSpeedMps:        tsMps,
		MaxHeartRate:             req.MaxHeartRate,
		HrCalcType:               req.HrCalcType,
		PwrCalcType:              req.PwrCalcType,
	}

	if err := h.DB.UpsertTrainingZonesManual(r.Context(), zone); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", fmt.Sprintf("failed to save training zones: %v", err), r.URL.Path)
		return
	}

	// Trigger background TSS recompute so historical PMC values reflect the new thresholds.
	if h.PMC != nil {
		entityName := "TSS-Neuberechnung"
		jobID, created, err := h.DB.InsertBackgroundJobGuarded(r.Context(), userID,
			"recompute_all_tss", nil, &entityName, 0)
		if err != nil {
			slog.Error("failed to create TSS recompute job", "err", err, "user_id", userID)
		} else if created {
			pmcSvc := h.PMC
			shutdown.Go(jobID, func(ctx context.Context) {
				err := pmcSvc.RecomputeAllTSS(ctx, userID)
				if err != nil {
					_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, true, err.Error())
				} else {
					_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, false, "")
				}
			})
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
