package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	pdfcpuapi "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpumodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	gearpkg "github.com/skadia-app/skadia/services/api/internal/gear"
	"github.com/skadia-app/skadia/services/api/internal/gearcompat"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

func (h *Handler) ListGear(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	// Lookup-by-name: ?name= returns a single item or 409 on ambiguity.
	if name := r.URL.Query().Get("name"); name != "" {
		candidates, err := h.DB.FindGearByName(r.Context(), userID, name)
		if err != nil {
			slog.Error("find gear by name failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to find gear", r.URL.Path)
			return
		}
		if len(candidates) == 0 {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
			return
		}
		if len(candidates) > 1 {
			httputil.WriteMachineError(w, http.StatusConflict, "gear.name_ambiguous",
				"multiple gear items match the given name",
				map[string]any{"candidates": candidates})
			return
		}
		// Unique match — load full gear item.
		item, err := h.DB.GetGear(r.Context(), candidates[0].ID)
		if err != nil {
			slog.Error("get gear by name failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get gear", r.URL.Path)
			return
		}
		if item == nil || item.UserID != userID {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, item)
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

	// Idempotency-Key replay check.
	if replayed, done := checkIdempotencyKey(w, r, h, userID, "POST /gear"); done {
		if replayed {
			return
		}
	}

	create, ok := decodeJSONIdempotent[model.GearCreate](w, r, h, userID, "POST /gear")
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

	// Validate type against the compiled schema, allowing a small legacy allow-list
	// for types that existed in production before the schema was formalized.
	// Legacy allow-list per Plan 2026-05-08 §10 (gear-rebuild on taxonomy) — backfill migration removes it.
	if !isKnownGearType(create.Type) {
		// Deterministic 4xx: store so replays return 400 without re-validating.
		errBody := httputil.MachineError{Error: "gear.unknown_gear_type", Message: "unknown gear type",
			Details: map[string]any{"type": create.Type}}
		storeIdempotencyResponse(r, h, userID, "POST /gear", http.StatusBadRequest, errBody)
		httputil.WriteMachineError(w, http.StatusBadRequest, errBody.Error, errBody.Message, errBody.Details)
		return
	}

	// Default scope from the gear-type schema when the caller omits it.
	// The DB column is free TEXT; API-layer defaulting ensures new rows are
	// pre-populated with a sensible scope without requiring the caller to know it.
	if create.Scope == "" {
		if gt, ok := gearpkg.LookupGearType(create.Type); ok {
			create.Scope = gt.Scope
		}
	}

	// Scope validation (API layer only — DB column is free TEXT).
	if create.Scope != "" {
		if verr := gearpkg.ValidateScope(create.Scope); verr != nil {
			// Deterministic 4xx: store so replays return 400 without re-validating.
			errBody := httputil.MachineError{Error: verr.Code, Message: verr.Message,
				Details: map[string]any{"field": "scope", "reason": verr.Message}}
			storeIdempotencyResponse(r, h, userID, "POST /gear", http.StatusBadRequest, errBody)
			httputil.WriteMachineError(w, http.StatusBadRequest, errBody.Error, errBody.Message, errBody.Details)
			return
		}
	}

	// Attributes validation against schema.
	if verr := gearpkg.ValidateAttributes(create.Type, create.Attributes); verr != nil {
		// Deterministic 4xx: store so replays return 400 without re-validating.
		errBody := httputil.MachineError{Error: verr.Code, Message: verr.Message,
			Details: map[string]any{"attribute": verr.Field, "reason": verr.Message}}
		storeIdempotencyResponse(r, h, userID, "POST /gear", http.StatusBadRequest, errBody)
		httputil.WriteMachineError(w, http.StatusBadRequest, errBody.Error, errBody.Message, errBody.Details)
		return
	}

	// Modifiers validation.
	if verr := gearpkg.ValidateModifiers(create.Modifiers); verr != nil {
		// Deterministic 4xx: store so replays return 400 without re-validating.
		errBody := httputil.MachineError{Error: verr.Code, Message: verr.Message,
			Details: map[string]any{"modifier": verr.Field, "reason": verr.Message}}
		storeIdempotencyResponse(r, h, userID, "POST /gear", http.StatusBadRequest, errBody)
		httputil.WriteMachineError(w, http.StatusBadRequest, errBody.Error, errBody.Message, errBody.Details)
		return
	}

	// Normalize applicable_sport_predicates (validates against sport taxonomy).
	// Three-state pointer semantics: prefer ApplicableSportPredicates, fall back
	// to DefaultSportPredicates. When both are nil (field absent or null — NOT
	// explicit []), compute defaults from the taxonomy projection so new gear
	// items are pre-populated with sensible sport bindings. Explicit [] (non-nil
	// pointer to empty slice) is respected as "user wants no sport filter".
	var rawPredicates []gearpkg.GearDefaultPredicate
	if create.ApplicableSportPredicates != nil {
		rawPredicates = *create.ApplicableSportPredicates
	} else if create.DefaultSportPredicates != nil {
		rawPredicates = *create.DefaultSportPredicates
	} else {
		computed, computeErr := gearpkg.ComputeApplicableSportPredicates(create.Type, create.Modifiers)
		if computeErr != nil {
			slog.Error("compute applicable sport predicates failed", "error", computeErr, "type", create.Type)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to compute sport predicates", r.URL.Path)
			return
		}
		rawPredicates = computed
	}
	predicates, ok := h.normalizeGearApplicablePredicatesIdempotent(w, r, userID, "POST /gear", rawPredicates)
	if !ok {
		return
	}
	create.ApplicableSportPredicates = &predicates
	create.DefaultSportPredicates = &predicates

	gear, err := h.DB.CreateGear(r.Context(), userID, &create)
	if err != nil {
		slog.Error("create gear failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create gear", r.URL.Path)
		return
	}

	storeIdempotencyResponse(r, h, userID, "POST /gear", http.StatusCreated, gear)
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

	// Validate the new type when the caller is changing it.
	if update.Type != nil && !isKnownGearType(*update.Type) {
		httputil.WriteMachineError(w, http.StatusBadRequest, "gear.unknown_gear_type",
			"unknown gear type",
			map[string]any{"type": *update.Type})
		return
	}

	// Scope validation.
	if update.Scope != nil && *update.Scope != "" {
		if verr := gearpkg.ValidateScope(*update.Scope); verr != nil {
			httputil.WriteMachineError(w, http.StatusBadRequest, verr.Code, verr.Message,
				map[string]any{"field": "scope", "reason": verr.Message})
			return
		}
	}

	// Attributes validation — use updated type if present, else existing type.
	typeKey := existing.Type
	if update.Type != nil {
		typeKey = *update.Type
	}
	if update.Attributes != nil {
		if verr := gearpkg.ValidateAttributes(typeKey, update.Attributes); verr != nil {
			httputil.WriteMachineError(w, http.StatusBadRequest, verr.Code, verr.Message,
				map[string]any{"attribute": verr.Field, "reason": verr.Message})
			return
		}
	} else if update.Type != nil && *update.Type != existing.Type {
		// Type is changing but no new attributes were provided: re-validate the
		// existing attributes against the new type so stale keys are rejected.
		if verr := gearpkg.ValidateAttributes(typeKey, existing.Attributes); verr != nil {
			httputil.WriteMachineError(w, http.StatusUnprocessableEntity, verr.Code, verr.Message,
				map[string]any{"attribute": verr.Field, "reason": verr.Message})
			return
		}
	}

	// Modifiers validation.
	if update.Modifiers != nil {
		if verr := gearpkg.ValidateModifiers(update.Modifiers); verr != nil {
			httputil.WriteMachineError(w, http.StatusBadRequest, verr.Code, verr.Message,
				map[string]any{"modifier": verr.Field, "reason": verr.Message})
			return
		}
	}

	// Normalize applicable_sport_predicates.
	// Three-state pointer semantics: prefer ApplicableSportPredicates, fall back
	// to DefaultSportPredicates. A nil pointer means the field was not sent —
	// leave the DB column unchanged. A non-nil pointer to an empty slice
	// explicitly clears sport predicates.
	var rawPredicatesForNorm []gearpkg.GearDefaultPredicate
	updatePredicatesInHandler := false
	if update.ApplicableSportPredicates != nil {
		rawPredicatesForNorm = *update.ApplicableSportPredicates
		updatePredicatesInHandler = true
	} else if update.DefaultSportPredicates != nil {
		rawPredicatesForNorm = *update.DefaultSportPredicates
		updatePredicatesInHandler = true
	}
	if updatePredicatesInHandler {
		predicates, ok := normalizeGearApplicablePredicatesForRequest(w, r, rawPredicatesForNorm)
		if !ok {
			return
		}
		update.ApplicableSportPredicates = &predicates
		update.DefaultSportPredicates = &predicates
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

func normalizeGearDefaultPredicatesForRequest(w http.ResponseWriter, r *http.Request, values []gearpkg.GearDefaultPredicate) ([]gearpkg.GearDefaultPredicate, bool) {
	predicates, err := gearpkg.NormalizeGearDefaultPredicates(values)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return nil, false
	}
	return predicates, true
}

func normalizeGearApplicablePredicatesForRequest(w http.ResponseWriter, r *http.Request, values []gearpkg.GearDefaultPredicate) ([]gearpkg.GearDefaultPredicate, bool) {
	predicates, err := gearpkg.NormalizeGearDefaultPredicates(values)
	if err != nil {
		httputil.WriteMachineError(w, http.StatusBadRequest, "gear.unknown_sport_predicate", err.Error(),
			map[string]any{"reason": err.Error()})
		return nil, false
	}
	return predicates, true
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
		Name              *string  `json:"name,omitempty"`
		Type              *string  `json:"type,omitempty"`
		Brand             *string  `json:"brand,omitempty"`
		Model             *string  `json:"model,omitempty"`
		Notes             *string  `json:"notes,omitempty"`
		IsRetired         *bool    `json:"is_retired,omitempty"`
		PurchasePrice     *float64 `json:"purchase_price,omitempty"`
		PurchaseDate      *string  `json:"purchase_date,omitempty"`
		SerialNumber      *string  `json:"serial_number,omitempty"`
		WeightG           *int     `json:"weight_g,omitempty"`
		PresetGearTypeKey *string  `json:"preset_gear_type_key,omitempty"`
	}
	req, ok := httputil.DecodeJSON[updateComponentReq](w, r)
	if !ok {
		return
	}

	component, err := h.DB.UpdateComponent(r.Context(), id, req.Name, req.Type, req.Brand, req.Model, req.Notes, req.IsRetired, req.PurchasePrice, req.PurchaseDate, req.SerialNumber, req.WeightG, req.PresetGearTypeKey)
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

func (h *Handler) RemoveComponent(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.DB.VerifyComponentOwnership(r.Context(), id, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
		return
	}
	if err := h.DB.RemoveComponent(r.Context(), id); err != nil {
		if errors.Is(err, database.ErrComponentNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
			return
		}
		slog.Error("remove component failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to remove component", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UpdateComponentStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.DB.VerifyComponentOwnership(r.Context(), id, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
		return
	}
	req, ok := httputil.DecodeJSON[struct {
		Status string `json:"status"`
	}](w, r)
	if !ok {
		return
	}
	if err := h.DB.UpdateComponentStatus(r.Context(), id, userID, req.Status); err != nil {
		if errors.Is(err, database.ErrComponentNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
			return
		}
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
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

	eligibility, err := h.DB.GetComponentTransferEligibilityOwnerVerified(r.Context(), componentID, req.ToGearID)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "component or target gear not found", r.URL.Path)
		return
	}
	schema, err := h.DB.GetEffectiveGearSchema(r.Context(), userID)
	if err != nil {
		slog.Error("load gear schema for component transfer failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load gear schema", r.URL.Path)
		return
	}
	// preset_gear_type_key check is implicit via gearcompat.CanTransferComponent:
	// when the component has a preset, the target must match that preset or a
	// descendant of it; otherwise the type/source-type fallback applies.
	if !gearcompat.CanTransferComponent(eligibility.Component, eligibility.TargetGear, schema) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
			"component is not compatible with target gear type", r.URL.Path)
		return
	}

	fromGearID := eligibility.FromGearID

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

	history, err := h.DB.GetComponentHistoryOwnerVerified(r.Context(), componentID)
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
	if err := h.DB.VerifyGearOwnership(r.Context(), gearID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

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
		if errors.Is(err, database.ErrGearNotFound) || errors.Is(err, database.ErrComponentNotFound) {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "chain rotation members must belong to this gear", r.URL.Path)
			return
		}
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
		Name                  *string  `json:"name,omitempty"`
		SwapIntervalKM        *float64 `json:"swap_interval_km"`
		SwapIntervalHM        *float64 `json:"swap_interval_hm"`
		SwapIntervalMetricKey *string  `json:"swap_interval_metric_key"`
		SwapIntervalValue     *float64 `json:"swap_interval_value"`
	}](w, r)
	if !ok {
		return
	}

	// Trim + length-validate name when present. An empty trimmed name is
	// rejected because the column is NOT NULL and an empty UI value is almost
	// certainly a mistake on the client side.
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name must not be empty", r.URL.Path)
			return
		}
		if len(trimmed) > 100 {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name too long (max 100 characters)", r.URL.Path)
			return
		}
		req.Name = &trimmed
	}

	if err := h.DB.UpdateChainRotation(r.Context(), groupID, req.Name, req.SwapIntervalKM, req.SwapIntervalHM, req.SwapIntervalMetricKey, req.SwapIntervalValue); err != nil {
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

func (h *Handler) ListServiceIntervals(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	componentID := chi.URLParam(r, "id")
	if err := h.DB.VerifyComponentOwnership(r.Context(), componentID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
		return
	}
	intervals, err := h.DB.ListServiceIntervals(r.Context(), componentID)
	if err != nil {
		slog.Error("list service intervals failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list service intervals", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, intervals)
}

func (h *Handler) CreateServiceInterval(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	componentID := chi.URLParam(r, "id")
	if err := h.DB.VerifyComponentOwnership(r.Context(), componentID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
		return
	}
	create, ok := httputil.DecodeJSON[model.GearServiceIntervalCreate](w, r)
	if !ok {
		return
	}
	if create.Name == "" || create.MetricKey == "" || create.IntervalValue <= 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name, metric_key, and positive interval_value are required", r.URL.Path)
		return
	}
	interval, err := h.DB.CreateServiceInterval(r.Context(), componentID, &create)
	if err != nil {
		slog.Error("create service interval failed", "error", err)
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "failed to create service interval", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, interval)
}

func (h *Handler) UpdateServiceInterval(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	intervalID := chi.URLParam(r, "id")
	if err := h.DB.VerifyServiceIntervalOwnership(r.Context(), intervalID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "service interval not found", r.URL.Path)
		return
	}
	update, ok := httputil.DecodeJSON[model.GearServiceIntervalUpdate](w, r)
	if !ok {
		return
	}
	interval, err := h.DB.UpdateServiceInterval(r.Context(), intervalID, &update)
	if err != nil {
		slog.Error("update service interval failed", "error", err)
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "failed to update service interval", r.URL.Path)
		return
	}
	if interval == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "service interval not found", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, interval)
}

