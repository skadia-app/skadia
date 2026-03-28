package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	pdfcpuapi "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpumodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

func (h *Handler) ListGear(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")

	items, err := h.DB.ListGear(r.Context(), userID, status)
	if err != nil {
		slog.Error("list gear failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list gear", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, items)
}

func (h *Handler) CreateGear(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	create, ok := httputil.DecodeJSON[model.GearCreate](w, r)
	if !ok {
		return
	}
	if create.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if create.Type == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "type is required", r.URL.Path)
		return
	}

	gear, err := h.DB.CreateGear(r.Context(), userID, &create)
	if err != nil {
		slog.Error("create gear failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create gear", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, gear)
}

func (h *Handler) GetGear(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	gear, err := h.DB.GetGear(r.Context(), id)
	if err != nil {
		slog.Error("get gear failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get gear", r.URL.Path)
		return
	}
	if gear == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}
	if gear.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, gear)
}

func (h *Handler) UpdateGear(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	// Verify ownership before updating.
	existing, err := h.DB.GetGear(r.Context(), id)
	if err != nil {
		slog.Error("update gear: get gear failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get gear", r.URL.Path)
		return
	}
	if existing == nil || existing.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	update, ok := httputil.DecodeJSON[model.GearUpdate](w, r)
	if !ok {
		return
	}

	gear, err := h.DB.UpdateGear(r.Context(), id, &update)
	if err != nil {
		slog.Error("update gear failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update gear", r.URL.Path)
		return
	}
	if gear == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, gear)
}

