package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"

	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// GarminCourse represents a single course from the Garmin Connect export JSON.
type GarminCourse struct {
	CourseID      int64            `json:"courseId"`
	CourseName    string           `json:"courseName"`
	CourseType    string           `json:"courseType"` // "hiking", "mountain_biking", "road_cycling", etc.
	ElevationGain float64          `json:"elevationGainMeter"`
	ElevationLoss float64          `json:"elevationLossMeter"`
	GeoPoints     []GarminGeoPoint `json:"geoPoints"`
}

// GarminGeoPoint is a single geographic point in a Garmin course.
type GarminGeoPoint struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Elevation float64 `json:"elevation"`
}

// CoursesImporter imports planned routes from Garmin DI-Connect-Routing export.
type CoursesImporter struct{}

func (i *CoursesImporter) Name() string        { return "courses" }
func (i *CoursesImporter) Description() string { return "Geplante Routen/Courses" }
func (i *CoursesImporter) Priority() int       { return 50 }

func (i *CoursesImporter) Detect(src ExportSource) (bool, int, error) {
	files, err := src.FindFiles("DI-Connect-Routing/*_courses_*.json")
	if err != nil {
		return false, 0, err
	}
	if len(files) == 0 {
		return false, 0, nil
	}
	total := 0
	for _, f := range files {
		var courses []GarminCourse
		if err := src.ReadJSON(f, &courses); err == nil {
			total += len(courses)
		}
	}
	return true, total, nil
}

func (i *CoursesImporter) Import(ctx context.Context, p ImportParams) (*ImportResult, error) {
	files, err := p.Source.FindFiles("DI-Connect-Routing/*_courses_*.json")
	if err != nil {
		return nil, fmt.Errorf("courses: find files: %w", err)
	}

	result := &ImportResult{}

	for _, f := range files {
		var courses []GarminCourse
		if err := p.Source.ReadJSON(f, &courses); err != nil {
			slog.Warn("garmin courses: cannot read file", "file", f, "error", err)
			result.Errors = append(result.Errors, "read "+f+": "+err.Error())
			continue
		}

		total := len(courses)
		for idx, course := range courses {
			if ctx.Err() != nil {
				break
			}

			if p.OnProgress != nil {
				p.OnProgress(idx+1, total, "courses: "+course.CourseName)
			}

			geojson := geoPointsToGeoJSON(course.GeoPoints)
			if geojson == "" {
				slog.Warn("garmin courses: skipping course with < 2 points", "name", course.CourseName)
				result.Skipped++
				continue
			}

			distM := courseCalculateDistance(course.GeoPoints)

			// Use courseId for dedup; fallback to name if 0
			sourceID := strconv.FormatInt(course.CourseID, 10)
			if course.CourseID == 0 {
				sourceID = course.CourseName
			}
			platform := "garmin"

			dup, err := importer.CheckRouteDuplicate(
				ctx, p.DB, p.UserID,
				platform, sourceID,
				geojson, distM,
				100.0, // hausdorff threshold: 100m
				2.0,   // length diff threshold: 2km
			)
			if err != nil {
				slog.Warn("garmin courses: dup check failed", "name", course.CourseName, "error", err)
				result.Errors = append(result.Errors, "dup check "+course.CourseName+": "+err.Error())
				continue
			}
			if dup != nil {
				result.Skipped++
				continue
			}

			sportType := garminCourseTypeToSport(course.CourseType)
			sourcePlatform := platform
			sid := sourceID
			elevGain := course.ElevationGain
			elevLoss := course.ElevationLoss
			d := distM

			_, err = p.DB.CreateRouteFromImport(ctx, p.UserID, &model.RouteImportData{
				Name:               course.CourseName,
				SportType:          sportType,
				Profile:            "outdoor",
				GeometryGeoJSON:    geojson,
				WaypointsJSON:      "[]",
				SegmentsJSON:       "[]",
				TotalDistanceM:     &d,
				TotalElevationGain: &elevGain,
				TotalElevationLoss: &elevLoss,
				SourcePlatform:     &sourcePlatform,
				SourceID:           &sid,
			})
			if err != nil {
				slog.Warn("garmin courses: create route failed", "name", course.CourseName, "error", err)
				result.Errors = append(result.Errors, "create "+course.CourseName+": "+err.Error())
				continue
			}
			result.Imported++
		}
	}

	slog.Info("garmin courses import done",
		"imported", result.Imported,
		"skipped", result.Skipped,
		"errors", len(result.Errors),
	)
	return result, nil
}

// garminCourseTypeToSport maps Garmin courseType strings to Skadia sport_type.
func garminCourseTypeToSport(courseType string) string {
	switch courseType {
	case "road_cycling", "cycling":
		return "cycling"
	case "mountain_biking":
		return "cycling"
	case "gravel_cycling":
		return "cycling"
	case "running":
		return "running"
	case "trail_running":
		return "trail_running"
	case "hiking":
		return "hiking"
	case "walking":
		return "walking"
	case "swimming":
		return "swimming"
	case "skiing":
		return "alpine_skiing"
	case "cross_country_skiing":
		return "cross_country_skiing"
	default:
		return "generic"
	}
}

// geoPointsToGeoJSON converts Garmin GeoPoints to a GeoJSON LineString string.
// Returns empty string if fewer than 2 points.
func geoPointsToGeoJSON(pts []GarminGeoPoint) string {
	if len(pts) < 2 {
		return ""
	}
	coords := make([]interface{}, 0, len(pts))
	for _, p := range pts {
		if p.Elevation != 0 {
			coords = append(coords, []float64{p.Longitude, p.Latitude, p.Elevation})
		} else {
			coords = append(coords, []float64{p.Longitude, p.Latitude})
		}
	}
	b, err := json.Marshal(map[string]interface{}{
		"type":        "LineString",
		"coordinates": coords,
	})
	if err != nil {
		return ""
	}
	return string(b)
}

// courseCalculateDistance computes the total distance of a course in meters using Haversine.
func courseCalculateDistance(pts []GarminGeoPoint) float64 {
	var total float64
	for i := 1; i < len(pts); i++ {
		total += courseHaversineM(pts[i-1].Latitude, pts[i-1].Longitude, pts[i].Latitude, pts[i].Longitude)
	}
	return total
}

// courseHaversineM returns the distance in meters between two lat/lng points.
func courseHaversineM(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000.0
	φ1 := lat1 * math.Pi / 180
	φ2 := lat2 * math.Pi / 180
	Δφ := (lat2 - lat1) * math.Pi / 180
	Δλ := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
