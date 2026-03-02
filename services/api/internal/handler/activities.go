package handler

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) ListActivities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	cursor := r.URL.Query().Get("cursor")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	sortBy := r.URL.Query().Get("sort_by")
	if sortBy == "" {
		sortBy = "started_at"
	}
	sortDir := r.URL.Query().Get("sort_dir")
	if sortDir == "" {
		sortDir = "desc"
	}

	// Parse filter parameters
	var filters model.ActivityFilters
	if st := r.URL.Query().Get("sport_type"); st != "" {
		filters.SportTypes = strings.Split(st, ",")
	}
	if sub := r.URL.Query().Get("sub_type"); sub != "" {
		filters.SubTypes = strings.Split(sub, ",")
	}
	filters.DateFrom = r.URL.Query().Get("date_from")
	filters.DateTo = r.URL.Query().Get("date_to")
	if v := r.URL.Query().Get("is_indoor"); v != "" {
		b := v == "true"
		filters.IsIndoor = &b
	}
	if v := r.URL.Query().Get("has_gps"); v != "" {
		b := v == "true"
		filters.HasGPS = &b
	}
	filters.GearID = r.URL.Query().Get("gear_id")
	filters.Search = r.URL.Query().Get("search")

	list, err := h.DB.ListActivities(r.Context(), userID, cursor, limit, sortBy, sortDir, filters)
	if err != nil {
		slog.Error("list activities failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) GetActivity(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("get activity failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get activity", r.URL.Path)
		return
	}
	if activity == nil {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, activity)
}

func (h *Handler) UpdateActivity(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var update model.ActivityUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	activity, err := h.DB.UpdateActivity(r.Context(), id, &update)
	if err != nil {
		slog.Error("update activity failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update activity", r.URL.Path)
		return
	}
	if activity == nil {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, activity)
}

