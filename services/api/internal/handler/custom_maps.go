package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/filestore"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) ListCustomMaps(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	maps, err := h.DB.ListCustomMaps(r.Context(), userID)
	if err != nil {
		slog.Error("list custom maps failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list custom maps", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, maps)
}

func (h *Handler) CreateCustomMap(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	req, ok := httputil.DecodeJSON[model.CreateCustomMapRequest](w, r)
	if !ok {
		return
	}
	if req.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if req.Color == "" {
		req.Color = "#F97316"
	}
	m, err := h.DB.CreateCustomMap(r.Context(), userID, req.Name, req.Color, req.Description)
	if err != nil {
		slog.Error("create custom map failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create custom map", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, m)
}

func (h *Handler) UpdateCustomMap(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	mapID := chi.URLParam(r, "mapID")
	req, ok := httputil.DecodeJSON[model.UpdateCustomMapRequest](w, r)
	if !ok {
		return
	}
	m, err := h.DB.UpdateCustomMap(r.Context(), mapID, userID, req)
	if err != nil {
		slog.Error("update custom map failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update custom map", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, m)
}

func (h *Handler) DeleteCustomMap(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	mapID := chi.URLParam(r, "mapID")
	if err := h.DB.DeleteCustomMap(r.Context(), mapID, userID); httputil.HandleDBErr(w, r, err, "custom map not found") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListCustomMapMarkers(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	mapID := chi.URLParam(r, "mapID")
	markers, err := h.DB.ListCustomMapMarkers(r.Context(), mapID, userID)
	if err != nil {
		slog.Error("list custom map markers failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list markers", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, markers)
}

func (h *Handler) CreateCustomMapMarker(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	mapID := chi.URLParam(r, "mapID")
	req, ok := httputil.DecodeJSON[model.CreateMarkerRequest](w, r)
	if !ok {
		return
	}
	if req.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	mk, err := h.DB.CreateCustomMapMarker(r.Context(), mapID, userID, req)
	if err != nil {
		slog.Error("create custom map marker failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create marker", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, mk)
}

func (h *Handler) UpdateCustomMapMarker(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	mapID := chi.URLParam(r, "mapID")
	markerID := chi.URLParam(r, "markerID")
	req, ok := httputil.DecodeJSON[model.UpdateMarkerRequest](w, r)
	if !ok {
		return
	}
	mk, err := h.DB.UpdateCustomMapMarker(r.Context(), markerID, mapID, userID, req)
	if err != nil {
		slog.Error("update custom map marker failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update marker", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, mk)
}

func (h *Handler) DeleteCustomMapMarker(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	mapID := chi.URLParam(r, "mapID")
	markerID := chi.URLParam(r, "markerID")

	// Resolve image_file_id before deletion so we can clean up FileStore.
	var imageFileID string
	_ = h.DB.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(image_file_id::text, '') FROM custom_map_markers WHERE id = $1 AND user_id = $2`,
		markerID, userID,
	).Scan(&imageFileID)

	if err := h.DB.DeleteCustomMapMarker(r.Context(), markerID, mapID, userID); httputil.HandleDBErr(w, r, err, "marker not found") {
		return
	}

	// Best-effort FileStore cleanup (marker image).
	if imageFileID != "" && h.FileStore != nil {
		if delErr := h.FileStore.Delete(r.Context(), userID, imageFileID); delErr != nil {
			slog.Warn("delete marker: FileStore.Delete image failed", "marker_id", markerID, "file_id", imageFileID, "error", delErr)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// UploadCustomMapMarkerImage uploads an image for a custom map marker.
// Supports multipart file upload (field "file") or JSON body with { "image_url": "..." }.
// POST /api/v1/custom-maps/{mapID}/markers/{markerID}/image
func (h *Handler) UploadCustomMapMarkerImage(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	mapID := chi.URLParam(r, "mapID")
	markerID := chi.URLParam(r, "markerID")

	ct := r.Header.Get("Content-Type")

	// JSON body path: client sends an existing image URL
	if strings.HasPrefix(ct, "application/json") {
		var req struct {
			ImageURL string `json:"image_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
			return
		}
		if req.ImageURL == "" {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "image_url is required", r.URL.Path)
			return
		}
		updateReq := model.UpdateMarkerRequest{ImageURL: &req.ImageURL}
		mk, err := h.DB.UpdateCustomMapMarker(r.Context(), markerID, mapID, userID, updateReq)
		if httputil.HandleDBErr(w, r, err, "marker not found") {
			return
		}
		httputil.WriteJSON(w, http.StatusOK, mk)
		return
	}

	// Multipart file upload path
	const maxImageSize = 10 << 20 // 10 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxImageSize)
	if err := r.ParseMultipartForm(maxImageSize); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "file too large (max 10 MB)", r.URL.Path)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "no file in request", r.URL.Path)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		slog.Error("read marker image data failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to read file", r.URL.Path)
		return
	}

	// Validate image type by inspecting the actual file bytes (magic bytes),
	// not the client-supplied Content-Type header which can be spoofed.
	detectedCT, errMsg := validateImageBytes(data)
	if errMsg != "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", errMsg, r.URL.Path)
		return
	}
	fileCT := detectedCT

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		ext = ".jpg"
	}
	s3Key := fmt.Sprintf("marker-images/%s/%s%s", userID, markerID, ext)
	bucket := "skadia-media"

	// FileStore path (preferred) — legacy S3 as fallback.
	var imageFileID string
	if h.FileStore != nil {
		rec, putErr := h.FileStore.Put(r.Context(), filestore.FileEntry{
			UserID:      userID,
			FileType:    "media",
			ContentType: fileCT,
			FileName:    header.Filename,
			SizeBytes:   int64(len(data)),
			ParentType:  func() *string { s := "marker"; return &s }(),
			ParentID:    &markerID,
			Tags:        map[string]string{"role": "marker_image"},
		}, bytes.NewReader(data))
		if putErr != nil {
			slog.Error("filestore upload marker image failed", "error", putErr)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to store marker image", r.URL.Path)
			return
		}
		imageFileID = rec.ID
	} else if h.Storage != nil {
		if err := h.Storage.UploadToBucket(r.Context(), bucket, s3Key, data, fileCT); err != nil {
			slog.Error("s3 upload marker image failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to store marker image", r.URL.Path)
			return
		}
	}

	imageURL := fmt.Sprintf("/api/v1/media/%s/%s", bucket, s3Key)
	updateReq := model.UpdateMarkerRequest{ImageURL: &imageURL}
	mk, err := h.DB.UpdateCustomMapMarker(r.Context(), markerID, mapID, userID, updateReq)
	if httputil.HandleDBErr(w, r, err, "marker not found") {
		return
	}

	// Link FileStore record to domain row.
	if imageFileID != "" {
		if _, dbErr := h.DB.Pool.Exec(r.Context(),
			`UPDATE custom_map_markers SET image_file_id = $1 WHERE id = $2`,
			imageFileID, markerID,
		); dbErr != nil {
			slog.Warn("custom_maps: set image_file_id failed", "marker_id", markerID, "error", dbErr)
		}
	}

	httputil.WriteJSON(w, http.StatusOK, mk)
}
