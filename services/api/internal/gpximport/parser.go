package gpximport

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/activityutil"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// GPX XML structures

type gpxFile struct {
	XMLName  xml.Name   `xml:"gpx"`
	Creator  string     `xml:"creator,attr"`
	Metadata gpxMeta    `xml:"metadata"`
	Tracks   []gpxTrack `xml:"trk"`
}

type gpxMeta struct {
	Time string `xml:"time"`
}

type gpxTrack struct {
	Name     string       `xml:"name"`
	Type     string       `xml:"type"`
	Segments []gpxSegment `xml:"trkseg"`
}

type gpxSegment struct {
	Points []gpxPoint `xml:"trkpt"`
}

type gpxPoint struct {
	Lat        float64       `xml:"lat,attr"`
	Lon        float64       `xml:"lon,attr"`
	Elevation  *float64      `xml:"ele"`
	Time       string        `xml:"time"`
	Extensions gpxExtensions `xml:"extensions"`
}

type gpxExtensions struct {
	TPExt gpxTPExtension `xml:"TrackPointExtension"`
}

type gpxTPExtension struct {
	HR   *int16   `xml:"hr"`
	Cad  *int16   `xml:"cad"`
	Temp *float64 `xml:"atemp"`
}

func ParseGPX(r io.Reader, filename string) (*model.ActivityData, error) {
	var gpx gpxFile
	if err := xml.NewDecoder(r).Decode(&gpx); err != nil {
		return nil, fmt.Errorf("decode GPX: %w", err)
	}

	if len(gpx.Tracks) == 0 {
		return nil, fmt.Errorf("no tracks found in GPX file")
	}

	trk := gpx.Tracks[0]

	// L35: Max 10M trackpoints to prevent OOM from malformed/malicious files.
	const maxTrackpoints = 10_000_000

	// Collect all points across segments
	var allPoints []gpxPoint
	for _, seg := range trk.Segments {
		allPoints = append(allPoints, seg.Points...)
		if len(allPoints) > maxTrackpoints {
			return nil, fmt.Errorf("GPX file exceeds %d trackpoint limit", maxTrackpoints)
		}
	}

	if len(allPoints) == 0 {
		return nil, fmt.Errorf("no trackpoints found in GPX file")
	}

	sport, subType := mapGPXType(trk.Type)
	source, sourceDevice, recordingDevice := parseGPXCreator(gpx.Creator)

	// E-Bike normalization
	isElectric := false
	if sport == "e_biking" || sport == "e_bike" {
		sport = "cycling"
		isElectric = true
	}

	data := &model.ActivityData{
		FileFormat:      "gpx",
		OriginalName:    filename,
		Source:          source,
		SourceDevice:    sourceDevice,
		SportType:       sport,
		SubType:         subType,
		IsElectric:         isElectric,
		RecordingDevice: recordingDevice,
	}

	// Build streams, track points, and calculate stats
	var streams []model.StreamPoint
	var trackPoints []model.TrackPoint
	var totalDistance float64
	var elevGain, elevLoss float64
	var maxEle, minEle float64
	var maxSpeed float64
	var totalHR float64
	var hrCount int
	var prevLat, prevLon float64
	var prevTime time.Time
	var allElevations []float64 // Collect elevations for filtering
	var allDistances []float64  // Collect cumulative distances for elevation filtering
	minEle = math.MaxFloat64
	maxEle = -math.MaxFloat64

	for i, pt := range allPoints {
		t, err := time.Parse(time.RFC3339, pt.Time)
		if err != nil {
			continue
		}

		sp := model.StreamPoint{
			Time: t,
		}

		if pt.Lat != 0 || pt.Lon != 0 {
			lat := pt.Lat
			lng := pt.Lon
			sp.Lat = &lat
			sp.Lng = &lng
			trackPoints = append(trackPoints, model.TrackPoint{Lat: lat, Lng: lng})
		}

		if pt.Elevation != nil {
			alt := *pt.Elevation
			sp.AltitudeM = &alt
			// Only collect elevation+distance for filter when GPS is present
			if sp.Lat != nil {
				allElevations = append(allElevations, alt)
				allDistances = append(allDistances, totalDistance)
			}
			// min/max independent of GPS
			if alt < minEle {
				minEle = alt
			}
			if alt > maxEle {
				maxEle = alt
			}
		}

		if pt.Extensions.TPExt.HR != nil {
			sp.HeartRate = pt.Extensions.TPExt.HR
			totalHR += float64(*pt.Extensions.TPExt.HR)
			hrCount++
		}
		if pt.Extensions.TPExt.Cad != nil {
			sp.Cadence = pt.Extensions.TPExt.Cad
		}
		if pt.Extensions.TPExt.Temp != nil {
			temp := int16(*pt.Extensions.TPExt.Temp)
			sp.TemperatureC = &temp
		}

		// Distance calculation (haversine)
		if i > 0 && sp.Lat != nil {
			d := activityutil.HaversineMeters(prevLat, prevLon, *sp.Lat, *sp.Lng)
			totalDistance += d

			dist := totalDistance
			sp.DistanceM = &dist

			// Speed — don't derive across gaps > 5min (pauses or session boundaries)
			dt := t.Sub(prevTime).Seconds()
			if dt > 0 && dt <= 300 {
				spd := d / dt
				sp.SpeedMS = &spd
				if spd > maxSpeed {
					maxSpeed = spd
				}
			}
		}

		if sp.Lat != nil {
			prevLat = *sp.Lat
			prevLon = *sp.Lng
		}
		prevTime = t
		streams = append(streams, sp)
	}

	// Parse start/end times
	firstTime, _ := time.Parse(time.RFC3339, allPoints[0].Time)
	lastTime, _ := time.Parse(time.RFC3339, allPoints[len(allPoints)-1].Time)

	data.StartedAt = firstTime
	data.EndedAt = lastTime
	elapsed := int(lastTime.Sub(firstTime).Seconds())
	data.ElapsedTimeS = elapsed

	// For merged GPX files with multi-hour gaps, cap ElapsedTimeS to recording time.
	if len(streams) >= 2 {
		recordingTime := activityutil.CalcRecordingTime(streams)
		if recordingTime > 0 && recordingTime < data.ElapsedTimeS {
			data.ElapsedTimeS = recordingTime
		}
	}

	// GPX files have speed values computed from position deltas above (sp.SpeedMS).
	// Use the stream-based moving time; fall back to elapsed time if no
	// usable data is present.
	streamMoving := activityutil.CalcMovingTime(streams, sport)
	if streamMoving > 0 {
		data.MovingTimeS = streamMoving
	} else {
		data.MovingTimeS = elapsed
	}

	data.DistanceM = totalDistance

	// Apply Savitzky-Golay filter to reduce elevation noise (GPS ±5-15m)
	// Calculate average speed for determining filter window size
	var avgSpeed float64
	if elapsed > 0 {
		avgSpeed = totalDistance / float64(elapsed)
	}
	if len(allElevations) > 0 {
		elevGain, elevLoss = enrichment.FilterElevationGainLoss(allElevations, allDistances, avgSpeed)
	}

	data.ElevationGainM = elevGain
	data.ElevationLossM = elevLoss
	if minEle != math.MaxFloat64 {
		data.MinElevationM = minEle
	}
	if maxEle != -math.MaxFloat64 {
		data.MaxElevationM = maxEle
	}

	if elapsed > 0 {
		data.AvgSpeedMS = totalDistance / float64(elapsed)
	}
	data.MaxSpeedMS = maxSpeed

	if hrCount > 0 {
		data.AvgHeartRate = totalHR / float64(hrCount)
	}

	data.Streams = streams
	data.TrackPoints = trackPoints
	data.HasGPS = len(trackPoints) >= 2
	data.IsIndoor = !data.HasGPS

	if data.HasGPS {
		data.StartLat = trackPoints[0].Lat
		data.StartLng = trackPoints[0].Lng
		last := trackPoints[len(trackPoints)-1]
		data.EndLat = last.Lat
		data.EndLng = last.Lng
	}

	return data, nil
}

