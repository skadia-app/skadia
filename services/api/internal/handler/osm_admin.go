package handler

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	osmregions "github.com/skadia-app/skadia/services/api/internal/osm"
)

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

	writeJSON(w, http.StatusOK, map[string]any{
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
		"peaks":       categories["peak"],
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
		       pbf_size_bytes, import_duration_s, categories, status
		FROM osm_import_regions ORDER BY continent, label`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
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
	}
	var regions []region
	for rows.Next() {
		var reg region
		if err := rows.Scan(&reg.RegionID, &reg.Label, &reg.Continent, &reg.ImportedAt,
			&reg.PBFDate, &reg.PBFSizeBytes, &reg.ImportDurationS, &reg.Categories, &reg.Status); err != nil {
			continue
		}
		regions = append(regions, reg)
	}
	if regions == nil {
		regions = []region{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"regions": regions})
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
	writeJSON(w, http.StatusOK, map[string]any{
		"tables":      tables,
		"total_bytes": totalBytes,
	})
}

// DeleteOSMRegion removes a region from tracking (does NOT delete OSM data).
// DELETE /api/v1/admin/osm-regions/{regionID}
func (h *Handler) DeleteOSMRegion(w http.ResponseWriter, r *http.Request) {
	regionID := chi.URLParam(r, "regionID")
	if regionID == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "missing region ID", r.URL.Path)
		return
	}
	tag, err := h.DB.Pool.Exec(r.Context(),
		`DELETE FROM osm_import_regions WHERE region_id = $1`, regionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Not Found", "region not found", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// SearchAddress performs trigram-based search across streets and places.
// GET /api/v1/geo/search?q=Bergstraße&type=street|place|all&limit=20&lat=47.1&lng=11.4
func (h *Handler) SearchAddress(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		writeError(w, http.StatusBadRequest, "Bad Request", "query must be at least 2 characters", r.URL.Path)
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
			  AND category IN ('settlement', 'peak', 'pass')
			ORDER BY similarity(name, $1) DESC
			LIMIT $2`
		placeRows, err := h.DB.Pool.Query(ctx, placeQuery, q, limit)
		if err == nil {
			for placeRows.Next() {
				var sr searchResult
				sr.Type = "place"
				var category string
				if err := placeRows.Scan(&sr.Name, &category, &sr.Subtype, &sr.Lat, &sr.Lng, &sr.Similarity); err == nil {
					sr.Type = category // peak, pass, settlement
					results = append(results, sr)
				}
			}
			placeRows.Close()
		}
	}

	if results == nil {
		results = []searchResult{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "count": len(results)})
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
		writeJSON(w, http.StatusOK, map[string]any{"sources": []demSource{}, "total_tiles": 0})
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
	writeJSON(w, http.StatusOK, map[string]any{"sources": sources, "total_tiles": totalTiles})
}

// RetryOSMEnrichment re-runs only the enrichment + splitting phases (6+7) on existing osm_ways.
// POST /api/v1/admin/osm-import/retry-enrichment
func (h *Handler) RetryOSMEnrichment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	entityName := "Enrichment Retry"
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "osm_import", nil, &entityName, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}

	go func() {
		ctx := context.Background()
		reporter := &jobReporter{db: h.DB, jobID: jobID}

		reporter.SetPhase("enrichment")
		reporter.SetProgress(0)
		if err := osmregions.RunSpatialEnrichment(ctx, h.DB.Pool, false, func(pct int) {
			reporter.SetProgress(int64(pct))
		}); err != nil {
			slog.Error("retry enrichment: spatial enrichment failed", "error", err)
		}

		reporter.SetPhase("splitting")
		reporter.SetProgress(0)
		if err := osmregions.SplitAtIntersections(ctx, h.DB.Pool, func(pct int) {
			reporter.SetProgress(int64(pct))
		}); err != nil {
			slog.Error("retry enrichment: splitting failed", "error", err)
		}

		reporter.Complete()
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// RetryOSMTopology re-runs only the topology build phase (5) on existing osm_ways.
// POST /api/v1/admin/osm-import/retry-topology
func (h *Handler) RetryOSMTopology(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	entityName := "Topology Retry"
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "osm_import", nil, &entityName, 5)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}

	go func() {
		ctx := context.Background()
		reporter := &jobReporter{db: h.DB, jobID: jobID}

		pipeline := &osmregions.Pipeline{
			Pool:    h.DB.Pool,
			DataDir: os.Getenv("OSM_DATA_DIR"),
			DBURL:   os.Getenv("DATABASE_URL"),
		}
		if pipeline.DataDir == "" {
			pipeline.DataDir = "/data/osm"
		}

		reporter.SetPhase("topology")
		reporter.SetProgress(0)
		pipeline.RunTopologyOnly(ctx, reporter)
		reporter.Complete()
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}
