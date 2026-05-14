package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/auth"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

const maxEmailTemplateBodyBytes = 256 * 1024

func (h *Handler) AdminListEmailTemplates(w http.ResponseWriter, r *http.Request) {
	items := make([]auth.EmailTemplate, 0, len(auth.EmailTemplateKeys()))
	for _, key := range auth.EmailTemplateKeys() {
		tpl, err := h.Mailer.GetEmailTemplate(r.Context(), key)
		if err != nil {
			slog.Error("admin list email template failed", "key", key, "error", err)
			httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		items = append(items, tpl)
	}
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]any{"templates": items})
}

func (h *Handler) AdminGetEmailTemplate(w http.ResponseWriter, r *http.Request) {
	key, ok := emailTemplateKeyFromRequest(w, r)
	if !ok {
		return
	}
	tpl, err := h.Mailer.GetEmailTemplate(r.Context(), key)
	if err != nil {
		writeEmailTemplateError(w, err)
		return
	}
	httputil.WriteJSONNoCache(w, http.StatusOK, tpl)
}

func (h *Handler) AdminPutEmailTemplate(w http.ResponseWriter, r *http.Request) {
	key, ok := emailTemplateKeyFromRequest(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxEmailTemplateBodyBytes)
	var req auth.EmailTemplateOverride
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeEmailTemplateDecodeError(w, err)
		return
	}
	adminID := middleware.GetUserID(r.Context())
	if err := h.Mailer.SaveEmailTemplate(r.Context(), key, req, adminID); err != nil {
		writeEmailTemplateError(w, err)
		return
	}
	logEmailTemplateAudit(r, h, adminID, "admin_email_template_update", key)
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (h *Handler) AdminPreviewEmailTemplate(w http.ResponseWriter, r *http.Request) {
	key, ok := emailTemplateKeyFromRequest(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxEmailTemplateBodyBytes)
	var req struct {
		Payload map[string]string `json:"payload"`
		Subject *string           `json:"subject"`
		HTML    *string           `json:"html"`
		Text    *string           `json:"text"`
		Enabled *bool             `json:"enabled"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeEmailTemplateDecodeError(w, err)
			return
		}
	}
	if req.Payload == nil {
		req.Payload = h.Mailer.SamplePayload(key)
	}
	var rendered auth.RenderedEmail
	var err error
	if req.Subject != nil || req.HTML != nil || req.Text != nil || req.Enabled != nil {
		if req.Subject == nil || req.HTML == nil || req.Text == nil {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
			return
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		rendered, err = h.Mailer.PreviewEmailTemplateOverride(key, auth.EmailTemplateOverride{
			Subject: *req.Subject,
			HTML:    *req.HTML,
			Text:    *req.Text,
			Enabled: enabled,
		}, req.Payload)
	} else {
		rendered, err = h.Mailer.PreviewEmailTemplate(r.Context(), key, req.Payload)
	}
	if err != nil {
		writeEmailTemplateError(w, err)
		return
	}
	httputil.WriteJSONNoCache(w, http.StatusOK, rendered)
}

func (h *Handler) AdminTestEmailTemplate(w http.ResponseWriter, r *http.Request) {
	key, ok := emailTemplateKeyFromRequest(w, r)
	if !ok {
		return
	}
	if !h.Mailer.IsConfigured() {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "smtp_not_configured"})
		return
	}
	adminID := middleware.GetUserID(r.Context())
	if remaining, ok := consumeTestEmailCooldown(r.Context(), h, adminID); !ok {
		httputil.WriteJSONNoCache(w, http.StatusTooManyRequests, map[string]any{
			"error":       "rate_limited",
			"retry_after": remaining,
		})
		return
	}
	admin, err := h.DB.GetUserByID(r.Context(), adminID)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	if admin.Email == nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "admin_has_no_email"})
		return
	}
	if err := h.Mailer.SendTemplateTestEmail(r.Context(), key, *admin.Email, admin.Username); err != nil {
		slog.Error("admin email template test failed", "key", key, "admin_id", adminID, "error", err)
		httputil.WriteJSONNoCache(w, http.StatusBadGateway, map[string]string{"error": "smtp_send_failed"})
		return
	}
	logEmailTemplateAudit(r, h, adminID, "admin_email_template_test", key)
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]string{"status": "test_email_sent"})
}

func (h *Handler) AdminResetEmailTemplate(w http.ResponseWriter, r *http.Request) {
	key, ok := emailTemplateKeyFromRequest(w, r)
	if !ok {
		return
	}
	adminID := middleware.GetUserID(r.Context())
	if err := h.Mailer.ResetEmailTemplate(r.Context(), key); err != nil {
		writeEmailTemplateError(w, err)
		return
	}
	logEmailTemplateAudit(r, h, adminID, "admin_email_template_reset", key)
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]string{"status": "reset"})
}

func logEmailTemplateAudit(r *http.Request, h *Handler, adminID, eventType string, key auth.EmailTemplateKey) {
	if h == nil || h.DB == nil {
		return
	}
	if err := h.DB.LogAuthEventStrict(r.Context(), adminID, eventType,
		middleware.ClientIP(r), r.UserAgent(), map[string]string{"template_key": string(key)}); err != nil {
		slog.Error("admin email template audit failed", "event_type", eventType, "admin_id", adminID, "key", key, "error", err)
	}
}

func writeEmailTemplateDecodeError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		httputil.WriteJSONNoCache(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request_too_large"})
		return
	}
	httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
}

func emailTemplateKeyFromRequest(w http.ResponseWriter, r *http.Request) (auth.EmailTemplateKey, bool) {
	key, err := auth.ParseEmailTemplateKey(chi.URLParam(r, "key"))
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "unknown_template"})
		return "", false
	}
	return key, true
}

func writeEmailTemplateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrEmailTemplateUnknown):
		httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "unknown_template"})
	case errors.Is(err, auth.ErrEmailTemplateValidation):
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		slog.Error("email template endpoint failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
	}
}
