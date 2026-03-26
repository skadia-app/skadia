package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/imageutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

// routePhotoParent is the ParentType for route photo files.
var routePhotoParent = "route_photo"

// UploadRoutePhoto handles POST /api/v1/routes/{id}/photos
// Accepts a multipart form with a "photo" field, generates thumbnails, uploads to ShadowFS.
func (h *Handler) UploadRoutePhoto(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	routeID := chi.URLParam(r, "id")

	if h.ShadowFS == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "storage not configured", r.URL.Path)
		return
	}

	// Verify ownership.
	_, err := h.DB.GetRoute(r.Context(), routeID, userID)
	if httputil.HandleDBErr(w, r, err, "route not found", "id", routeID) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoSize)
	if err := r.ParseMultipartForm(maxPhotoSize); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "file too large (max 20 MB)", r.URL.Path)
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "no photo in request", r.URL.Path)
		return
	}
	defer file.Close()

	// Quick pre-check on client-supplied Content-Type.
	ctFromHeader := header.Header.Get("Content-Type")
	if ctFromHeader != "" && !strings.HasPrefix(ctFromHeader, "image/") {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "only images allowed", r.URL.Path)
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		slog.Error("read route photo data failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to read file", r.URL.Path)
		return
	}

	// Magic-byte detection for actual content type.
	sniff := data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	ct := http.DetectContentType(sniff)
	if !strings.HasPrefix(ct, "image/") {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "only images allowed", r.URL.Path)
		return
	}

	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	// Generate thumbnail (200x200).
	thumbData, _ := imageutil.Resize(data, 200, 200, 80)

	// Generate preview thumbnail.
	var previewData []byte
	if settings, err := h.DB.GetEnrichmentSettings(r.Context(), userID); err == nil {
		if maxW, maxH := imageutil.PreviewSizePixels(settings.PhotoPreviewSize); maxW > 0 {
			previewData, _ = imageutil.Resize(data, maxW, maxH, 85)
		}
	}

	// Capture context for cleanup closures — request context may be cancelled after response.
	cleanupCtx := context.WithoutCancel(r.Context())

	// Upload original photo to ShadowFS.
	rec, putErr := h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
		UserID:      userID,
		FileType:    "media",
		ContentType: ct,
		FileName:    header.Filename,
		SizeBytes:   int64(len(data)),
		FileHash:    hash,
		ParentType:  &routePhotoParent,
		ParentID:    &routeID,
		Tags:        map[string]string{"role": "photo"},
	}, bytes.NewReader(data))
	if putErr != nil {
		slog.Error("shadowfs upload route photo failed", "error", putErr)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to store photo", r.URL.Path)
		return
	}
	photoFileID := rec.ID

	// Track ShadowFS file IDs for cleanup on failure.
	var storedFileIDs []string
	storedFileIDs = append(storedFileIDs, photoFileID)
	cleanup := func() {
		for _, fid := range storedFileIDs {
			if delErr := h.ShadowFS.Delete(cleanupCtx, userID, fid); delErr != nil {
				slog.Warn("shadowfs orphan cleanup failed", "file_id", fid, "error", delErr)
			}
		}
	}

	// Upload thumbnail to ShadowFS.
	if thumbData != nil {
		thumbRec, thumbErr := h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "thumb",
			ContentType: "image/jpeg",
			FileName:    "thumb_200x200.jpg",
			SizeBytes:   int64(len(thumbData)),
			ParentType:  &routePhotoParent,
			ParentID:    &routeID,
			Tags:        map[string]string{"role": "photo_thumb", "source_hash": hash},
		}, bytes.NewReader(thumbData))
		if thumbErr != nil {
			slog.Warn("shadowfs upload route thumb failed", "error", thumbErr)
		} else {
			storedFileIDs = append(storedFileIDs, thumbRec.ID)
		}
	}

	// Upload preview to ShadowFS.
	if previewData != nil {
		previewRec, prevErr := h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "thumb",
			ContentType: "image/jpeg",
			FileName:    "preview.jpg",
			SizeBytes:   int64(len(previewData)),
			ParentType:  &routePhotoParent,
			ParentID:    &routeID,
			Tags:        map[string]string{"role": "photo_preview", "source_hash": hash},
		}, bytes.NewReader(previewData))
		if prevErr != nil {
			slog.Warn("shadowfs upload route preview failed", "error", prevErr)
		} else {
			storedFileIDs = append(storedFileIDs, previewRec.ID)
		}
	}

	photo := model.RoutePhoto{
		RouteID:      routeID,
		UserID:       userID,
		FileHash:     hash,
		OriginalName: &header.Filename,
	}

	saved, err := h.DB.InsertRoutePhoto(r.Context(), photo)
	if err != nil {
		slog.Error("insert route photo failed", "error", err)
		cleanup()
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to save photo", r.URL.Path)
		return
	}

	// Link ShadowFS record to domain row.
	if _, dbErr := h.DB.Pool.Exec(r.Context(),
		`UPDATE route_photos SET file_id = $1 WHERE id = $2`,
		photoFileID, saved.ID,
	); dbErr != nil {
		slog.Warn("route_photos: set file_id failed", "photo_id", saved.ID, "error", dbErr)
	}

	httputil.WriteJSON(w, http.StatusCreated, saved)
}

