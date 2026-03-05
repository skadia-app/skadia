package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/draw"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

const maxPhotoSize = 20 << 20 // 20 MB

func (h *Handler) UploadActivityPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership before accepting upload.
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("upload photo: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoSize)
	if err := r.ParseMultipartForm(maxPhotoSize); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "file too large (max 20 MB)", r.URL.Path)
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "no photo in request", r.URL.Path)
		return
	}
	defer file.Close()

	// Content-Type pruefen
	ct := header.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/jpeg"
	}
	if !strings.HasPrefix(ct, "image/") {
		writeError(w, http.StatusBadRequest, "Bad Request", "only images allowed", r.URL.Path)
		return
	}

	// Datei vollstaendig lesen
	data, err := io.ReadAll(file)
	if err != nil {
		slog.Error("read photo data failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to read file", r.URL.Path)
		return
	}

	// Hash
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	// EXIF-Daten extrahieren (GPS + Zeitstempel)
	takenAt, lat, lng, locationSource := extractExifData(data)

	// Falls kein EXIF-GPS: Track-Interpolation versuchen
	if lat == nil && takenAt != nil && activityID != "" {
		lat, lng = h.interpolateFromTrack(r.Context(), activityID, userID, *takenAt)
		if lat != nil {
			ls := "activity_track"
			locationSource = &ls
		}
	}

	// Thumbnail generieren
	thumbData, err := generateThumbnail(data, 200, 200)
	if err != nil {
		// Thumbnail-Fehler nicht fatal
		thumbData = nil
	}

	// S3-Pfade
	ext := ".jpg"
	if strings.Contains(ct, "png") {
		ext = ".png"
	}
	s3Key := fmt.Sprintf("activity-photos/%s/%s%s", userID, hash, ext)
	s3KeyThumb := fmt.Sprintf("activity-photos/%s/%s_thumb.jpg", userID, hash)
	bucket := "skadia-media"

	// Upload Original
	if err := h.Storage.UploadToBucket(r.Context(), bucket, s3Key, data, ct); err != nil {
		slog.Error("s3 upload photo failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to store photo", r.URL.Path)
		return
	}

	// Upload Thumbnail
	if thumbData != nil {
		_ = h.Storage.UploadToBucket(r.Context(), bucket, s3KeyThumb, thumbData, "image/jpeg")
	} else {
		s3KeyThumb = s3Key // Fallback: Original als Thumbnail
	}

	photo := model.ActivityPhoto{
		UserID:           userID,
		ActivityID:       &activityID,
		S3Bucket:         bucket,
		S3Key:            s3Key,
		S3KeyThumb:       s3KeyThumb,
		OriginalFilename: header.Filename,
		ContentType:      ct,
		FileSizeBytes:    int64(len(data)),
		FileHash:         &hash,
		TakenAt:          takenAt,
		Lat:              lat,
		Lng:              lng,
		LocationSource:   locationSource,
	}

	saved, err := h.DB.InsertActivityPhoto(r.Context(), photo)
	if err != nil {
		slog.Error("insert activity photo failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to save photo", r.URL.Path)
		return
	}

	saved.ThumbURL = "/api/v1/photos/" + saved.ID + "/thumb"
	saved.FullURL = "/api/v1/photos/" + saved.ID + "/full"
	writeJSON(w, http.StatusCreated, saved)
}

func (h *Handler) ListActivityPhotos(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership before listing photos.
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("list activity photos: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	photos, err := h.DB.ListActivityPhotos(r.Context(), activityID, userID)
	if err != nil {
		slog.Error("list activity photos failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list photos", r.URL.Path)
		return
	}
	for i := range photos {
		photos[i].ThumbURL = "/api/v1/photos/" + photos[i].ID + "/thumb"
		photos[i].FullURL = "/api/v1/photos/" + photos[i].ID + "/full"
	}
	writeJSON(w, http.StatusOK, photos)
}

func (h *Handler) DeleteActivityPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	photoID := chi.URLParam(r, "photoID")
	bucket, key, keyThumb, err := h.DB.DeleteActivityPhoto(r.Context(), photoID, userID)
	if err != nil {
		slog.Error("delete activity photo failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete photo", r.URL.Path)
		return
	}
	_ = h.Storage.DeleteFromBucket(r.Context(), bucket, key)
	if keyThumb != key {
		_ = h.Storage.DeleteFromBucket(r.Context(), bucket, keyThumb)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) StreamActivityPhotoThumb(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	photoID := chi.URLParam(r, "photoID")
	photo, err := h.DB.GetActivityPhoto(r.Context(), photoID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not Found", "photo not found", r.URL.Path)
		return
	}
	data, _, err := h.Storage.DownloadFromBucket(r.Context(), photo.S3Bucket, photo.S3KeyThumb)
	if err != nil {
		slog.Error("s3 download thumb failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load thumbnail", r.URL.Path)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(data)
}

func (h *Handler) StreamActivityPhotoFull(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	photoID := chi.URLParam(r, "photoID")
	photo, err := h.DB.GetActivityPhoto(r.Context(), photoID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not Found", "photo not found", r.URL.Path)
		return
	}
	data, ct, err := h.Storage.DownloadFromBucket(r.Context(), photo.S3Bucket, photo.S3Key)
	if err != nil {
		slog.Error("s3 download photo failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load photo", r.URL.Path)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(data)
}

func (h *Handler) ListMapPhotos(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	entries, err := h.DB.ListMapPhotos(r.Context(), userID)
	if err != nil {
		slog.Error("list map photos failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list map photos", r.URL.Path)
		return
	}
	for i := range entries {
		entries[i].ThumbURL = "/api/v1/photos/" + entries[i].ID + "/thumb"
	}
	writeJSON(w, http.StatusOK, entries)
}

// extractExifData versucht GPS-Koordinaten und Aufnahmezeit aus JPEG-EXIF zu lesen.
// Gibt nil-Werte zurueck wenn kein EXIF vorhanden oder nicht parsebar.
func extractExifData(data []byte) (takenAt *time.Time, lat *float64, lng *float64, locationSource *string) {
	x, err := exif.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, nil, nil
	}

	// Aufnahmezeitpunkt
	if tm, err := x.DateTime(); err == nil {
		takenAt = &tm
	}

	// GPS-Koordinaten (Fallback: nur wenn beide vorhanden)
	if la, lo, err := x.LatLong(); err == nil {
		ls := "exif"
		lat = &la
		lng = &lo
		locationSource = &ls
	}

	return takenAt, lat, lng, locationSource
}

// interpolateFromTrack sucht den GPS-Punkt der Aktivitaet zum Zeitpunkt takenAt.
// TODO: Query activity_streams fuer naechsten Zeitpunkt zu takenAt
func (h *Handler) interpolateFromTrack(ctx context.Context, activityID, userID string, takenAt time.Time) (lat *float64, lng *float64) {
	sp, err := h.DB.GetStreamPointNearTime(ctx, activityID, takenAt)
	if err != nil || sp == nil {
		return nil, nil
	}
	return sp.Lat, sp.Lng
}

// generateThumbnail erstellt ein max-200x200 Thumbnail aus JPEG/PNG Daten.
func generateThumbnail(data []byte, maxW, maxH int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Scale-Factor berechnen
	scaleW := float64(maxW) / float64(w)
	scaleH := float64(maxH) / float64(h)
	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}
	if scale >= 1 {
		scale = 1 // Nicht hochskalieren
	}

	newW := int(float64(w) * scale)
	newH := int(float64(h) * scale)

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
