package handler

import (
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// FusionQueueEntry ist ein angereichter FusionCandidate für die API.
// sport_type_a und sport_type_b können unterschiedlich sein — z.B. wenn dieselbe
// Wanderung auf einem Radcomputer als "cycling" und auf einem GPSmap als "hiking" geloggt wurde.
// In diesem Fall wählt der Nutzer im DuplicateReviewPanel welchen Sport-Typ er behalten will.
type FusionQueueEntry struct {
	ActivityIDA    string  `json:"activity_id_a"`
	ActivityIDB    string  `json:"activity_id_b"`
	TitleA         string  `json:"title_a"`
	TitleB         string  `json:"title_b"`
	StartedAtA     string  `json:"started_at_a"`
	StartedAtB     string  `json:"started_at_b"`
	SportTypeA     string  `json:"sport_type_a"`
	SportTypeB     string  `json:"sport_type_b"`
	SubTypeA       string  `json:"sub_type_a"`
	SubTypeB       string  `json:"sub_type_b"`
	IsEbikeA       bool    `json:"is_ebike_a"`
	IsEbikeB       bool    `json:"is_ebike_b"`
	DistanceMa     float64 `json:"distance_m_a"`
	DistanceMb     float64 `json:"distance_m_b"`
	OverlapPercent float64 `json:"overlap_percent"`
	FieldRichnessA int     `json:"field_richness_a"`
	FieldRichnessB int     `json:"field_richness_b"`
	TimeDiffSec    float64 `json:"time_diff_sec"`
}

// activityTitle gibt den besten verfügbaren Titel einer Aktivität zurück.
func activityTitle(a *model.Activity) string {
	if a.Title != nil && *a.Title != "" {
		return *a.Title
	}
	if a.TitleAuto != nil {
		return *a.TitleAuto
	}
	return ""
}

// GetFusionQueue gibt Tier-2 Fusionskandidaten zurück.
// GET /api/v1/fusion-queue?limit=20
func (h *Handler) GetFusionQueue(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	limit := 20
	if ls := r.URL.Query().Get("limit"); ls != "" {
		if n, err := strconv.Atoi(ls); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	// Nutzer-Schwellwert und Zeitfenster laden
	settings, _ := h.DB.GetImportSettings(r.Context(), userID)
	threshold := float64(75)
	timeWindowMin := 30
	if settings != nil {
		threshold = float64(settings.GPSDuplicateThreshold)
		if settings.FusionTimeWindowMin > 0 {
			timeWindowMin = settings.FusionTimeWindowMin
		}
	}

	// Aktivitäten laden die für Tier-2 in Frage kommen
	activityIDs, err := h.DB.GetFusionQueueActivities(r.Context(), userID, 50)
	if err != nil {
		slog.Error("fusion queue: load activities failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}

	seen := make(map[string]bool) // verhindert doppelte Paare (A,B) vs (B,A)
	var entries []FusionQueueEntry

	for _, actID := range activityIDs {
		if len(entries) >= limit {
			break
		}
		candidates, err := importer.CheckTier2(r.Context(), h.DB, userID, actID, threshold, timeWindowMin)
		if err != nil {
			slog.Warn("fusion queue: tier-2 check failed", "id", actID, "error", err)
			continue
		}
		for _, c := range candidates {
			key := c.ActivityIDA + ":" + c.ActivityIDB
			if c.ActivityIDA > c.ActivityIDB {
				key = c.ActivityIDB + ":" + c.ActivityIDA
			}
			if seen[key] {
				continue
			}
			seen[key] = true

			actA, _ := h.DB.GetActivity(r.Context(), c.ActivityIDA)
			actB, _ := h.DB.GetActivity(r.Context(), c.ActivityIDB)
			if actA == nil || actB == nil {
				continue
			}

			entry := FusionQueueEntry{
				ActivityIDA:    c.ActivityIDA,
				ActivityIDB:    c.ActivityIDB,
				TitleA:         activityTitle(actA),
				TitleB:         activityTitle(actB),
				StartedAtA:     actA.StartedAt.Format("2006-01-02T15:04:05Z"),
				StartedAtB:     actB.StartedAt.Format("2006-01-02T15:04:05Z"),
				SportTypeA:     actA.SportType,
				SportTypeB:     actB.SportType,
				SubTypeA:       derefStr(actA.SubType),
				SubTypeB:       derefStr(actB.SubType),
				IsEbikeA:       actA.IsEbike,
				IsEbikeB:       actB.IsEbike,
				OverlapPercent: math.Round(c.OverlapPercent*10) / 10,
				FieldRichnessA: c.FieldRichnessA,
				FieldRichnessB: c.FieldRichnessB,
				TimeDiffSec:    c.TimeDiffSec,
			}
			if actA.DistanceM != nil {
				entry.DistanceMa = *actA.DistanceM
			}
			if actB.DistanceM != nil {
				entry.DistanceMb = *actB.DistanceM
			}
			entries = append(entries, entry)
		}
	}

	if entries == nil {
		entries = []FusionQueueEntry{}
	}
	httputil.WriteJSON(w, http.StatusOK, entries)
}
