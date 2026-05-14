package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

// ListImportQueue handles GET /api/v1/import-queue
// Query param: status=pending|resolved|dismissed|all (default: pending)
func (h *Handler) ListImportQueue(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}

	items, err := h.DB.ListImportQueue(r.Context(), userID, status)
	if err != nil {
		slog.Error("list import queue failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list import queue", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, items)
}

// CountImportQueue handles GET /api/v1/import-queue/count
func (h *Handler) CountImportQueue(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	count, err := h.DB.CountPendingImportQueue(r.Context(), userID)
	if err != nil {
		slog.Error("count import queue failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to count import queue", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]int{"count": count})
}

// PatchImportQueueItem handles PATCH /api/v1/import-queue/{id}
// Body: {"status": "resolved"|"dismissed"|"pending"}
func (h *Handler) PatchImportQueueItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid request body", r.URL.Path)
		return
	}
	if req.Status != "resolved" && req.Status != "dismissed" && req.Status != "pending" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "status must be resolved, dismissed, or pending", r.URL.Path)
		return
	}

	if req.Status == "resolved" {
		if err := h.DB.ResolveImportQueueItem(r.Context(), id, userID, ""); err != nil {
			slog.Error("resolve import queue item failed", "id", id, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to resolve queue item", r.URL.Path)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var resolvedAt *time.Time
	if req.Status == "dismissed" {
		now := time.Now()
		resolvedAt = &now
	}

	if err := h.DB.UpdateImportQueueStatus(r.Context(), id, userID, req.Status, resolvedAt); err != nil {
		slog.Error("update import queue status failed", "id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update queue item", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteImportQueueItem handles DELETE /api/v1/import-queue/{id}
func (h *Handler) DeleteImportQueueItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	// Clean up ShadowFS files stored in metadata before deleting the queue item.
	if h.ShadowFS != nil {
		item, itemErr := h.DB.GetImportQueueItem(r.Context(), id, userID)
		if itemErr == nil && item != nil && item.Metadata != nil {
			for _, metaKey := range []string{"file_store_id", "file_store_id_thumb"} {
				if fileID, ok := item.Metadata[metaKey].(string); ok && fileID != "" {
					if delErr := h.ShadowFS.Delete(r.Context(), userID, fileID); delErr != nil {
						slog.Warn("delete import queue item: shadowfs delete failed",
							"queue_id", id, "meta_key", metaKey, "file_id", fileID, "error", delErr)
					}
				}
			}
		}
	}

	if err := h.DB.DeleteImportQueueItem(r.Context(), id, userID); err != nil {
		slog.Error("delete import queue item failed", "id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete queue item", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AssignImportQueueItem handles POST /api/v1/import-queue/{id}/assign
// Assigns a queued image_no_match item to an activity via the ImportService pipeline.
// Body: {"activity_id": "uuid"}
func (h *Handler) AssignImportQueueItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	var req struct {
		ActivityID string `json:"activity_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.ActivityID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "activity_id is required", r.URL.Path)
		return
	}

	// Verify activity ownership before processing
	activity, err := h.DB.GetActivity(r.Context(), req.ActivityID)
	if err != nil || activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	result, err := h.ImportService.ResolveQueuedPhotoWithOptions(r.Context(), userID, id, req.ActivityID, importer.PhotoProcessOptions{
		EnforceActivityPhotoLimit: true,
	})
	if err != nil {
		if writeMediaUploadProblem(w, r, err) {
			return
		}
		slog.Error("assign queue item: ResolveQueuedPhoto failed", "id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, result)
}

// ResolvePDFQueueItem handles POST /api/v1/import-queue/{id}/resolve-pdf.
// It turns a queued PDF into a gear document and resolves the linked import log.
func (h *Handler) ResolvePDFQueueItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	var req struct {
		DocType string `json:"doc_type"`
		GearID  string `json:"gear_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	req.DocType = strings.TrimSpace(req.DocType)
	req.GearID = strings.TrimSpace(req.GearID)
	if req.DocType == "" {
		req.DocType = "other"
	}
	switch req.DocType {
	case "receipt", "invoice", "other":
	default:
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "doc_type must be receipt, invoice, or other", r.URL.Path)
		return
	}
	if req.GearID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "gear_id is required", r.URL.Path)
		return
	}
	if err := h.DB.VerifyGearOwnership(r.Context(), req.GearID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}
	if h.ShadowFS == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "queued file not available", r.URL.Path)
		return
	}

	item, err := h.DB.GetImportQueueItem(r.Context(), id, userID)
	if err != nil || item == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "queue item not found", r.URL.Path)
		return
	}
	if item.QueueType != "pdf_untyped" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "queue item is not a PDF review item", r.URL.Path)
		return
	}
	if item.Status != "pending" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "queue item is already resolved", r.URL.Path)
		return
	}
	queueFileID, ok := item.Metadata["file_store_id"].(string)
	if !ok || queueFileID == "" {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "queued file not available", r.URL.Path)
		return
	}

	rc, err := h.ShadowFS.Get(r.Context(), userID, queueFileID)
	if err != nil {
		slog.Error("resolve PDF queue item: ShadowFS.Get failed", "id", id, "file_id", queueFileID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to retrieve queued PDF", r.URL.Path)
		return
	}
	data, readErr := io.ReadAll(rc)
	closeErr := rc.Close()
	if readErr != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to read queued PDF", r.URL.Path)
		return
	}
	if closeErr != nil {
		slog.Warn("resolve PDF queue item: close queued file failed", "id", id, "error", closeErr)
	}

	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])
	if exists, dupErr := h.DB.GearDocumentExistsByHashAndGear(r.Context(), userID, hashStr, req.GearID); dupErr != nil {
		slog.Error("resolve PDF queue item: duplicate check failed", "id", id, "error", dupErr)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to check duplicate document", r.URL.Path)
		return
	} else if exists {
		if err := h.DB.ResolveImportQueueItem(r.Context(), id, userID, ""); err != nil {
			slog.Error("resolve PDF queue item: resolve duplicate queue failed", "id", id, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to resolve duplicate PDF", r.URL.Path)
			return
		}
		if delErr := h.ShadowFS.Delete(r.Context(), userID, queueFileID); delErr != nil {
			slog.Warn("resolve PDF queue item: delete duplicate queued file failed", "id", id, "file_id", queueFileID, "error", delErr)
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "duplicate"})
		return
	}

	parentType := "gear"
	stored, err := h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
		UserID:            userID,
		FileType:          "document",
		ContentType:       item.ContentType,
		FileName:          item.OriginalFilename,
		SizeBytes:         int64(len(data)),
		FileHash:          hashStr,
		ParentType:        &parentType,
		ParentID:          &req.GearID,
		Tags:              map[string]string{"doc_type": req.DocType},
		QuotaCreditFileID: queueFileID,
	}, bytes.NewReader(data))
	if err != nil {
		slog.Error("resolve PDF queue item: ShadowFS.Put failed", "id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to store PDF document", r.URL.Path)
		return
	}

	title := item.OriginalFilename
	shouldProcessDocument := h.DocumentUploadedPublisher != nil &&
		h.featureNotAvailableReasonWithContext(r.Context(), capabilities.KeyDocumentML) == ""
	doc := &model.GearDocument{
		UserID:           userID,
		GearID:           &req.GearID,
		DocType:          req.DocType,
		Title:            &title,
		OriginalFilename: item.OriginalFilename,
		ContentType:      item.ContentType,
		FileSizeBytes:    int64(len(data)),
		FileHash:         &hashStr,
		FileID:           &stored.ID,
	}
	if shouldProcessDocument {
		doc.ProcessingStatus = "pending"
	}
	result, err := h.DB.InsertGearDocument(r.Context(), doc)
	if err != nil {
		if delErr := h.ShadowFS.Delete(r.Context(), userID, stored.ID); delErr != nil {
			slog.Warn("resolve PDF queue item: cleanup stored document failed", "file_id", stored.ID, "error", delErr)
		}
		slog.Error("resolve PDF queue item: insert gear document failed", "id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Database error", "failed to save document", r.URL.Path)
		return
	}

	var extractedText *string
	var isTextPDF bool
	if text := extractPDFText(data); text != "" {
		extractedText = &text
		isTextPDF = true
		var updateErr error
		if shouldProcessDocument {
			updateErr = h.DB.UpdateGearDocumentExtractedText(r.Context(), userID, result.ID, text)
		} else {
			updateErr = h.DB.UpdateGearDocumentText(r.Context(), userID, result.ID, text)
		}
		if updateErr != nil {
			slog.Warn("resolve PDF queue item: failed to update extracted text", "doc_id", result.ID, "error", updateErr)
		} else {
			result.ExtractedText = &text
			if shouldProcessDocument {
				result.ProcessingStatus = "pending"
			} else {
				result.ProcessingStatus = "processed"
			}
		}
	}

	if err := h.DB.ResolveImportQueueItem(r.Context(), id, userID, ""); err != nil {
		if fileID, delDocErr := h.DB.DeleteGearDocument(r.Context(), result.ID, userID); delDocErr != nil {
			slog.Warn("resolve PDF queue item: cleanup gear document after queue failure failed", "doc_id", result.ID, "error", delDocErr)
		} else if fileID != "" {
			if delFileErr := h.ShadowFS.Delete(r.Context(), userID, fileID); delFileErr != nil {
				slog.Warn("resolve PDF queue item: cleanup stored file after queue failure failed", "file_id", fileID, "error", delFileErr)
			}
		}
		slog.Error("resolve PDF queue item: resolve queue failed", "id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to resolve queue item", r.URL.Path)
		return
	}

	if delErr := h.ShadowFS.Delete(r.Context(), userID, queueFileID); delErr != nil {
		slog.Warn("resolve PDF queue item: delete queued file failed", "id", id, "file_id", queueFileID, "error", delErr)
	}
	if shouldProcessDocument {
		importMeta := directDocumentImportMetadata("gear_document", result.ID, stored.ID, "gear_id", req.GearID, req.DocType, item.ContentType, true)
		if isTextPDF && extractedText != nil {
			markDirectDocumentStep(importMeta, "text_extraction", "completed")
		}
		if importMetaBytes, marshalErr := json.Marshal(importMeta); marshalErr != nil {
			slog.Warn("resolve PDF queue item: marshal document import metadata failed", "doc_id", result.ID, "error", marshalErr)
		} else if err := h.DB.SetQueuedDocumentImportProcessing(r.Context(), userID, id, importMetaBytes); err != nil {
			slog.Warn("resolve PDF queue item: failed to mark import log processing", "queue_id", id, "error", err)
		}
		evt := event.DocumentUploadedEvent{
			DocID:          result.ID,
			GearID:         result.GearID,
			ComponentID:    result.ComponentID,
			UserID:         userID,
			ContentType:    result.ContentType,
			IsTextPDF:      isTextPDF,
			ExtractedText:  extractedText,
			FileID:         stored.ID,
			FileType:       stored.FileType,
			StorageBackend: stored.StorageBackend,
			StoragePath:    stored.StoragePath,
			SizeBytes:      stored.SizeBytes,
		}
		if err := h.DocumentUploadedPublisher.PublishDocumentUploaded(evt); err != nil {
			if statusErr := h.DB.UpdateGearDocumentProcessingStatus(r.Context(), userID, result.ID, "failed"); statusErr != nil {
				slog.Warn("resolve PDF queue item: failed to mark document event failure", "doc_id", result.ID, "error", statusErr)
			}
			if logErr := h.DB.SetQueuedDocumentImportError(r.Context(), userID, id, err.Error()); logErr != nil {
				slog.Warn("resolve PDF queue item: failed to mark import log event failure", "queue_id", id, "error", logErr)
			}
			slog.Warn("resolve PDF queue item: failed to publish document uploaded event", "error", err)
		}
	}

	httputil.WriteJSON(w, http.StatusCreated, result)
}

