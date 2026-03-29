package handler

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/importer"
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

type fusionQueueResponse struct {
	Items     []FusionQueueEntry `json:"items"`
	Computing bool               `json:"computing"`
}

// fusionRowToEntry konvertiert eine FusionCandidateRow in einen FusionQueueEntry.
func fusionRowToEntry(row database.FusionCandidateRow) FusionQueueEntry {
	titleA := ""
	if row.TitleA != nil && *row.TitleA != "" {
		titleA = *row.TitleA
	} else if row.TitleAutoA != nil {
		titleA = *row.TitleAutoA
	}

	titleB := ""
	if row.TitleB != nil && *row.TitleB != "" {
		titleB = *row.TitleB
	} else if row.TitleAutoB != nil {
		titleB = *row.TitleAutoB
	}

	distA := float64(0)
	if row.DistanceMA != nil {
		distA = *row.DistanceMA
	}

	distB := float64(0)
	if row.DistanceMB != nil {
		distB = *row.DistanceMB
	}

	return FusionQueueEntry{
		ActivityIDA:    row.ActivityIDA,
		ActivityIDB:    row.ActivityIDB,
		TitleA:         titleA,
		TitleB:         titleB,
		StartedAtA:     row.StartedAtA.Format("2006-01-02T15:04:05Z"),
		StartedAtB:     row.StartedAtB.Format("2006-01-02T15:04:05Z"),
		SportTypeA:     row.SportTypeA,
		SportTypeB:     row.SportTypeB,
		SubTypeA:       derefStr(row.SubTypeA),
		SubTypeB:       derefStr(row.SubTypeB),
		IsEbikeA:       row.IsEbikeA,
		IsEbikeB:       row.IsEbikeB,
		DistanceMa:     distA,
		DistanceMb:     distB,
		OverlapPercent: math.Round(row.OverlapPercent*10) / 10,
		FieldRichnessA: row.FieldRichnessA,
		FieldRichnessB: row.FieldRichnessB,
		TimeDiffSec:    row.TimeDiffSec,
	}
}

