package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/paulmach/orb"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/geo"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// RouteFromActivityWaypoints returns waypoints derived from an activity's GPS track
// WITHOUT saving a route to the database. Used by "Open in Route Planner" button.
// GET /api/v1/routes/from-activity/{activityId}/waypoints
func (h *Handler) RouteFromActivityWaypoints(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "activityId")

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("route waypoints from activity: get activity failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}
	if !activity.HasGPS {
		httputil.WriteError(w, http.StatusUnprocessableEntity, "No GPS", "activity has no GPS track", r.URL.Path)
		return
	}

	gpsPoints, err := h.DB.GetActivityGPSStream(r.Context(), activityID)
	if err != nil {
		slog.Error("route waypoints from activity: load GPS stream failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load GPS track", r.URL.Path)
		return
	}
	if len(gpsPoints) < 2 {
		httputil.WriteError(w, http.StatusUnprocessableEntity, "Insufficient Data", "activity has too few GPS points", r.URL.Path)
		return
	}

	simplified := simplifyToTarget(gpsPoints, 50)
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
			LatLng: [2]float64{p[1], p[0]}, // orb.Point is [lng, lat]
			Type:   wpType,
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"waypoints": waypoints})
}

// RouteFromActivity creates a new planned route from an existing activity's GPS track.
// POST /api/v1/routes/from-activity/{activityId}
func (h *Handler) RouteFromActivity(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "activityId")

	// Load activity and verify ownership.
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("route from activity: get activity failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}
	if !activity.HasGPS {
		httputil.WriteError(w, http.StatusUnprocessableEntity, "No GPS", "activity has no GPS track", r.URL.Path)
		return
	}

	// Load GPS stream points.
	gpsPoints, err := h.DB.GetActivityGPSStream(r.Context(), activityID)
	if err != nil {
		slog.Error("route from activity: load GPS stream failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load GPS track", r.URL.Path)
		return
	}
	if len(gpsPoints) < 2 {
		httputil.WriteError(w, http.StatusUnprocessableEntity, "Insufficient Data", "activity has too few GPS points", r.URL.Path)
		return
	}

	// Build GeoJSON LineString from all GPS points.
	ls := gpsStreamToLineString(gpsPoints)
	gjBytes, err := geo.MarshalGeoJSON(ls)
	if err != nil {
		slog.Error("route from activity: marshal GeoJSON failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to build geometry", r.URL.Path)
		return
	}
	geojsonStr := string(gjBytes)

	// Simplify to ~50 waypoints using Ramer-Douglas-Peucker.
	simplified := simplifyToTarget(gpsPoints, 50)
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
			LatLng: [2]float64{p[1], p[0]}, // orb.Point is [lng, lat]
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
		GeometryGeoJSON:    geojsonStr,
		Waypoints:          waypoints,
		Segments:           segments,
		TotalDistanceM:     activity.DistanceM,
		TotalElevationGain: activity.ElevationGainM,
		TotalElevationLoss: activity.ElevationLossM,
	}

	id, err := h.DB.CreateRoute(r.Context(), &route)
	if err != nil {
		slog.Error("route from activity: create route failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create route", r.URL.Path)
		return
	}

	route.ID = id
	httputil.WriteJSON(w, http.StatusCreated, route)
}

// gpsStreamToLineString converts GPS stream points to an orb.LineString.
func gpsStreamToLineString(points []database.GPSStreamPoint) orb.LineString {
	return geo.LineStringFromLatLng(points, func(p database.GPSStreamPoint) (float64, float64) {
		return p.Lat, p.Lng
	})
}

// simplifyToTarget reduces the GPS stream to approximately targetCount points
// using geo.SimplifyDouglasPeucker with iteratively adjusted threshold (meters).
func simplifyToTarget(points []database.GPSStreamPoint, targetCount int) orb.LineString {
	ls := gpsStreamToLineString(points)
	if len(ls) <= targetCount {
		return ls
	}

	// Binary search for threshold in meters that yields ~targetCount points.
	lo, hi := 0.0, 5000.0 // meters
	var result orb.LineString
	for range 20 {
		mid := (lo + hi) / 2
		result = geo.SimplifyDouglasPeucker(ls, mid)
		if len(result) > targetCount {
			lo = mid
		} else {
			hi = mid
		}
	}
	return result
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
