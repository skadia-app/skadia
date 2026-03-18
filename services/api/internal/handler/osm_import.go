package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	osmregions "github.com/skadia-app/skadia/services/api/internal/osm"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
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
	Regions       []string `json:"regions"`
	Categories    []string `json:"categories"`
	BuildBRouter  bool     `json:"build_brouter"`
	RunEnrichment *bool    `json:"run_enrichment"` // nil = true (backwards compatible)
	BuildTiles    bool     `json:"build_tiles"`    // opt-in, default false
}

func (r osmImportRequest) shouldRunEnrichment() bool {
	return r.RunEnrichment == nil || *r.RunEnrichment
}

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
	shutdown.Cancel(jobID)
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

	// Dependency check: tiles require enrichment
	if req.BuildTiles && !req.shouldRunEnrichment() {
		writeError(w, http.StatusBadRequest, "Bad Request",
			"Tiles benötigen Enrichment. build_tiles=true und run_enrichment=false können nicht kombiniert werden.", r.URL.Path)
		return
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

	shutdown.Go(jobID, func(ctx context.Context) {
		h.runOSMImport(ctx, jobID, userID, validRegions, req)
	})

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

func (h *Handler) runOSMImport(ctx context.Context, jobID, userID string, regions []osmregions.Region, req osmImportRequest) {
	pipeline := &osmregions.Pipeline{
		Pool:    h.DB.Pool,
		DataDir: os.Getenv("OSM_DATA_DIR"),
		DBURL:   os.Getenv("DATABASE_URL"),
	}
	if pipeline.DataDir == "" {
		pipeline.DataDir = "/data/osm"
	}
	mainReporter := &jobReporter{db: h.DB, jobID: jobID}

	var succeededRegions []osmregions.Region
	var failedRegionIDs []string

	for i, reg := range regions {
		if ctx.Err() != nil {
			break
		}
		mainReporter.SetPhase(fmt.Sprintf("region_%d_%s", i+1, reg.ID))
		slog.Info("osm import: importing region", "region", reg.ID, "index", i+1, "total", len(regions))

		regReporter := &regionReporter{inner: mainReporter}
		if pipeline.RunPerRegion(ctx, reg, osmregions.PipelineConfig{
			Categories:   req.Categories,
			BuildBRouter: req.BuildBRouter,
		}, regReporter) {
			succeededRegions = append(succeededRegions, reg)
		} else {
			failedRegionIDs = append(failedRegionIDs, reg.ID)
			slog.Error("osm import: region failed, continuing with next",
				"region", reg.ID, "error", regReporter.lastErr)
			ctx5s, cancel5s := context.WithTimeout(context.Background(), 5*time.Second)
			h.DB.Pool.Exec(ctx5s,
				`UPDATE osm_import_regions SET status = 'failed' WHERE region_id = $1`, reg.ID)
			cancel5s()
			if strings.Contains(regReporter.lastErr, "Korruption") || strings.Contains(regReporter.lastErr, "XX0") {
				ctx5s, cancel5s := context.WithTimeout(context.Background(), 5*time.Second)
				h.DB.Pool.Exec(ctx5s,
					`INSERT INTO notifications (user_id, category, title, body)
					 VALUES ($1, 'error', 'Datenbank-Korruption bei OSM Import', $2)`,
					userID, fmt.Sprintf("Region %s: %s", reg.Label, regReporter.lastErr))
				cancel5s()
			}
		}
	}

	if req.BuildBRouter && len(succeededRegions) > 0 {
		segmentsDir := os.Getenv("BROUTER_SEGMENTS_DIR")
		if segmentsDir == "" {
			segmentsDir = "/data/brouter/segments"
		}
		mainReporter.SetPhase("brouter_segments")
		mainReporter.SetProgress(0)
		slog.Info("osm import: building BRouter segments", "dir", segmentsDir)
		if err := pipeline.BuildBRouterSegments(ctx, succeededRegions, pipeline.DataDir, segmentsDir, mainReporter); err != nil {
			slog.Error("osm import: BRouter segment build failed", "error", err)
		}
	}

	if len(succeededRegions) == 0 && len(failedRegionIDs) > 0 {
		mainReporter.Fail("Alle Regionen fehlgeschlagen: " + strings.Join(failedRegionIDs, ", "))
		return
	}
	if len(failedRegionIDs) > 0 {
		failMsg := fmt.Sprintf("%d/%d Regionen importiert. Fehlgeschlagen: %s",
			len(succeededRegions), len(regions), strings.Join(failedRegionIDs, ", "))
		slog.Warn("osm import: partial success",
			"succeeded", len(succeededRegions), "failed", failedRegionIDs)
		ctx5s, cancel5s := context.WithTimeout(context.Background(), 5*time.Second)
		h.DB.Pool.Exec(ctx5s,
			`UPDATE background_jobs SET error = $1 WHERE id = $2`, failMsg, jobID)
		cancel5s()
		ctx5s, cancel5s = context.WithTimeout(context.Background(), 5*time.Second)
		h.DB.Pool.Exec(ctx5s,
			`INSERT INTO notifications (user_id, category, title, body)
			 VALUES ($1, 'warning', 'OSM Import teilweise fehlgeschlagen', $2)`,
			userID, failMsg)
		cancel5s()
	}
	mainReporter.Complete()

	// Pipeline post-processing: Enrichment → Tiles (sequential)
	waysImported := slices.Contains(req.Categories, "ways") && len(succeededRegions) > 0

	var enrichmentFailed bool

	// Enrichment
	if req.shouldRunEnrichment() && waysImported {
		enrichName := "Enrichment + Splitting"
		enrichJobID, err := h.DB.InsertBackgroundJob(context.Background(), userID,
			"osm_enrichment", nil, &enrichName, 100)
		if err != nil {
			slog.Error("osm import: failed to create enrichment job", "error", err)
		} else if req.BuildTiles {
			// Synchronous: enrichment must finish before tiles can be built
			enrichReporter := &jobReporter{db: h.DB, jobID: enrichJobID}
			regionIDs := make([]string, len(succeededRegions))
			for i, r := range succeededRegions {
				regionIDs[i] = r.ID
			}
			pipeline.RunEnrichmentAndSplitting(ctx, strings.Join(regionIDs, ","), enrichReporter)
			enrichmentFailed = enrichReporter.failed
			if enrichmentFailed {
				slog.Error("osm import: enrichment failed, skipping tile build")
			}
		} else {
			// Asynchronous: enrichment as separate background job (existing behavior)
			shutdown.Go(enrichJobID, func(enrichCtx context.Context) {
				enrichReporter := &jobReporter{db: h.DB, jobID: enrichJobID}
				regionIDs := make([]string, len(succeededRegions))
				for i, r := range succeededRegions {
					regionIDs[i] = r.ID
				}
				pipeline.RunEnrichmentAndSplitting(enrichCtx, strings.Join(regionIDs, ","), enrichReporter)
			})
		}
	}

	// Tiles (only if enrichment ran synchronously and succeeded — defense-in-depth guard)
	if req.BuildTiles && req.shouldRunEnrichment() && waysImported && !enrichmentFailed {
		tilesName := "Routing Tiles"
		tilesJobID, err := h.DB.InsertBackgroundJob(context.Background(), userID,
			"tile_build", nil, &tilesName, 0)
		if err != nil {
			slog.Error("osm import: failed to create tile build job", "error", err)
		} else {
			// BBox-Union for region filter
			filterBBox := [4]float64{180, 90, -180, -90}
			for _, reg := range succeededRegions {
				filterBBox[0] = min(filterBBox[0], reg.MinLon)
				filterBBox[1] = min(filterBBox[1], reg.MinLat)
				filterBBox[2] = max(filterBBox[2], reg.MaxLon)
				filterBBox[3] = max(filterBBox[3], reg.MaxLat)
			}

			graphPath := os.Getenv("ROUTING_GRAPH_PATH")
			if graphPath == "" {
				graphPath = "/data/routing/graph.bin"
			}
			tileDir := filepath.Join(filepath.Dir(graphPath), "tiles")
			hgtDirs := []string{"/data/dem/sonny-05", "/data/dem/sonny-1", "/data/dem/glo30"}

			mainReporter.SetPhase("tiles")
			// Implicit force=true: fresh import data → stale tiles must be replaced
			err := h.GraphManager.RebuildTiles(ctx, h.DB.Pool, tileDir, 1_000_000_000,
				hgtDirs, 2, filterBBox, true,
				func(tileID uint32, step string, pct int) {
					if step == "discovering" {
						_ = h.DB.UpdateBackgroundJobTotal(context.Background(), tilesJobID, int64(pct))
					} else if pct == 100 {
						_ = h.DB.UpdateBackgroundJobProgress(context.Background(), tilesJobID, 1)
					}
				})
			if err != nil {
				slog.Error("osm import: tile build failed", "error", err)
				_ = h.DB.CompleteBackgroundJob(context.Background(), tilesJobID, true)
			} else {
				slog.Info("osm import: tile build completed", "job_id", tilesJobID)
				_ = h.DB.CompleteBackgroundJob(context.Background(), tilesJobID, false)
			}
		}
	}
}

// jobReporter implements osmregions.ProgressReporter by delegating to background_jobs.
type jobReporter struct {
	db     *database.DB
	jobID  string
	failed bool
}

func (r *jobReporter) SetPhase(phase string) {
	r.db.UpdateBackgroundJobPhase(context.Background(), r.jobID, phase)
}

func (r *jobReporter) SetTotal(total int64) {
	r.db.UpdateBackgroundJobTotal(context.Background(), r.jobID, total)
}

func (r *jobReporter) SetProgress(current int64) {
	r.db.UpdateBackgroundJobProgress(context.Background(), r.jobID, current)
}

func (r *jobReporter) Fail(errMsg string) {
	r.failed = true
	dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r.db.Pool.Exec(dbCtx, `UPDATE background_jobs SET error = $1 WHERE id = $2`, errMsg, r.jobID)
	r.db.CompleteBackgroundJob(dbCtx, r.jobID, true)
}

func (r *jobReporter) Complete() {
	r.db.CompleteBackgroundJob(context.Background(), r.jobID, false)
}

// regionReporter wraps a ProgressReporter and captures Fail() calls instead
// of propagating them to the main job reporter. This allows individual region
// failures without aborting the entire multi-region import.
type regionReporter struct {
	inner   osmregions.ProgressReporter
	lastErr string
}

func (r *regionReporter) SetPhase(p string)  { r.inner.SetPhase(p) }
func (r *regionReporter) SetTotal(t int64)   { r.inner.SetTotal(t) }
func (r *regionReporter) SetProgress(c int64) { r.inner.SetProgress(c) }
func (r *regionReporter) Fail(msg string)     { r.lastErr = msg }
func (r *regionReporter) Complete()           {}
