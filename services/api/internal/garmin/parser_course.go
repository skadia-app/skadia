package garmin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

// processCourseFIT parses a Garmin course FIT file (type 6) and imports the
// GPS track as a planned route in Skadia.
func processCourseFIT(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID string) error {
	// Store original FIT file in ShadowFS with file_type "route".
	if len(data) > 14 {
		ts := time.Now()
		storeHealthFIT(ctx, db, fs, data, userID, "route", fmt.Sprintf("%s_course.fit", ts.Format("2006-01-02")), ts)
	}

	// Decode FIT messages.
	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		return fmt.Errorf("processCourseFIT: decode: %w", err)
	}

	// --- Collect data from messages ---

	var courseName string
	var sport typedef.Sport
	var subSport typedef.SubSport

	type gpsPoint struct {
		Lat float64
		Lng float64
		Ele float64
	}
	var points []gpsPoint

	type coursePoint struct {
		Lat      float64
		Lng      float64
		Type     string
		Name     string
		Distance float64
	}
	var coursePoints []coursePoint

	var totalDistanceM *float64
	var totalAscent *float64
	var totalDescent *float64

	for i := range fit.Messages {
		msg := &fit.Messages[i]
		switch msg.Num {

		case 31: // Course (msg 31)
			c := mesgdef.NewCourse(msg)
			if c.Name != basetype.StringInvalid && c.Name != "" {
				courseName = c.Name
			}
			sport = c.Sport
			subSport = c.SubSport

		case 20: // Record (msg 20) — GPS track points
			rec := mesgdef.NewRecord(msg)
			if rec.PositionLat == basetype.Sint32Invalid || rec.PositionLong == basetype.Sint32Invalid {
				continue
			}
			lat := float64(rec.PositionLat) * (180.0 / math.Pow(2, 31))
			lng := float64(rec.PositionLong) * (180.0 / math.Pow(2, 31))
			ele := 0.0
			if rec.EnhancedAltitude != basetype.Uint32Invalid {
				ele = float64(rec.EnhancedAltitude)/5.0 - 500.0
			} else if rec.Altitude != basetype.Uint16Invalid {
				ele = float64(rec.Altitude)/5.0 - 500.0
			}
			points = append(points, gpsPoint{Lat: lat, Lng: lng, Ele: ele})

		case 32: // CoursePoint (msg 32) — named waypoints along the course
			cp := mesgdef.NewCoursePoint(msg)
			if cp.PositionLat == basetype.Sint32Invalid || cp.PositionLong == basetype.Sint32Invalid {
				continue
			}
			lat := float64(cp.PositionLat) * (180.0 / math.Pow(2, 31))
			lng := float64(cp.PositionLong) * (180.0 / math.Pow(2, 31))
			dist := 0.0
			if cp.Distance != basetype.Uint32Invalid {
				dist = float64(cp.Distance) / 100.0
			}
			name := ""
			if cp.Name != basetype.StringInvalid {
				name = cp.Name
			}
			coursePoints = append(coursePoints, coursePoint{
				Lat:      lat,
				Lng:      lng,
				Type:     coursePointTypeName(cp.Type),
				Name:     name,
				Distance: dist,
			})

		case 19: // Lap (msg 19) — aggregate distance and elevation
			lap := mesgdef.NewLap(msg)
			if lap.TotalDistance != basetype.Uint32Invalid {
				d := float64(lap.TotalDistance) / 100.0
				totalDistanceM = &d
			}
			if lap.TotalAscent != basetype.Uint16Invalid {
				a := float64(lap.TotalAscent) // RAW meters — no scaling
				totalAscent = &a
			}
			if lap.TotalDescent != basetype.Uint16Invalid {
				d := float64(lap.TotalDescent) // RAW meters — no scaling
				totalDescent = &d
			}
		}
	}

	if len(points) < 2 {
		slog.Info("processCourseFIT: skipping course with < 2 GPS points", "name", courseName)
		return nil
	}

	// --- Build GeoJSON LineString ---
	coords := make([]string, len(points))
	for i, p := range points {
		coords[i] = fmt.Sprintf("[%f,%f,%f]", p.Lng, p.Lat, p.Ele)
	}
	geoJSON := fmt.Sprintf(`{"type":"LineString","coordinates":[%s]}`, strings.Join(coords, ","))

	// --- Build waypoints JSONB from CoursePoints ---
	waypointsJSON := "[]"
	if len(coursePoints) > 0 {
		b, err := json.Marshal(coursePoints)
		if err == nil {
			waypointsJSON = string(b)
		}
	}

	// --- Map FIT sport enum to taxonomy sport type ---
	sportType := fitSportToSportType(sport, subSport)
	profile := defaultProfileForSport(sportType)

	// --- SourceID: SHA256(name + sport string) for dedup ---
	sourceIDRaw := fmt.Sprintf("%s|%s", courseName, sportType)
	sourceIDHash := sha256hex([]byte(sourceIDRaw))
	sourcePlatform := "garmin_fit"

	route := &model.RouteImportData{
		Name:               courseName,
		SportType:          sportType,
		Profile:            profile,
		GeometryGeoJSON:    geoJSON,
		WaypointsJSON:      waypointsJSON,
		SegmentsJSON:       "[]",
		TotalDistanceM:     totalDistanceM,
		TotalElevationGain: totalAscent,
		TotalElevationLoss: totalDescent,
		SourcePlatform:     &sourcePlatform,
		SourceID:           &sourceIDHash,
	}

	_, err = db.CreateRouteFromImport(ctx, userID, route)
	if err != nil {
		return fmt.Errorf("processCourseFIT: create route: %w", err)
	}

	slog.Info("processCourseFIT: route imported", "name", courseName, "sport", sportType, "points", len(points))
	return nil
}

