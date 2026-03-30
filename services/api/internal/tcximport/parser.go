package tcximport

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/activityutil"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// xmlFlexInt unmarshals XML text as int16, tolerating float notation ("711.0", "143.56").
type xmlFlexInt int16

func (f *xmlFlexInt) UnmarshalText(text []byte) error {
	s := string(text)
	if v, err := strconv.ParseInt(s, 10, 16); err == nil {
		*f = xmlFlexInt(v)
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*f = xmlFlexInt(math.Round(v))
	return nil
}

// xmlFlexIntLarge is like xmlFlexInt but for int (Calories can exceed int16 range).
type xmlFlexIntLarge int

func (f *xmlFlexIntLarge) UnmarshalText(text []byte) error {
	s := string(text)
	if v, err := strconv.ParseInt(s, 10, 32); err == nil {
		*f = xmlFlexIntLarge(v)
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*f = xmlFlexIntLarge(math.Round(v))
	return nil
}

// TCX XML structures

type tcxDatabase struct {
	XMLName    xml.Name      `xml:"TrainingCenterDatabase"`
	Activities tcxActivities `xml:"Activities"`
}

type tcxActivities struct {
	Activity []tcxActivity `xml:"Activity"`
}

type tcxActivity struct {
	Sport   string     `xml:"Sport,attr"`
	ID      string     `xml:"Id"`
	Creator tcxCreator `xml:"Creator"`
	Laps    []tcxLap   `xml:"Lap"`
}

type tcxCreator struct {
	Name      string `xml:"Name"`
	ProductID int    `xml:"ProductID"`
}

type tcxLap struct {
	StartTime        string           `xml:"StartTime,attr"`
	TotalTimeSeconds float64          `xml:"TotalTimeSeconds"`
	DistanceMeters   float64          `xml:"DistanceMeters"`
	Calories         xmlFlexIntLarge  `xml:"Calories"`
	AvgHR            *tcxHRVal        `xml:"AverageHeartRateBpm"`
	MaxHR            *tcxHRVal        `xml:"MaximumHeartRateBpm"`
	AvgCadence       *xmlFlexInt      `xml:"Cadence"`
	Track            []tcxTrack       `xml:"Track"`
}

type tcxTrack struct {
	Points []tcxTrackpoint `xml:"Trackpoint"`
}

type tcxHRVal struct {
	Value xmlFlexInt `xml:"Value"`
}

type tcxTrackpoint struct {
	Time      string       `xml:"Time"`
	Position  *tcxPosition `xml:"Position"`
	Altitude  *float64     `xml:"AltitudeMeters"`
	Distance  *float64     `xml:"DistanceMeters"`
	HR        *tcxHRVal    `xml:"HeartRateBpm"`
	Cadence   *xmlFlexInt  `xml:"Cadence"`
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
	Watts *xmlFlexInt `xml:"Watts"`
	Speed *float64    `xml:"Speed"`
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
	source, sourceDevice, recordingDevice := parseTCXCreator(act.Creator)

	// E-Bike normalization
	isElectric := false
	if sport == "e_biking" || sport == "e_bike" {
		sport = "cycling"
		isElectric = true
	}

	data := &model.ActivityData{
		FileFormat:      "tcx",
		OriginalName:    filename,
		Source:          source,
		SourceDevice:    sourceDevice,
		SportType:       sport,
		SubType:         subType,
		IsElectric:         isElectric,
		RecordingDevice: recordingDevice,
	}

	// Aggregate lap data
	var totalTime float64
	var totalDistance float64
	var totalHRWeighted float64
	var totalHRTime float64
	var maxHR int

	// L35: Max 10M trackpoints to prevent OOM from malformed/malicious files.
	const maxTrackpoints = 10_000_000
	var totalPoints int

	var streams []model.StreamPoint
	var trackPoints []model.TrackPoint
	var elevGain, elevLoss float64
	var maxEle, minEle float64
	var maxSpeed float64
	var allElevations []float64 // Collect elevations for filtering
	var allDistances []float64  // Collect cumulative distances for elevation filtering
	minEle = math.MaxFloat64
	maxEle = -math.MaxFloat64

	for _, lap := range act.Laps {
		totalTime += lap.TotalTimeSeconds
		totalDistance += lap.DistanceMeters

		// Count trackpoints for limit check.
		for _, track := range lap.Track {
			totalPoints += len(track.Points)
		}
		if totalPoints > maxTrackpoints {
			return nil, fmt.Errorf("TCX file exceeds %d trackpoint limit", maxTrackpoints)
		}

		if lap.AvgHR != nil && lap.TotalTimeSeconds > 0 {
			totalHRWeighted += float64(lap.AvgHR.Value) * lap.TotalTimeSeconds
			totalHRTime += lap.TotalTimeSeconds
		}
		if lap.MaxHR != nil && int(lap.MaxHR.Value) > maxHR {
			maxHR = int(lap.MaxHR.Value)
		}

		// Build Lap entry
		lapStart, _ := time.Parse(time.RFC3339, lap.StartTime)
		l := model.LapRecord{
			ID:                uuid.New().String(),
			StartTime:         lapStart,
			TotalElapsedTimeS: lap.TotalTimeSeconds,
			TotalDistanceM:    lap.DistanceMeters,
			TotalCalories:     int(lap.Calories),
		}
		if lap.AvgHR != nil {
			l.AvgHeartRate = int(lap.AvgHR.Value)
		}
		if lap.MaxHR != nil {
			l.MaxHeartRate = int(lap.MaxHR.Value)
		}
		if lap.AvgCadence != nil {
			l.AvgCadence = int(*lap.AvgCadence)
		}
		data.Laps = append(data.Laps, l)

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
					// Only collect elevation+distance for filter when distance is present
					if pt.Distance != nil {
						allElevations = append(allElevations, alt)
						allDistances = append(allDistances, *pt.Distance)
					}
					// min/max independent of distance availability
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
					hr := int16(pt.HR.Value)
					sp.HeartRate = &hr
				}

				if pt.Cadence != nil {
					v := int16(*pt.Cadence)
					sp.Cadence = &v
				}

				if pt.Extension.TPX.Watts != nil {
					v := int16(*pt.Extension.TPX.Watts)
					sp.PowerW = &v
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

	// For merged TCX files with multi-hour gaps, cap ElapsedTimeS to recording time.
	if len(streams) >= 2 {
		recordingTime := activityutil.CalcRecordingTime(streams)
		if recordingTime > 0 && recordingTime < data.ElapsedTimeS {
			data.ElapsedTimeS = recordingTime
		}
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

	streams = activityutil.FillMissingDistanceAndSpeed(activityutil.FixDistanceResets(streams))
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
func parseTCXCreator(creator tcxCreator) (source, device string, du *model.DeviceUpsert) {
	name := strings.TrimSpace(creator.Name)
	if name == "" {
		return "", "", nil
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

	if device != "" {
		du = &model.DeviceUpsert{
			Type:         "recorder",
			Manufacturer: source,
			ProductName:  device,
		}
	}

	return source, device, du
}
