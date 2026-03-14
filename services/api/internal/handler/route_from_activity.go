package handler

import (
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// RouteFromActivityWaypoints returns waypoints derived from an activity's GPS track
// WITHOUT saving a route to the database. Used by "Open in Route Planner" button.
// GET /api/v1/routes/from-activity/{activityId}/waypoints
func (h *Handler) RouteFromActivityWaypoints(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "activityId")

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("route waypoints from activity: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}
	if !activity.HasGPS {
		writeError(w, http.StatusUnprocessableEntity, "No GPS", "activity has no GPS track", r.URL.Path)
		return
	}

	gpsPoints, err := h.DB.GetActivityGPSStream(r.Context(), activityID)
	if err != nil {
		slog.Error("route waypoints from activity: load GPS stream failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load GPS track", r.URL.Path)
		return
	}
	if len(gpsPoints) < 2 {
		writeError(w, http.StatusUnprocessableEntity, "Insufficient Data", "activity has too few GPS points", r.URL.Path)
		return
	}

	simplified := rdpSimplifyGPSPoints(gpsPoints, 50)
	waypoints := make([]model.RouteWaypoint, len(simplified))
	for i, p := range simplified {
		wpType := "via"
		if i == 0 {
			wpType = "start"
		} else if i == len(simplified)-1 {
			wpType = "end"
		}
		waypoints[i] = model.RouteWaypoint{
			ID:     uuid.New().String(),
			LatLng: [2]float64{p.Lat, p.Lng},
			Type:   wpType,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"waypoints": waypoints})
}

// RouteFromActivity creates a new planned route from an existing activity's GPS track.
// POST /api/v1/routes/from-activity/{activityId}
func (h *Handler) RouteFromActivity(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "activityId")

	// Load activity and verify ownership.
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("route from activity: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}
	if !activity.HasGPS {
		writeError(w, http.StatusUnprocessableEntity, "No GPS", "activity has no GPS track", r.URL.Path)
		return
	}

	// Load GPS stream points.
	gpsPoints, err := h.DB.GetActivityGPSStream(r.Context(), activityID)
	if err != nil {
		slog.Error("route from activity: load GPS stream failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load GPS track", r.URL.Path)
		return
	}
	if len(gpsPoints) < 2 {
		writeError(w, http.StatusUnprocessableEntity, "Insufficient Data", "activity has too few GPS points", r.URL.Path)
		return
	}

	// Build GeoJSON LineString from all GPS points.
	geojson := buildLineStringGeoJSON(gpsPoints)

	// Simplify to ~50 waypoints using Ramer-Douglas-Peucker.
	simplified := rdpSimplifyGPSPoints(gpsPoints, 50)
	waypoints := make([]model.RouteWaypoint, len(simplified))
	for i, p := range simplified {
		wpType := "via"
		if i == 0 {
			wpType = "start"
		} else if i == len(simplified)-1 {
			wpType = "end"
		}
		waypoints[i] = model.RouteWaypoint{
			ID:     uuid.New().String(),
			LatLng: [2]float64{p.Lat, p.Lng},
			Type:   wpType,
		}
	}

	// Build a single segment from start to end.
	var segments []model.RouteSegment
	if len(waypoints) >= 2 {
		segments = append(segments, model.RouteSegment{
			FromWaypoint:  waypoints[0].ID,
			ToWaypoint:    waypoints[len(waypoints)-1].ID,
			Mode:          "offroad",
			DistanceM:     derefFloat64(activity.DistanceM),
			ElevationGain: derefFloat64(activity.ElevationGainM),
			ElevationLoss: derefFloat64(activity.ElevationLossM),
		})
	}

	routeName := "Route"
	if activity.Title != nil && *activity.Title != "" {
		routeName = "Route: " + *activity.Title
	}

	route := model.PlannedRoute{
		UserID:             userID,
		Name:               routeName,
		SportType:          activity.SportType,
		Profile:            sportTypeToProfile(activity.SportType),
		GeometryGeoJSON:    geojson,
		Waypoints:          waypoints,
		Segments:           segments,
		TotalDistanceM:     activity.DistanceM,
		TotalElevationGain: activity.ElevationGainM,
		TotalElevationLoss: activity.ElevationLossM,
	}

	id, err := h.DB.CreateRoute(r.Context(), &route)
	if err != nil {
		slog.Error("route from activity: create route failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create route", r.URL.Path)
		return
	}

	route.ID = id
	writeJSON(w, http.StatusCreated, route)
}

// buildLineStringGeoJSON builds a GeoJSON LineString string from GPS stream points.
func buildLineStringGeoJSON(points []database.GPSStreamPoint) string {
	var sb strings.Builder
	sb.WriteString(`{"type":"LineString","coordinates":[`)
	for i, p := range points {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(fmt.Sprintf("[%f,%f]", p.Lng, p.Lat))
	}
	sb.WriteString("]}")
	return sb.String()
}

// gpsPoint is a lightweight lat/lng point for internal use in simplification.
type gpsPoint struct {
	Lat float64
	Lng float64
}

// rdpSimplifyGPSPoints reduces the GPS stream to approximately targetCount points
// using Ramer-Douglas-Peucker with iteratively adjusted epsilon.
func rdpSimplifyGPSPoints(points []database.GPSStreamPoint, targetCount int) []gpsPoint {
	all := make([]gpsPoint, len(points))
	for i, p := range points {
		all[i] = gpsPoint{Lat: p.Lat, Lng: p.Lng}
	}

	if len(all) <= targetCount {
		return all
	}

	// Binary search for epsilon that yields ~targetCount points.
	lo, hi := 0.0, 0.01 // degrees
	var result []gpsPoint
	for range 20 {
		mid := (lo + hi) / 2
		result = rdpGeo(all, mid)
		if len(result) > targetCount {
			lo = mid
		} else {
			hi = mid
		}
	}
	return result
}

// rdpGeo applies Ramer-Douglas-Peucker simplification to geographic points.
func rdpGeo(points []gpsPoint, epsilon float64) []gpsPoint {
	if len(points) < 3 {
		return points
	}

	maxDist := 0.0
	maxIdx := 0
	for i := 1; i < len(points)-1; i++ {
		d := perpDistGeo(points[i], points[0], points[len(points)-1])
		if d > maxDist {
			maxDist = d
			maxIdx = i
		}
	}

	if maxDist < epsilon {
		return []gpsPoint{points[0], points[len(points)-1]}
	}

	left := rdpGeo(points[:maxIdx+1], epsilon)
	right := rdpGeo(points[maxIdx:], epsilon)
	return append(left[:len(left)-1], right...)
}

// perpDistGeo computes the perpendicular distance (in degrees, approximate)
// from point p to the line segment a-b.
func perpDistGeo(p, a, b gpsPoint) float64 {
	dx := b.Lng - a.Lng
	dy := b.Lat - a.Lat
	if dx == 0 && dy == 0 {
		return math.Sqrt((p.Lng-a.Lng)*(p.Lng-a.Lng) + (p.Lat-a.Lat)*(p.Lat-a.Lat))
	}
	num := math.Abs(dy*p.Lng - dx*p.Lat + b.Lng*a.Lat - b.Lat*a.Lng)
	den := math.Sqrt(dx*dx + dy*dy)
	return num / den
}

// derefFloat64 safely dereferences a *float64, returning 0 if nil.
func derefFloat64(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

// sportTypeToProfile returns a routing profile name for a given sport type.
func sportTypeToProfile(sportType string) string {
	switch sportType {
	case "cycling", "road_cycling", "gravel_cycling":
		return "cycling-road"
	case "mountain_biking":
		return "cycling-mountain"
	case "running", "trail_running":
		return "running"
	default:
		return "hiking-sac2"
	}
}
