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
	"github.com/skadia-app/skadia/services/api/internal/fusion"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

// FusionQueueEntry ist ein angereichter FusionCandidate für die API.
// sport_type_a und sport_type_b können unterschiedlich sein — z.B. wenn dieselbe
// Wanderung auf einem Radcomputer als "cycling" und auf einem GPSmap als "hiking" geloggt wurde.
// In diesem Fall wählt der Nutzer im DuplicateReviewPanel welchen Sport-Typ er behalten will.
type FusionQueueEntry struct {
	ActivityIDA         string  `json:"activity_id_a"`
	ActivityIDB         string  `json:"activity_id_b"`
	TitleA              string  `json:"title_a"`
	TitleB              string  `json:"title_b"`
	StartedAtA          string  `json:"started_at_a"`
	StartedAtB          string  `json:"started_at_b"`
	SportTypeA          string  `json:"sport_type_a"`
	SportTypeB          string  `json:"sport_type_b"`
	SubTypeA            string  `json:"sub_type_a"`
	SubTypeB            string  `json:"sub_type_b"`
	IsElectricA         bool    `json:"is_electric_a"`
	IsElectricB         bool    `json:"is_electric_b"`
	DistanceMa          float64 `json:"distance_m_a"`
	DistanceMb          float64 `json:"distance_m_b"`
	OverlapPercent      float64 `json:"overlap_percent"`
	FieldRichnessA      int     `json:"field_richness_a"`
	FieldRichnessB      int     `json:"field_richness_b"`
	TimeDiffSec         float64 `json:"time_diff_sec"`
	SourceDeviceA       string  `json:"source_device_a"`
	SourceA             string  `json:"source_a"`
	OriginalFileFormatA string  `json:"original_file_format_a"`
	SourceDeviceB       string  `json:"source_device_b"`
	SourceB             string  `json:"source_b"`
	OriginalFileFormatB string  `json:"original_file_format_b"`
}

type fusionQueueResponse struct {
	Items     []FusionQueueEntry `json:"items"`
	Computing bool               `json:"computing"`
}

func resolveTitle(title, titleAuto *string) string {
	if title != nil && *title != "" {
		return *title
	}
	if titleAuto != nil {
		return *titleAuto
	}
	return ""
}