func (h *Handler) DeleteGear(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	// Ownership check BEFORE any file cleanup to prevent IDOR.
	if err := h.DB.VerifyGearOwnership(r.Context(), id, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	// Clean up associated document files before deleting the gear row
	// (ON DELETE CASCADE removes gear_documents rows; we must remove files first).
	docEntries, listErr := h.DB.ListGearDocumentFileIDsByGearID(r.Context(), id)
	if listErr != nil {
		slog.Warn("delete gear: list document file ids failed", "gear_id", id, "error", listErr)
	}
	for _, e := range docEntries {
		if h.ShadowFS != nil && e.FileID != "" {
			if delErr := h.ShadowFS.Delete(r.Context(), userID, e.FileID); delErr != nil {
				slog.Warn("delete gear: shadowfs delete document failed",
					"gear_id", id, "file_id", e.FileID, "error", delErr)
			}
		}
	}

	if err := h.DB.DeleteGear(r.Context(), userID, id); err != nil {
		if errors.Is(err, database.ErrGearNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
			return
		}
		slog.Error("delete gear failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete gear", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UpdateGearStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	// Verify ownership before updating status.
	existing, err := h.DB.GetGear(r.Context(), id)
	if err != nil {
		slog.Error("update gear status: get gear failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get gear", r.URL.Path)
		return
	}
	if existing == nil || existing.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	req, ok := httputil.DecodeJSON[struct {
		Status string `json:"status"`
	}](w, r)
	if !ok {
		return
	}
	if req.Status != "active" && req.Status != "stored" && req.Status != "retired" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "status must be active, stored, or retired", r.URL.Path)
		return
	}

	if err := h.DB.UpdateGearStatus(r.Context(), id, userID, req.Status); err != nil {
		if errors.Is(err, database.ErrGearNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
			return
		}
		slog.Error("update gear status failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update gear status", r.URL.Path)
		return
	}

	gear, err := h.DB.GetGear(r.Context(), id)
	if err != nil {
		slog.Error("get gear after status update failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get gear", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, gear)
}

func (h *Handler) CreateComponent(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	gearID := chi.URLParam(r, "id")

	// Verify gear exists and belongs to the user
	gear, err := h.DB.GetGear(r.Context(), gearID)
	if err != nil {
		slog.Error("get gear for component failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get gear", r.URL.Path)
		return
	}
	if gear == nil || gear.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	create, ok := httputil.DecodeJSON[model.GearComponentCreate](w, r)
	if !ok {
		return
	}
	if create.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if create.Type == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "type is required", r.URL.Path)
		return
	}

	component, err := h.DB.CreateComponent(r.Context(), gearID, &create)
	if err != nil {
		slog.Error("create component failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create component", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, component)
}

func (h *Handler) UpdateComponent(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.DB.VerifyComponentOwnership(r.Context(), id, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
		return
	}

	type updateComponentReq struct {
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
	req, ok := httputil.DecodeJSON[updateComponentReq](w, r)
	if !ok {
		return
	}

	component, err := h.DB.UpdateComponent(r.Context(), id, req.Name, req.Type, req.Brand, req.Model, req.Notes, req.IsRetired, req.PurchasePrice, req.PurchaseDate, req.SerialNumber, req.WeightG)
	if err != nil {
		slog.Error("update component failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update component", r.URL.Path)
		return
	}
	if component == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, component)
}

func (h *Handler) DeleteComponent(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.DB.VerifyComponentOwnership(r.Context(), id, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
		return
	}

	// Clean up ShadowFS files for component documents before DB delete.
	docs, docsErr := h.DB.ListComponentDocuments(r.Context(), id)
	if docsErr == nil {
		for _, doc := range docs {
			if doc.FileID != nil && *doc.FileID != "" && h.ShadowFS != nil {
				if delErr := h.ShadowFS.Delete(r.Context(), userID, *doc.FileID); delErr != nil {
					slog.Warn("delete component: shadowfs delete document failed",
						"component_id", id, "doc_id", doc.ID, "file_id", *doc.FileID, "error", delErr)
				}
			}
		}
	}

	if err := h.DB.DeleteComponent(r.Context(), id); err != nil {
		if errors.Is(err, database.ErrComponentNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
			return
		}
		slog.Error("delete component failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete component", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) TransferComponent(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	componentID := chi.URLParam(r, "id")
	if err := h.DB.VerifyComponentOwnership(r.Context(), componentID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
		return
	}

	req, ok := httputil.DecodeJSON[struct {
		ToGearID string `json:"to_gear_id"`
	}](w, r)
	if !ok {
		return
	}
	if req.ToGearID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "to_gear_id is required", r.URL.Path)
		return
	}
	if err := h.DB.VerifyGearOwnership(r.Context(), req.ToGearID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "target gear not found", r.URL.Path)
		return
	}

	// Get the component to find current gear
	history, err := h.DB.GetComponentHistory(r.Context(), componentID)
	if err != nil {
		slog.Error("get component history for transfer failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get component", r.URL.Path)
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
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "target gear not found", r.URL.Path)
			return
		}
		// Use a simple query to get the current gear_id
		fromGearID = "" // Will still work with transfer
	}

	if err := h.DB.TransferComponent(r.Context(), componentID, fromGearID, req.ToGearID); err != nil {
		slog.Error("transfer component failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to transfer component", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetComponentHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	componentID := chi.URLParam(r, "id")
	if err := h.DB.VerifyComponentOwnership(r.Context(), componentID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
		return
	}

	history, err := h.DB.GetComponentHistory(r.Context(), componentID)
	if err != nil {
		slog.Error("get component history failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get component history", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, history)
}

func (h *Handler) GetChainRotation(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	gearID := chi.URLParam(r, "id")
	if err := h.DB.VerifyGearOwnership(r.Context(), gearID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	group, err := h.DB.GetChainRotation(r.Context(), gearID)
	if err != nil {
		slog.Error("get chain rotation failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get chain rotation", r.URL.Path)
		return
	}
	if group == nil {
		httputil.WriteJSON(w, http.StatusOK, nil)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, group)
}

func (h *Handler) CreateChainRotation(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	gearID := chi.URLParam(r, "id")

	create, ok := httputil.DecodeJSON[database.ChainRotationGroupCreate](w, r)
	if !ok {
		return
	}
	if create.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if len(create.ComponentIDs) < 2 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "at least 2 components required", r.URL.Path)
		return
	}

	group, err := h.DB.CreateChainRotation(r.Context(), userID, gearID, &create)
	if err != nil {
		slog.Error("create chain rotation failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create chain rotation", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, group)
}

func (h *Handler) SwapChain(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	gearID := chi.URLParam(r, "id")
	if err := h.DB.VerifyGearOwnership(r.Context(), gearID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	// Find the chain rotation group for this gear
	group, err := h.DB.GetChainRotation(r.Context(), gearID)
	if err != nil {
		slog.Error("get chain rotation for swap failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get chain rotation", r.URL.Path)
		return
	}
	if group == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "chain rotation not found for this gear", r.URL.Path)
		return
	}

	if err := h.DB.SwapChain(r.Context(), group.ID); err != nil {
		slog.Error("swap chain failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to swap chain", r.URL.Path)
		return
	}

	// Return updated group
	updated, err := h.DB.GetChainRotation(r.Context(), gearID)
	if err != nil {
		slog.Error("get chain rotation after swap failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get chain rotation", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, updated)
}

func (h *Handler) UpdateChainRotation(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	groupID := chi.URLParam(r, "id")
	if err := h.DB.VerifyChainRotationOwnership(r.Context(), groupID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "chain rotation not found", r.URL.Path)
		return
	}

	req, ok := httputil.DecodeJSON[struct {
		SwapIntervalKM *float64 `json:"swap_interval_km"`
		SwapIntervalHM *float64 `json:"swap_interval_hm"`
	}](w, r)
	if !ok {
		return
	}

	if err := h.DB.UpdateChainRotation(r.Context(), groupID, req.SwapIntervalKM, req.SwapIntervalHM); err != nil {
		slog.Error("update chain rotation failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update chain rotation", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteChainRotation(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	groupID := chi.URLParam(r, "id")
	if err := h.DB.VerifyChainRotationOwnership(r.Context(), groupID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "chain rotation not found", r.URL.Path)
		return
	}

	if err := h.DB.DeleteChainRotation(r.Context(), groupID); err != nil {
		if errors.Is(err, database.ErrChainRotationNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "chain rotation not found", r.URL.Path)
			return
		}
		slog.Error("delete chain rotation failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete chain rotation", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListServiceEvents(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	componentID := chi.URLParam(r, "id")
	if err := h.DB.VerifyComponentOwnership(r.Context(), componentID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
		return
	}

	events, err := h.DB.ListServiceEvents(r.Context(), componentID)
	if err != nil {
		slog.Error("list service events failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list service events", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, events)
}

func (h *Handler) CreateServiceEvent(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	componentID := chi.URLParam(r, "id")
	if err := h.DB.VerifyComponentOwnership(r.Context(), componentID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
		return
	}

	create, ok := httputil.DecodeJSON[database.ServiceEventCreate](w, r)
	if !ok {
		return
	}
	if create.PerformedAt == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "performed_at is required", r.URL.Path)
		return
	}

	event, err := h.DB.CreateServiceEvent(r.Context(), componentID, &create)
	if err != nil {
		slog.Error("create service event failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create service event", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, event)
}

func (h *Handler) DeleteServiceEvent(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	eventID := chi.URLParam(r, "id")
	if err := h.DB.VerifyServiceEventOwnership(r.Context(), eventID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "service event not found", r.URL.Path)
		return
	}

	// Clean up document files linked to this service event before DB delete
	// (ON DELETE CASCADE removes gear_documents rows; files must be removed first).
	docEntries, listErr := h.DB.ListGearDocumentFileIDsByServiceEventID(r.Context(), eventID)
	if listErr != nil {
		slog.Warn("delete service event: list document file ids failed", "event_id", eventID, "error", listErr)
	}
	for _, e := range docEntries {
		if h.ShadowFS != nil && e.FileID != "" {
			if delErr := h.ShadowFS.Delete(r.Context(), userID, e.FileID); delErr != nil {
				slog.Warn("delete service event: shadowfs delete document failed",
					"event_id", eventID, "file_id", e.FileID, "error", delErr)
			}
		}
	}

	if err := h.DB.DeleteServiceEvent(r.Context(), eventID); err != nil {
		if errors.Is(err, database.ErrServiceEventNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "service event not found", r.URL.Path)
			return
		}
		slog.Error("delete service event failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete service event", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListComponentPresets(w http.ResponseWriter, r *http.Request) {
	_, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	gearType := r.URL.Query().Get("gear_type")

	presets, err := h.DB.ListComponentPresets(r.Context(), gearType)
	if err != nil {
		slog.Error("list component presets failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list component presets", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, presets)
}

// --- Bike Setups ---

func (h *Handler) ListBikeSetups(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	gearID := chi.URLParam(r, "id")
	if err := h.DB.VerifyGearOwnership(r.Context(), gearID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	setups, err := h.DB.ListBikeSetups(r.Context(), gearID)
	if err != nil {
		slog.Error("list bike setups failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list bike setups", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, setups)
}

func (h *Handler) CreateBikeSetup(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	gearID := chi.URLParam(r, "id")
	if err := h.DB.VerifyGearOwnership(r.Context(), gearID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	create, ok := httputil.DecodeJSON[model.BikeSetupCreate](w, r)
	if !ok {
		return
	}
	if create.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}

	setup, err := h.DB.CreateBikeSetup(r.Context(), gearID, &create)
	if err != nil {
		slog.Error("create bike setup failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create bike setup", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, setup)
}

func (h *Handler) UpdateBikeSetup(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.DB.VerifyBikeSetupOwnership(r.Context(), id, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "bike setup not found", r.URL.Path)
		return
	}

	update, ok := httputil.DecodeJSON[model.BikeSetupUpdate](w, r)
	if !ok {
		return
	}

	setup, err := h.DB.UpdateBikeSetup(r.Context(), id, &update)
	if err != nil {
		slog.Error("update bike setup failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update bike setup", r.URL.Path)
		return
	}
	if setup == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "bike setup not found", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, setup)
}

func (h *Handler) DeleteBikeSetup(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.DB.VerifyBikeSetupOwnership(r.Context(), id, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "bike setup not found", r.URL.Path)
		return
	}

	if err := h.DB.DeleteBikeSetup(r.Context(), id); err != nil {
		if errors.Is(err, database.ErrBikeSetupNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "bike setup not found", r.URL.Path)
			return
		}
		slog.Error("delete bike setup failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete bike setup", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ActivateBikeSetup(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.DB.VerifyBikeSetupOwnership(r.Context(), id, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "bike setup not found", r.URL.Path)
		return
	}

	setup, err := h.DB.ActivateBikeSetup(r.Context(), id)
	if err != nil {
		slog.Error("activate bike setup failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to activate bike setup", r.URL.Path)
		return
	}
	if setup == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "bike setup not found", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, setup)
}

func (h *Handler) GetMaintenanceAlerts(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	alerts, err := h.DB.GetMaintenanceAlerts(r.Context(), userID)
	if err != nil {
		slog.Error("get maintenance alerts failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get maintenance alerts", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, alerts)
}

// --- Gear Documents ---

// docUploadConfig holds the caller-specific parameters for processDocumentUpload.
type docUploadConfig struct {
	// parentType is the ShadowFS parent type string ("gear" or "gear_component").
	parentType string
	// parentID is the gear or component UUID.
	parentID string
	// logPrefix is used in slog messages (e.g. "gear document", "component document").
	logPrefix string
	// readDocType controls whether doc_type is read from the form value (gear) or
	// derived solely from the content type (component).
	readDocType bool
	// buildDoc returns a GearDocument with the caller-specific foreign-key field set.
	buildDoc func(userID, docType string, titlePtr *string, filename, contentType string, size int64, hashStr string) *model.GearDocument
	// dupCheck returns (exists, error) for the caller-specific duplicate query.
	dupCheck func(userID, hashStr string) (bool, error)
	// extractPDF enables PDF text extraction + DB update.
	extractPDF bool
	// publishEvent enables the NATS DocumentUploaded event.
	publishEvent bool
}

// processDocumentUpload implements the shared multipart upload pipeline used by
// UploadGearDocument and UploadComponentDocument. It writes the HTTP response
// directly and returns false on any error so callers can return immediately.
func (h *Handler) processDocumentUpload(w http.ResponseWriter, r *http.Request, userID string, cfg docUploadConfig) {
	// Limit to 30MB
	r.Body = http.MaxBytesReader(w, r.Body, 30<<20)
	if err := r.ParseMultipartForm(30 << 20); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "File too large", "Maximum file size is 30MB", r.URL.Path)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Missing file", "No file provided", r.URL.Path)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Read error", "Failed to read file", r.URL.Path)
		return
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	allowed := map[string]bool{
		"image/jpeg":      true,
		"image/png":       true,
		"image/webp":      true,
		"image/gif":       true,
		"image/tiff":      true,
		"image/bmp":       true,
		"application/pdf": true,
	}
	if !allowed[contentType] {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid file type", "Allowed: JPEG, PNG, WebP, GIF, TIFF, BMP, PDF", r.URL.Path)
		return
	}

	// Check pixel dimensions for images (max 200 megapixels)
	if strings.HasPrefix(contentType, "image/") {
		cfg2, _, decErr := image.DecodeConfig(bytes.NewReader(data))
		if decErr == nil && cfg2.Width > 0 && cfg2.Height > 0 {
			if int64(cfg2.Width)*int64(cfg2.Height) > 200_000_000 {
				httputil.WriteError(w, http.StatusBadRequest, "Image too large", "Maximum 200 megapixels per image", r.URL.Path)
				return
			}
		}
	}

	// SHA256 hash
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	// Duplikat-Prüfung (per gear/component, not global — same file allowed on different items)
	if exists, err := cfg.dupCheck(userID, hashStr); err == nil && exists {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "duplicate"})
		return
	}

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
		case "image/gif":
			ext = ".gif"
		case "image/tiff":
			ext = ".tiff"
		case "image/bmp":
			ext = ".bmp"
		case "application/pdf":
			ext = ".pdf"
		}
	}
	_ = ext // computed for potential future use (e.g. forced filename)

	var docType string
	if cfg.readDocType {
		docType = r.FormValue("doc_type")
	}
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

	// Upload to ShadowFS — images go to media/, everything else to documents/.
	fileType := "document"
	if strings.HasPrefix(contentType, "image/") {
		fileType = "media"
	}
	var storedFileID string
	if h.ShadowFS != nil {
		rec, putErr := h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
			UserID:      userID,
			FileType:    fileType,
			ContentType: contentType,
			FileName:    header.Filename,
			SizeBytes:   int64(len(data)),
			FileHash:    hashStr,
			ParentType:  shadowfs.Ptr(cfg.parentType),
			ParentID:    shadowfs.Ptr(cfg.parentID),
			Tags: map[string]string{
				"doc_type": docType,
			},
		}, bytes.NewReader(data))
		if putErr != nil {
			slog.Error(cfg.logPrefix+": shadowfs put failed", "error", putErr)
			httputil.WriteError(w, http.StatusInternalServerError, "Upload failed", "Failed to store file", r.URL.Path)
			return
		}
		storedFileID = rec.ID
	}

	doc := cfg.buildDoc(userID, docType, titlePtr, header.Filename, contentType, int64(len(data)), hashStr)

	result, err := h.DB.InsertGearDocument(r.Context(), doc)
	if err != nil {
		// Clean up the already-stored file to avoid orphans.
		if storedFileID != "" && h.ShadowFS != nil {
			if delErr := h.ShadowFS.Delete(r.Context(), userID, storedFileID); delErr != nil {
				slog.Warn(cfg.logPrefix+": shadowfs cleanup after db failure failed",
					"file_id", storedFileID, "error", delErr)
			}
		}
		slog.Error("insert "+cfg.logPrefix+" failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Database error", "Failed to save document", r.URL.Path)
		return
	}

	// Persist the ShadowFS file_id on the row.
	if storedFileID != "" {
		if setErr := h.DB.UpdateGearDocumentFileID(r.Context(), result.ID, storedFileID); setErr != nil {
			slog.Warn(cfg.logPrefix+": failed to set file_id", "doc_id", result.ID, "file_id", storedFileID, "error", setErr)
		} else {
			result.FileID = &storedFileID
		}
	}

	// PDF text extraction via pdfcpu (best-effort)
	var extractedText *string
	var isTextPDF bool
	if cfg.extractPDF && contentType == "application/pdf" {
		if text := extractPDFText(data); text != "" {
			extractedText = &text
			isTextPDF = true
			if err := h.DB.UpdateGearDocumentText(r.Context(), result.ID, text); err != nil {
				slog.Warn("failed to update extracted text", "doc_id", result.ID, "error", err)
			}
		}
	}

	// NATS event (graceful - no error if NATS unavailable)
	if cfg.publishEvent && h.Events != nil {
		evt := event.DocumentUploadedEvent{
			DocID:         result.ID,
			GearID:        result.GearID,
			ComponentID:   result.ComponentID,
			UserID:        userID,
			ContentType:   result.ContentType,
			IsTextPDF:     isTextPDF,
			ExtractedText: extractedText,
			FileID:        storedFileID,
		}
		if err := h.Events.PublishDocumentUploaded(evt); err != nil {
			slog.Warn("failed to publish document uploaded event", "error", err)
		}
	}

	httputil.WriteJSON(w, http.StatusCreated, result)
}

