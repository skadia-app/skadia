package tcximport

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

// TCX XML structures

type tcxDatabase struct {
	XMLName    xml.Name      `xml:"TrainingCenterDatabase"`
	Activities tcxActivities `xml:"Activities"`
}

type tcxActivities struct {
	Activity []tcxActivity `xml:"Activity"`
}

type tcxActivity struct {
	Sport   string      `xml:"Sport,attr"`
	ID      string      `xml:"Id"`
	Creator tcxCreator  `xml:"Creator"`
	Laps    []tcxLap    `xml:"Lap"`
}

type tcxCreator struct {
	Name      string `xml:"Name"`
	ProductID int    `xml:"ProductID"`
}

type tcxLap struct {
	StartTime        string     `xml:"StartTime,attr"`
	TotalTimeSeconds float64    `xml:"TotalTimeSeconds"`
	DistanceMeters   float64    `xml:"DistanceMeters"`
	Calories         int        `xml:"Calories"`
	AvgHR            *tcxHRVal  `xml:"AverageHeartRateBpm"`
	MaxHR            *tcxHRVal  `xml:"MaximumHeartRateBpm"`
	AvgCadence       *int16     `xml:"Cadence"`
	Track            []tcxTrack `xml:"Track"`
}

type tcxTrack struct {
	Points []tcxTrackpoint `xml:"Trackpoint"`
}

type tcxHRVal struct {
	Value int16 `xml:"Value"`
}

type tcxTrackpoint struct {
	Time      string       `xml:"Time"`
	Position  *tcxPosition `xml:"Position"`
	Altitude  *float64     `xml:"AltitudeMeters"`
	Distance  *float64     `xml:"DistanceMeters"`
	HR        *tcxHRVal    `xml:"HeartRateBpm"`
	Cadence   *int16       `xml:"Cadence"`
	Extension tcxTPExt     `xml:"Extensions"`
}

type tcxPosition struct {
	Lat float64 `xml:"LatitudeDegrees"`
	Lon float64 `xml:"LongitudeDegrees"`
}

type tcxTPExt struct {
	TPX tcxTPX `xml:"TPX"`
}

type tcxTPX struct {
	Watts *int16   `xml:"Watts"`
	Speed *float64 `xml:"Speed"`
}

