package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) ListCustomMaps(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	maps, err := h.DB.ListCustomMaps(r.Context(), userID)
	if err != nil {
		slog.Error("list custom maps failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list custom maps", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, maps)
}

func (h *Handler) CreateCustomMap(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req model.CreateCustomMapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if req.Color == "" {
		req.Color = "#F97316"
	}
	m, err := h.DB.CreateCustomMap(r.Context(), userID, req.Name, req.Color, req.Description)
	if err != nil {
		slog.Error("create custom map failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create custom map", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handler) UpdateCustomMap(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	mapID := chi.URLParam(r, "mapID")
	var req model.UpdateCustomMapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	m, err := h.DB.UpdateCustomMap(r.Context(), mapID, userID, req)
	if err != nil {
		slog.Error("update custom map failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update custom map", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handler) DeleteCustomMap(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	mapID := chi.URLParam(r, "mapID")
	if err := h.DB.DeleteCustomMap(r.Context(), mapID, userID); err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Not Found", "custom map not found", r.URL.Path)
			return
		}
		slog.Error("delete custom map failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete custom map", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListCustomMapMarkers(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	mapID := chi.URLParam(r, "mapID")
	markers, err := h.DB.ListCustomMapMarkers(r.Context(), mapID, userID)
	if err != nil {
		slog.Error("list custom map markers failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list markers", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, markers)
}

func (h *Handler) CreateCustomMapMarker(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	mapID := chi.URLParam(r, "mapID")
	var req model.CreateMarkerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	mk, err := h.DB.CreateCustomMapMarker(r.Context(), mapID, userID, req)
	if err != nil {
		slog.Error("create custom map marker failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create marker", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusCreated, mk)
}

func (h *Handler) UpdateCustomMapMarker(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	mapID := chi.URLParam(r, "mapID")
	markerID := chi.URLParam(r, "markerID")
	var req model.UpdateMarkerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	mk, err := h.DB.UpdateCustomMapMarker(r.Context(), markerID, mapID, userID, req)
	if err != nil {
		slog.Error("update custom map marker failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update marker", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, mk)
}

func (h *Handler) DeleteCustomMapMarker(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	mapID := chi.URLParam(r, "mapID")
	markerID := chi.URLParam(r, "markerID")
	if err := h.DB.DeleteCustomMapMarker(r.Context(), markerID, mapID, userID); err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Not Found", "marker not found", r.URL.Path)
			return
		}
		slog.Error("delete custom map marker failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete marker", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UploadCustomMapMarkerImage uploads an image for a custom map marker.
// Supports multipart file upload (field "file") or JSON body with { "image_url": "..." }.
// POST /api/v1/custom-maps/{mapID}/markers/{markerID}/image
func (h *Handler) UploadCustomMapMarkerImage(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
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
			writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
			return
		}
		if req.ImageURL == "" {
			writeError(w, http.StatusBadRequest, "Bad Request", "image_url is required", r.URL.Path)
			return
		}
		updateReq := model.UpdateMarkerRequest{ImageURL: &req.ImageURL}
		mk, err := h.DB.UpdateCustomMapMarker(r.Context(), markerID, mapID, userID, updateReq)
		if err != nil {
			if err == pgx.ErrNoRows {
				writeError(w, http.StatusNotFound, "Not Found", "marker not found", r.URL.Path)
				return
			}
			slog.Error("update marker image_url failed", "error", err)
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update marker", r.URL.Path)
			return
		}
		writeJSON(w, http.StatusOK, mk)
		return
	}

	// Multipart file upload path
	const maxImageSize = 10 << 20 // 10 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxImageSize)
	if err := r.ParseMultipartForm(maxImageSize); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "file too large (max 10 MB)", r.URL.Path)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "no file in request", r.URL.Path)
		return
	}
	defer file.Close()

	fileCT := header.Header.Get("Content-Type")
	if fileCT == "" {
		fileCT = "image/jpeg"
	}
	if !strings.HasPrefix(fileCT, "image/") {
		writeError(w, http.StatusBadRequest, "Bad Request", "only images allowed", r.URL.Path)
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		slog.Error("read marker image data failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to read file", r.URL.Path)
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		ext = ".jpg"
	}
	s3Key := fmt.Sprintf("marker-images/%s/%s%s", userID, markerID, ext)
	bucket := "skadia-media"

	if err := h.Storage.UploadToBucket(r.Context(), bucket, s3Key, data, fileCT); err != nil {
		slog.Error("s3 upload marker image failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to store marker image", r.URL.Path)
		return
	}

	imageURL := fmt.Sprintf("/api/v1/media/%s/%s", bucket, s3Key)
	updateReq := model.UpdateMarkerRequest{ImageURL: &imageURL}
	mk, err := h.DB.UpdateCustomMapMarker(r.Context(), markerID, mapID, userID, updateReq)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Not Found", "marker not found", r.URL.Path)
			return
		}
		slog.Error("update marker image_url failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update marker", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, mk)
}