func (h *Handler) DeleteActivity(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.DB.DeleteActivity(r.Context(), id); err != nil {
		if err.Error() == "activity not found" {
			writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
			return
		}
		slog.Error("delete activity failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete activity", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ExportActivity exports an activity in the requested format.
func (h *Handler) ExportActivity(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "original"
	}

	activity, err := h.DB.GetActivity(r.Context(), id)
	if err != nil || activity == nil {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	switch format {
	case "original":
		exportOriginal(h, w, r, activity)
	case "gpx":
		streams, err := h.DB.GetActivityStreams(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load streams", r.URL.Path)
			return
		}
		exportGPX(w, activity, streams)
	case "tcx":
		streams, err := h.DB.GetActivityStreams(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load streams", r.URL.Path)
			return
		}
		exportTCX(w, activity, streams)
	case "fit":
		streams, err := h.DB.GetActivityStreams(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load streams", r.URL.Path)
			return
		}
		exportFIT(w, activity, streams)
	default:
		writeError(w, http.StatusBadRequest, "Bad Request", "unsupported format: "+format, r.URL.Path)
	}
}

func exportOriginal(h *Handler, w http.ResponseWriter, r *http.Request, activity *model.Activity) {
	ext := "fit"
	if activity.OriginalFilename != nil {
		parts := strings.Split(*activity.OriginalFilename, ".")
		if len(parts) > 1 {
			ext = strings.ToLower(parts[len(parts)-1])
		}
	}
	filename := fmt.Sprintf("activity-%s.%s", activity.StartedAt.Format("2006-01-02"), ext)

	if activity.FileHash == nil {
		writeError(w, http.StatusNotFound, "Not Found", "original file not available", r.URL.Path)
		return
	}

	// S3 key format matches the upload handler: {userID}/{fileHash}.{ext}
	s3Key := fmt.Sprintf("%s/%s.%s", activity.UserID, *activity.FileHash, ext)
	// Original files are stored in the skadia-originals bucket
	data, _, err := h.Storage.DownloadFromBucket(r.Context(), "skadia-originals", s3Key)
	if err != nil {
		slog.Warn("export original download failed", "activity_id", activity.ID, "key", s3Key, "error", err)
		writeError(w, http.StatusNotFound, "Not Found", "original file not available", r.URL.Path)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

// --- GPX Export ---

type gpxRoot struct {
	XMLName     xml.Name `xml:"gpx"`
	Version     string   `xml:"version,attr"`
	Creator     string   `xml:"creator,attr"`
	Xmlns       string   `xml:"xmlns,attr"`
	XmlnsGpxTpx string  `xml:"xmlns:gpxtpx,attr"`
	Metadata    gpxMeta  `xml:"metadata"`
	Trk         gpxTrack `xml:"trk"`
}
type gpxMeta struct {
	Time string `xml:"time"`
}
type gpxTrack struct {
	Name string `xml:"name"`
	Seg  gpxSeg `xml:"trkseg"`
}
type gpxSeg struct {
	Points []gpxPoint `xml:"trkpt"`
}
type gpxPoint struct {
	Lat  string  `xml:"lat,attr"`
	Lon  string  `xml:"lon,attr"`
	Ele  *string `xml:"ele,omitempty"`
	Time string  `xml:"time"`
	Ext  *gpxExt `xml:"extensions,omitempty"`
}
type gpxExt struct {
	TrackPointExt *gpxTpExt `xml:"gpxtpx:TrackPointExtension,omitempty"`
}
type gpxTpExt struct {
	HR       *int16       `xml:"gpxtpx:hr,omitempty"`
	Cad      *int16       `xml:"gpxtpx:cad,omitempty"`
	Atemp    *int16       `xml:"gpxtpx:atemp,omitempty"`
	PowerExt *gpxPowerExt `xml:"gpxtpx:Extensions,omitempty"`
}
type gpxPowerExt struct {
	Power *int16 `xml:"power,omitempty"`
}

func exportGPX(w http.ResponseWriter, activity *model.Activity, streams []model.StreamPoint) {
	name := "Activity"
	if activity.Title != nil && *activity.Title != "" {
		name = *activity.Title
	}

	gpx := gpxRoot{
		Version:     "1.1",
		Creator:     "Skadia",
		Xmlns:       "http://www.topografix.com/GPX/1/1",
		XmlnsGpxTpx: "http://www.garmin.com/xmlschemas/TrackPointExtension/v1",
		Metadata:    gpxMeta{Time: activity.StartedAt.Format(time.RFC3339)},
		Trk: gpxTrack{
			Name: name,
		},
	}

	var points []gpxPoint
	for _, s := range streams {
		if s.Lat == nil || s.Lng == nil {
			continue
		}
		pt := gpxPoint{
			Lat:  strconv.FormatFloat(*s.Lat, 'f', 7, 64),
			Lon:  strconv.FormatFloat(*s.Lng, 'f', 7, 64),
			Time: s.Time.Format(time.RFC3339),
		}
		if s.AltitudeM != nil {
			ele := strconv.FormatFloat(*s.AltitudeM, 'f', 1, 64)
			pt.Ele = &ele
		}
		if s.HeartRate != nil || s.Cadence != nil || s.TemperatureC != nil || s.PowerW != nil {
			tpExt := &gpxTpExt{
				HR:    s.HeartRate,
				Cad:   s.Cadence,
				Atemp: s.TemperatureC,
			}
			if s.PowerW != nil {
				tpExt.PowerExt = &gpxPowerExt{Power: s.PowerW}
			}
			pt.Ext = &gpxExt{TrackPointExt: tpExt}
		}
		points = append(points, pt)
	}
	gpx.Trk.Seg.Points = points

	filename := fmt.Sprintf("activity-%s.gpx", activity.StartedAt.Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/gpx+xml")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>`))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	enc.Encode(gpx)
}

// --- TCX Export ---

type tcxDatabase struct {
	XMLName    xml.Name      `xml:"TrainingCenterDatabase"`
	Xmlns      string        `xml:"xmlns,attr"`
	XmlnsAE2   string        `xml:"xmlns:ae2,attr"`
	Activities tcxActivities `xml:"Activities"`
}
type tcxActivities struct {
	Activity tcxActivity `xml:"Activity"`
}
type tcxActivity struct {
	Sport string      `xml:"Sport,attr"`
	Id    string      `xml:"Id"`
	Lap   tcxLap      `xml:"Lap"`
}
type tcxLap struct {
	StartTime        string     `xml:"StartTime,attr"`
	TotalTimeSeconds float64    `xml:"TotalTimeSeconds,omitempty"`
	DistanceMeters   float64    `xml:"DistanceMeters,omitempty"`
	Track            tcxTrack   `xml:"Track"`
	Extensions       *tcxLapExt `xml:"Extensions,omitempty"`
}
type tcxLapExt struct {
	LX *tcxLX `xml:"ae2:LX,omitempty"`
}
type tcxLX struct {
	AvgWatts *int `xml:"ae2:AvgWatts,omitempty"`
}
type tcxTrack struct {
	Points []tcxTrackpoint `xml:"Trackpoint"`
}
type tcxTrackpoint struct {
	Time           string          `xml:"Time"`
	Position       *tcxPosition    `xml:"Position,omitempty"`
	AltitudeMeters *float64        `xml:"AltitudeMeters,omitempty"`
	DistanceMeters *float64        `xml:"DistanceMeters,omitempty"`
	HeartRateBpm   *tcxHeartRate   `xml:"HeartRateBpm,omitempty"`
	Cadence        *int16          `xml:"Cadence,omitempty"`
	Extensions     *tcxTpExtension `xml:"Extensions,omitempty"`
}
type tcxPosition struct {
	LatitudeDegrees  float64 `xml:"LatitudeDegrees"`
	LongitudeDegrees float64 `xml:"LongitudeDegrees"`
}
type tcxHeartRate struct {
	Value int16 `xml:"Value"`
}
type tcxTpExtension struct {
	TPX *tcxTPX `xml:"ae2:TPX,omitempty"`
}
type tcxTPX struct {
	Watts *int16   `xml:"ae2:Watts,omitempty"`
	Speed *float64 `xml:"ae2:Speed,omitempty"`
}

func tcxSportFromActivity(activity *model.Activity) string {
	switch activity.SportType {
	case "cycling", "e_biking":
		return "Biking"
	case "running":
		return "Running"
	default:
		return "Other"
	}
}

func exportTCX(w http.ResponseWriter, activity *model.Activity, streams []model.StreamPoint) {
	var points []tcxTrackpoint
	for _, s := range streams {
		tp := tcxTrackpoint{
			Time: s.Time.Format(time.RFC3339),
		}
		if s.Lat != nil && s.Lng != nil {
			tp.Position = &tcxPosition{LatitudeDegrees: *s.Lat, LongitudeDegrees: *s.Lng}
		}
		if s.AltitudeM != nil {
			tp.AltitudeMeters = s.AltitudeM
		}
		if s.DistanceM != nil {
			tp.DistanceMeters = s.DistanceM
		}
		if s.HeartRate != nil {
			tp.HeartRateBpm = &tcxHeartRate{Value: *s.HeartRate}
		}
		if s.Cadence != nil {
			tp.Cadence = s.Cadence
		}
		if s.PowerW != nil || s.SpeedMS != nil {
			tpx := &tcxTPX{}
			if s.PowerW != nil {
				tpx.Watts = s.PowerW
			}
			if s.SpeedMS != nil {
				tpx.Speed = s.SpeedMS
			}
			tp.Extensions = &tcxTpExtension{TPX: tpx}
		}
		points = append(points, tp)
	}

	lap := tcxLap{
		StartTime: activity.StartedAt.Format(time.RFC3339),
		Track:     tcxTrack{Points: points},
	}
	if activity.ElapsedTimeS != nil {
		lap.TotalTimeSeconds = float64(*activity.ElapsedTimeS)
	}
	if activity.DistanceM != nil {
		lap.DistanceMeters = *activity.DistanceM
	}
	if activity.AvgPowerW != nil {
		avgPwr := int(*activity.AvgPowerW)
		lap.Extensions = &tcxLapExt{LX: &tcxLX{AvgWatts: &avgPwr}}
	}

	db := tcxDatabase{
		Xmlns:    "http://www.garmin.com/xmlschemas/TrainingCenterDatabase/v2",
		XmlnsAE2: "http://www.garmin.com/xmlschemas/ActivityExtension/v2",
		Activities: tcxActivities{
			Activity: tcxActivity{
				Sport: tcxSportFromActivity(activity),
				Id:    activity.StartedAt.Format(time.RFC3339),
				Lap:   lap,
			},
		},
	}

	filename := fmt.Sprintf("activity-%s.tcx", activity.StartedAt.Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/vnd.garmin.tcx+xml")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>`))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	enc.Encode(db)
}

// --- FIT Export ---

func degToSemicircles(deg float64) int32 {
	return int32(deg * (math.Pow(2, 31) / 180.0))
}

func exportFIT(w http.ResponseWriter, activity *model.Activity, streams []model.StreamPoint) {
	filename := fmt.Sprintf("activity-%s.fit", activity.StartedAt.Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	var messages []proto.Message

	// FileId message
	fileId := mesgdef.NewFileId(nil)
	fileId.Type = typedef.FileActivity
	fileId.Manufacturer = typedef.ManufacturerDevelopment
	fileId.TimeCreated = activity.StartedAt
	messages = append(messages, fileId.ToMesg(nil))

	// Record messages from streams
	for _, s := range streams {
		rec := mesgdef.NewRecord(nil)
		rec.Timestamp = s.Time
		if s.Lat != nil && s.Lng != nil {
			rec.PositionLat = degToSemicircles(*s.Lat)
			rec.PositionLong = degToSemicircles(*s.Lng)
		}
		if s.AltitudeM != nil {
			rec.EnhancedAltitude = uint32((*s.AltitudeM + 500) * 5)
			rec.Altitude = uint16((*s.AltitudeM + 500) * 5)
		}
		if s.HeartRate != nil {
			rec.HeartRate = uint8(*s.HeartRate)
		}
		if s.PowerW != nil {
			rec.Power = uint16(*s.PowerW)
		}
		if s.Cadence != nil {
			rec.Cadence = uint8(*s.Cadence)
		}
		if s.SpeedMS != nil {
			rec.EnhancedSpeed = uint32(*s.SpeedMS * 1000)
			rec.Speed = uint16(*s.SpeedMS * 1000)
		}
		if s.DistanceM != nil {
			rec.Distance = uint32(*s.DistanceM * 100)
		}
		if s.TemperatureC != nil {
			rec.Temperature = int8(*s.TemperatureC)
		}
		messages = append(messages, rec.ToMesg(nil))
	}

	// Session message
	sess := mesgdef.NewSession(nil)
	sess.StartTime = activity.StartedAt
	if activity.ElapsedTimeS != nil {
		sess.TotalElapsedTime = uint32(*activity.ElapsedTimeS * 1000)
		sess.TotalTimerTime = uint32(*activity.ElapsedTimeS * 1000)
		if activity.MovingTimeS != nil {
			sess.TotalTimerTime = uint32(*activity.MovingTimeS * 1000)
		}
	}
	if activity.DistanceM != nil {
		sess.TotalDistance = uint32(*activity.DistanceM * 100)
	}
	if activity.ElevationGainM != nil {
		sess.TotalAscent = uint16(*activity.ElevationGainM)
	}
	if activity.AvgHeartRate != nil {
		sess.AvgHeartRate = uint8(*activity.AvgHeartRate)
	}
	if activity.MaxHeartRate != nil {
		sess.MaxHeartRate = uint8(*activity.MaxHeartRate)
	}
	if activity.AvgPowerW != nil {
		sess.AvgPower = uint16(*activity.AvgPowerW)
	}
	if activity.MaxPowerW != nil {
		sess.MaxPower = uint16(*activity.MaxPowerW)
	}
	if activity.NormalizedPowerW != nil {
		sess.NormalizedPower = uint16(*activity.NormalizedPowerW)
	}
	if activity.AvgCadence != nil {
		sess.AvgCadence = uint8(*activity.AvgCadence)
	}
	if activity.AvgSpeedMS != nil {
		sess.AvgSpeed = uint16(*activity.AvgSpeedMS * 1000)
		sess.EnhancedAvgSpeed = uint32(*activity.AvgSpeedMS * 1000)
	}
	switch activity.SportType {
	case "cycling", "e_biking":
		sess.Sport = typedef.SportCycling
	case "running":
		sess.Sport = typedef.SportRunning
	case "hiking":
		sess.Sport = typedef.SportHiking
	case "swimming":
		sess.Sport = typedef.SportSwimming
	case "skiing":
		sess.Sport = typedef.SportAlpineSkiing
	default:
		sess.Sport = typedef.SportGeneric
	}
	messages = append(messages, sess.ToMesg(nil))

	// Activity message
	act := mesgdef.NewActivity(nil)
	act.Timestamp = activity.StartedAt
	act.NumSessions = 1
	act.Type = typedef.ActivityManual
	messages = append(messages, act.ToMesg(nil))

	fit := &proto.FIT{Messages: messages}
	enc := encoder.New(w)
	if err := enc.Encode(fit); err != nil {
		slog.Warn("FIT export encode failed", "activity_id", activity.ID, "error", err)
	}
}

// --- Zone Distribution ---

// GetZoneDistribution returns the HR and power zone distribution for an activity.
func (h *Handler) GetZoneDistribution(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	// Try pre-computed zones first
	zones, err := h.DB.GetTimeInZones(r.Context(), id)
	if err != nil {
		slog.Warn("get time in zones failed", "activity_id", id, "error", err)
	}

	var hrZones, powerZones []model.TimeInZone
	hasFitZones := false
	for _, z := range zones {
		if z.ZoneType == "hr" {
			hrZones = append(hrZones, z)
		} else if z.ZoneType == "power" {
			powerZones = append(powerZones, z)
		}
		hasFitZones = true
	}

	// Fallback: compute HR zones from streams
	if len(hrZones) == 0 {
		profile, _ := h.DB.GetUserProfile(r.Context(), userID)
		if profile != nil {
			streams, err := h.DB.GetActivityStreams(r.Context(), id)
			if err == nil && len(streams) > 0 {
				hrZones = computeHRZones(streams, profile.EffectiveMaxHR())
			}
		}
	}

	resp := model.ZoneDistributionResponse{
		HRZones:     hrZones,
		PowerZones:  powerZones,
		HasFitZones: hasFitZones,
	}
	if resp.HRZones == nil {
		resp.HRZones = []model.TimeInZone{}
	}
	if resp.PowerZones == nil {
		resp.PowerZones = []model.TimeInZone{}
	}
	writeJSON(w, http.StatusOK, resp)
}

// computeHRZones calculates 5-zone HR distribution from stream data.
func computeHRZones(streams []model.StreamPoint, maxHR int) []model.TimeInZone {
	if maxHR <= 0 {
		return nil
	}
	zones := [5]int{} // seconds per zone
	boundaries := [5]float64{0.60, 0.70, 0.80, 0.90, 1.01}
	for _, s := range streams {
		if s.HeartRate == nil || *s.HeartRate <= 0 {
			continue
		}
		ratio := float64(*s.HeartRate) / float64(maxHR)
		for i, b := range boundaries {
			if ratio < b {
				zones[i]++
				break
			}
		}
	}
	var result []model.TimeInZone
	for i, secs := range zones {
		if secs > 0 {
			high := float64(maxHR) * boundaries[i]
			result = append(result, model.TimeInZone{
				ZoneType:  "hr",
				ZoneIndex: i,
				ZoneHigh:  &high,
				Seconds:   secs,
			})
		}
	}
	return result
}