// GetImportQueueItemThumb handles GET /api/v1/import-queue/{id}/thumb
// Returns the thumbnail image for a queued item stored in ShadowFS or S3.
func (h *Handler) GetImportQueueItemThumb(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	item, err := h.DB.GetImportQueueItem(r.Context(), id, userID)
	if err != nil || item == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "queue item not found", r.URL.Path)
		return
	}

	// ShadowFS path: serve thumbnail from ShadowFS when file_store_id_thumb is set.
	if h.ShadowFS != nil {
		if thumbFileID, hasThumb := item.Metadata["file_store_id_thumb"].(string); hasThumb && thumbFileID != "" {
			rc, getErr := h.ShadowFS.Get(r.Context(), userID, thumbFileID)
			if getErr != nil {
				slog.Error("get queue item thumb: ShadowFS.Get failed", "id", id, "file_id", thumbFileID, "error", getErr)
				httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to retrieve thumbnail", r.URL.Path)
				return
			}
			defer rc.Close()
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Cache-Control", "private, max-age=3600")
			w.WriteHeader(http.StatusOK)
			_, _ = io.Copy(w, rc)
			return
		}
		// ShadowFS active but no thumb — check if there is at least the main file to serve as thumb.
		if mainFileID, hasMain := item.Metadata["file_store_id"].(string); hasMain && mainFileID != "" {
			rc, getErr := h.ShadowFS.Get(r.Context(), userID, mainFileID)
			if getErr != nil {
				slog.Error("get queue item thumb: ShadowFS.Get (main) failed", "id", id, "file_id", mainFileID, "error", getErr)
				httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to retrieve thumbnail", r.URL.Path)
				return
			}
			defer rc.Close()
			ct := item.ContentType
			if ct == "" {
				ct = "image/jpeg"
			}
			w.Header().Set("Content-Type", ct)
			w.Header().Set("Cache-Control", "private, max-age=3600")
			w.WriteHeader(http.StatusOK)
			_, _ = io.Copy(w, rc)
			return
		}
	}

	// No thumbnail available.
	httputil.WriteError(w, http.StatusNotFound, "Not Found", "no thumbnail available for this queue item", r.URL.Path)
}

