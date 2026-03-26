package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ExportActivitiesZip exports multiple activities as a single ZIP archive.
//
// POST /api/v1/activities/export-zip
// Body: {"ids": ["uuid1", ...], "format": "original"|"gpx"|"tcx"|"fit"}
//
// If the request body is absent or ids is empty/omitted, ALL activities for the
// authenticated user are exported. Callers that want a subset must provide ids.
// This "no body = full export" behavior is intentional for a one-click user-data export.
func (h *Handler) ExportActivitiesZip(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var body struct {
		IDs    []string `json:"ids"`
		Format string   `json:"format"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}
	if body.Format == "" {
		body.Format = "original"
	}
	switch body.Format {
	case "original", "gpx", "tcx", "fit":
		// valid
	default:
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "unsupported format: "+body.Format, r.URL.Path)
		return
	}

	// If no IDs given → export all activities for the user.
	ids := body.IDs
	if len(ids) == 0 {
		var err error
		ids, err = h.DB.ListActivityIDsForUser(r.Context(), userID)
		if err != nil {
			slog.Error("export-zip: list all activity ids failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
			return
		}
	}
	if len(ids) > 1000 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "max 1000 activities per ZIP request", r.URL.Path)
		return
	}

	// Build ZIP in memory.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	addToZip := func(activity *model.Activity) {
		fileBytes, filename, err := h.exportActivityBytes(r.Context(), activity, body.Format)
		if err != nil {
			slog.Warn("export-zip: export failed, skipping",
				"id", activity.ID, "format", body.Format, "error", err)
			return
		}
		f, err := zw.Create(filename)
		if err != nil {
			slog.Error("export-zip: zip create entry failed", "error", err)
			return
		}
		if _, err := f.Write(fileBytes); err != nil {
			slog.Error("export-zip: zip write failed", "error", err)
		}
	}

	for _, id := range ids {
		activity, err := h.DB.GetActivity(r.Context(), id)
		if err != nil || activity == nil {
			slog.Warn("export-zip: activity not found, skipping", "id", id)
			continue
		}
		// Ownership check
		if activity.UserID != userID {
			continue
		}

		// Multisport parents have no own file — expand to children instead.
		if activity.SportType == "multisport" && (activity.FileHash == nil || *activity.FileHash == "") {
			children, cerr := h.DB.GetChildActivities(r.Context(), activity.ID)
			if cerr != nil || len(children) == 0 {
				slog.Warn("export-zip: multisport has no children, skipping", "id", id)
				continue
			}
			for i := range children {
				addToZip(&children[i])
			}
			continue
		}

		addToZip(activity)
	}

	if err := zw.Close(); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to finalize ZIP", r.URL.Path)
		return
	}

	dateStr := time.Now().Format("2006-01-02")
	zipFilename := fmt.Sprintf("skadia-export-%s.zip", dateStr)

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, zipFilename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", buf.Len()))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(buf.Bytes()); err != nil { // L67: log write errors
		slog.Warn("export-zip: write response failed", "error", err)
	}
}

// exportActivityBytes returns the file content and filename for a single activity
// in the requested format.
func (h *Handler) exportActivityBytes(ctx context.Context, activity *model.Activity, format string) ([]byte, string, error) {
	date := activity.StartedAt.Format("2006-01-02")

	switch format {
	case "original":
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
		if activity.FileHash == nil && (activity.FileID == nil || *activity.FileID == "") {
			return nil, "", fmt.Errorf("no original file stored")
		}

		// ShadowFS path (preferred): read via file_id when available.
		var data []byte
		if h.ShadowFS != nil && activity.FileID != nil && *activity.FileID != "" {
			rc, getErr := h.ShadowFS.Get(ctx, activity.UserID, *activity.FileID)
			if getErr == nil {
				var readErr error
				data, readErr = io.ReadAll(rc)
				rc.Close()
				if readErr != nil {
					slog.Warn("export: ShadowFS read failed, falling back to S3",
						"activity_id", activity.ID, "file_id", *activity.FileID, "error", readErr)
					data = nil
				}
			} else {
				slog.Warn("export: ShadowFS.Get failed, falling back to S3",
					"activity_id", activity.ID, "file_id", *activity.FileID, "error", getErr)
			}
		}
		// Legacy S3 fallback.
		if data == nil && h.Storage != nil && activity.FileHash != nil {
			s3Key := fmt.Sprintf("%s/%s.%s", activity.UserID, *activity.FileHash, ext)
			var dlErr error
			data, _, dlErr = h.Storage.DownloadFromBucket(ctx, "skadia-originals", s3Key)
			if dlErr != nil {
				return nil, "", fmt.Errorf("download original: %w", dlErr)
			}
		}
		if data == nil {
			return nil, "", fmt.Errorf("no storage available for activity %s", activity.ID)
		}
		filename := fmt.Sprintf("activity-%s-%s.%s", date, activity.ID[:8], ext)
		return data, filename, nil

	case "gpx", "tcx", "fit":
		streams, err := h.DB.GetActivityStreams(ctx, activity.ID)
		if err != nil {
			return nil, "", fmt.Errorf("load streams: %w", err)
		}
		var streamBuf bytes.Buffer
		switch format {
		case "gpx":
			writeGPXBytes(&streamBuf, activity, streams)
			return streamBuf.Bytes(), fmt.Sprintf("activity-%s-%s.gpx", date, activity.ID[:8]), nil
		case "tcx":
			writeTCXBytes(&streamBuf, activity, streams)
			return streamBuf.Bytes(), fmt.Sprintf("activity-%s-%s.tcx", date, activity.ID[:8]), nil
		case "fit":
			writeFITBytes(&streamBuf, activity, streams)
			return streamBuf.Bytes(), fmt.Sprintf("activity-%s-%s.fit", date, activity.ID[:8]), nil
		}
	}
	return nil, "", fmt.Errorf("unsupported format: %s", format)
}

// writeGPXBytes writes GPX data to w. Mirrors exportGPX in activities.go.
func writeGPXBytes(w *bytes.Buffer, activity *model.Activity, streams []model.StreamPoint) {
	// Reuse the same XML structs defined in activities.go (same package).
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
		Trk:         gpxTrack{Name: name},
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
		if s.HeartRate != nil || s.PowerW != nil || s.Cadence != nil || s.TemperatureC != nil {
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
	w.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	enc.Encode(gpx)
}

// writeTCXBytes writes TCX data to w. Mirrors exportTCX in activities.go.
func writeTCXBytes(w *bytes.Buffer, activity *model.Activity, streams []model.StreamPoint) {
	bw := &bytesWriter{buf: w}
	exportTCX(bw, activity, streams)
}

// writeFITBytes writes FIT data to w. Mirrors exportFIT in activities.go.
func writeFITBytes(w *bytes.Buffer, activity *model.Activity, streams []model.StreamPoint) {
	bw := &bytesWriter{buf: w}
	exportFIT(bw, activity, streams)
}

// bytesWriter adapts bytes.Buffer to http.ResponseWriter (only Write is used by exportTCX/exportFIT).
type bytesWriter struct {
	buf     *bytes.Buffer
	headers http.Header
}

func (bw *bytesWriter) Header() http.Header {
	if bw.headers == nil {
		bw.headers = make(http.Header)
	}
	return bw.headers
}
func (bw *bytesWriter) WriteHeader(int)             {}
func (bw *bytesWriter) Write(b []byte) (int, error) { return bw.buf.Write(b) }

// jsonDecodeBody decodes the JSON request body into v.
func jsonDecodeBody(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	return json.NewDecoder(r.Body).Decode(v)
}
