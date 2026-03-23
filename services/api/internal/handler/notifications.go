package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ListNotifications returns recent notifications for the authenticated user.
//
// GET /api/v1/notifications?limit=50
func (h *Handler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	items, err := h.DB.ListNotifications(r.Context(), userID, limit)
	if err != nil {
		slog.Error("list notifications failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list notifications", r.URL.Path)
		return
	}
	if items == nil {
		items = []model.Notification{}
	}

	unread, err := h.DB.GetUnreadCount(r.Context(), userID)
	if err != nil {
		slog.Warn("get unread count failed", "error", err)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"items":        items,
		"unread_count": unread,
	})
}

// MarkNotificationRead marks a single notification as read.
//
// POST /api/v1/notifications/{id}/read
func (h *Handler) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	notifID := chi.URLParam(r, "id")

	if err := h.DB.MarkRead(r.Context(), notifID, userID); httputil.HandleDBErr(w, r, err, "notification not found") {
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// MarkAllNotificationsRead marks all notifications as read for the authenticated user.
//
// POST /api/v1/notifications/read-all
func (h *Handler) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	if err := h.DB.MarkAllRead(r.Context(), userID); err != nil {
		slog.Error("mark all notifications read failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to mark all notifications as read", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteNotification löscht eine einzelne Benachrichtigung des authentifizierten Users.
//
// DELETE /api/v1/notifications/{id}
func (h *Handler) DeleteNotification(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	notifID := chi.URLParam(r, "id")

	if err := h.DB.DeleteNotification(r.Context(), notifID, userID); httputil.HandleDBErr(w, r, err, "notification not found") {
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteAllNotifications löscht alle Benachrichtigungen des authentifizierten Users.
//
// DELETE /api/v1/notifications
func (h *Handler) DeleteAllNotifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	if err := h.DB.DeleteAllNotifications(r.Context(), userID); err != nil {
		slog.Error("delete all notifications failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete all notifications", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetUnreadCount returns just the unread notification count.
//
// GET /api/v1/notifications/unread-count
func (h *Handler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	count, err := h.DB.GetUnreadCount(r.Context(), userID)
	if err != nil {
		slog.Error("get unread count failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get unread count", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"unread_count": count})
}
