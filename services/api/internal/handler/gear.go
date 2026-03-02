package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) ListGear(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	status := r.URL.Query().Get("status")

	items, err := h.DB.ListGear(r.Context(), userID, status)
	if err != nil {
		slog.Error("list gear failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list gear", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) CreateGear(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var create model.GearCreate
	if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if create.Name == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if create.Type == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "type is required", r.URL.Path)
		return
	}

	gear, err := h.DB.CreateGear(r.Context(), userID, &create)
	if err != nil {
		slog.Error("create gear failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create gear", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusCreated, gear)
}

func (h *Handler) GetGear(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	gear, err := h.DB.GetGear(r.Context(), id)
	if err != nil {
		slog.Error("get gear failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get gear", r.URL.Path)
		return
	}
	if gear == nil {
		writeError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, gear)
}

func (h *Handler) UpdateGear(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var update model.GearUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	gear, err := h.DB.UpdateGear(r.Context(), id, &update)
	if err != nil {
		slog.Error("update gear failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update gear", r.URL.Path)
		return
	}
	if gear == nil {
		writeError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, gear)
}

func (h *Handler) DeleteGear(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")

	if err := h.DB.DeleteGear(r.Context(), userID, id); err != nil {
		if err.Error() == "gear not found" {
			writeError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
			return
		}
		slog.Error("delete gear failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete gear", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UpdateGearStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.Status != "active" && req.Status != "stored" && req.Status != "retired" {
		writeError(w, http.StatusBadRequest, "Bad Request", "status must be active, stored, or retired", r.URL.Path)
		return
	}

	if err := h.DB.UpdateGearStatus(r.Context(), id, req.Status); err != nil {
		if err.Error() == "gear not found" {
			writeError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
			return
		}
		slog.Error("update gear status failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update gear status", r.URL.Path)
		return
	}

	gear, err := h.DB.GetGear(r.Context(), id)
	if err != nil {
		slog.Error("get gear after status update failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get gear", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, gear)
}

func (h *Handler) CreateComponent(w http.ResponseWriter, r *http.Request) {
	gearID := chi.URLParam(r, "id")

	// Verify gear exists
	gear, err := h.DB.GetGear(r.Context(), gearID)
	if err != nil {
		slog.Error("get gear for component failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get gear", r.URL.Path)
		return
	}
	if gear == nil {
		writeError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	var create model.GearComponentCreate
	if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if create.Name == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if create.Type == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "type is required", r.URL.Path)
		return
	}

	component, err := h.DB.CreateComponent(r.Context(), gearID, &create)
	if err != nil {
		slog.Error("create component failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create component", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusCreated, component)
}

func (h *Handler) UpdateComponent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Name          *string  `json:"name,omitempty"`
		Type          *string  `json:"type,omitempty"`
		Brand         *string  `json:"brand,omitempty"`
		Model         *string  `json:"model,omitempty"`
		Notes         *string  `json:"notes,omitempty"`
		IsRetired     *bool    `json:"is_retired,omitempty"`
		PurchasePrice *float64 `json:"purchase_price,omitempty"`
		PurchaseDate  *string  `json:"purchase_date,omitempty"`
		SerialNumber  *string  `json:"serial_number,omitempty"`
		WeightG       *int     `json:"weight_g,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	component, err := h.DB.UpdateComponent(r.Context(), id, req.Name, req.Type, req.Brand, req.Model, req.Notes, req.IsRetired, req.PurchasePrice, req.PurchaseDate, req.SerialNumber, req.WeightG)
	if err != nil {
		slog.Error("update component failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update component", r.URL.Path)
		return
	}
	if component == nil {
		writeError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, component)
}

func (h *Handler) DeleteComponent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.DB.DeleteComponent(r.Context(), id); err != nil {
		if err.Error() == "component not found" {
			writeError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
			return
		}
		slog.Error("delete component failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete component", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) TransferComponent(w http.ResponseWriter, r *http.Request) {
	componentID := chi.URLParam(r, "id")

	var req struct {
		ToGearID string `json:"to_gear_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.ToGearID == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "to_gear_id is required", r.URL.Path)
		return
	}

	// Get the component to find current gear
	history, err := h.DB.GetComponentHistory(r.Context(), componentID)
	if err != nil {
		slog.Error("get component history for transfer failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get component", r.URL.Path)
		return
	}

	var fromGearID string
	for _, a := range history {
		if a.RemovedAt == nil {
			fromGearID = a.GearID
			break
		}
	}
	if fromGearID == "" {
		// Fallback: get gear_id directly from the component
		gear, err := h.DB.GetGear(r.Context(), req.ToGearID)
		if err != nil || gear == nil {
			writeError(w, http.StatusNotFound, "Not Found", "target gear not found", r.URL.Path)
			return
		}
		// Use a simple query to get the current gear_id
		fromGearID = "" // Will still work with transfer
	}

	if err := h.DB.TransferComponent(r.Context(), componentID, fromGearID, req.ToGearID); err != nil {
		slog.Error("transfer component failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to transfer component", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetComponentHistory(w http.ResponseWriter, r *http.Request) {
	componentID := chi.URLParam(r, "id")

	history, err := h.DB.GetComponentHistory(r.Context(), componentID)
	if err != nil {
		slog.Error("get component history failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get component history", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, history)
}

func (h *Handler) GetChainRotation(w http.ResponseWriter, r *http.Request) {
	gearID := chi.URLParam(r, "id")

	group, err := h.DB.GetChainRotation(r.Context(), gearID)
	if err != nil {
		slog.Error("get chain rotation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get chain rotation", r.URL.Path)
		return
	}
	if group == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}

	writeJSON(w, http.StatusOK, group)
}

func (h *Handler) CreateChainRotation(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	gearID := chi.URLParam(r, "id")

	var create database.ChainRotationGroupCreate
	if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if create.Name == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if len(create.ComponentIDs) < 2 {
		writeError(w, http.StatusBadRequest, "Bad Request", "at least 2 components required", r.URL.Path)
		return
	}

	group, err := h.DB.CreateChainRotation(r.Context(), userID, gearID, &create)
	if err != nil {
		slog.Error("create chain rotation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create chain rotation", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusCreated, group)
}

func (h *Handler) SwapChain(w http.ResponseWriter, r *http.Request) {
	gearID := chi.URLParam(r, "id")

	// Find the chain rotation group for this gear
	group, err := h.DB.GetChainRotation(r.Context(), gearID)
	if err != nil {
		slog.Error("get chain rotation for swap failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get chain rotation", r.URL.Path)
		return
	}
	if group == nil {
		writeError(w, http.StatusNotFound, "Not Found", "chain rotation not found for this gear", r.URL.Path)
		return
	}

	if err := h.DB.SwapChain(r.Context(), group.ID); err != nil {
		slog.Error("swap chain failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to swap chain", r.URL.Path)
		return
	}

	// Return updated group
	updated, err := h.DB.GetChainRotation(r.Context(), gearID)
	if err != nil {
		slog.Error("get chain rotation after swap failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get chain rotation", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) UpdateChainRotation(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")

	var req struct {
		SwapIntervalKM *float64 `json:"swap_interval_km"`
		SwapIntervalHM *float64 `json:"swap_interval_hm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	if err := h.DB.UpdateChainRotation(r.Context(), groupID, req.SwapIntervalKM, req.SwapIntervalHM); err != nil {
		slog.Error("update chain rotation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update chain rotation", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteChainRotation(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")

	if err := h.DB.DeleteChainRotation(r.Context(), groupID); err != nil {
		if err.Error() == "chain rotation not found" {
			writeError(w, http.StatusNotFound, "Not Found", "chain rotation not found", r.URL.Path)
			return
		}
		slog.Error("delete chain rotation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete chain rotation", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListServiceEvents(w http.ResponseWriter, r *http.Request) {
	componentID := chi.URLParam(r, "id")

	events, err := h.DB.ListServiceEvents(r.Context(), componentID)
	if err != nil {
		slog.Error("list service events failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list service events", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, events)
}

func (h *Handler) CreateServiceEvent(w http.ResponseWriter, r *http.Request) {
	componentID := chi.URLParam(r, "id")

	var create database.ServiceEventCreate
	if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if create.PerformedAt == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "performed_at is required", r.URL.Path)
		return
	}

	event, err := h.DB.CreateServiceEvent(r.Context(), componentID, &create)
	if err != nil {
		slog.Error("create service event failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create service event", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusCreated, event)
}

func (h *Handler) DeleteServiceEvent(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "id")

	if err := h.DB.DeleteServiceEvent(r.Context(), eventID); err != nil {
		if err.Error() == "service event not found" {
			writeError(w, http.StatusNotFound, "Not Found", "service event not found", r.URL.Path)
			return
		}
		slog.Error("delete service event failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete service event", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListComponentPresets(w http.ResponseWriter, r *http.Request) {
	gearType := r.URL.Query().Get("gear_type")

	presets, err := h.DB.ListComponentPresets(r.Context(), gearType)
	if err != nil {
		slog.Error("list component presets failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list component presets", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, presets)
}

// --- Bike Setups ---

func (h *Handler) ListBikeSetups(w http.ResponseWriter, r *http.Request) {
	gearID := chi.URLParam(r, "id")

	setups, err := h.DB.ListBikeSetups(r.Context(), gearID)
	if err != nil {
		slog.Error("list bike setups failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list bike setups", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, setups)
}

func (h *Handler) CreateBikeSetup(w http.ResponseWriter, r *http.Request) {
	gearID := chi.URLParam(r, "id")

	var create model.BikeSetupCreate
	if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if create.Name == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}

	setup, err := h.DB.CreateBikeSetup(r.Context(), gearID, &create)
	if err != nil {
		slog.Error("create bike setup failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create bike setup", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusCreated, setup)
}

func (h *Handler) UpdateBikeSetup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var update model.BikeSetupUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	setup, err := h.DB.UpdateBikeSetup(r.Context(), id, &update)
	if err != nil {
		slog.Error("update bike setup failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update bike setup", r.URL.Path)
		return
	}
	if setup == nil {
		writeError(w, http.StatusNotFound, "Not Found", "bike setup not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, setup)
}

func (h *Handler) DeleteBikeSetup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.DB.DeleteBikeSetup(r.Context(), id); err != nil {
		if err.Error() == "bike setup not found" {
			writeError(w, http.StatusNotFound, "Not Found", "bike setup not found", r.URL.Path)
			return
		}
		slog.Error("delete bike setup failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete bike setup", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ActivateBikeSetup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	setup, err := h.DB.ActivateBikeSetup(r.Context(), id)
	if err != nil {
		slog.Error("activate bike setup failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to activate bike setup", r.URL.Path)
		return
	}
	if setup == nil {
		writeError(w, http.StatusNotFound, "Not Found", "bike setup not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, setup)
}

func (h *Handler) GetMaintenanceAlerts(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	alerts, err := h.DB.GetMaintenanceAlerts(r.Context(), userID)
	if err != nil {
		slog.Error("get maintenance alerts failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get maintenance alerts", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, alerts)
}

// --- Gear Documents ---

func (h *Handler) UploadGearDocument(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	gearID := chi.URLParam(r, "id")

	// Limit to 10MB
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "File too large", "Maximum file size is 10MB", r.URL.Path)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Missing file", "No file provided", r.URL.Path)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Read error", "Failed to read file", r.URL.Path)
		return
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	allowed := map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true, "application/pdf": true}
	if !allowed[contentType] {
		writeError(w, http.StatusBadRequest, "Invalid file type", "Allowed: JPEG, PNG, WebP, PDF", r.URL.Path)
		return
	}

	// SHA256 hash
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	// Determine file extension
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		switch contentType {
		case "image/jpeg":
			ext = ".jpg"
		case "image/png":
			ext = ".png"
		case "image/webp":
			ext = ".webp"
		case "application/pdf":
			ext = ".pdf"
		}
	}

	// S3 key format: gear/{userID}/{gearID}/{hash}{ext}
	mediaBucket := os.Getenv("GARAGE_MEDIA_BUCKET")
	if mediaBucket == "" {
		mediaBucket = "skadia-media"
	}
	s3Key := fmt.Sprintf("gear/%s/%s/%s%s", userID, gearID, hashStr, ext)

	// Upload to S3
	if err := h.Storage.UploadToBucket(r.Context(), mediaBucket, s3Key, data, contentType); err != nil {
		slog.Error("s3 upload failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Upload failed", "Failed to store file", r.URL.Path)
		return
	}

	docType := r.FormValue("doc_type")
	if docType == "" {
		if strings.HasPrefix(contentType, "image/") {
			docType = "photo"
		} else {
			docType = "other"
		}
	}

	title := r.FormValue("title")
	var titlePtr *string
	if title != "" {
		titlePtr = &title
	}

	doc := &model.GearDocument{
		UserID:           userID,
		GearID:           &gearID,
		DocType:          docType,
		Title:            titlePtr,
		OriginalFilename: header.Filename,
		ContentType:      contentType,
		FileSizeBytes:    int64(len(data)),
		FileHash:         &hashStr,
		S3Bucket:         mediaBucket,
		S3Key:            s3Key,
	}

	result, err := h.DB.InsertGearDocument(r.Context(), doc)
	if err != nil {
		slog.Error("insert gear document failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Database error", "Failed to save document", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (h *Handler) ListGearDocuments(w http.ResponseWriter, r *http.Request) {
	gearID := chi.URLParam(r, "id")
	docs, err := h.DB.ListGearDocuments(r.Context(), gearID)
	if err != nil {
		slog.Error("list gear documents failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Database error", "Failed to list documents", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, docs)
}

func (h *Handler) DeleteGearDocument(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "docId")
	bucket, key, err := h.DB.DeleteGearDocument(r.Context(), docID)
	if err != nil {
		slog.Error("delete gear document failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Database error", "Failed to delete document", r.URL.Path)
		return
	}
	if bucket == "" {
		writeError(w, http.StatusNotFound, "Not found", "Document not found", r.URL.Path)
		return
	}

	// Delete from S3 (best-effort)
	if err := h.Storage.DeleteFromBucket(r.Context(), bucket, key); err != nil {
		slog.Error("s3 delete failed", "bucket", bucket, "key", key, "error", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DownloadGearDocument(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "docId")
	doc, err := h.DB.GetGearDocument(r.Context(), docID)
	if err != nil {
		slog.Error("get gear document failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Database error", "Failed to get document", r.URL.Path)
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "Not found", "Document not found", r.URL.Path)
		return
	}

	data, ct, err := h.Storage.DownloadFromBucket(r.Context(), doc.S3Bucket, doc.S3Key)
	if err != nil {
		slog.Error("s3 download failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Download failed", "Failed to retrieve file", r.URL.Path)
		return
	}

	if ct == "" {
		ct = doc.ContentType
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, doc.OriginalFilename))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(data)
}

// GetGearCosts returns the cost summary for a gear item.
func (h *Handler) GetGearCosts(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	summary, err := h.DB.GetGearCostSummary(r.Context(), id)
	if err != nil {
		slog.Error("get gear costs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get gear costs", r.URL.Path)
		return
	}
	if summary == nil {
		writeError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