// coursePointTypeName maps a FIT CoursePoint enum to a string label.
func coursePointTypeName(t typedef.CoursePoint) string {
	switch t {
	case 0:
		return "generic"
	case 1:
		return "summit"
	case 2:
		return "valley"
	case 3:
		return "water"
	case 4:
		return "food"
	case 5:
		return "danger"
	case 6:
		return "left"
	case 7:
		return "right"
	case 8:
		return "straight"
	case 9:
		return "first_aid"
	case 10:
		return "4th_category"
	case 11:
		return "3rd_category"
	case 12:
		return "2nd_category"
	case 13:
		return "1st_category"
	case 14:
		return "hors_category"
	case 15:
		return "sprint"
	case 16:
		return "left_fork"
	case 17:
		return "right_fork"
	case 18:
		return "middle_fork"
	case 19:
		return "slight_left"
	case 20:
		return "sharp_left"
	case 21:
		return "slight_right"
	case 22:
		return "sharp_right"
	case 23:
		return "u_turn"
	case 24:
		return "segment_start"
	case 25:
		return "segment_end"
	case 26:
		return "campsite"
	case 27:
		return "aid_station"
	case 28:
		return "rest_stop"
	case 40:
		return "general_distance"
	case 48:
		return "custom"
	default:
		return "generic"
	}
}

// fitSportToSportType maps a FIT sport + sub-sport enum to a Skadia taxonomy sport type.
func fitSportToSportType(sport typedef.Sport, subSport typedef.SubSport) string {
	switch sport {
	case typedef.SportCycling:
		switch subSport {
		case typedef.SubSportMountain:
			return "mountain_biking"
		case typedef.SubSportGravelCycling:
			return "gravel_cycling"
		case typedef.SubSportIndoorCycling:
			return "indoor_cycling"
		default:
			return "cycling"
		}
	case typedef.SportRunning:
		switch subSport {
		case typedef.SubSportTrail:
			return "trail_running"
		default:
			return "running"
		}
	case typedef.SportHiking:
		return "hiking"
	case typedef.SportWalking:
		return "walking"
	case typedef.SportSwimming:
		return "swimming"
	case typedef.SportAlpineSkiing:
		return "alpine_skiing"
	case typedef.SportCrossCountrySkiing:
		return "cross_country_skiing"
	case typedef.SportPaddling:
		return "kayaking"
	case typedef.SportRowing:
		return "rowing"
	default:
		return "generic"
	}
}

// defaultProfileForSport returns a sensible routing profile for the given sport type.
func defaultProfileForSport(sportType string) string {
	switch sportType {
	case "cycling", "gravel_cycling", "mountain_biking", "indoor_cycling":
		return "bike"
	case "running", "trail_running", "hiking", "walking":
		return "foot"
	default:
		return "outdoor"
	}
}
