package handler

import (
	"net/http"
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