func mapGPXType(gpxType string) (sport, subType string) {
	t := strings.ToLower(strings.TrimSpace(gpxType))
	if t == "" {
		return "unknown", ""
	}

	switch t {
	// Strava numeric type IDs
	case "1":
		return "cycling", ""
	case "2":
		return "running", ""
	case "3":
		return "swimming", ""
	case "4":
		return "hiking", ""
	case "5":
		return "walking", ""
	case "6":
		return "e_biking", ""
	case "9":
		return "alpine_skiing", ""
	case "10":
		return "cross_country_skiing", ""
	case "11":
		return "rock_climbing", ""
	case "12":
		return "rowing", ""
	case "13":
		return "stand_up_paddleboarding", ""
	case "14":
		return "kayaking", ""
	case "15":
		return "strength_training", ""
	case "17":
		return "training", "yoga"

	// Running
	case "running", "run":
		return "running", ""
	case "trail_running", "trailrunning", "trailrun", "trail running":
		return "running", "trail"

	// Cycling
	case "cycling", "biking", "bike", "ride", "road_cycling", "roadcycling":
		return "cycling", ""
	case "mountainbikeride", "mountain_biking", "mountainbiking", "mtb":
		return "cycling", "mountain"
	case "ebikeride", "e_bike_ride", "ebike", "e-bike":
		return "e_biking", ""
	case "gravelride", "gravel_ride", "gravel":
		return "cycling", "gravel_cycling"

	// Walking / Hiking
	case "hiking", "hike":
		return "hiking", ""
	case "walking", "walk":
		return "walking", ""
	case "mountaineering":
		return "mountaineering", ""

	// Swimming
	case "swimming", "swim":
		return "swimming", ""
	case "open_water_swimming":
		return "swimming", "open_water"
	case "lap_swimming":
		return "swimming", "lap_swimming"

	// Winter
	case "skiing", "ski", "alpine_skiing":
		return "alpine_skiing", ""
	case "cross_country_skiing", "nordicski":
		return "cross_country_skiing", ""
	case "snowboarding", "snowboard":
		return "snowboarding", ""
	case "snowshoeing":
		return "snowshoeing", ""
	case "ice_skating":
		return "ice_skating", ""

	// Water
	case "rowing", "row":
		return "rowing", ""
	case "kayaking", "canoeing":
		return "kayaking", ""
	case "paddling", "paddle":
		return "paddling", ""
	case "stand_up_paddleboarding", "sup":
		return "stand_up_paddleboarding", ""
	case "surfing", "surf":
		return "surfing", ""
	case "sailing", "sail":
		return "sailing", ""

	// Fitness
	case "strength_training", "strength", "weighttraining":
		return "strength_training", ""
	case "training":
		return "training", ""
	case "yoga":
		return "training", "yoga"
	case "pilates":
		return "training", "pilates"
	case "hiit":
		return "hiit", ""

	// Climb
	case "rock_climbing", "climbing":
		return "rock_climbing", ""
	case "bouldering":
		return "rock_climbing", "bouldering"

	// Game
	case "soccer", "football":
		return "soccer", ""
	case "tennis":
		return "tennis", ""
	case "golf":
		return "golf", ""

	// Other
	case "inline_skating", "inlineskating":
		return "inline_skating", ""
	case "horseback_riding":
		return "horseback_riding", ""
	case "multisport":
		return "multisport", ""

	default:
		return "unknown", ""
	}
}