// GetFusionQueue gibt Tier-2 Fusionskandidaten zurück.
// GET /api/v1/fusion-queue?limit=20
func (h *Handler) GetFusionQueue(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	limit := 20
	if ls := r.URL.Query().Get("limit"); ls != "" {
		if n, err := strconv.Atoi(ls); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	// Check cache staleness
	stale, invalidationSeq, err := h.DB.IsFusionCacheStale(r.Context(), userID)
	if err != nil {
		slog.Error("fusion queue: cache check failed", "error", err)
		// DB error → return cached data without starting a job
		// (avoids K2: invalidationSeq=0 would cause permanent stale loop)
	}

	computing := stale

	// Start background job if stale and no job already running.
	// Only attempt if IsFusionCacheStale succeeded (err == nil) to avoid
	// storing invalidationSeq=0 which would cause an endless recompute loop.
	if err == nil && stale {
		running, runErr := h.DB.HasRunningJob(r.Context(), userID, "fusion_queue_compute")
		if runErr != nil {
			slog.Warn("fusion queue: check running job failed", "error", runErr)
		}
		if !running {
			jobID, jobErr := h.DB.InsertBackgroundJob(r.Context(), userID, "fusion_queue_compute", nil, nil, 0)
			if jobErr != nil {
				// Unique violation (23505) means another user's job is running → not an error
				slog.Info("fusion queue: job not created (likely already running)", "error", jobErr)
			} else {
				go h.runFusionQueueCompute(jobID, userID, invalidationSeq)
			}
		}
	}

	// Return cached results (may be empty on first load)
	rows, err := h.DB.GetCachedFusionCandidates(r.Context(), userID, limit)
	if err != nil {
		slog.Error("fusion queue: cache read failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}

	entries := make([]FusionQueueEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, fusionRowToEntry(row))
	}

	httputil.WriteJSON(w, http.StatusOK, fusionQueueResponse{
		Items:     entries,
		Computing: computing,
	})
}

// MaybeTriggerFusionQueueCompute startet einen Fusion-Queue-Recompute im Hintergrund,
// wenn der Cache stale ist und kein Job läuft. Wird nach Imports aufgerufen.
func (h *Handler) MaybeTriggerFusionQueueCompute(ctx context.Context, userID string) {
	stale, invalidationSeq, err := h.DB.IsFusionCacheStale(ctx, userID)
	if err != nil || !stale {
		return
	}
	running, _ := h.DB.HasRunningJob(ctx, userID, "fusion_queue_compute")
	if running {
		return
	}
	jobID, err := h.DB.InsertBackgroundJob(ctx, userID, "fusion_queue_compute", nil, nil, 0)
	if err != nil {
		return
	}
	go h.runFusionQueueCompute(jobID, userID, invalidationSeq)
}

// runFusionQueueCompute führt die Tier-2-Berechnung im Hintergrund aus und
// speichert die Ergebnisse im Cache (fusion_candidates).
func (h *Handler) runFusionQueueCompute(jobID, userID string, startSeq int64) {
	ctx := context.Background()
	start := time.Now()

	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("fusion queue compute panicked", "panic", rec, "job_id", jobID)
			h.DB.CompleteBackgroundJob(ctx, jobID, true, fmt.Sprintf("panic: %v", rec)) //nolint:errcheck
		}
	}()

	slog.Info("fusion queue compute: starting", "job_id", jobID, "user_id", userID)

	settings, _ := h.DB.GetImportSettings(ctx, userID)
	threshold := float64(75)
	timeWindowMin := 30
	if settings != nil {
		threshold = float64(settings.GPSDuplicateThreshold)
		if settings.FusionTimeWindowMin > 0 {
			timeWindowMin = settings.FusionTimeWindowMin
		}
	}

	// No limit in background job — check ALL eligible activities.
	// FindFusionCandidates returns fast for activities without temporal neighbors.
	activityIDs, err := h.DB.GetFusionQueueActivities(ctx, userID, 0)
	if err != nil {
		slog.Error("fusion queue compute: load activities failed", "error", err)
		h.DB.CompleteBackgroundJob(ctx, jobID, true, err.Error()) //nolint:errcheck
		return
	}

	h.DB.UpdateBackgroundJobTotal(ctx, jobID, int64(len(activityIDs))) //nolint:errcheck

	seen := make(map[string]bool)
	var candidates []database.FusionCandidateInsert

	for i, actID := range activityIDs {
		matches, err := importer.CheckTier2(ctx, h.DB, userID, actID, threshold, timeWindowMin)
		if err != nil {
			slog.Warn("fusion queue compute: tier-2 check failed", "id", actID, "error", err)
			continue
		}
		for _, c := range matches {
			key := c.ActivityIDA + ":" + c.ActivityIDB
			if c.ActivityIDA > c.ActivityIDB {
				key = c.ActivityIDB + ":" + c.ActivityIDA
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			candidates = append(candidates, database.FusionCandidateInsert{
				ActivityIDA:    c.ActivityIDA,
				ActivityIDB:    c.ActivityIDB,
				OverlapPercent: c.OverlapPercent,
				FieldRichnessA: c.FieldRichnessA,
				FieldRichnessB: c.FieldRichnessB,
				TimeDiffSec:    c.TimeDiffSec,
			})
		}
		h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(i+1)) //nolint:errcheck
	}

	if err := h.DB.ReplaceFusionCandidates(ctx, userID, candidates, startSeq); err != nil {
		slog.Error("fusion queue compute: replace candidates failed", "error", err)
		h.DB.CompleteBackgroundJob(ctx, jobID, true, err.Error()) //nolint:errcheck
		return
	}

	slog.Info("fusion queue compute: done", "user_id", userID, "candidates", len(candidates), "duration", time.Since(start))
	h.DB.CompleteBackgroundJob(ctx, jobID, false, "") //nolint:errcheck
}
