package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ListNotifications returns recent notifications for the authenticated user.
//
// GET /api/v1/notifications?limit=50
func (h *Handler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	items, err := h.DB.ListNotifications(r.Context(), userID, limit)
	if err != nil {
		slog.Error("list notifications failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list notifications", r.URL.Path)
		return
	}
	if items == nil {
		items = []model.Notification{}
	}

	unread, err := h.DB.GetUnreadCount(r.Context(), userID)
	if err != nil {
		slog.Warn("get unread count failed", "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":        items,
		"unread_count": unread,
	})
}

// MarkNotificationRead marks a single notification as read.
//
// POST /api/v1/notifications/{id}/read
func (h *Handler) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	notifID := chi.URLParam(r, "id")

	if err := h.DB.MarkRead(r.Context(), notifID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "Not Found", "notification not found", r.URL.Path)
			return
		}
		slog.Error("mark notification read failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to mark notification as read", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// MarkAllNotificationsRead marks all notifications as read for the authenticated user.
//
// POST /api/v1/notifications/read-all
func (h *Handler) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	if err := h.DB.MarkAllRead(r.Context(), userID); err != nil {
		slog.Error("mark all notifications read failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to mark all notifications as read", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetUnreadCount returns just the unread notification count.
//
// GET /api/v1/notifications/unread-count
func (h *Handler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	count, err := h.DB.GetUnreadCount(r.Context(), userID)
	if err != nil {
		slog.Error("get unread count failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get unread count", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"unread_count": count})
}