func (h *Handler) DeleteServiceInterval(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	intervalID := chi.URLParam(r, "id")
	if err := h.DB.VerifyServiceIntervalOwnership(r.Context(), intervalID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "service interval not found", r.URL.Path)
		return
	}
	if err := h.DB.DeleteServiceInterval(r.Context(), intervalID); err != nil {
		slog.Error("delete service interval failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete service interval", r.URL.Path)
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

	presets, err := h.DB.ListComponentPresets(r.Context(), "")
	if err != nil {
		slog.Error("list component presets failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list component presets", r.URL.Path)
		return
	}

	presets = filterComponentPresetsForGearType(presets, gearType)
	httputil.WriteJSON(w, http.StatusOK, presets)
}

func filterComponentPresetsForGearType(presets []model.ComponentTypePreset, gearType string) []model.ComponentTypePreset {
	componentLabels := make(map[string]struct{}, len(gearpkg.Schema.ComponentTypes))
	for _, ct := range gearpkg.Schema.ComponentTypes {
		componentLabels[strings.ToLower(ct.LabelDE)] = struct{}{}
	}

	targetScope := ""
	if gearType != "" {
		for _, gt := range gearpkg.Schema.GearTypes {
			if gt.Key == gearType {
				targetScope = gt.Scope
				break
			}
		}
	}

	gearLabels := make(map[string]struct{})
	for _, gt := range gearpkg.Schema.GearTypes {
		if targetScope != "" && gt.Scope != targetScope {
			continue
		}
		gearLabels[strings.ToLower(gt.LabelDE)] = struct{}{}
	}

	out := make([]model.ComponentTypePreset, 0, len(presets))
	for _, preset := range presets {
		if gearType != "" && preset.GearType != nil && *preset.GearType != "" &&
			!gearpkg.Schema.GearTypeAppliesToTarget(*preset.GearType, gearType) {
			continue
		}

		nameKey := strings.ToLower(preset.Name)
		_, isGearTypeLabel := gearLabels[nameKey]
		_, isComponentLabel := componentLabels[nameKey]
		if isGearTypeLabel && !isComponentLabel {
			continue
		}

		out = append(out, preset)
	}
	return out
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

	// activation does NOT mutate gear_components; component_snapshot is read-only history.
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

func classifyGearDocumentContentType(declared string, data []byte) string {
	detected := http.DetectContentType(data)
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("%PDF-")) {
		return "application/pdf"
	}
	if strings.HasPrefix(detected, "image/") {
		return detected
	}
	if declared != "" {
		return declared
	}
	return detected
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

	contentType := classifyGearDocumentContentType(header.Header.Get("Content-Type"), data)

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

	importType := model.ImportTypeDocument
	if strings.HasPrefix(contentType, "image/") {
		importType = model.ImportTypePhoto
	}
	shouldProcessDocument := cfg.extractPDF && cfg.publishEvent && contentType == "application/pdf" && h.DocumentUploadedPublisher != nil &&
		h.featureNotAvailableReasonWithContext(r.Context(), capabilities.KeyDocumentML) == ""
	parentMetaKey := "gear_id"
	if cfg.parentType == "gear_component" {
		parentMetaKey = "component_id"
	}
	tracker := directUploadTracker(r.Context(), h.DB, userID, header.Filename, int64(len(data)), importType, hashStr, map[string]any{
		"target_type":  cfg.parentType + "_document",
		parentMetaKey:  cfg.parentID,
		"doc_type":     docType,
		"content_type": contentType,
		"steps":        []directImportStep{{Key: "upload", Label: "Upload", Status: "pending"}},
	})

	// Duplikat-Prüfung (per gear/component, not global — same file allowed on different items)
	if exists, err := cfg.dupCheck(userID, hashStr); err == nil && exists {
		tracker.Skipped(r.Context(), "duplicate", nil)
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

	title := r.FormValue("title")
	var titlePtr *string
	if title != "" {
		titlePtr = &title
	}

	var documentDatePtr *string
	if documentDate := strings.TrimSpace(r.FormValue("document_date")); documentDate != "" {
		documentDatePtr = &documentDate
	}
	var documentAmountPtr *float64
	if amountValue := strings.TrimSpace(r.FormValue("document_amount")); amountValue != "" {
		amount, parseErr := strconv.ParseFloat(amountValue, 64)
		if parseErr != nil || amount < 0 {
			err := fmt.Errorf("document_amount must be a non-negative number")
			tracker.Error(r.Context(), err)
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
			return
		}
		documentAmountPtr = &amount
	}
	var documentCurrencyPtr *string
	if currency := strings.TrimSpace(r.FormValue("document_currency")); currency != "" {
		documentCurrencyPtr = &currency
	}

	// Upload to ShadowFS — images go to media/, everything else to documents/.
	fileType := "document"
	if strings.HasPrefix(contentType, "image/") {
		fileType = "media"
	}
	var storedFileID string
	var storedFileType string
	var storedStorageBackend string
	var storedStoragePath string
	var storedSizeBytes int64
	if h.ShadowFS != nil {
		rec, putErr := h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
			UserID:      userID,
			FileType:    fileType,
			ContentType: contentType,
			FileName:    header.Filename,
			SizeBytes:   int64(len(data)),
			FileHash:    hashStr,
			ParentType:  &cfg.parentType,
			ParentID:    &cfg.parentID,
			Tags: map[string]string{
				"doc_type": docType,
			},
		}, bytes.NewReader(data))
		if putErr != nil {
			tracker.Error(r.Context(), putErr)
			slog.Error(cfg.logPrefix+": shadowfs put failed", "error", putErr)
			httputil.WriteError(w, http.StatusInternalServerError, "Upload failed", "Failed to store file", r.URL.Path)
			return
		}
		storedFileID = rec.ID
		storedFileType = rec.FileType
		storedStorageBackend = rec.StorageBackend
		storedStoragePath = rec.StoragePath
		storedSizeBytes = rec.SizeBytes
	}

	doc := cfg.buildDoc(userID, docType, titlePtr, header.Filename, contentType, int64(len(data)), hashStr)
	if shouldProcessDocument {
		doc.ProcessingStatus = "pending"
	} else {
		doc.ProcessingStatus = "idle"
	}
	doc.DocumentDate = documentDatePtr
	doc.DocumentAmount = documentAmountPtr
	doc.DocumentCurrency = documentCurrencyPtr

	result, err := h.DB.InsertGearDocument(r.Context(), doc)
	if err != nil {
		tracker.Error(r.Context(), err)
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

	metadata := directDocumentImportMetadata(cfg.parentType+"_document", result.ID, storedFileID, parentMetaKey, cfg.parentID, docType, contentType, shouldProcessDocument)

	// PDF text extraction via pdfcpu (best-effort)
	var extractedText *string
	var isTextPDF bool
	if cfg.extractPDF && contentType == "application/pdf" {
		if text := extractPDFText(data); text != "" {
			extractedText = &text
			isTextPDF = true
			var err error
			if shouldProcessDocument {
				err = h.DB.UpdateGearDocumentExtractedText(r.Context(), userID, result.ID, text)
			} else {
				err = h.DB.UpdateGearDocumentText(r.Context(), userID, result.ID, text)
			}
			if err != nil {
				slog.Warn("failed to update extracted text", "doc_id", result.ID, "error", err)
			} else {
				result.ExtractedText = &text
				if shouldProcessDocument {
					markDirectDocumentStep(metadata, "text_extraction", "completed")
				} else {
					result.ProcessingStatus = "processed"
				}
			}
		}
	}

	// Document event (graceful - no error if unavailable)
	if shouldProcessDocument {
		evt := event.DocumentUploadedEvent{
			DocID:          result.ID,
			GearID:         result.GearID,
			ComponentID:    result.ComponentID,
			UserID:         userID,
			ContentType:    result.ContentType,
			IsTextPDF:      isTextPDF,
			ExtractedText:  extractedText,
			FileID:         storedFileID,
			FileType:       storedFileType,
			StorageBackend: storedStorageBackend,
			StoragePath:    storedStoragePath,
			SizeBytes:      storedSizeBytes,
		}
		setDirectImportMetadata(r.Context(), tracker, metadata)
		tracker.Processing(r.Context())
		if err := h.DocumentUploadedPublisher.PublishDocumentUploaded(evt); err != nil {
			if statusErr := h.DB.UpdateGearDocumentProcessingStatus(r.Context(), userID, result.ID, "failed"); statusErr != nil {
				slog.Warn("failed to mark document upload event failure", "doc_id", result.ID, "error", statusErr)
			} else {
				result.ProcessingStatus = "failed"
			}
			failPendingDirectDocumentSteps(metadata)
			metadata["document_processing"] = map[string]any{
				"status": "failed",
				"reason": "publish_failed",
			}
			setDirectImportMetadata(r.Context(), tracker, metadata)
			tracker.Completed(r.Context(), "")
			slog.Warn("failed to publish document uploaded event", "error", err)
		}
	} else {
		setDirectImportMetadata(r.Context(), tracker, metadata)
		tracker.Completed(r.Context(), "")
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

	// The API container runs with HOME=/nonexistent. pdfcpu exits the process
	// if it cannot initialize its config dir, so use the built-in defaults.
	pdfcpuapi.DisableConfigDir()
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

// --- Idempotency key helpers (DB-backed, 24h TTL via pg_cron) ---

// checkIdempotencyKey reads the Idempotency-Key header. If present:
//   - replayed=true, done=true  → key was seen before; response already written
//   - replayed=false, done=true → DB error; response written
//   - replayed=false, done=false → key inserted; caller must run mutation and call storeIdempotencyResponse
//
// If the header is absent, returns replayed=false, done=false (no-op).
func checkIdempotencyKey(w http.ResponseWriter, r *http.Request, h *Handler, userID, route string) (replayed, done bool) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		return false, false
	}
	result, err := h.DB.TouchGearIdempotencyKey(r.Context(), userID, route, key)
	if err != nil {
		slog.Error("idempotency key touch failed", "error", err, "route", route)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "idempotency key error", r.URL.Path)
		return false, true
	}
	if !result.Inserted {
		// Replay: re-emit stored response.
		if string(result.ResponseJSON) == "null" || len(result.ResponseJSON) == 0 {
			// In-flight race: the first request hasn't stored its response yet.
			// Treat as a cache miss so this request runs the handler too, keeping
			// the two concurrent requests racing rather than returning nothing.
			return false, false
		}
		// Replay using the original HTTP status so 201 Created and 4xx errors
		// are preserved faithfully instead of always emitting 200 OK.
		replayStatus := result.StatusCode
		if replayStatus == 0 {
			// Pre-migration rows lack a stored status; default conservatively to 200.
			replayStatus = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(replayStatus)
		_, _ = w.Write(result.ResponseJSON)
		_, _ = w.Write([]byte("\n"))
		return true, true
	}
	return false, false
}