// DeleteRoutePhoto handles DELETE /api/v1/routes/{id}/photos/{photoId}
func (h *Handler) DeleteRoutePhoto(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	routeID := chi.URLParam(r, "id")
	photoID := chi.URLParam(r, "photoId")

	// Verify route ownership.
	_, err := h.DB.GetRoute(r.Context(), routeID, userID)
	if httputil.HandleDBErr(w, r, err, "route not found", "id", routeID) {
		return
	}

	fileID, err := h.DB.DeleteRoutePhoto(r.Context(), photoID, userID, routeID)
	if err != nil {
		slog.Error("delete route photo failed", "error", err)
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "photo not found", r.URL.Path)
		return
	}

	if h.ShadowFS != nil && fileID != "" {
		// Delete the main photo file.
		if delErr := h.ShadowFS.Delete(r.Context(), userID, fileID); delErr != nil {
			slog.Warn("delete route photo: ShadowFS.Delete failed", "file_id", fileID, "error", delErr)
		}
		// Delete all derivatives (thumb, preview) registered under parent_type/parent_id.
		// Route photo derivatives are stored with parent_type='route_photo' and parent_id=photoID.
		derivRows, derivErr := h.DB.Pool.Query(r.Context(),
			`SELECT id::text FROM user_files
			 WHERE user_id = $1 AND parent_type = 'route_photo' AND parent_id = $2`,
			userID, photoID,
		)
		if derivErr == nil {
			defer derivRows.Close()
			for derivRows.Next() {
				var derivID string
				if scanErr := derivRows.Scan(&derivID); scanErr == nil && derivID != "" {
					if delErr := h.ShadowFS.Delete(r.Context(), userID, derivID); delErr != nil {
						slog.Warn("delete route photo: ShadowFS.Delete derivative failed", "deriv_id", derivID, "error", delErr)
					}
				}
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// reorderRoutePhotosRequest is the JSON body for PUT /routes/{id}/photos/reorder.
type reorderRoutePhotosRequest struct {
	PhotoIDs []string `json:"photo_ids"`
}

// ReorderRoutePhotos handles PUT /api/v1/routes/{id}/photos/reorder
func (h *Handler) ReorderRoutePhotos(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	routeID := chi.URLParam(r, "id")

	// Verify route ownership.
	_, err := h.DB.GetRoute(r.Context(), routeID, userID)
	if httputil.HandleDBErr(w, r, err, "route not found", "id", routeID) {
		return
	}

	req, ok := httputil.DecodeJSON[reorderRoutePhotosRequest](w, r)
	if !ok {
		return
	}
	if len(req.PhotoIDs) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "photo_ids required", r.URL.Path)
		return
	}

	if err := h.DB.ReorderRoutePhotos(r.Context(), routeID, req.PhotoIDs); err != nil {
		slog.Error("reorder route photos failed", "error", err, "route_id", routeID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to reorder photos", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
