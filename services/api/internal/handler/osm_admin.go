package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
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

	// Non-POI table counts
	var ways, lifts, pistes, waterways, forests, glaciers, wetlands, routes, boundaries int64
	h.DB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM osm_ways").Scan(&ways)
	h.DB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM osm_lifts").Scan(&lifts)
	h.DB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM osm_pistes").Scan(&pistes)
	h.DB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM osm_waterways").Scan(&waterways)
	h.DB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM osm_forests").Scan(&forests)
	h.DB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM osm_glaciers").Scan(&glaciers)
	h.DB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM osm_wetlands").Scan(&wetlands)
	h.DB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM osm_routes").Scan(&routes)
	h.DB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM osm_boundaries").Scan(&boundaries)

	hasData := len(categories) > 0 || ways > 0 || lifts > 0 || routes > 0

	writeJSON(w, http.StatusOK, map[string]any{
		"categories": categories,
		"ways":       ways,
		"lifts":      lifts,
		"pistes":     pistes,
		"waterways":  waterways,
		"forests":    forests,
		"glaciers":   glaciers,
		"wetlands":   wetlands,
		"routes":     routes,
		"boundaries": boundaries,
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
// GET /api/v1/admin/osm-disk-usage
func (h *Handler) GetOSMDiskUsage(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT tablename, total_bytes FROM osm_disk_usage`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}
	defer rows.Close()

	type tableSize struct {
		Tablename  string `json:"tablename"`
		TotalBytes int64  `json:"total_bytes"`
	}
	var tables []tableSize
	var totalBytes int64
	for rows.Next() {
		var t tableSize
		if err := rows.Scan(&t.Tablename, &t.TotalBytes); err != nil {
			continue
		}
		tables = append(tables, t)
		totalBytes += t.TotalBytes
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

// ensure strconv and strings are used (Task 6 placeholder)
var _ = strconv.Itoa
var _ = strings.TrimSpace