// storeIdempotencyResponse persists the response body and HTTP status for a
// completed mutation so future replays return an identical response.
// Best-effort: errors are only logged, never propagated to the caller.
//
// Only deterministic responses should be stored: successful mutations (2xx)
// and request-body-driven validation errors (4xx). Transient failures (5xx)
// must NOT be stored — the caller must not invoke this function for 5xx paths.
//
// Auth failures (401/403) are not request-body-driven and must NOT be stored —
// they depend on session state. Callers must not invoke this function for them.
func storeIdempotencyResponse(r *http.Request, h *Handler, userID, route string, statusCode int, body any) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		return
	}
	data, err := json.Marshal(body)
	if err != nil {
		slog.Warn("idempotency: marshal response failed", "error", err)
		return
	}
	if err := h.DB.UpdateGearIdempotencyResponse(r.Context(), userID, route, key, statusCode, data); err != nil {
		slog.Warn("idempotency: store response failed", "error", err)
	}
}

// captureResponseWriter buffers a single error response (status + body) so it
// can be persisted as an idempotency replay before being written to the real
// response writer. Used for deterministic 4xx paths inside helpers that
// already write directly to a ResponseWriter (DecodeJSON, normalize…).
//
// Usage: pass a *captureResponseWriter to the helper, check captured.status
// after the helper returns. If status is in the deterministic-4xx range, call
// storeIdempotencyResponse with the captured raw body, then flush the same
// bytes to the real writer.
type captureResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newCaptureResponseWriter() *captureResponseWriter {
	return &captureResponseWriter{header: http.Header{}}
}