func (h *Handler) UploadGearDocument(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	gearID := chi.URLParam(r, "id")
	if err := h.DB.VerifyGearOwnership(r.Context(), gearID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	h.processDocumentUpload(w, r, userID, docUploadConfig{
		parentType:   "gear",
		parentID:     gearID,
		logPrefix:    "gear document",
		readDocType:  true,
		extractPDF:   true,
		publishEvent: true,
		dupCheck: func(uid, hash string) (bool, error) {
			return h.DB.GearDocumentExistsByHashAndGear(r.Context(), uid, hash, gearID)
		},
		buildDoc: func(uid, docType string, titlePtr *string, filename, contentType string, size int64, hashStr string) *model.GearDocument {
			return &model.GearDocument{
				UserID:           uid,
				GearID:           &gearID,
				DocType:          docType,
				Title:            titlePtr,
				OriginalFilename: filename,
				ContentType:      contentType,
				FileSizeBytes:    size,
				FileHash:         &hashStr,
			}
		},
	})
}

// generatePDFThumbnail rendert die erste Seite eines PDFs als JPEG.
func generatePDFThumbnail(pdfData []byte, maxWidth int) ([]byte, error) {
	tmpPDF, err := os.CreateTemp("", "skadia-pdf-*.pdf")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpPDF.Name())
	if _, err := tmpPDF.Write(pdfData); err != nil {
		tmpPDF.Close()
		return nil, err
	}
	tmpPDF.Close()

	tmpOut, err := os.CreateTemp("", "skadia-thumb-*")
	if err != nil {
		return nil, err
	}
	tmpOutBase := strings.TrimSuffix(tmpOut.Name(), filepath.Ext(tmpOut.Name()))
	tmpOut.Close()
	os.Remove(tmpOut.Name())

	cmd := exec.Command("pdftoppm",
		"-f", "1", "-l", "1",
		"-jpeg",
		"-scale-to", fmt.Sprintf("%d", maxWidth),
		tmpPDF.Name(), tmpOutBase,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftoppm: %w: %s", err, out)
	}

	outPath := tmpOutBase + "-1.jpg"
	defer os.Remove(outPath)
	return os.ReadFile(outPath)
}