func derefFloat(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func fusionRowToEntry(row database.FusionCandidateRow) FusionQueueEntry {
	return FusionQueueEntry{
		ActivityIDA:         row.ActivityIDA,
		ActivityIDB:         row.ActivityIDB,
		TitleA:              resolveTitle(row.TitleA, row.TitleAutoA),
		TitleB:              resolveTitle(row.TitleB, row.TitleAutoB),
		StartedAtA:          row.StartedAtA.Format("2006-01-02T15:04:05Z"),
		StartedAtB:          row.StartedAtB.Format("2006-01-02T15:04:05Z"),
		SportTypeA:          row.SportTypeA,
		SportTypeB:          row.SportTypeB,
		SubTypeA:            derefStr(row.SubTypeA),
		SubTypeB:            derefStr(row.SubTypeB),
		IsElectricA:         row.IsElectricA,
		IsElectricB:         row.IsElectricB,
		DistanceMa:          derefFloat(row.DistanceMA),
		DistanceMb:          derefFloat(row.DistanceMB),
		OverlapPercent:      math.Round(row.OverlapPercent*10) / 10,
		FieldRichnessA:      row.FieldRichnessA,
		FieldRichnessB:      row.FieldRichnessB,
		TimeDiffSec:         row.TimeDiffSec,
		SourceDeviceA:       derefStr(row.SourceDeviceA),
		SourceA:             derefStr(row.SourceA),
		OriginalFileFormatA: derefStr(row.OriginalFileFormatA),
		SourceDeviceB:       derefStr(row.SourceDeviceB),
		SourceB:             derefStr(row.SourceB),
		OriginalFileFormatB: derefStr(row.OriginalFileFormatB),
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

	// Check staleness + maybe trigger background recompute
	stale, _, err := h.DB.IsFusionCacheStale(r.Context(), userID)
	if err != nil {
		slog.Error("fusion queue: cache check failed", "error", err)
	}
	computing := stale
	if err == nil && stale {
		h.MaybeTriggerFusionQueueCompute(r.Context(), userID)
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
// wenn der Cache stale ist und kein Job läuft. Wird nach Imports und vom GET-Endpoint aufgerufen.
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
	shutdown.Go("fusion-queue-compute-"+userID, func(ctx context.Context) {
		h.runFusionQueueCompute(ctx, jobID, userID, invalidationSeq)
	})
}

// runFusionQueueCompute führt die Tier-2-Berechnung im Hintergrund aus und
// speichert die Ergebnisse im Cache (fusion_candidates).
func (h *Handler) runFusionQueueCompute(ctx context.Context, jobID, userID string, startSeq int64) {
	start := time.Now()

	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("fusion queue compute panicked", "panic", rec, "job_id", jobID)
			h.DB.CompleteBackgroundJob(context.Background(), jobID, true, fmt.Sprintf("panic: %v", rec)) //nolint:errcheck
		}
	}()

	slog.Info("fusion queue compute: starting", "job_id", jobID, "user_id", userID)

	settings, _ := h.DB.GetImportSettings(ctx, userID)
	threshold := 75.0
	timeWindowMin := 30
	if settings != nil {
		threshold = float64(settings.GPSDuplicateThreshold)
		if settings.FusionTimeWindowMin > 0 {
			timeWindowMin = settings.FusionTimeWindowMin
		}
	}

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
		if ctx.Err() != nil {
			slog.Info("fusion queue compute: cancelled", "job_id", jobID)
			h.DB.CompleteBackgroundJob(context.Background(), jobID, true, "cancelled") //nolint:errcheck
			return
		}

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

	// Clique-Integration: Gruppen von Duplikaten automatisch als RecordingGroups anlegen.
	if err := h.runCliqueIntegration(ctx, userID, threshold); err != nil {
		// Nicht-fataler Fehler: Clique-Integration schlägt den Job nicht fehl.
		slog.Warn("fusion queue compute: clique integration failed", "error", err)
	}

	slog.Info("fusion queue compute: done", "user_id", userID, "candidates", len(candidates), "duration", time.Since(start))
	h.DB.CompleteBackgroundJob(ctx, jobID, false, "") //nolint:errcheck
}

// runCliqueIntegration lädt alle Fusion-Kandidatenpaare, baut Cliquen und
// legt für jede Clique eine RecordingGroup an (oder erweitert eine bestehende).
func (h *Handler) runCliqueIntegration(ctx context.Context, userID string, threshold float64) error {
	pairs, err := h.DB.GetAllFusionCandidatePairs(ctx, userID)
	if err != nil {
		return fmt.Errorf("load fusion candidate pairs: %w", err)
	}
	if len(pairs) == 0 {
		return nil
	}

	// Alle Aktivitäts-IDs aus den Paaren sammeln.
	idSet := make(map[string]bool, len(pairs)*2)
	for _, p := range pairs {
		idSet[p.ActivityIDA] = true
		idSet[p.ActivityIDB] = true
	}
	allIDs := make([]string, 0, len(idSet))
	for id := range idSet {
		allIDs = append(allIDs, id)
	}

	// Ausgeschlossene Aktivitäten herausfiltern: Multisport-Kinder (parent_id IS NOT NULL)
	// und Fusion-Quellen (is_fusion_source = TRUE, Spec §3.4).
	children, err := h.DB.GetMultisportChildIDs(ctx, allIDs)
	if err != nil {
		return fmt.Errorf("get multisport child ids: %w", err)
	}

	// Paare filtern: beide IDs dürfen weder Multisport-Kinder noch Fusion-Quellen sein.
	filtered := pairs[:0]
	for _, p := range pairs {
		if !children[p.ActivityIDA] && !children[p.ActivityIDB] {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	cliques := fusion.BuildCliques(filtered, threshold)

	for _, clique := range cliques {
		if len(clique) < 2 {
			continue
		}

		// Prüfen ob eines der Clique-Mitglieder bereits in einer Gruppe ist.
		var existingGroup *database.RecordingGroup
		for _, memberID := range clique {
			g, err := h.DB.GetRecordingGroupForActivity(ctx, memberID)
			if err != nil {
				slog.Warn("clique integration: get recording group failed", "activity_id", memberID, "error", err)
				continue
			}
			if g != nil {
				existingGroup = g
				break
			}
		}

		if existingGroup != nil {
			// Gruppe erweitern: neue Mitglieder hinzufügen die noch nicht drin sind.
			for _, memberID := range clique {
				g, err := h.DB.GetRecordingGroupForActivity(ctx, memberID)
				if err != nil {
					slog.Warn("clique integration: membership check failed", "activity_id", memberID, "error", err)
					continue
				}
				if g == nil {
					if err := h.DB.AddRecordingGroupMember(ctx, existingGroup.ID, memberID); err != nil {
						slog.Warn("clique integration: add member failed", "group_id", existingGroup.ID, "activity_id", memberID, "error", err)
					}
				}
			}
		} else {
			// Neue Gruppe erstellen.
			primaryID := fusion.BestPrimary(filtered, clique)
			members := []string(clique)
			if _, err := h.DB.CreateRecordingGroup(ctx, userID, members, primaryID, ""); err != nil {
				slog.Warn("clique integration: create recording group failed", "error", err)
			}
		}
	}
	return nil
}