func (c *captureResponseWriter) Header() http.Header { return c.header }
func (c *captureResponseWriter) Write(b []byte) (int, error) {
	return c.body.Write(b)
}
func (c *captureResponseWriter) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

// flushTo copies the captured headers, status, and body to the real writer.
func (c *captureResponseWriter) flushTo(w http.ResponseWriter) {
	for k, vv := range c.header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	if c.status != 0 {
		w.WriteHeader(c.status)
	}
	_, _ = w.Write(c.body.Bytes())
}

// decodeJSONIdempotent wraps httputil.DecodeJSON with idempotency persistence.
// On decode failure it captures the 400 body, persists it as the idempotency
// response (so replays return the identical 400), and flushes the captured
// response to w. Caller behaves exactly like with the bare DecodeJSON: returns
// (zero, false) means "stop processing".
//
// Idempotency is opt-in via the Idempotency-Key request header; when absent
// this is a transparent pass-through to DecodeJSON (still emits the 400 to w).
func decodeJSONIdempotent[T any](w http.ResponseWriter, r *http.Request, h *Handler, userID, route string) (T, bool) {
	cap := newCaptureResponseWriter()
	v, ok := httputil.DecodeJSON[T](cap, r)
	if ok {
		// Successful decode: nothing buffered, fall through.
		return v, true
	}
	// Decode failed. Persist the captured 400 (deterministic given request body)
	// before flushing, so a replay returns the same body even if the second
	// request bytes differ.
	if cap.status >= 400 && cap.status < 500 {
		storeIdempotencyResponse(r, h, userID, route, cap.status, json.RawMessage(cap.body.Bytes()))
	}
	cap.flushTo(w)
	return v, false
}