// extractPDFText uses pdfcpu to extract content streams from PDF data.
// Returns concatenated text content, or empty string on failure.
func extractPDFText(pdfData []byte) string {
	tmpDir, err := os.MkdirTemp("", "pdfcpu-extract-*")
	if err != nil {
		slog.Warn("pdfcpu: failed to create temp dir", "error", err)
		return ""
	}
	defer os.RemoveAll(tmpDir)

	// Write PDF to temp file
	tmpFile, err := os.CreateTemp(tmpDir, "doc-*.pdf")
	if err != nil {
		slog.Warn("pdfcpu: failed to create temp file", "error", err)
		return ""
	}
	if _, err := tmpFile.Write(pdfData); err != nil {
		tmpFile.Close()
		return ""
	}
	tmpFile.Close()

	conf := pdfcpumodel.NewDefaultConfiguration()
	outDir, err := os.MkdirTemp("", "pdfcpu-out-*")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(outDir)

	if err := pdfcpuapi.ExtractContentFile(tmpFile.Name(), outDir, nil, conf); err != nil {
		slog.Debug("pdfcpu: content extraction failed", "error", err)
		return ""
	}

	// Read all extracted content files
	entries, err := os.ReadDir(outDir)
	if err != nil || len(entries) == 0 {
		return ""
	}

	var parts []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(outDir, e.Name()))
		if err != nil {
			continue
		}
		parts = append(parts, string(content))
	}

	text := strings.Join(parts, "\n")
	if len(text) < 100 {
		return "" // not enough text to be useful
	}
	return text
}

