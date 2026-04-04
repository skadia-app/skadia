package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	osmregions "github.com/skadia-app/skadia/services/api/internal/osm"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
	"github.com/go-chi/chi/v5"
)

var validRegionID = regexp.MustCompile(`^[a-z0-9_-]+$`)

// GetOSMStatus returns counts from OSM tables.
// GET /api/v1/admin/osm-status
func (h *Handler) GetOSMStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// POI categories
	categories := make(map[string]int64)
	rows, err := h.DB.Pool.Query(ctx, "SELECT category, COUNT(*) FROM osm_pois GROUP BY category")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var cat string
			var cnt int64
			if rows.Scan(&cat, &cnt) == nil {
				categories[cat] = cnt
			}
		}
	}

	// Non-POI table counts (approximate via pg_class.reltuples — instant, no sequential scan)
	var ways, lifts, pistes, pisteAreas, waterways, forests, glaciers, wetlands, routes, boundaries, landuse, railways int64
	type tableCount struct {
		table string
		ptr   *int64
	}
	approxCounts := []tableCount{
		{"osm_ways", &ways}, {"osm_lifts", &lifts}, {"osm_pistes", &pistes},
		{"osm_piste_areas", &pisteAreas},
		{"osm_waterways", &waterways}, {"osm_forests", &forests},
		{"osm_glaciers", &glaciers}, {"osm_wetlands", &wetlands},
		{"osm_routes", &routes}, {"osm_boundaries", &boundaries},
		{"osm_landuse", &landuse}, {"osm_railways", &railways},
	}
	for _, tc := range approxCounts {
		h.DB.Pool.QueryRow(ctx,
			`SELECT COALESCE(reltuples, 0)::bigint FROM pg_class WHERE relname = $1`,
			tc.table).Scan(tc.ptr)
	}

	hasData := len(categories) > 0 || ways > 0 || lifts > 0 || routes > 0

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"categories": categories,
		"ways":       ways,
		"lifts":      lifts,
		"pistes":     pistes + pisteAreas,
		"waterways":  waterways,
		"forests":    forests,
		"glaciers":   glaciers,
		"wetlands":   wetlands,
		"routes":     routes,
		"boundaries": boundaries,
		"landuse":    landuse,
		"railways":   railways,
		"has_data":   hasData,
		// Legacy fields
		"summits":     categories["summit"],
		"passes":      categories["pass"],
		"settlements": categories["settlement"],
		"amenities":   categories["gastronomy"] + categories["accommodation"] + categories["shelter"] + categories["transport"],
	})
}

// GetOSMRegionStatus returns all tracked import regions with metadata.
// GET /api/v1/admin/osm-regions/status
func (h *Handler) GetOSMRegionStatus(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Pool.Query(r.Context(), `
		SELECT region_id, label, continent, imported_at, pbf_date,
		       pbf_size_bytes, import_duration_s, categories, status,
		       data_date, update_interval, replication_url, last_updated_at
		FROM osm_import_regions ORDER BY continent, label`)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}
	defer rows.Close()

	type region struct {
		RegionID        string   `json:"region_id"`
		Label           string   `json:"label"`
		Continent       string   `json:"continent"`
		ImportedAt      string   `json:"imported_at"`
		PBFDate         *string  `json:"pbf_date"`
		PBFSizeBytes    *int64   `json:"pbf_size_bytes"`
		ImportDurationS *int     `json:"import_duration_s"`
		Categories      []string `json:"categories"`
		Status          string   `json:"status"`
		DataDate        *string  `json:"data_date"`
		UpdateInterval  string   `json:"update_interval"`
		ReplicationURL  *string  `json:"replication_url"`
		LastUpdatedAt   *string  `json:"last_updated_at"`
	}
	var regions []region
	for rows.Next() {
		var reg region
		var importedAt time.Time
		var dataDate *time.Time
		var lastUpdatedAt *time.Time
		if err := rows.Scan(&reg.RegionID, &reg.Label, &reg.Continent, &importedAt,
			&reg.PBFDate, &reg.PBFSizeBytes, &reg.ImportDurationS, &reg.Categories, &reg.Status,
			&dataDate, &reg.UpdateInterval, &reg.ReplicationURL, &lastUpdatedAt); err != nil {
			slog.Warn("osm regions: scan failed", "error", err)
			continue
		}
		reg.ImportedAt = importedAt.Format(time.RFC3339)
		if dataDate != nil {
			s := dataDate.Format(time.RFC3339)
			reg.DataDate = &s
		}
		if lastUpdatedAt != nil {
			s := lastUpdatedAt.Format(time.RFC3339)
			reg.LastUpdatedAt = &s
		}
		regions = append(regions, reg)
	}
	if regions == nil {
		regions = []region{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"regions": regions})
}

