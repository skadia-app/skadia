package handler

import (
	"archive/zip"
	"bytes"
	"context"
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
	"github.com/google/uuid"
	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// GetSportTypeStats returns activity counts grouped by sport_type for the authenticated user.
// GET /api/v1/activities/sport-type-stats
func (h *Handler) GetSportTypeStats(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}
	stats, err := h.DB.GetSportTypeStats(r.Context(), userID)
	if err != nil {
		slog.Error("get sport type stats failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get sport type stats", r.URL.Path)
		return
	}
	if stats == nil {
		stats = []database.SportTypeStatRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sport_types": stats})
}

func (h *Handler) ListActivities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	cursor := r.URL.Query().Get("cursor")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 2000 {
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
	if v := r.URL.Query().Get("include_tags"); v != "" {
		filters.IncludeTags = strings.Split(v, ",")
	}
	if v := r.URL.Query().Get("exclude_tags"); v != "" {
		filters.ExcludeTags = strings.Split(v, ",")
	}
	if v := r.URL.Query().Get("include_gear"); v != "" {
		filters.IncludeGear = strings.Split(v, ",")
	}
	if v := r.URL.Query().Get("exclude_gear"); v != "" {
		filters.ExcludeGear = strings.Split(v, ",")
	}

	list, err := h.DB.ListActivities(r.Context(), userID, cursor, limit, sortBy, sortDir, filters)
	if err != nil {
		slog.Error("list activities failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) GetActivity(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	id := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivityWithWeather(r.Context(), id)
	if err != nil {
		slog.Error("get activity failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, activity)
}

// GetChildActivities returns all child activities for a multisport parent.
// GET /api/v1/activities/{id}/children
func (h *Handler) GetChildActivities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	id := chi.URLParam(r, "id")

	// Verify ownership
	parent, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("get child activities: get parent failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load parent activity", r.URL.Path)
		return
	}
	if parent == nil || parent.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	children, err := h.DB.GetChildActivities(r.Context(), id)
	if err != nil {
		slog.Error("get child activities failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get child activities", r.URL.Path)
		return
	}
	if children == nil {
		children = []model.Activity{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"children": children})
}

func (h *Handler) UpdateActivity(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	id := chi.URLParam(r, "id")

	// Verify ownership before mutating.
	existing, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("update activity: get activity failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if existing == nil || existing.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	var update model.ActivityUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	activity, err := h.DB.UpdateActivity(r.Context(), id, userID, &update)
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
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	id := chi.URLParam(r, "id")

	// Verify ownership before deleting.
	activity, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("delete activity: get activity failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	// Quellen-Handling für Fusion-Ergebnis und Multisport-Parent.
	sourcesMode := r.URL.Query().Get("sources")
	if sourcesMode == "restore" {
		// Alle Kinder/Quellen wieder sichtbar machen.
		if _, err := h.DB.Pool.Exec(r.Context(),
			`UPDATE activities SET parent_id = NULL WHERE parent_id = $1 AND user_id = $2`,
			id, userID,
		); err != nil {
			slog.Warn("delete activity: restore sources failed", "error", err)
		}
	} else if sourcesMode == "delete" {
		// Alle Kinder/Quellen ebenfalls löschen (rekursiv eine Ebene).
		childIDs := []string{}
		rows, _ := h.DB.Pool.Query(r.Context(),
			`SELECT id FROM activities WHERE parent_id = $1 AND user_id = $2`, id, userID)
		if rows != nil {
			for rows.Next() {
				var cid string
				if err := rows.Scan(&cid); err != nil {
					slog.Error("delete activity: scan child id failed", "error", err)
					rows.Close()
					writeError(w, http.StatusInternalServerError, "Internal Error", "failed to scan child activity", r.URL.Path)
					return
				}
				childIDs = append(childIDs, cid)
			}
			if err := rows.Err(); err != nil {
				slog.Error("delete activity: rows iteration error", "error", err)
				rows.Close()
				writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list child activities", r.URL.Path)
				return
			}
			rows.Close()
		}
		for _, cid := range childIDs {
			if err := h.DB.DeleteActivity(r.Context(), userID, cid); err != nil {
				slog.Warn("delete activity: delete child failed", "child_id", cid, "error", err)
			}
		}
	}

	if err := h.DB.DeleteActivity(r.Context(), userID, id); err != nil {
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

// DeleteActivities bulk-deletes multiple activities owned by the authenticated user.
//
// DELETE /api/v1/activities
// Body: {"ids": ["uuid1", "uuid2", ...]}
func (h *Handler) DeleteActivities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "ids array required", r.URL.Path)
		return
	}
	if len(body.IDs) > 500 {
		writeError(w, http.StatusBadRequest, "Bad Request", "max 500 ids per request", r.URL.Path)
		return
	}

	deleted, err := h.DB.DeleteActivities(r.Context(), userID, body.IDs)
	if err != nil {
		slog.Error("bulk delete activities failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete activities", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}

// ExportActivity exports an activity in the requested format.
func (h *Handler) ExportActivity(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	id := chi.URLParam(r, "id")
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "original"
	}

	activity, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("export activity: get activity failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	switch format {
	case "zip":
		if activity.SportType != "multisport" {
			writeError(w, http.StatusBadRequest, "Bad Request", "zip export only available for multisport activities", r.URL.Path)
			return
		}
		exportMultisportZip(h, w, r, activity)
		return
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
		if activity.SportType == "multisport" {
			children, err := h.DB.GetChildActivities(r.Context(), id)
			if err == nil && len(children) > 0 {
				exportMultiSessionFIT(w, activity, children, h.DB, r.Context())
				return
			}
		}
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
	// Use stored file format from DB if available (fix for .gz/.zip extraction from compressed filenames)
	if activity.OriginalFileFormat != nil && *activity.OriginalFileFormat != "" {
		ext = *activity.OriginalFileFormat
	} else if activity.OriginalFilename != nil {
		// Fallback: extract from filename for backward compatibility
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

	// Check if this is a fallback hash (file reading failed during import)
	if strings.HasSuffix(*activity.FileHash, "-missing") {
		writeError(w, http.StatusNotFound, "Not Found", "original file could not be read during import", r.URL.Path)
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
	XmlnsGpxTpx string   `xml:"xmlns:gpxtpx,attr"`
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
	Sport string `xml:"Sport,attr"`
	Id    string `xml:"Id"`
	Lap   tcxLap `xml:"Lap"`
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
	case "cycling":
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

// buildFITFileId builds a FileId message for the given activity.
func buildFITFileId(act *model.Activity) proto.Message {
	fileId := mesgdef.NewFileId(nil)
	fileId.Type = typedef.FileActivity
	fileId.Manufacturer = typedef.ManufacturerDevelopment
	fileId.TimeCreated = act.StartedAt
	return fileId.ToMesg(nil)
}

// buildFITRecord builds a Record message from a StreamPoint.
func buildFITRecord(s model.StreamPoint) proto.Message {
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
	return rec.ToMesg(nil)
}

// buildFITSession builds a Session message from an activity's metadata.
func buildFITSession(act *model.Activity) proto.Message {
	sess := mesgdef.NewSession(nil)
	sess.StartTime = act.StartedAt
	if act.ElapsedTimeS != nil {
		sess.TotalElapsedTime = uint32(*act.ElapsedTimeS * 1000)
		sess.TotalTimerTime = uint32(*act.ElapsedTimeS * 1000)
		if act.MovingTimeS != nil {
			sess.TotalTimerTime = uint32(*act.MovingTimeS * 1000)
		}
	}
	if act.DistanceM != nil {
		sess.TotalDistance = uint32(*act.DistanceM * 100)
	}
	if act.ElevationGainM != nil {
		sess.TotalAscent = uint16(*act.ElevationGainM)
	}
	if act.AvgHeartRate != nil {
		sess.AvgHeartRate = uint8(*act.AvgHeartRate)
	}
	if act.MaxHeartRate != nil {
		sess.MaxHeartRate = uint8(*act.MaxHeartRate)
	}
	if act.AvgPowerW != nil {
		sess.AvgPower = uint16(*act.AvgPowerW)
	}
	if act.MaxPowerW != nil {
		sess.MaxPower = uint16(*act.MaxPowerW)
	}
	if act.NormalizedPowerW != nil {
		sess.NormalizedPower = uint16(*act.NormalizedPowerW)
	}
	if act.AvgCadence != nil {
		sess.AvgCadence = uint8(*act.AvgCadence)
	}
	if act.AvgSpeedMS != nil {
		sess.AvgSpeed = uint16(*act.AvgSpeedMS * 1000)
		sess.EnhancedAvgSpeed = uint32(*act.AvgSpeedMS * 1000)
	}
	switch act.SportType {
	case "cycling":
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
	return sess.ToMesg(nil)
}

// buildFITActivity builds the Activity wrapper message.
func buildFITActivity(act *model.Activity, numSessions int) proto.Message {
	a := mesgdef.NewActivity(nil)
	a.Timestamp = act.StartedAt
	a.NumSessions = uint16(numSessions)
	a.Type = typedef.ActivityManual
	return a.ToMesg(nil)
}

func exportFIT(w http.ResponseWriter, activity *model.Activity, streams []model.StreamPoint) {
	filename := fmt.Sprintf("activity-%s.fit", activity.StartedAt.Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	var messages []proto.Message
	messages = append(messages, buildFITFileId(activity))
	for _, s := range streams {
		messages = append(messages, buildFITRecord(s))
	}
	messages = append(messages, buildFITSession(activity))
	messages = append(messages, buildFITActivity(activity, 1))

	fit := &proto.FIT{Messages: messages}
	enc := encoder.New(w)
	if err := enc.Encode(fit); err != nil {
		slog.Warn("FIT export encode failed", "activity_id", activity.ID, "error", err)
	}
}

// exportMultisportZip packages the original files of all child activities into a single ZIP.
func exportMultisportZip(h *Handler, w http.ResponseWriter, r *http.Request, parent *model.Activity) {
	children, err := h.DB.GetChildActivities(r.Context(), parent.ID)
	if err != nil || len(children) == 0 {
		writeError(w, http.StatusNotFound, "Not Found", "no child activities found", r.URL.Path)
		return
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for i, child := range children {
		if child.FileHash == nil || strings.HasSuffix(*child.FileHash, "-missing") {
			continue
		}
		ext := "fit"
		if child.OriginalFileFormat != nil && *child.OriginalFileFormat != "" {
			ext = *child.OriginalFileFormat
		} else if child.OriginalFilename != nil {
			parts := strings.Split(*child.OriginalFilename, ".")
			if len(parts) > 1 {
				ext = strings.ToLower(parts[len(parts)-1])
			}
		}
		s3Key := fmt.Sprintf("%s/%s.%s", child.UserID, *child.FileHash, ext)
		data, _, err := h.Storage.DownloadFromBucket(r.Context(), "skadia-originals", s3Key)
		if err != nil {
			slog.Warn("multisport zip: child download failed", "child_id", child.ID, "key", s3Key, "error", err)
			continue
		}
		entryName := fmt.Sprintf("%02d_%s_%s.%s", i+1, child.StartedAt.Format("2006-01-02"), child.SportType, ext)
		fw, err := zw.Create(entryName)
		if err != nil {
			slog.Error("multisport zip: create entry failed", "name", entryName, "error", err)
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create zip entry", r.URL.Path)
			return
		}
		if _, err := fw.Write(data); err != nil {
			slog.Error("multisport zip: write entry failed", "name", entryName, "error", err)
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to write zip entry", r.URL.Path)
			return
		}
	}

	if err := zw.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to finalize zip", r.URL.Path)
		return
	}

	filename := fmt.Sprintf("multisport-%s.zip", parent.StartedAt.Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Write(buf.Bytes())
}

// exportMultiSessionFIT writes a multi-session FIT file for a multisport parent activity.
// Each child activity contributes its own block of records followed by a session message,
// producing a valid multi-sport FIT file that consumer fitness tools can parse.
func exportMultiSessionFIT(
	w http.ResponseWriter, parent *model.Activity,
	children []model.Activity, db *database.DB, ctx context.Context,
) {
	var messages []proto.Message
	messages = append(messages, buildFITFileId(parent))

	actualSessions := 0
	for _, child := range children {
		streams, err := db.GetActivityStreams(ctx, child.ID)
		if err != nil {
			slog.Warn("multi-session FIT: child streams not found", "child_id", child.ID, "error", err)
			continue
		}
		for _, s := range streams {
			messages = append(messages, buildFITRecord(s))
		}
		c := child // avoid loop variable capture
		messages = append(messages, buildFITSession(&c))
		actualSessions++
	}

	if actualSessions == 0 {
		// No child streams available — emit a single session from the parent metadata
		actualSessions = 1
		messages = append(messages, buildFITSession(parent))
	}

	messages = append(messages, buildFITActivity(parent, actualSessions))

	buf := new(bytes.Buffer)
	enc := encoder.New(buf)
	if err := enc.Encode(&proto.FIT{Messages: messages}); err != nil {
		slog.Error("multi-session FIT encode failed", "parent_id", parent.ID, "error", err)
		http.Error(w, "export failed", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("%s_multisport.fit", parent.StartedAt.Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Write(buf.Bytes())
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

	// Verify ownership.
	activity, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("get zone distribution: get activity failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
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

// GetCombinedTrack returns a GeoJSON FeatureCollection with one Feature per child activity track.
// GET /api/v1/activities/{id}/combined-track
func (h *Handler) GetCombinedTrack(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	activityID := chi.URLParam(r, "id")
	ctx := r.Context()

	activity, err := h.DB.GetActivity(ctx, activityID)
	if err != nil || activity == nil || activity.UserID != userID || activity.SportType != "multisport" {
		writeError(w, http.StatusNotFound, "Not Found", "multisport activity not found", r.URL.Path)
		return
	}

	children, err := h.DB.GetChildActivities(ctx, activityID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB Error", err.Error(), r.URL.Path)
		return
	}

	type Feature struct {
		Type       string          `json:"type"`
		Geometry   json.RawMessage `json:"geometry"`
		Properties map[string]any  `json:"properties"`
	}

	var features []Feature
	for i, child := range children {
		var geojson json.RawMessage
		err := h.DB.Pool.QueryRow(ctx,
			`SELECT ST_AsGeoJSON(track_full) FROM activity_tracks WHERE activity_id = $1`,
			child.ID).Scan(&geojson)
		if err != nil || len(geojson) == 0 {
			continue
		}
		title := ""
		if child.Title != nil {
			title = *child.Title
		}
		features = append(features, Feature{
			Type:     "Feature",
			Geometry: geojson,
			Properties: map[string]any{
				"activity_id": child.ID,
				"sport_type":  child.SportType,
				"title":       title,
				"child_index": i,
			},
		})
	}

	if features == nil {
		features = []Feature{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"type":     "FeatureCollection",
		"features": features,
	})
}

// GroupActivities creates a multisport parent activity and links the given activities to it.
// POST /api/v1/activities/group
// Body: { "activity_ids": ["uuid1", "uuid2", ...], "title": "Mein Triathlon" }
func (h *Handler) GroupActivities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	ctx := r.Context()

	var req struct {
		ActivityIDs []string `json:"activity_ids"`
		Title       string   `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}
	if len(req.ActivityIDs) < 2 {
		writeError(w, http.StatusBadRequest, "Bad Request", "at least 2 activities required", r.URL.Path)
		return
	}
	if len(req.ActivityIDs) > 20 {
		writeError(w, http.StatusBadRequest, "Bad Request", "max 20 activities", r.URL.Path)
		return
	}

	// Load all activities and verify ownership.
	activities, err := h.DB.GetActivitiesByIDs(ctx, userID, req.ActivityIDs)
	if err != nil || len(activities) != len(req.ActivityIDs) {
		writeError(w, http.StatusBadRequest, "Bad Request", "one or more activities not found", r.URL.Path)
		return
	}

	// Use the earliest started_at as the parent timestamp.
	earliest := activities[0].StartedAt
	for _, a := range activities[1:] {
		if a.StartedAt.Before(earliest) {
			earliest = a.StartedAt
		}
	}

	title := req.Title
	if title == "" {
		title = "Multisport"
	}

	parent := &model.Activity{
		ID:        uuid.New().String(),
		UserID:    userID,
		SportType: "multisport",
		Title:     &title,
		StartedAt: earliest,
	}
	if err := h.DB.InsertMultisportParent(ctx, parent); err != nil {
		writeError(w, http.StatusInternalServerError, "DB Error", err.Error(), r.URL.Path)
		return
	}

	if err := h.DB.SetActivitiesParent(ctx, userID, req.ActivityIDs, parent.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "DB Error", err.Error(), r.URL.Path)
		return
	}

	if err := h.DB.AggregateParentMetrics(ctx, parent.ID); err != nil {
		slog.Warn("GroupActivities: aggregate metrics failed", "parent_id", parent.ID, "error", err)
	}

	writeJSON(w, http.StatusCreated, parent)
}

// UngroupActivities dissolves a multisport group: clears parent_id on all children, then deletes the parent.
// DELETE /api/v1/activities/{id}/group
func (h *Handler) UngroupActivities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	parentID := chi.URLParam(r, "id")
	ctx := r.Context()

	parent, err := h.DB.GetActivity(ctx, parentID)
	if err != nil || parent == nil || parent.UserID != userID || parent.SportType != "multisport" {
		writeError(w, http.StatusNotFound, "Not Found", "multisport activity not found", r.URL.Path)
		return
	}

	// Release children.
	if _, err := h.DB.Pool.Exec(ctx,
		`UPDATE activities SET parent_id = NULL WHERE parent_id = $1 AND user_id = $2`,
		parentID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "DB Error", err.Error(), r.URL.Path)
		return
	}

	// Delete the parent.
	if _, err := h.DB.Pool.Exec(ctx,
		`DELETE FROM activities WHERE id = $1 AND user_id = $2 AND sport_type = 'multisport'`,
		parentID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "DB Error", err.Error(), r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
