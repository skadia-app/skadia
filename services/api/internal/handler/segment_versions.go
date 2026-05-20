package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) UpdateSegmentVersionValidity(w http.ResponseWriter, r *http.Request) {
	if !h.requireFeatureAvailable(w, r, capabilities.KeySegmentVersionWorkflow) {
		return
	}
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	update, err := decodeSegmentVersionValidityUpdate(r)
	if err != nil {
		httputil.WriteError(w, http.StatusUnprocessableEntity, "Unprocessable Entity", err.Error(), r.URL.Path)
		return
	}
	revision, err := h.DB.UpdateSegmentVersionValidity(
		r.Context(),
		chi.URLParam(r, "id"),
		chi.URLParam(r, "version_id"),
		userID,
		middleware.GetIsAdmin(r.Context()),
		update,
	)
	if err != nil {
		writeSegmentVersionWorkflowError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, revision)
}

func (h *Handler) UpdateSegmentVersionGeometry(w http.ResponseWriter, r *http.Request) {
	if !h.requireFeatureAvailable(w, r, capabilities.KeySegmentVersionWorkflow) {
		return
	}
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	var update model.SegmentGeometryUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if strings.TrimSpace(update.OperationID) == "" {
		httputil.WriteError(w, http.StatusUnprocessableEntity, "Unprocessable Entity", "operation_id is required", r.URL.Path)
		return
	}
	revision, err := h.DB.UpdateSegmentVersionGeometry(
		r.Context(),
		chi.URLParam(r, "id"),
		chi.URLParam(r, "version_id"),
		userID,
		middleware.GetIsAdmin(r.Context()),
		update,
	)
	if err != nil {
		writeSegmentVersionWorkflowError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, revision)
}

func (h *Handler) RollbackSegmentVersionRevision(w http.ResponseWriter, r *http.Request) {
	if !h.requireFeatureAvailable(w, r, capabilities.KeySegmentVersionWorkflow) {
		return
	}
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	revision, err := h.DB.RollbackSegmentVersionRevision(
		r.Context(),
		chi.URLParam(r, "id"),
		chi.URLParam(r, "revision_id"),
		userID,
		middleware.GetIsAdmin(r.Context()),
		body.Reason,
	)
	if err != nil {
		writeSegmentVersionWorkflowError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, revision)
}

func (h *Handler) ListSegmentVersionRevisions(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	limit := parseBoundedIntQuery(r, "limit", 50, 1, 100)
	offset := parseBoundedIntQuery(r, "offset", 0, 0, 100000)
	revisions, err := h.DB.ListSegmentVersionRevisions(
		r.Context(),
		chi.URLParam(r, "id"),
		userID,
		middleware.GetIsAdmin(r.Context()),
		limit,
		offset,
	)
	if err != nil {
		writeSegmentVersionWorkflowError(w, r, err)
		return
	}
	summaries := make([]segmentVersionRevisionSummary, 0, len(revisions))
	for _, revision := range revisions {
		summaries = append(summaries, segmentVersionRevisionSummaryFromModel(revision))
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"revisions": summaries,
		"limit":     limit,
		"offset":    offset,
	})
}

func (h *Handler) GetSegmentVersionRevision(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	revision, err := h.DB.GetSegmentVersionRevision(
		r.Context(),
		chi.URLParam(r, "id"),
		chi.URLParam(r, "revision_id"),
		userID,
		middleware.GetIsAdmin(r.Context()),
	)
	if err != nil {
		writeSegmentVersionWorkflowError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, revision)
}

type segmentVersionRevisionSummary struct {
	ID                   string     `json:"id"`
	SegmentID            string     `json:"segment_id"`
	Operation            string     `json:"operation"`
	Reason               *string    `json:"reason,omitempty"`
	AffectedFromAt       *time.Time `json:"affected_from_at,omitempty"`
	AffectedUntilAt      *time.Time `json:"affected_until_at,omitempty"`
	RecalculationRunID   *string    `json:"recalculation_run_id,omitempty"`
	RollbackOfRevisionID *string    `json:"rollback_of_revision_id,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	HasSnapshots         bool       `json:"has_snapshots"`
}

func segmentVersionRevisionSummaryFromModel(revision model.SegmentVersionRevision) segmentVersionRevisionSummary {
	return segmentVersionRevisionSummary{
		ID:                   revision.ID,
		SegmentID:            revision.SegmentID,
		Operation:            revision.Operation,
		Reason:               revision.Reason,
		AffectedFromAt:       revision.AffectedFromAt,
		AffectedUntilAt:      revision.AffectedUntilAt,
		RecalculationRunID:   revision.RecalculationRunID,
		RollbackOfRevisionID: revision.RollbackOfRevisionID,
		CreatedAt:            revision.CreatedAt,
		HasSnapshots:         len(revision.BeforeSnapshot) > 0 && len(revision.AfterSnapshot) > 0,
	}
}

func decodeSegmentVersionValidityUpdate(r *http.Request) (model.SegmentVersionValidityUpdate, error) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return model.SegmentVersionValidityUpdate{}, fmt.Errorf("invalid JSON body")
	}
	var update model.SegmentVersionValidityUpdate
	if v, ok := raw["operation_id"]; ok {
		_ = json.Unmarshal(v, &update.OperationID)
	}
	if strings.TrimSpace(update.OperationID) == "" {
		return update, fmt.Errorf("operation_id is required")
	}
	from, err := parseRequiredNullableRFC3339(raw, "valid_from_at")
	if err != nil {
		return update, err
	}
	until, err := parseRequiredNullableRFC3339(raw, "valid_until_at")
	if err != nil {
		return update, err
	}
	update.ValidFromAt = from
	update.ValidUntilAt = until
	if v, ok := raw["reason"]; ok {
		_ = json.Unmarshal(v, &update.Reason)
	}
	if v, ok := raw["recalculate"]; ok {
		_ = json.Unmarshal(v, &update.Recalculate)
	}
	if v, ok := raw["equivalence_decision"]; ok {
		_ = json.Unmarshal(v, &update.EquivalenceDecision)
	}
	return update, nil
}

func parseRequiredNullableRFC3339(raw map[string]json.RawMessage, key string) (*time.Time, error) {
	value, ok := raw[key]
	if !ok {
		return nil, fmt.Errorf("%s is required; use null for unbounded", key)
	}
	if string(value) == "null" {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(value, &s); err != nil {
		return nil, fmt.Errorf("%s must be null or an RFC3339 timestamp", key)
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil, fmt.Errorf("%s must be an RFC3339 timestamp with timezone", key)
	}
	utc := t.UTC()
	return &utc, nil
}

func parseBoundedIntQuery(r *http.Request, key string, fallback, min, max int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func writeSegmentVersionWorkflowError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, database.ErrSegmentNotFound):
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment revision not found", r.URL.Path)
	case errors.Is(err, database.ErrSegmentNotAuthorized):
		httputil.WriteError(w, http.StatusForbidden, "Forbidden", "not authorized", r.URL.Path)
	case errors.Is(err, database.ErrSegmentValidation):
		httputil.WriteError(w, http.StatusUnprocessableEntity, "Unprocessable Entity", err.Error(), r.URL.Path)
	default:
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "55P03", "40001":
				httputil.WriteError(w, http.StatusServiceUnavailable, "Service Unavailable", "lock conflict, please try again", r.URL.Path)
				return
			}
		}
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "segment version workflow failed", r.URL.Path)
	}
}