func ParseTCX(r io.Reader, filename string) (*model.ActivityData, error) {
	var tcx tcxDatabase
	if err := xml.NewDecoder(r).Decode(&tcx); err != nil {
		return nil, fmt.Errorf("decode TCX: %w", err)
	}

	if len(tcx.Activities.Activity) == 0 {
		return nil, fmt.Errorf("no activities found in TCX file")
	}

	act := tcx.Activities.Activity[0]

	sport, subType := mapTCXSport(act.Sport)
	source, sourceDevice := parseTCXCreator(act.Creator)

	// E-Bike normalization
	isEbike := false
	if sport == "e_biking" || sport == "e_bike" {
		sport = "cycling"
		isEbike = true
	}

	data := &model.ActivityData{
		FileFormat:   "tcx",
		OriginalName: filename,
		Source:       source,
		SourceDevice: sourceDevice,
		SportType:    sport,
		SubType:      subType,
		IsEbike:      isEbike,
	}

	// Aggregate lap data
	var totalTime float64
	var totalDistance float64
	var totalHRWeighted float64
	var totalHRTime float64
	var maxHR int

	var streams []model.StreamPoint
	var trackPoints []model.TrackPoint
	var elevGain, elevLoss float64
	var maxEle, minEle float64
	var maxSpeed float64
	var allElevations []float64 // Collect elevations for filtering
	minEle = math.MaxFloat64
	maxEle = -math.MaxFloat64

	for _, lap := range act.Laps {
		totalTime += lap.TotalTimeSeconds
		totalDistance += lap.DistanceMeters

		if lap.AvgHR != nil && lap.TotalTimeSeconds > 0 {
			totalHRWeighted += float64(lap.AvgHR.Value) * lap.TotalTimeSeconds
			totalHRTime += lap.TotalTimeSeconds
		}
		if lap.MaxHR != nil && int(lap.MaxHR.Value) > maxHR {
			maxHR = int(lap.MaxHR.Value)
		}

		for _, track := range lap.Track {
			for _, pt := range track.Points {
				t, err := time.Parse(time.RFC3339, pt.Time)
				if err != nil {
					continue
				}

				sp := model.StreamPoint{Time: t}

				if pt.Position != nil {
					lat := pt.Position.Lat
					lng := pt.Position.Lon
					sp.Lat = &lat
					sp.Lng = &lng
					trackPoints = append(trackPoints, model.TrackPoint{Lat: lat, Lng: lng})
				}

				if pt.Altitude != nil {
					alt := *pt.Altitude
					sp.AltitudeM = &alt
					allElevations = append(allElevations, alt)
					if alt < minEle {
						minEle = alt
					}
					if alt > maxEle {
						maxEle = alt
					}
				}

				if pt.Distance != nil {
					sp.DistanceM = pt.Distance
				}

				if pt.HR != nil {
					hr := pt.HR.Value
					sp.HeartRate = &hr
				}

				if pt.Cadence != nil {
					sp.Cadence = pt.Cadence
				}

				if pt.Extension.TPX.Watts != nil {
					sp.PowerW = pt.Extension.TPX.Watts
				}

				if pt.Extension.TPX.Speed != nil {
					sp.SpeedMS = pt.Extension.TPX.Speed
					if *pt.Extension.TPX.Speed > maxSpeed {
						maxSpeed = *pt.Extension.TPX.Speed
					}
				}

				streams = append(streams, sp)
			}
		}
	}

	// Time
	startTime, _ := time.Parse(time.RFC3339, act.ID)
	if startTime.IsZero() && len(streams) > 0 {
		startTime = streams[0].Time
	}
	data.StartedAt = startTime
	data.ElapsedTimeS = int(totalTime)
	if len(streams) > 0 {
		data.EndedAt = streams[len(streams)-1].Time
	}

	// TCX laps provide TotalTimeSeconds which already excludes pauses recorded
	// by the device. Use stream-based calculation when speed or GPS data is
	// available to additionally filter stop-and-go situations the device did
	// not pause for. Take the minimum to stay conservative.
	deviceMoving := int(totalTime)
	if activityutil.HasSpeedData(streams) || activityutil.HasGPSData(streams) {
		streamMoving := activityutil.CalcMovingTime(streams, sport)
		if streamMoving > 0 {
			data.MovingTimeS = activityutil.MinMovingTime(deviceMoving, streamMoving)
		} else {
			data.MovingTimeS = deviceMoving
		}
	} else {
		data.MovingTimeS = deviceMoving
	}

	// Distance / Elevation
	data.DistanceM = totalDistance

	// Apply Savitzky-Goyal filter to reduce elevation noise (GPS ±5-15m)
	// Calculate average speed for determining filter window size
	var avgSpeed float64
	if totalTime > 0 {
		avgSpeed = totalDistance / totalTime
	}
	if len(allElevations) > 0 {
		elevGain, elevLoss = enrichment.FilterElevationGainLoss(allElevations, avgSpeed)
	}

	data.ElevationGainM = elevGain
	data.ElevationLossM = elevLoss
	if minEle != math.MaxFloat64 {
		data.MinElevationM = minEle
	}
	if maxEle != -math.MaxFloat64 {
		data.MaxElevationM = maxEle
	}

	// Speed
	if totalTime > 0 {
		data.AvgSpeedMS = totalDistance / totalTime
	}
	data.MaxSpeedMS = maxSpeed

	// Heart rate
	if totalHRTime > 0 {
		data.AvgHeartRate = totalHRWeighted / totalHRTime
	}
	data.MaxHeartRate = maxHR

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

func mapTCXSport(sport string) (sportType, subType string) {
	t := strings.ToLower(strings.TrimSpace(sport))
	if t == "" {
		return "unknown", ""
	}

	switch t {
	// Running
	case "running", "run":
		return "running", ""
	case "trail_running", "trailrunning":
		return "running", "trail"

	// Cycling (TCX standard: "Biking")
	case "biking", "cycling", "ride":
		return "cycling", ""
	case "mountainbikeride", "mountain_biking":
		return "cycling", "mountain"
	case "ebikeride", "e_bike_ride", "ebike":
		return "e_biking", ""
	case "gravelride", "gravel_ride":
		return "cycling", "gravel_cycling"

	// Walking / Hiking
	case "hiking", "hike":
		return "hiking", ""
	case "walking", "walk":
		return "walking", ""

	// Swimming
	case "swimming", "swim":
		return "swimming", ""

	// Winter
	case "skiing", "alpine_skiing":
		return "alpine_skiing", ""
	case "cross_country_skiing":
		return "cross_country_skiing", ""
	case "snowboarding":
		return "snowboarding", ""

	// Other
	case "strength_training", "strength":
		return "strength_training", ""
	case "rowing":
		return "rowing", ""
	case "kayaking":
		return "kayaking", ""
	case "multisport":
		return "multisport", ""
	case "other":
		return "unknown", ""

	default:
		return "unknown", ""
	}
}

// parseTCXCreator extracts source app and device from TCX Creator element.
func parseTCXCreator(creator tcxCreator) (source, device string) {
	name := strings.TrimSpace(creator.Name)
	if name == "" {
		return "", ""
	}

	nl := strings.ToLower(name)
	switch {
	case strings.Contains(nl, "strava"):
		source = "Strava"
	case strings.HasPrefix(nl, "garmin"):
		source = "Garmin"
		device = name
	case strings.Contains(nl, "wahoo"):
		source = "Wahoo"
		device = name
	case strings.Contains(nl, "hammerhead"):
		source = "Hammerhead"
		device = name
	case strings.Contains(nl, "suunto"):
		source = "Suunto"
		device = name
	case strings.Contains(nl, "polar"):
		source = "Polar"
		device = name
	case strings.Contains(nl, "coros"):
		source = "Coros"
		device = name
	case strings.Contains(nl, "zwift"):
		source = "Zwift"
	case strings.Contains(nl, "trainingpeaks"):
		source = "TrainingPeaks"
	default:
		source = name
	}

	return source, device
}
