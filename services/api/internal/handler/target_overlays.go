package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) GetTargetOverlaySummary(w http.ResponseWriter, r *http.Request) {
	subjectKind, subjectID, targetVersionID := targetOverlayQuery(r)
	summary, err := h.DB.GetTargetOverlaySummary(r.Context(), subjectKind, subjectID, targetVersionID)
	if handleTargetOverlayErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"summary": summary})
}

func (h *Handler) ListTargetObservations(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	scopes := h.resolveTargetVisibleScopes(w, r, userID)
	if scopes == nil && userID != "" {
		return
	}
	subjectKind, subjectID, targetVersionID := targetOverlayQuery(r)
	observations, err := h.DB.ListTargetActivityObservations(r.Context(), model.ListTargetActivityObservationsParams{
		SubjectKind:           subjectKind,
		SubjectID:             subjectID,
		TargetVersionID:       targetVersionID,
		UserID:                userID,
		VisibleAccessScopeIDs: scopes,
		Limit:                 parseIntDefault(r.URL.Query().Get("limit"), 50),
		Offset:                parseIntDefault(r.URL.Query().Get("offset"), 0),
		Admin:                 middleware.GetIsAdmin(r.Context()),
	})
	if handleTargetOverlayErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"observations": observations})
}

func (h *Handler) ListTargetReports(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	scopes := h.resolveTargetVisibleScopes(w, r, userID)
	if scopes == nil && userID != "" {
		return
	}
	subjectKind, subjectID, targetVersionID := targetOverlayQuery(r)
	reports, err := h.DB.ListTargetReports(r.Context(), model.ListTargetReportsParams{
		SubjectKind:           subjectKind,
		SubjectID:             subjectID,
		TargetVersionID:       targetVersionID,
		UserID:                userID,
		VisibleAccessScopeIDs: scopes,
		ReportType:            r.URL.Query().Get("type"),
		Limit:                 parseIntDefault(r.URL.Query().Get("limit"), 50),
		Offset:                parseIntDefault(r.URL.Query().Get("offset"), 0),
		Admin:                 middleware.GetIsAdmin(r.Context()),
	})
	if handleTargetOverlayErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"reports": reports})
}

func (h *Handler) CreateTargetReport(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.CreateTargetReportInput](w, r)
	if !ok {
		return
	}
	report, err := h.DB.CreateTargetReport(r.Context(), userID, input)
	if handleTargetOverlayErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"report": report})
}

func (h *Handler) GetTargetReport(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	scopes := h.resolveTargetVisibleScopes(w, r, userID)
	if scopes == nil && userID != "" {
		return
	}
	report, err := h.DB.GetTargetReport(r.Context(), userID, chi.URLParam(r, "id"), scopes)
	if handleTargetOverlayErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"report": report})
}

func (h *Handler) AddTargetReportStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	reportID := chi.URLParam(r, "id")
	input, ok := httputil.DecodeJSON[model.AddTargetReportStatusInput](w, r)
	if !ok {
		return
	}
	report, err := h.DB.GetTargetReportForManagement(r.Context(), reportID)
	if handleTargetOverlayErr(w, r, err) {
		return
	}
	isOwner := report.AuthorUserID != nil && *report.AuthorUserID == userID
	if !isOwner && !middleware.GetIsAdmin(r.Context()) {
		canManage, err := h.DB.CanManageTargetReports(r.Context(), userID, report.SubjectKind, report.SubjectID, "manage_reports")
		if handleTargetOverlayErr(w, r, err) {
			return
		}
		if !canManage {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "target report not found", r.URL.Path)
			return
		}
	}
	event, err := h.DB.AddTargetReportStatus(r.Context(), userID, reportID, input)
	if handleTargetOverlayErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"status_event": event})
}

func (h *Handler) ListTargetUserContent(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	scopes := h.resolveTargetVisibleScopes(w, r, userID)
	if scopes == nil && userID != "" {
		return
	}
	subjectKind, subjectID, targetVersionID := targetOverlayQuery(r)
	content, err := h.DB.ListTargetUserContent(r.Context(), model.ListTargetUserContentParams{
		SubjectKind:           subjectKind,
		SubjectID:             subjectID,
		TargetVersionID:       targetVersionID,
		UserID:                userID,
		VisibleAccessScopeIDs: scopes,
		ContentKind:           r.URL.Query().Get("kind"),
		Limit:                 parseIntDefault(r.URL.Query().Get("limit"), 50),
		Offset:                parseIntDefault(r.URL.Query().Get("offset"), 0),
		Admin:                 middleware.GetIsAdmin(r.Context()),
	})
	if handleTargetOverlayErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"content": content})
}

func (h *Handler) CreateTargetUserContent(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.CreateTargetUserContentInput](w, r)
	if !ok {
		return
	}
	content, err := h.DB.CreateTargetUserContent(r.Context(), userID, input)
	if handleTargetOverlayErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"content": content})
}

func (h *Handler) DeleteTargetUserContent(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	if err := h.DB.DeleteTargetUserContent(r.Context(), userID, chi.URLParam(r, "id")); handleTargetOverlayErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func targetOverlayQuery(r *http.Request) (string, string, *string) {
	q := r.URL.Query()
	targetVersionID := q.Get("target_version_id")
	if targetVersionID == "" {
		return q.Get("subject_kind"), q.Get("subject_id"), nil
	}
	return q.Get("subject_kind"), q.Get("subject_id"), &targetVersionID
}

func handleTargetOverlayErr(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, database.ErrTargetOverlayValidation) || errors.Is(err, database.ErrObjectAccessValidation) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return true
	}
	if errors.Is(err, database.ErrTargetNotFound) || errors.Is(err, database.ErrObjectAccessNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "target overlay not found", r.URL.Path)
		return true
	}
	httputil.HandleDBErr(w, r, err, "target overlay not found")
	return true
}