func (h *Handler) ListGearDocuments(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	gearID := chi.URLParam(r, "id")
	if err := h.DB.VerifyGearOwnership(r.Context(), gearID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	docs, err := h.DB.ListGearDocuments(r.Context(), gearID)
	if err != nil {
		slog.Error("list gear documents failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Database error", "Failed to list documents", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, docs)
}

// UploadComponentDocument uploads a photo/document for a specific gear component.
func (h *Handler) UploadComponentDocument(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	componentID := chi.URLParam(r, "id")
	if err := h.DB.VerifyComponentOwnership(r.Context(), componentID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
		return
	}

	h.processDocumentUpload(w, r, userID, docUploadConfig{
		parentType:   "gear_component",
		parentID:     componentID,
		logPrefix:    "component document",
		readDocType:  false,
		extractPDF:   false,
		publishEvent: false,
		dupCheck: func(uid, hash string) (bool, error) {
			return h.DB.GearDocumentExistsByHashAndComponent(r.Context(), uid, hash, componentID)
		},
		buildDoc: func(uid, docType string, titlePtr *string, filename, contentType string, size int64, hashStr string) *model.GearDocument {
			return &model.GearDocument{
				UserID:           uid,
				ComponentID:      &componentID,
				DocType:          docType,
				Title:            titlePtr,
				OriginalFilename: filename,
				ContentType:      contentType,
				FileSizeBytes:    size,
				FileHash:         &hashStr,
			}
		},
	})
}

// ListComponentDocuments returns all documents for a gear component.
func (h *Handler) ListComponentDocuments(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	componentID := chi.URLParam(r, "id")
	if err := h.DB.VerifyComponentOwnership(r.Context(), componentID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
		return
	}

	docs, err := h.DB.ListComponentDocuments(r.Context(), componentID)
	if err != nil {
		slog.Error("list component documents failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Database error", "Failed to list documents", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, docs)
}

func (h *Handler) DeleteGearDocument(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	docID := chi.URLParam(r, "docId")
	if err := h.DB.VerifyGearDocumentOwnership(r.Context(), docID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not found", "Document not found", r.URL.Path)
		return
	}

	fileID, err := h.DB.DeleteGearDocument(r.Context(), docID, userID)
	if err != nil {
		slog.Error("delete gear document failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Database error", "Failed to delete document", r.URL.Path)
		return
	}

	// Delete from ShadowFS (best-effort).
	if h.ShadowFS != nil && fileID != "" {
		if delErr := h.ShadowFS.Delete(r.Context(), userID, fileID); delErr != nil {
			slog.Error("shadowfs delete failed", "file_id", fileID, "error", delErr)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DownloadGearDocument(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	docID := chi.URLParam(r, "docId")
	if err := h.DB.VerifyGearDocumentOwnership(r.Context(), docID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not found", "Document not found", r.URL.Path)
		return
	}

	doc, err := h.DB.GetGearDocument(r.Context(), docID)
	if err != nil {
		slog.Error("get gear document failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Database error", "Failed to get document", r.URL.Path)
		return
	}
	if doc == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not found", "Document not found", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", doc.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, doc.OriginalFilename))
	w.Header().Set("Cache-Control", "private, max-age=86400")

	// Serve from ShadowFS.
	if h.ShadowFS != nil && doc.FileID != nil && *doc.FileID != "" {
		rc, getErr := h.ShadowFS.Get(r.Context(), userID, *doc.FileID)
		if getErr != nil {
			slog.Error("shadowfs get failed", "file_id", *doc.FileID, "error", getErr)
			httputil.WriteError(w, http.StatusInternalServerError, "Download failed", "Failed to retrieve file", r.URL.Path)
			return
		}
		defer rc.Close()
		io.Copy(w, rc) //nolint:errcheck
		return
	}

	httputil.WriteError(w, http.StatusNotFound, "Not Found", "file not available", r.URL.Path)
}

// PatchGearReview updates the review status of a gear item.
func (h *Handler) PatchGearReview(w http.ResponseWriter, r *http.Request) {
	gearID := chi.URLParam(r, "id")
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	req, ok := httputil.DecodeJSON[model.GearReviewRequest](w, r)
	if !ok {
		return
	}
	if req.ReviewStatus != "confirmed" && req.ReviewStatus != "rejected" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "review_status must be confirmed or rejected", r.URL.Path)
		return
	}
	if req.ReviewStatus == "rejected" {
		if err := h.DB.DeleteGear(r.Context(), userID, gearID); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete gear", r.URL.Path)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	gear, err := h.DB.UpdateGearReviewStatus(r.Context(), gearID, userID, req.ReviewStatus)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update review status", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, gear)
}

// ListPendingReviewGear returns gear items awaiting user review.
func (h *Handler) ListPendingReviewGear(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	items, err := h.DB.ListPendingReviewGear(r.Context(), userID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list pending review gear", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, items)
}

// GetGearCosts returns the cost summary for a gear item.
func (h *Handler) GetGearCosts(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.DB.VerifyGearOwnership(r.Context(), id, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	summary, err := h.DB.GetGearCostSummary(r.Context(), id)
	if err != nil {
		slog.Error("get gear costs failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get gear costs", r.URL.Path)
		return
	}
	if summary == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, summary)
}
