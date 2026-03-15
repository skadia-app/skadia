package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	osmregions "github.com/skadia-app/skadia/services/api/internal/osm"
)

// Kategorie-Definitionen: ID → betroffene Tabellen.
// Kategorien die in osm_pois landen, werden mit einem DELETE-Filter gelöscht (kein TRUNCATE).
var osmCategories = map[string][]string{
	"peaks":          {"osm_pois"},
	"passes":         {"osm_pois"},
	"settlements":    {"osm_pois"},
	"gastronomy":     {"osm_pois"},
	"accommodation":  {"osm_pois"},
	"infrastructure": {"osm_pois"},
	"transport":      {"osm_pois"},
	"viewpoints":     {"osm_pois"},
	"shops":          {"osm_pois"},
	"sport":          {"osm_pois"},
	"camping":        {"osm_pois"},
	"information":    {"osm_pois"},
	"landmarks":      {"osm_pois"},
	"religion":       {"osm_pois"},
	"ways":           {"osm_ways"},
	"waterways":      {"osm_waterways", "osm_waters"},
	"forests":        {"osm_forests"},
	"glaciers":       {"osm_glaciers"},
	"wetlands":       {"osm_wetlands"},
	"boundaries":     {"osm_boundaries"},
	"pistes":         {"osm_pistes", "osm_piste_areas"},
	"lifts":          {"osm_lifts"},
	"ferries":        {}, // shares osm_ways — cleared via DELETE WHERE highway='ferry'
	"climbing":       {}, // shares osm_ways — cleared via DELETE WHERE highway='via_ferrata'
	"routes":         {"osm_routes"},
	"landuse":        {"osm_landuse"},
	"railways":       {"osm_railways"},
	"wind_turbines":  {"osm_pois"},
}

type osmImportRequest struct {
	Regions      []string `json:"regions"`
	Categories   []string `json:"categories"`
	BuildBRouter bool     `json:"build_brouter"`
}

// osmJobCancels speichert CancelFuncs für laufende OSM-Import-Jobs.
var osmJobCancels sync.Map

// CancelOSMImport bricht einen laufenden OSM-Import ab.
// POST /api/v1/admin/osm-import/cancel/{jobID}
func (h *Handler) CancelOSMImport(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "missing job ID", r.URL.Path)
		return
	}
	_, err := h.DB.Pool.Exec(r.Context(),
		`UPDATE background_jobs SET status = 'failed',
		 finished_at = now() WHERE id = $1 AND status IN ('running', 'pending')`, jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}
	if fn, ok := osmJobCancels.LoadAndDelete(jobID); ok {
		fn.(context.CancelFunc)()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// ListOSMRegions gibt den Regions-Katalog zurück.
// GET /api/v1/admin/osm-regions
func (h *Handler) ListOSMRegions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"regions": osmregions.Regions})
}

// StartOSMImport startet einen asynchronen OSM-Import-Job.
// POST /api/v1/admin/osm-import/start
func (h *Handler) StartOSMImport(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req osmImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Regions) == 0 || len(req.Categories) == 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "regions and categories required", r.URL.Path)
		return
	}

	// Regions validieren
	var validRegions []osmregions.Region
	for _, id := range req.Regions {
		reg, ok := osmregions.RegionByID(id)
		if !ok {
			writeError(w, http.StatusBadRequest, "Bad Request", "unknown region: "+id, r.URL.Path)
			return
		}
		validRegions = append(validRegions, reg)
	}

	// Kategorien validieren
	for _, cat := range req.Categories {
		if _, ok := osmCategories[cat]; !ok {
			writeError(w, http.StatusBadRequest, "Bad Request", "unknown category: "+cat, r.URL.Path)
			return
		}
	}

	// Region-Namen für entity_name zusammenstellen
	var labels []string
	var totalSizeMB int
	for _, reg := range validRegions {
		labels = append(labels, reg.Label)
		totalSizeMB += reg.SizeMB
	}
	entityName := strings.Join(labels, ", ")

	// Initialer progress_total = geschätzte Gesamtgröße in Bytes (int64, kann >2 GB sein)
	progressTotal := int64(totalSizeMB) * 1024 * 1024

	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "osm_import", nil, &entityName, progressTotal)
	if err != nil {
		slog.Error("failed to insert osm import job", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}

	go h.runOSMImport(jobID, userID, validRegions, req)

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

func (h *Handler) runOSMImport(jobID, userID string, regions []osmregions.Region, req osmImportRequest) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	osmJobCancels.Store(jobID, cancel)
	defer osmJobCancels.Delete(jobID)

	pipeline := &osmregions.Pipeline{
		Pool:    h.DB.Pool,
		DataDir: os.Getenv("OSM_DATA_DIR"),
		DBURL:   os.Getenv("DATABASE_URL"),
	}
	if pipeline.DataDir == "" {
		pipeline.DataDir = "/data/osm"
	}
	reporter := &jobReporter{db: h.DB, jobID: jobID}

	// Sequentiell: jede Region einzeln importieren
	for i, reg := range regions {
		if ctx.Err() != nil {
			break
		}
		reporter.SetPhase(fmt.Sprintf("region_%d_%s", i+1, reg.ID))
		slog.Info("osm import: importing region", "region", reg.ID, "index", i+1, "total", len(regions))

		if !pipeline.RunPerRegion(ctx, reg, osmregions.PipelineConfig{
			Categories:   req.Categories,
			BuildBRouter: req.BuildBRouter,
		}, reporter) {
			return // RunPerRegion hat reporter.Fail() aufgerufen
		}
	}

	reporter.Complete()

	// Enrichment NUR starten wenn ways importiert wurden
	if !slices.Contains(req.Categories, "ways") {
		return
	}

	enrichName := "Enrichment + Splitting"
	enrichJobID, err := h.DB.InsertBackgroundJob(context.Background(), userID,
		"osm_enrichment", nil, &enrichName, 100)
	if err != nil {
		slog.Error("osm import: failed to create enrichment job", "error", err)
		return
	}

	enrichCtx, enrichCancel := context.WithCancel(context.Background())
	osmJobCancels.Store(enrichJobID, enrichCancel)

	go func() {
		defer osmJobCancels.Delete(enrichJobID)
		defer enrichCancel()
		enrichReporter := &jobReporter{db: h.DB, jobID: enrichJobID}
		regionIDs := make([]string, len(regions))
		for i, r := range regions {
			regionIDs[i] = r.ID
		}
		regionFilter := strings.Join(regionIDs, ",")
		pipeline.RunEnrichmentAndSplitting(enrichCtx, regionFilter, enrichReporter)
	}()
}

// jobReporter implements osmregions.ProgressReporter by delegating to background_jobs.
type jobReporter struct {
	db    *database.DB
	jobID string
}

func (r *jobReporter) SetPhase(phase string) {
	r.db.UpdateBackgroundJobPhase(context.Background(), r.jobID, phase)
}

func (r *jobReporter) SetProgress(current int64) {
	r.db.UpdateBackgroundJobProgress(context.Background(), r.jobID, current)
}

func (r *jobReporter) Fail(errMsg string) {
	dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r.db.Pool.Exec(dbCtx, `UPDATE background_jobs SET error = $1 WHERE id = $2`, errMsg, r.jobID)
	r.db.CompleteBackgroundJob(dbCtx, r.jobID, true)
}

func (r *jobReporter) Complete() {
	r.db.CompleteBackgroundJob(context.Background(), r.jobID, false)
}
