package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/auth"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/settings"
)

func emailTemplateTestRequest(method, path, key, body string, adminID string) (*http.Request, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("key", key)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, adminID))
	return req, httptest.NewRecorder()
}

func TestAdminEmailTemplateOverridePreviewReset(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	defer pool.Close()
	db := &database.DB{Pool: pool}
	adminID := uuid.NewString()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		 VALUES ($1, $2, $3, true, 'test', true)
		 ON CONFLICT (id) DO NOTHING`,
		adminID, "admin-"+adminID, "admin-"+adminID+"@example.test",
	)
	if err != nil {
		t.Fatalf("insert admin: %v", err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM email_templates WHERE key = $1`, string(auth.EmailTemplatePasswordReset))

	st := settings.NewStoreForTest(map[string]string{
		"app.name":      "Skadia",
		"app.url":       "https://fit.example.test",
		"smtp.host":     "smtp.example.test",
		"smtp.port":     "587",
		"smtp.from":     "noreply@example.test",
		"smtp.tls":      "starttls",
		"smtp.username": "",
		"smtp.password": "",
	})
	h := &Handler{DB: db, Settings: st, Mailer: auth.NewMailerWithDB(db, st)}

	req, rr := emailTemplateTestRequest(http.MethodPut, "/api/v1/admin/email/templates/reset_password", "reset_password",
		`{"subject":"Reset {{.Username}}","html":"<p>{{.ActionURL}}</p>","text":"{{.ActionURL}}","enabled":true}`,
		adminID)
	h.AdminPutEmailTemplate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", rr.Code, rr.Body.String())
	}
	assertAuthAuditEvent(t, pool, adminID, "admin_email_template_update", "reset_password")

	req, rr = emailTemplateTestRequest(http.MethodPost, "/api/v1/admin/email/templates/reset_password/preview", "reset_password",
		`{"payload":{"AppName":"Skadia","AppURL":"https://fit.example.test","Username":"ada","ActionURL":"https://fit.example.test/reset-password?token=raw"}}`,
		adminID)
	h.AdminPreviewEmailTemplate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", rr.Code, rr.Body.String())
	}
	var preview map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if preview["subject"] != "Reset ada" {
		t.Fatalf("preview subject=%q, want Reset ada", preview["subject"])
	}

	req, rr = emailTemplateTestRequest(http.MethodPost, "/api/v1/admin/email/templates/reset_password/preview", "reset_password",
		`{"subject":"Draft {{.Username}}","html":"<p>Draft {{.ActionURL}}</p>","text":"Draft {{.ActionURL}}","enabled":true,"payload":{"AppName":"Skadia","AppURL":"https://fit.example.test","Username":"ada","ActionURL":"https://fit.example.test/reset-password?token=raw"}}`,
		adminID)
	h.AdminPreviewEmailTemplate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("draft preview status=%d body=%s", rr.Code, rr.Body.String())
	}
	preview = map[string]string{}
	if err := json.Unmarshal(rr.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode draft preview: %v", err)
	}
	if preview["subject"] != "Draft ada" {
		t.Fatalf("draft preview subject=%q, want Draft ada", preview["subject"])
	}

	req, rr = emailTemplateTestRequest(http.MethodDelete, "/api/v1/admin/email/templates/reset_password", "reset_password", "", adminID)
	h.AdminResetEmailTemplate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reset status=%d body=%s", rr.Code, rr.Body.String())
	}
	assertAuthAuditEvent(t, pool, adminID, "admin_email_template_reset", "reset_password")

	req, rr = emailTemplateTestRequest(http.MethodGet, "/api/v1/admin/email/templates/reset_password", "reset_password", "", adminID)
	h.AdminGetEmailTemplate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.Source != "default" {
		t.Fatalf("source=%q, want default", got.Source)
	}
}

func TestAdminEmailTemplateRejectsOversizedDraft(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	defer pool.Close()
	db := &database.DB{Pool: pool}
	adminID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		 VALUES ($1, $2, $3, true, 'test', true)
		 ON CONFLICT (id) DO NOTHING`,
		adminID, "admin-"+adminID, "admin-"+adminID+"@example.test",
	); err != nil {
		t.Fatalf("insert admin: %v", err)
	}
	st := settings.NewStoreForTest(map[string]string{"app.name": "Skadia", "app.url": "https://fit.example.test"})
	h := &Handler{DB: db, Settings: st, Mailer: auth.NewMailerWithDB(db, st)}

	oversized := strings.Repeat("x", maxEmailTemplateBodyBytes+1)
	req, rr := emailTemplateTestRequest(http.MethodPut, "/api/v1/admin/email/templates/reset_password", "reset_password",
		`{"subject":"Reset","html":"`+oversized+`","text":"Reset","enabled":true}`,
		adminID)
	h.AdminPutEmailTemplate(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("put oversized status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminTemplateTestEmailUsesSharedCooldown(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	defer pool.Close()
	db := &database.DB{Pool: pool}
	adminID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		 VALUES ($1, $2, $3, true, 'test', true)
		 ON CONFLICT (id) DO NOTHING`,
		adminID, "admin-"+adminID, "admin-"+adminID+"@example.test",
	); err != nil {
		t.Fatalf("insert admin: %v", err)
	}
	st := settings.NewStoreForTest(map[string]string{
		"app.name":  "Skadia",
		"app.url":   "https://fit.example.test",
		"smtp.host": "smtp.example.test",
		"smtp.port": "587",
		"smtp.from": "noreply@example.test",
		"smtp.tls":  "starttls",
	})
	mailer := auth.NewMailerWithDB(db, st)
	mailer.SetDeliverRawForTest(func(to, subject, templateName, htmlBody, textBody string) error { return nil })
	h := &Handler{DB: db, Settings: st, Mailer: mailer}

	req, rr := emailTemplateTestRequest(http.MethodPost, "/api/v1/admin/email/templates/test_email/test", "test_email", "", adminID)
	h.AdminTestEmailTemplate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first test status=%d body=%s", rr.Code, rr.Body.String())
	}
	assertAuthAuditEvent(t, pool, adminID, "admin_email_template_test", "test_email")

	req, rr = emailTemplateTestRequest(http.MethodPost, "/api/v1/admin/email/templates/test_email/test", "test_email", "", adminID)
	h.AdminTestEmailTemplate(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("second test status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func assertAuthAuditEvent(t *testing.T, pool *pgxpool.Pool, adminID, eventType, templateKey string) {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*)
		   FROM auth_audit_log
		  WHERE user_id = $1
		    AND event_type = $2
		    AND details->>'template_key' = $3`,
		adminID, eventType, templateKey,
	).Scan(&count); err != nil {
		t.Fatalf("query auth audit log: %v", err)
	}
	if count != 1 {
		t.Fatalf("auth audit event %s/%s count=%d, want 1", eventType, templateKey, count)
	}
}