// normalizeGearApplicablePredicatesIdempotent wraps the normalize helper with
// idempotency persistence. Same pattern as decodeJSONIdempotent: captures the
// 400 body, persists it, then flushes.
func (h *Handler) normalizeGearApplicablePredicatesIdempotent(
	w http.ResponseWriter, r *http.Request, userID, route string,
	values []gearpkg.GearDefaultPredicate,
) ([]gearpkg.GearDefaultPredicate, bool) {
	cap := newCaptureResponseWriter()
	predicates, ok := normalizeGearApplicablePredicatesForRequest(cap, r, values)
	if ok {
		return predicates, true
	}
	if cap.status >= 400 && cap.status < 500 {
		storeIdempotencyResponse(r, h, userID, route, cap.status, json.RawMessage(cap.body.Bytes()))
	}
	cap.flushTo(w)
	return nil, false
}

// writeErrorIdempotent emits a problem-detail 4xx response and persists it as
// the idempotency replay body. Used for SwapComponent early-validation paths
// that previously called httputil.WriteError directly.
func writeErrorIdempotent(
	w http.ResponseWriter, r *http.Request, h *Handler, userID, route string,
	status int, title, detail string,
) {
	cap := newCaptureResponseWriter()
	httputil.WriteError(cap, status, title, detail, r.URL.Path)
	if cap.status >= 400 && cap.status < 500 {
		storeIdempotencyResponse(r, h, userID, route, cap.status, json.RawMessage(cap.body.Bytes()))
	}
	cap.flushTo(w)
}