// parseGPXCreator extracts source app and device from the GPX creator attribute.
func parseGPXCreator(creator string) (source, device string, du *model.DeviceUpsert) {
	c := strings.TrimSpace(creator)
	cl := strings.ToLower(c)

	switch {
	case strings.Contains(cl, "stravagpx") || strings.Contains(cl, "strava"):
		source = "Strava"
	case strings.Contains(cl, "komoot"):
		source = "Komoot"
	case strings.Contains(cl, "osmtracker"):
		source = "OSMTracker"
	case strings.HasPrefix(cl, "garmin"):
		source = "Garmin"
		// Creator is often the device name: "Garmin Instinct 2X"
		device = c
	case strings.Contains(cl, "wahoo"):
		source = "Wahoo"
		device = c
	case strings.Contains(cl, "hammerhead"):
		source = "Hammerhead"
		device = c
	case strings.Contains(cl, "suunto"):
		source = "Suunto"
		device = c
	case strings.Contains(cl, "polar"):
		source = "Polar"
		device = c
	case strings.Contains(cl, "coros"):
		source = "Coros"
		device = c
	case strings.Contains(cl, "bryton"):
		source = "Bryton"
		device = c
	case strings.Contains(cl, "zwift"):
		source = "Zwift"
	case strings.Contains(cl, "endomondo"):
		source = "Endomondo"
	case strings.Contains(cl, "runtastic"):
		source = "Runtastic"
	case strings.Contains(cl, "runalyze"):
		source = "Runalyze"
	case strings.Contains(cl, "trainingpeaks"):
		source = "TrainingPeaks"
	case c != "":
		source = c
	}

	if device != "" {
		du = &model.DeviceUpsert{
			Type:         "recorder",
			Manufacturer: source,
			ProductName:  device,
		}
	}

	return source, device, du
}
