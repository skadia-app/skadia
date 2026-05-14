package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/settings"
)

const testOutboxEncryptionKey = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

func testMailDB(t *testing.T) *database.DB {
	t.Helper()
	pool := testdb.Open(t)
	t.Cleanup(pool.Close)
	return &database.DB{Pool: pool}
}

func cleanupMailTables(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := context.Background()
	_, _ = db.Pool.Exec(ctx, `DELETE FROM email_outbox`)
	_, _ = db.Pool.Exec(ctx, `DELETE FROM email_templates WHERE key = ANY($1)`, []string{
		string(EmailTemplatePasswordReset),
		string(EmailTemplateVerification),
	})
}

func TestEmailTemplateStoreFallsBackToEmbeddedDefault(t *testing.T) {
	db := testMailDB(t)
	cleanupMailTables(t, db)
	store := NewEmailTemplateStore(db)

	template, err := store.GetEffective(context.Background(), EmailTemplatePasswordReset)
	if err != nil {
		t.Fatalf("GetEffective: %v", err)
	}
	if template.Source != "default" {
		t.Fatalf("expected default template source, got %q", template.Source)
	}

	rendered, err := store.Render(context.Background(), EmailTemplatePasswordReset, map[string]string{
		"AppName":   "Skadia",
		"AppURL":    "https://fit.example.test",
		"Username":  "ada",
		"ActionURL": "https://fit.example.test/reset-password?token=raw",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(rendered.HTMLBody, "https://fit.example.test/reset-password?token=raw") {
		t.Fatalf("rendered HTML body does not contain action URL: %s", rendered.HTMLBody)
	}
	if !strings.Contains(rendered.TextBody, "https://fit.example.test/reset-password?token=raw") {
		t.Fatalf("rendered text body does not contain action URL: %s", rendered.TextBody)
	}
	if rendered.Subject == "" {
		t.Fatal("rendered subject must not be empty")
	}
}

func TestEmailTemplateStoreRejectsUnknownVariables(t *testing.T) {
	db := testMailDB(t)
	cleanupMailTables(t, db)
	store := NewEmailTemplateStore(db)

	err := store.SaveOverride(context.Background(), EmailTemplatePasswordReset, EmailTemplateOverride{
		Subject: "{{.Unknown}}",
		HTML:    "<p>{{.Username}}</p>",
		Text:    "{{.Username}}",
		Enabled: true,
	}, "00000000-0000-0000-0000-000000000001")
	if !errors.Is(err, ErrEmailTemplateValidation) {
		t.Fatalf("expected validation error for unknown variable, got %v", err)
	}
}

func TestEmailTemplateStoreRejectsDisablingMandatoryTemplates(t *testing.T) {
	db := testMailDB(t)
	cleanupMailTables(t, db)
	store := NewEmailTemplateStore(db)

	err := store.SaveOverride(context.Background(), EmailTemplatePasswordReset, EmailTemplateOverride{
		Subject: "[{{.AppName}}] reset",
		HTML:    "<p>{{.ActionURL}}</p>",
		Text:    "{{.ActionURL}}",
		Enabled: false,
	}, "00000000-0000-0000-0000-000000000001")
	if !errors.Is(err, ErrEmailTemplateValidation) {
		t.Fatalf("expected validation error for disabled mandatory template, got %v", err)
	}
}

func TestEmailOutboxClaimPreventsDoubleDelivery(t *testing.T) {
	db := testMailDB(t)
	cleanupMailTables(t, db)
	store := NewEmailOutboxStore(db, testOutboxEncryptionKey)
	ctx := context.Background()

	jobID, err := store.Enqueue(ctx, EmailJob{
		TemplateKey: EmailTemplatePasswordReset,
		Recipient:   "claim-once@example.test",
		Payload: map[string]string{
			"AppName":   "Skadia",
			"AppURL":    "https://fit.example.test",
			"Username":  "ada",
			"ActionURL": "https://fit.example.test/reset-password?token=raw",
		},
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	first, err := store.Claim(ctx, "worker-a", 5, time.Now())
	if err != nil {
		t.Fatalf("first Claim: %v", err)
	}
	if len(first) != 1 || first[0].ID != jobID {
		t.Fatalf("expected worker-a to claim job %s, got %#v", jobID, first)
	}

	second, err := store.Claim(ctx, "worker-b", 5, time.Now())
	if err != nil {
		t.Fatalf("second Claim: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("expected worker-b to claim no already-running jobs, got %#v", second)
	}

	if err := store.MarkSent(ctx, jobID); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
}

func TestEmailOutboxEncryptsQueuedRecipientAndPayload(t *testing.T) {
	db := testMailDB(t)
	cleanupMailTables(t, db)
	store := NewEmailOutboxStore(db, testOutboxEncryptionKey)
	ctx := context.Background()

	jobID, err := store.Enqueue(ctx, EmailJob{
		TemplateKey: EmailTemplatePasswordReset,
		Recipient:   "encrypted-recipient@example.test",
		Payload: map[string]string{
			"AppName":   "Skadia",
			"AppURL":    "https://fit.example.test",
			"Username":  "ada",
			"ActionURL": "https://fit.example.test/reset-password?token=secret-token",
		},
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	var recipientCiphertext, payloadCiphertext string
	if err := db.Pool.QueryRow(ctx,
		`SELECT recipient_ciphertext, payload_ciphertext FROM email_outbox WHERE id = $1`,
		jobID,
	).Scan(&recipientCiphertext, &payloadCiphertext); err != nil {
		t.Fatalf("query encrypted job: %v", err)
	}
	if strings.Contains(recipientCiphertext, "encrypted-recipient@example.test") || strings.Contains(payloadCiphertext, "secret-token") {
		t.Fatalf("outbox stored plaintext recipient/payload: recipient=%q payload=%q", recipientCiphertext, payloadCiphertext)
	}

	claimed, err := store.Claim(ctx, "decrypt-worker", 1, time.Now())
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed jobs=%d, want 1", len(claimed))
	}
	if claimed[0].Recipient != "encrypted-recipient@example.test" || claimed[0].Payload["ActionURL"] == "" {
		t.Fatalf("claimed decrypted job = %#v", claimed[0])
	}
}

func TestAdminApprovalNotificationStoresPendingUserID(t *testing.T) {
	db := testMailDB(t)
	cleanupMailTables(t, db)
	ctx := context.Background()
	pendingUserID := "00000000-0000-0000-0000-000000000011"
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, 'pending-user', 'pending-user@example.test', false, 'email')
		 ON CONFLICT (id) DO NOTHING`,
		pendingUserID,
	); err != nil {
		t.Fatalf("insert pending user: %v", err)
	}

	st := settings.NewStoreForTest(map[string]string{
		"app.name":  "Skadia",
		"app.url":   "https://fit.example.test",
		"smtp.host": "smtp.example.test",
		"smtp.port": "587",
		"smtp.from": "noreply@example.test",
		"smtp.tls":  "starttls",
	})
	mailer := NewMailerWithDB(db, st, testOutboxEncryptionKey)

	mailer.SendAdminApprovalNeeded(pendingUserID, "admin@example.test", "pending-user", "pending-user@example.test")

	var storedUserID string
	if err := db.Pool.QueryRow(ctx,
		`SELECT user_id::text FROM email_outbox WHERE template_key = $1`,
		string(EmailTemplateAdminApprovalNeeded),
	).Scan(&storedUserID); err != nil {
		t.Fatalf("query admin approval outbox row: %v", err)
	}
	if storedUserID != pendingUserID {
		t.Fatalf("stored user_id=%q, want pending user %q", storedUserID, pendingUserID)
	}
}

func TestEmailOutboxRedactsTerminalJobs(t *testing.T) {
	db := testMailDB(t)
	cleanupMailTables(t, db)
	store := NewEmailOutboxStore(db, testOutboxEncryptionKey)
	ctx := context.Background()
	userID := "00000000-0000-0000-0000-000000000001"
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, 'outbox-user', 'outbox@example.test', true, 'test')
		 ON CONFLICT (id) DO NOTHING`,
		userID,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	jobID, err := store.Enqueue(ctx, EmailJob{
		UserID:      userID,
		TemplateKey: EmailTemplatePasswordReset,
		Recipient:   "reset@example.test",
		Payload: map[string]string{
			"AppName":   "Skadia",
			"AppURL":    "https://fit.example.test",
			"Username":  "ada",
			"ActionURL": "https://fit.example.test/reset-password?token=secret-token",
		},
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := store.MarkSent(ctx, jobID); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	var recipient string
	var payload map[string]string
	if err := db.Pool.QueryRow(ctx, `SELECT recipient, payload FROM email_outbox WHERE id = $1`, jobID).Scan(&recipient, &payload); err != nil {
		t.Fatalf("query sent job: %v", err)
	}
	if recipient != "" || len(payload) != 0 {
		t.Fatalf("terminal job retained recipient/payload: recipient=%q payload=%v", recipient, payload)
	}
}

func TestEmailWorkerExpiresStalePendingJob(t *testing.T) {
	db := testMailDB(t)
	cleanupMailTables(t, db)
	store := NewEmailOutboxStore(db, testOutboxEncryptionKey)
	ctx := context.Background()

	jobID, err := store.Enqueue(ctx, EmailJob{
		TemplateKey: EmailTemplatePasswordReset,
		Recipient:   "expired@example.test",
		Payload: map[string]string{
			"AppName":   "Skadia",
			"AppURL":    "https://fit.example.test",
			"Username":  "ada",
			"ActionURL": "https://fit.example.test/reset-password?token=expired-token",
		},
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE email_outbox SET expires_at = now() - interval '1 minute' WHERE id = $1`, jobID); err != nil {
		t.Fatalf("expire job: %v", err)
	}

	st := settings.NewStoreForTest(map[string]string{
		"app.name":  "Skadia",
		"app.url":   "https://fit.example.test",
		"smtp.host": "smtp.example.test",
		"smtp.port": "587",
		"smtp.from": "noreply@example.test",
		"smtp.tls":  "starttls",
	})
	mailer := NewMailerWithDB(db, st, testOutboxEncryptionKey)
	mailer.deliverRaw = func(to, subject, templateName, htmlBody, textBody string) error {
		t.Fatalf("expired job must not be delivered")
		return nil
	}

	worker := NewEmailWorker("expiry-worker", mailer, 5)
	if err := worker.ProcessOnce(ctx); err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}

	var status, recipient string
	var payload map[string]string
	if err := db.Pool.QueryRow(ctx, `SELECT status, recipient, payload FROM email_outbox WHERE id = $1`, jobID).Scan(&status, &recipient, &payload); err != nil {
		t.Fatalf("query expired job: %v", err)
	}
	if status != "failed" || recipient != "" || len(payload) != 0 {
		t.Fatalf("expired job status/recipient/payload = %q/%q/%v, want failed/empty/empty", status, recipient, payload)
	}
}

func TestEmailWorkerExpiresStaleRunningJob(t *testing.T) {
	db := testMailDB(t)
	cleanupMailTables(t, db)
	store := NewEmailOutboxStore(db, testOutboxEncryptionKey)
	ctx := context.Background()

	jobID, err := store.Enqueue(ctx, EmailJob{
		TemplateKey: EmailTemplatePasswordReset,
		Recipient:   "expired-running@example.test",
		Payload: map[string]string{
			"AppName":   "Skadia",
			"AppURL":    "https://fit.example.test",
			"Username":  "ada",
			"ActionURL": "https://fit.example.test/reset-password?token=expired-running-token",
		},
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := store.Claim(ctx, "dead-worker", 1, time.Now()); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		`UPDATE email_outbox
		    SET expires_at = now() - interval '1 minute',
		        claimed_at = now() - interval '20 minutes'
		  WHERE id = $1`,
		jobID,
	); err != nil {
		t.Fatalf("age running job: %v", err)
	}

	st := settings.NewStoreForTest(map[string]string{
		"app.name":  "Skadia",
		"app.url":   "https://fit.example.test",
		"smtp.host": "smtp.example.test",
		"smtp.port": "587",
		"smtp.from": "noreply@example.test",
		"smtp.tls":  "starttls",
	})
	mailer := NewMailerWithDB(db, st, testOutboxEncryptionKey)
	mailer.deliverRaw = func(to, subject, templateName, htmlBody, textBody string) error {
		t.Fatalf("expired running job must not be delivered")
		return nil
	}

	worker := NewEmailWorker("expiry-worker", mailer, 5)
	if err := worker.ProcessOnce(ctx); err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}

	var status, recipient string
	var payload map[string]string
	if err := db.Pool.QueryRow(ctx, `SELECT status, recipient, payload FROM email_outbox WHERE id = $1`, jobID).Scan(&status, &recipient, &payload); err != nil {
		t.Fatalf("query expired running job: %v", err)
	}
	if status != "failed" || recipient != "" || len(payload) != 0 {
		t.Fatalf("expired running job status/recipient/payload = %q/%q/%v, want failed/empty/empty", status, recipient, payload)
	}
}

func TestEmailWorkerRequeuesStaleRunningJob(t *testing.T) {
	db := testMailDB(t)
	cleanupMailTables(t, db)
	ctx := context.Background()
	store := NewEmailOutboxStore(db, testOutboxEncryptionKey)

	jobID, err := store.Enqueue(ctx, EmailJob{
		TemplateKey: EmailTemplatePasswordReset,
		Recipient:   "stale-claim@example.test",
		Payload: map[string]string{
			"AppName":   "Skadia",
			"AppURL":    "https://fit.example.test",
			"Username":  "ada",
			"ActionURL": "https://fit.example.test/reset-password?token=raw",
		},
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	claimed, err := store.Claim(ctx, "dead-worker", 1, time.Now())
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected initial claim, got %#v", claimed)
	}
	if _, err := db.Pool.Exec(ctx,
		`UPDATE email_outbox SET claimed_at = $2 WHERE id = $1`,
		jobID, time.Now().Add(-emailRunningJobTimeout-time.Minute),
	); err != nil {
		t.Fatalf("age claimed job: %v", err)
	}

	st := settings.NewStoreForTest(map[string]string{
		"app.name":  "Skadia",
		"app.url":   "https://fit.example.test",
		"smtp.host": "smtp.example.test",
		"smtp.port": "587",
		"smtp.from": "noreply@example.test",
		"smtp.tls":  "starttls",
	})
	mailer := NewMailerWithDB(db, st, testOutboxEncryptionKey)
	var deliveredTo string
	mailer.deliverRaw = func(to, subject, templateName, htmlBody, textBody string) error {
		deliveredTo = to
		return nil
	}

	worker := NewEmailWorker("replacement-worker", mailer, 5)
	if err := worker.ProcessOnce(ctx); err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	if deliveredTo != "stale-claim@example.test" {
		t.Fatalf("delivered to %q, want stale-claim@example.test", deliveredTo)
	}

	var status, claimedBy string
	if err := db.Pool.QueryRow(ctx, `SELECT status, claimed_by FROM email_outbox WHERE id = $1`, jobID).Scan(&status, &claimedBy); err != nil {
		t.Fatalf("query job status: %v", err)
	}
	if status != "sent" || claimedBy != "replacement-worker" {
		t.Fatalf("job status/claim = %q/%q, want sent/replacement-worker", status, claimedBy)
	}
}

func TestEmailWorkerSendsClaimedJobAndMarksSent(t *testing.T) {
	db := testMailDB(t)
	cleanupMailTables(t, db)
	ctx := context.Background()

	st := settings.NewStoreForTest(map[string]string{
		"app.name":      "Skadia",
		"app.url":       "https://fit.example.test",
		"smtp.host":     "smtp.example.test",
		"smtp.port":     "587",
		"smtp.from":     "noreply@example.test",
		"smtp.username": "",
		"smtp.password": "",
		"smtp.tls":      "starttls",
	})
	mailer := NewMailerWithDB(db, st, testOutboxEncryptionKey)

	var deliveredTo, deliveredSubject, deliveredHTML string
	mailer.deliverRaw = func(to, subject, templateName, htmlBody, textBody string) error {
		deliveredTo = to
		deliveredSubject = subject
		deliveredHTML = htmlBody
		return nil
	}

	jobID, err := NewEmailOutboxStore(db, testOutboxEncryptionKey).Enqueue(ctx, EmailJob{
		TemplateKey: EmailTemplatePasswordReset,
		Recipient:   "worker-send@example.test",
		Payload: map[string]string{
			"AppName":   "Skadia",
			"AppURL":    "https://fit.example.test",
			"Username":  "ada",
			"ActionURL": "https://fit.example.test/reset-password?token=raw",
		},
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	worker := NewEmailWorker("worker-test", mailer, 5)
	if err := worker.ProcessOnce(ctx); err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}

	if deliveredTo != "worker-send@example.test" {
		t.Fatalf("delivered to %q, want worker-send@example.test", deliveredTo)
	}
	if !strings.Contains(deliveredSubject, "Skadia") {
		t.Fatalf("subject does not contain app name: %q", deliveredSubject)
	}
	if !strings.Contains(deliveredHTML, "https://fit.example.test/reset-password?token=raw") {
		t.Fatalf("html body does not contain action URL: %s", deliveredHTML)
	}

	var status string
	if err := db.Pool.QueryRow(ctx, `SELECT status FROM email_outbox WHERE id = $1`, jobID).Scan(&status); err != nil {
		t.Fatalf("query job status: %v", err)
	}
	if status != "sent" {
		t.Fatalf("job status = %q, want sent", status)
	}
}

func TestEmailWorkerRetriesUntilExpiryInsteadOfMaxAttempts(t *testing.T) {
	db := testMailDB(t)
	cleanupMailTables(t, db)
	ctx := context.Background()
	store := NewEmailOutboxStore(db, testOutboxEncryptionKey)

	jobID, err := store.Enqueue(ctx, EmailJob{
		TemplateKey: EmailTemplatePasswordReset,
		Recipient:   "retry-until-expiry@example.test",
		Payload: map[string]string{
			"AppName":   "Skadia",
			"AppURL":    "https://fit.example.test",
			"Username":  "ada",
			"ActionURL": "https://fit.example.test/reset-password?token=still-valid",
		},
		ExpiresAt: time.Now().Add(23 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE email_outbox SET attempt_count = max_attempts WHERE id = $1`, jobID); err != nil {
		t.Fatalf("prime attempts: %v", err)
	}
	if err := store.MarkFailed(ctx, jobID, errors.New("temporary smtp outage"), time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	var status, recipientCiphertext, payloadCiphertext string
	var payload map[string]string
	if err := db.Pool.QueryRow(ctx, `SELECT status, recipient_ciphertext, payload_ciphertext, payload FROM email_outbox WHERE id = $1`, jobID).Scan(&status, &recipientCiphertext, &payloadCiphertext, &payload); err != nil {
		t.Fatalf("query retry job: %v", err)
	}
	if status != "pending" || recipientCiphertext == "" || payloadCiphertext == "" || len(payload) != 0 {
		t.Fatalf("retryable job status/recipient_ciphertext/payload_ciphertext/payload = %q/%q/%q/%v, want pending/retained ciphertext/redacted plaintext", status, recipientCiphertext, payloadCiphertext, payload)
	}
}