// --- Gear summary endpoint ---

// GetGearSummary returns aggregated usage statistics across a user's gear.
func (h *Handler) GetGearSummary(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	filter := database.GearSummaryFilter{}
	if scope := q.Get("scope"); scope != "" {
		filter.Scope = &scope
	}
	if typeKey := q.Get("type_key"); typeKey != "" {
		filter.TypeKey = &typeKey
	}
	if since := q.Get("since"); since != "" {
		filter.Since = &since
	}
	if until := q.Get("until"); until != "" {
		filter.Until = &until
	}
	if raw := q.Get("include_retired"); raw != "" {
		parsed, parseErr := strconv.ParseBool(raw)
		if parseErr != nil {
			httputil.WriteMachineError(w, http.StatusBadRequest, "gear.invalid_param",
				"include_retired must be a boolean (true/false/1/0)",
				map[string]any{"param": "include_retired", "value": raw})
			return
		}
		filter.IncludeRetired = parsed
	}

	summary, err := h.DB.GetGearSummary(r.Context(), userID, filter)
	if err != nil {
		slog.Error("get gear summary failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get gear summary", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, summary)
}

// --- Component swap endpoint ---

// swapComponentRequest is the request body for the component swap endpoint.
type swapComponentRequest struct {
	SlotKey                       string                     `json:"slot_key"`
	NewComponentPayload           *model.GearComponentCreate `json:"new_component_payload,omitempty"`
	ExistingComponentLookupByName string                     `json:"existing_component_lookup_by_name,omitempty"`
	SwapDate                      *string                    `json:"swap_date,omitempty"`
	SwapReason                    *string                    `json:"swap_reason,omitempty"`
}

// SwapComponent atomically retires an existing component and assigns a new one,
// recording a swap event in gear_component_swap_events.
func (h *Handler) SwapComponent(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	gearID := chi.URLParam(r, "id")

	// Ownership check BEFORE idempotency write to prevent leaking idempotency
	// state when the caller doesn't own the gear.
	existing, err := h.DB.GetGear(r.Context(), gearID)
	if err != nil {
		slog.Error("swap component: get gear failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get gear", r.URL.Path)
		return
	}
	if existing == nil || existing.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	// Idempotency-Key check (ownership verified above).
	swapRoute := "POST /gear/" + gearID + "/components/swap"
	if replayed, done := checkIdempotencyKey(w, r, h, userID, swapRoute); done {
		if replayed {
			return
		}
	}

	req, ok := decodeJSONIdempotent[swapComponentRequest](w, r, h, userID, swapRoute)
	if !ok {
		return
	}
	if req.SlotKey == "" {
		writeErrorIdempotent(w, r, h, userID, swapRoute, http.StatusBadRequest, "Bad Request", "slot_key is required")
		return
	}
	if req.NewComponentPayload == nil {
		writeErrorIdempotent(w, r, h, userID, swapRoute, http.StatusBadRequest, "Bad Request", "new_component_payload is required")
		return
	}
	if req.NewComponentPayload.Name == "" {
		writeErrorIdempotent(w, r, h, userID, swapRoute, http.StatusBadRequest, "Bad Request", "new_component_payload.name is required")
		return
	}

	// Look up old component by name if requested.
	var oldComponentID *string
	if req.ExistingComponentLookupByName != "" {
		comps, err := h.DB.FindComponentsByName(r.Context(), gearID, req.ExistingComponentLookupByName)
		if err != nil {
			slog.Error("swap component: find old component failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to find component", r.URL.Path)
			return
		}
		if len(comps) == 0 {
			httputil.WriteMachineError(w, http.StatusNotFound, "gear.component_not_found",
				"no component found with that name",
				map[string]any{"name": req.ExistingComponentLookupByName})
			return
		}
		if len(comps) > 1 {
			httputil.WriteMachineError(w, http.StatusConflict, "gear.name_ambiguous",
				"multiple components match the given name",
				map[string]any{"candidates": comps})
			return
		}
		oldComponentID = &comps[0].ID
	}

	result, err := h.DB.SwapGearComponent(r.Context(), userID, gearID, req.SlotKey,
		oldComponentID, req.NewComponentPayload, req.SwapDate, req.SwapReason)
	if err != nil {
		slog.Error("swap component failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to swap component", r.URL.Path)
		return
	}

	storeIdempotencyResponse(r, h, userID, swapRoute, http.StatusCreated, result)
	httputil.WriteJSON(w, http.StatusCreated, result)
}

// gearTypeLegacyAllowList contains gear type keys that are not in the current
// schema but exist in production data from before the schema was formalized.
// Legacy allow-list per Plan 2026-05-08 §10 (gear-rebuild on taxonomy) — backfill migration removes it.
var gearTypeLegacyAllowList = map[string]bool{
	"e_bike": true, // legacy electric bike type; maps to scope='bike' via migration backfill
}

// isKnownGearType returns true when the type key is present in the compiled
// schema OR in the legacy allow-list.
func isKnownGearType(typeKey string) bool {
	if _, ok := gearpkg.LookupGearType(typeKey); ok {
		return true
	}
	return gearTypeLegacyAllowList[typeKey]
}