// GetImportQueueItemPreview handles GET /api/v1/import-queue/{id}/preview
// It streams the queued original file for manual review, falling back to the thumbnail.
func (h *Handler) GetImportQueueItemPreview(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	item, err := h.DB.GetImportQueueItem(r.Context(), id, userID)
	if err != nil || item == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "queue item not found", r.URL.Path)
		return
	}
	if h.ShadowFS == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "preview not available for this queue item", r.URL.Path)
		return
	}

	if mainFileID, hasMain := item.Metadata["file_store_id"].(string); hasMain && mainFileID != "" {
		rc, getErr := h.ShadowFS.Get(r.Context(), userID, mainFileID)
		if getErr != nil {
			slog.Warn("get queue item preview: original unavailable, falling back to thumbnail", "id", id, "file_id", mainFileID, "error", getErr)
		} else {
			defer rc.Close()
			ct := item.ContentType
			if ct == "" {
				ct = "image/jpeg"
			}
			w.Header().Set("Content-Type", ct)
			w.Header().Set("Cache-Control", "private, max-age=3600")
			w.WriteHeader(http.StatusOK)
			_, _ = io.Copy(w, rc)
			return
		}
	}

	if thumbFileID, hasThumb := item.Metadata["file_store_id_thumb"].(string); hasThumb && thumbFileID != "" {
		rc, getErr := h.ShadowFS.Get(r.Context(), userID, thumbFileID)
		if getErr != nil {
			slog.Error("get queue item preview: ShadowFS.Get thumb failed", "id", id, "file_id", thumbFileID, "error", getErr)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to retrieve preview", r.URL.Path)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, rc)
		return
	}

	httputil.WriteError(w, http.StatusNotFound, "Not Found", "preview not available for this queue item", r.URL.Path)
}