// GetOSMDiskUsage returns estimated disk usage of OSM tables.
// Uses direct query instead of view for robustness (view may not exist or have permission issues).
// GET /api/v1/admin/osm-disk-usage
func (h *Handler) GetOSMDiskUsage(w http.ResponseWriter, r *http.Request) {
	// Bekannte OSM-Tabellen direkt abfragen — vermeidet Abhängigkeit auf osm_disk_usage View
	osmTables := []string{
		"osm_pois", "osm_ways", "osm_lifts", "osm_waterways", "osm_waters",
		"osm_forests", "osm_glaciers", "osm_wetlands", "osm_boundaries",
		"osm_pistes", "osm_piste_areas", "osm_routes", "osm_import_regions",
		"osm_landuse", "osm_railways",
		"enrichment.raster_tiles",
		// osm2pgsql-interne Staging-Tabellen (während und nach Import vorhanden)
		"planet_osm_ways", "planet_osm_nodes", "planet_osm_rels",
	}

	type tableSize struct {
		Tablename  string `json:"tablename"`
		TotalBytes int64  `json:"total_bytes"`
	}
	var tables []tableSize
	var totalBytes int64

	for _, tbl := range osmTables {
		var size int64
		err := h.DB.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(pg_total_relation_size($1::regclass), 0)`, tbl).Scan(&size)
		if err != nil {
			// Tabelle existiert nicht (noch kein osm2pgsql-Lauf) — überspringen
			continue
		}
		tables = append(tables, tableSize{Tablename: tbl, TotalBytes: size})
		totalBytes += size
	}

	if tables == nil {
		tables = []tableSize{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"tables":      tables,
		"total_bytes": totalBytes,
	})
}

// DeleteOSMRegion removes a region: deletes OSM data, tracking entry, and PBF files.
// DELETE /api/v1/admin/osm-regions/{regionID}
func (h *Handler) DeleteOSMRegion(w http.ResponseWriter, r *http.Request) {
	regionID := chi.URLParam(r, "regionID")
	if regionID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing region ID", r.URL.Path)
		return
	}
	if !validRegionID.MatchString(regionID) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid region ID", r.URL.Path)
		return
	}

	// Fetch status (also verifies the region exists)
	var status string
	err := h.DB.Pool.QueryRow(r.Context(),
		"SELECT status FROM osm_import_regions WHERE region_id = $1", regionID).Scan(&status)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "Region nicht gefunden", r.URL.Path)
		return
	}
	if status == "importing" {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{
			"status":  "importing",
			"message": "Region wird gerade importiert — Löschen nicht möglich",
		})
		return
	}

	// Delete OSM data associated with this region
	pipeline := &osmregions.Pipeline{Pool: h.DB.Pool}
	if err := pipeline.ClearRegionData(r.Context(), regionID); err != nil {
		slog.Error("delete region: clear data failed", "region", regionID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}

	// Delete tracking entry
	h.DB.Pool.Exec(r.Context(),
		`DELETE FROM osm_import_regions WHERE region_id = $1`, regionID)

	// Delete PBF files to reclaim disk space
	dataDir := os.Getenv("OSM_DATA_DIR")
	if dataDir == "" {
		dataDir = "/data/osm"
	}
	pbfPath := filepath.Join(dataDir, regionID+"-latest.osm.pbf")
	os.Remove(pbfPath)
	os.Remove(strings.TrimSuffix(pbfPath, ".osm.pbf") + "-filtered.osm.pbf")

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// SearchAddress performs trigram-based search across streets and places.
// GET /api/v1/geo/search?q=Bergstraße&type=street|place|all&limit=20&lat=47.1&lng=11.4
func (h *Handler) SearchAddress(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "query must be at least 2 characters", r.URL.Path)
		return
	}
	searchType := r.URL.Query().Get("type")
	if searchType == "" {
		searchType = "all"
	}
	limit := 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}

	// Proximity-Sortierung wenn lat/lng gegeben
	hasLocation := r.URL.Query().Has("lat") && r.URL.Query().Has("lng")
	lat, _ := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, _ := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)

	type searchResult struct {
		Name       string   `json:"name"`
		Type       string   `json:"type"`
		Subtype    string   `json:"subtype,omitempty"`
		Lat        float64  `json:"lat"`
		Lng        float64  `json:"lng"`
		Similarity float64  `json:"similarity"`
		DistanceM  *float64 `json:"distance_m,omitempty"`
	}

	ctx := r.Context()
	var results []searchResult

	// Straßensuche
	if searchType == "all" || searchType == "street" {
		var streetQuery string
		var streetParams []any
		if hasLocation {
			streetQuery = `
				SELECT name, highway,
				       ST_Y(ST_Centroid(geom)) AS lat, ST_X(ST_Centroid(geom)) AS lng,
				       similarity(name, $1) AS sim,
				       ST_Distance(geom::geography, ST_Point($2,$3)::geography) AS dist_m
				FROM osm_ways
				WHERE name % $1 AND name IS NOT NULL
				ORDER BY geom::geography <-> ST_Point($2,$3)::geography
				LIMIT $4`
			streetParams = []any{q, lng, lat, limit}
		} else {
			streetQuery = `
				SELECT name, highway,
				       ST_Y(ST_Centroid(geom)) AS lat, ST_X(ST_Centroid(geom)) AS lng,
				       similarity(name, $1) AS sim,
				       NULL::float8 AS dist_m
				FROM osm_ways
				WHERE name % $1 AND name IS NOT NULL
				ORDER BY similarity(name, $1) DESC
				LIMIT $2`
			streetParams = []any{q, limit}
		}

		streetRows, err := h.DB.Pool.Query(ctx, streetQuery, streetParams...)
		if err == nil {
			for streetRows.Next() {
				var sr searchResult
				sr.Type = "street"
				if err := streetRows.Scan(&sr.Name, &sr.Subtype, &sr.Lat, &sr.Lng, &sr.Similarity, &sr.DistanceM); err == nil {
					results = append(results, sr)
				}
			}
			streetRows.Close()
		}
	}

	// Ortssuche
	if searchType == "all" || searchType == "place" {
		placeQuery := `
			SELECT name, category, subtype,
			       ST_Y(ST_Centroid(geom)) AS lat, ST_X(ST_Centroid(geom)) AS lng,
			       similarity(name, $1) AS sim
			FROM osm_pois
			WHERE name % $1 AND name IS NOT NULL
			  AND category IN ('settlement', 'summit', 'pass')
			ORDER BY similarity(name, $1) DESC
			LIMIT $2`
		placeRows, err := h.DB.Pool.Query(ctx, placeQuery, q, limit)
		if err == nil {
			for placeRows.Next() {
				var sr searchResult
				sr.Type = "place"
				var category string
				if err := placeRows.Scan(&sr.Name, &category, &sr.Subtype, &sr.Lat, &sr.Lng, &sr.Similarity); err == nil {
					sr.Type = category // summit, pass, settlement
					results = append(results, sr)
				}
			}
			placeRows.Close()
		}
	}

	if results == nil {
		results = []searchResult{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"results": results, "count": len(results)})
}

// GetDEMStatus returns available DEM data sources and tile counts.
// GET /api/v1/admin/dem-status
func (h *Handler) GetDEMStatus(w http.ResponseWriter, r *http.Request) {
	type demSource struct {
		Source      string  `json:"source"`
		ResolutionM float64 `json:"resolution_m"`
		TileCount   int     `json:"tile_count"`
	}
	rows, err := h.DB.Pool.Query(r.Context(), `
		SELECT COALESCE(source, 'unknown') AS source,
		       COALESCE(resolution_m, 0) AS resolution_m,
		       COUNT(*) AS tile_count
		FROM dem.raster_tiles
		GROUP BY source, resolution_m
		ORDER BY resolution_m ASC`)
	if err != nil {
		// dem schema might not exist — return empty
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"sources": []demSource{}, "total_tiles": 0})
		return
	}
	defer rows.Close()

	var sources []demSource
	var totalTiles int
	for rows.Next() {
		var s demSource
		if err := rows.Scan(&s.Source, &s.ResolutionM, &s.TileCount); err == nil {
			sources = append(sources, s)
			totalTiles += s.TileCount
		}
	}
	if sources == nil {
		sources = []demSource{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"sources": sources, "total_tiles": totalTiles})
}

// RetryOSMEnrichment re-runs only the enrichment + splitting phases (6+7) on existing osm_ways.
// POST /api/v1/admin/osm-import/retry-enrichment
func (h *Handler) RetryOSMEnrichment(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		Regions []string `json:"regions"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Concurrent-Guard: nur ein Enrichment gleichzeitig
	if running, _ := h.DB.HasRunningJobGlobal(r.Context(), "osm_enrichment"); running {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	// Ways-Vorprüfung wenn Regionen angegeben
	if len(req.Regions) > 0 {
		var wayCount int
		h.DB.Pool.QueryRow(r.Context(),
			`SELECT count(*) FROM osm_way_regions WHERE region_id = ANY($1)`, req.Regions).Scan(&wayCount)
		if wayCount == 0 {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
				"Keine Ways für die gewählten Regionen vorhanden. Enrichment benötigt importierte Ways.", r.URL.Path)
			return
		}
	}

	entityName := "Enrichment"
	if len(req.Regions) > 0 {
		entityName = "Enrichment: " + strings.Join(req.Regions, ", ")
	}
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "osm_enrichment", nil, &entityName, 100)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}

	regionFilter := strings.Join(req.Regions, ",")
	shutdown.Go(jobID, func(ctx context.Context) {
		reporter := &jobReporter{db: h.DB, jobID: jobID}
		dataDir := os.Getenv("OSM_DATA_DIR")
		if dataDir == "" {
			dataDir = "/data/osm"
		}
		pipeline := &osmregions.Pipeline{Pool: h.DB.Pool, DataDir: dataDir, DBURL: os.Getenv("DATABASE_URL")}
		pipeline.RunEnrichmentAndSplitting(ctx, regionFilter, reporter)
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// RetryOSMTopology was removed: BadRouter's CSR graph builder computes topology
// from osm_ways directly (routing/graph/topology.go).

// GetOSMSchedule returns current scheduler settings shared across all imported regions.
// GET /api/v1/admin/osm-schedule
func (h *Handler) GetOSMSchedule(w http.ResponseWriter, r *http.Request) {
	var schedule struct {
		Interval string `json:"interval"`
		Weekday  int    `json:"weekday"`
		Day      int    `json:"day"`
		Hour     int    `json:"hour"`
	}
	err := h.DB.Pool.QueryRow(r.Context(), `
		SELECT update_interval, update_weekday, update_day, update_hour
		FROM osm_import_regions WHERE status IN ('imported', 'needs_redownload') LIMIT 1`).
		Scan(&schedule.Interval, &schedule.Weekday, &schedule.Day, &schedule.Hour)
	if err != nil {
		// No imported regions yet — return defaults
		schedule.Interval = "manual"
		schedule.Weekday = 1
		schedule.Day = 1
		schedule.Hour = 3
	}
	httputil.WriteJSON(w, http.StatusOK, schedule)
}

// SetOSMSchedule configures the automatic update schedule for all imported regions.
// POST /api/v1/admin/osm-schedule
// Body: { "interval": "weekly"|"monthly"|"manual", "weekday": 1, "day": 1, "hour": 3 }
func (h *Handler) SetOSMSchedule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Interval string `json:"interval"`
		Weekday  int    `json:"weekday"`
		Day      int    `json:"day"`
		Hour     int    `json:"hour"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}
	switch req.Interval {
	case "manual", "weekly", "monthly":
	default:
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "interval must be manual|weekly|monthly", r.URL.Path)
		return
	}

	// Include needs_redownload: schedule is preserved so it automatically resumes
	// after the user re-imports the region (status goes back to 'imported').
	h.DB.Pool.Exec(r.Context(), `
		UPDATE osm_import_regions SET
			update_interval = $1, update_weekday = $2, update_day = $3, update_hour = $4
		WHERE status IN ('imported', 'needs_redownload')`,
		req.Interval, req.Weekday, req.Day, req.Hour)

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// StartOSMUpdateScheduler launches a background goroutine that checks every hour
// whether any imported region has an automatic update due and triggers it.
// Call once from main after the handler is initialised.
func (h *Handler) StartOSMUpdateScheduler(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.checkAndRunScheduledUpdates(ctx)
			}
		}
	}()
}

func (h *Handler) checkAndRunScheduledUpdates(ctx context.Context) {
	rows, err := h.DB.Pool.Query(ctx, `
		SELECT region_id, label, update_interval, update_weekday, update_day,
		       update_hour, COALESCE(last_updated_at, imported_at), replication_url, last_sequence_number, pbf_path
		FROM osm_import_regions
		WHERE status = 'imported' AND update_interval != 'manual'
		  AND replication_url IS NOT NULL AND pbf_path IS NOT NULL`)
	if err != nil {
		slog.Warn("osm scheduler: query failed", "error", err)
		return
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var (
			regionID, label, interval string
			weekday, day, hour        int
			lastSeq                   int
			lastUpdatedAt             time.Time
			replURL                   string  // safe: WHERE replication_url IS NOT NULL
			pbfPath                   string  // safe: WHERE pbf_path IS NOT NULL
		)
		if err := rows.Scan(&regionID, &label, &interval, &weekday, &day,
			&hour, &lastUpdatedAt, &replURL, &lastSeq, &pbfPath); err != nil {
			slog.Warn("osm scheduler: scan failed", "error", err)
			continue
		}

		if !isUpdateDue(interval, weekday, day, hour, lastUpdatedAt, now) {
			continue
		}

		slog.Info("osm scheduler: update due", "region", regionID, "label", label, "interval", interval)

		// Guard: no running import or update
		if running, _ := h.DB.HasRunningJobGlobal(ctx, "osm_import"); running {
			slog.Info("osm scheduler: skipping, import already running")
			continue
		}
		if running, _ := h.DB.HasRunningJobGlobal(ctx, "osm_update"); running {
			slog.Info("osm scheduler: skipping, update already running")
			continue
		}

		entityName := "OSM Update: " + label
		jobID, created, err := h.DB.InsertBackgroundJobGuarded(ctx, "system", "osm_update", nil, &entityName, 0)
		if err != nil || !created {
			slog.Info("osm scheduler: osm_update job conflict", "region", regionID, "error", err)
			continue
		}

		// Capture loop variables for goroutine
		rID, rLabel, rReplURL, rPbfPath, rLastSeq := regionID, label, replURL, pbfPath, lastSeq
		shutdown.Go(jobID, func(jobCtx context.Context) {
			h.runRegionUpdate(jobCtx, jobID, rID, rLabel, rReplURL, rPbfPath, rLastSeq)
		})

		// Only one region per scheduler tick (sequential, OOM-safe)
		break
	}
}

// isUpdateDue returns true when the scheduled update window is reached.
// Weekly: at least 6 days since last update, matching weekday and hour.
// Monthly: on the configured day-of-month at the configured hour, at least 1 month after last update.
func isUpdateDue(interval string, weekday, day, hour int, lastUpdatedAt time.Time, now time.Time) bool {
	if lastUpdatedAt.IsZero() {
		return false // Never updated — scheduler cannot decide
	}
	switch interval {
	case "weekly":
		daysSince := now.Sub(lastUpdatedAt).Hours() / 24
		if daysSince < 6 {
			return false
		}
		isoWeekday := (int(now.Weekday())+6)%7 + 1
		return isoWeekday == weekday && now.Hour() == hour
	case "monthly":
		if now.Day() != day {
			return false
		}
		monthsSince := (now.Year()-lastUpdatedAt.Year())*12 + int(now.Month()-lastUpdatedAt.Month())
		return monthsSince >= 1 && now.Hour() == hour
	}
	return false
}

// runRegionUpdate patches a region's PBF and then runs a full reimport via RunPerRegion.
// Called both by the scheduler and by the manual TriggerOSMRegionUpdate endpoint.
// NOTE: jobReporter is defined in osm_import.go (same package).
func (h *Handler) runRegionUpdate(ctx context.Context, jobID, regionID, label, replURL, pbfPath string, lastSeq int) {
	slog.Info("osm update: starting", "region", regionID, "seq", lastSeq)

	// Phase 1: Patch PBF (download .osc.gz diffs + osmium apply-changes)
	newSeq, err := osmregions.PatchPBF(ctx, pbfPath, replURL, lastSeq)
	if errors.Is(err, osmregions.ErrTooOld) {
		slog.Warn("osm update: PBF too old for incremental update", "region", regionID)
		// Recovery path: user re-imports the region via the Import Assistant.
		// RunPerRegion uses ON CONFLICT DO UPDATE SET status='importing', which works
		// regardless of the previous status — so 'needs_redownload' is a valid starting point.
		h.DB.Pool.Exec(ctx, `UPDATE osm_import_regions SET status = 'needs_redownload' WHERE region_id = $1`, regionID)
		h.DB.CompleteBackgroundJob(ctx, jobID, true, "PBF zu alt für inkrementelles Update — bitte neu herunterladen")
		return
	}
	if err != nil {
		slog.Error("osm update: patchPBF failed", "region", regionID, "error", err)
		h.DB.CompleteBackgroundJob(ctx, jobID, true, err.Error())
		return
	}

	// Phase 2: Full reimport from patched PBF (osmium filter → osm2pgsql → merge)
	// RunPerRegion skips PBF download because patched file has fresh ModTime (< 7d threshold).
	// It also updates replication_url, last_sequence_number, data_date from PBF header.
	dataDir := os.Getenv("OSM_DATA_DIR")
	if dataDir == "" {
		dataDir = "/data/osm"
	}
	reg, ok := osmregions.RegionByID(regionID)
	if !ok {
		slog.Error("osm update: region not in catalog", "region", regionID)
		h.DB.CompleteBackgroundJob(ctx, jobID, true, "Region nicht im Katalog: "+regionID)
		return
	}

	reporter := &jobReporter{db: h.DB, jobID: jobID}
	pipeline := &osmregions.Pipeline{Pool: h.DB.Pool, DataDir: dataDir, DBURL: os.Getenv("DATABASE_URL")}

	// Load categories from DB (what was imported last time)
	var categories []string
	h.DB.Pool.QueryRow(ctx,
		`SELECT categories FROM osm_import_regions WHERE region_id = $1`, regionID).Scan(&categories)
	if len(categories) == 0 {
		// Fallback: use all known categories
		categories = allCategoryIDs()
	}

	config := osmregions.PipelineConfig{Categories: categories}
	success := pipeline.RunPerRegion(ctx, reg, config, reporter)
	if !success {
		slog.Error("osm update: reimport failed", "region", regionID)
		// RunPerRegion already calls reporter.Fail, job status is set via reporter
		return
	}

	// Phase 3: Update last_updated_at (imported_at is already set by RunPerRegion)
	h.DB.Pool.Exec(ctx, `
		UPDATE osm_import_regions SET last_updated_at = now()
		WHERE region_id = $1`, regionID)

	// Phase 4: Enrichment (synchronous — OOM-safe, one region at a time)
	reporter.SetPhase("enrichment")
	pipeline.RunEnrichmentAndSplitting(ctx, regionID, reporter)

	slog.Info("osm update: completed", "region", regionID, "new_seq", newSeq)
	h.DB.CompleteBackgroundJob(ctx, jobID, false, "")
}

// TriggerOSMRegionUpdate triggers an immediate incremental update for a single region.
// POST /api/v1/admin/osm-regions/{regionID}/update
func (h *Handler) TriggerOSMRegionUpdate(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	regionID := chi.URLParam(r, "regionID")
	if regionID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "regionID required", r.URL.Path)
		return
	}

	// Load region data
	var label string
	var replURL, pbfPath *string
	var lastSeq int
	err := h.DB.Pool.QueryRow(r.Context(), `
		SELECT label, replication_url, pbf_path, last_sequence_number
		FROM osm_import_regions
		WHERE region_id = $1 AND status = 'imported'`, regionID).
		Scan(&label, &replURL, &pbfPath, &lastSeq)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "Region nicht gefunden oder nicht im Status 'imported'", r.URL.Path)
		return
	}
	if replURL == nil || *replURL == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "Region hat keine Replication-URL — inkrementelles Update nicht möglich", r.URL.Path)
		return
	}
	if pbfPath == nil || *pbfPath == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "Region hat keinen PBF-Pfad — bitte neu importieren", r.URL.Path)
		return
	}

	// Guard: no running import or update
	if running, _ := h.DB.HasRunningJobGlobal(r.Context(), "osm_import"); running {
		httputil.WriteJSON(w, http.StatusConflict, map[string]string{"error": "Import läuft bereits"})
		return
	}

	entityName := "OSM Update: " + label
	jobID, created, err := h.DB.InsertBackgroundJobGuarded(r.Context(), userID, "osm_update", nil, &entityName, 0)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}
	if !created {
		httputil.WriteJSON(w, http.StatusConflict, map[string]string{"error": "Update läuft bereits"})
		return
	}

	rReplURL, rPbfPath := *replURL, *pbfPath
	shutdown.Go(jobID, func(jobCtx context.Context) {
		h.runRegionUpdate(jobCtx, jobID, regionID, label, rReplURL, rPbfPath, lastSeq)
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// allCategoryIDs returns all known OSM category IDs as fallback for incremental updates.
func allCategoryIDs() []string {
	ids := make([]string, 0, len(osmCategories))
	for id := range osmCategories {
		ids = append(ids, id)
	}
	return ids
}
