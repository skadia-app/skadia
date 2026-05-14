package auth

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"time"

	gomail "github.com/wneessen/go-mail"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/settings"
)

//go:embed templates/*.html templates/*.txt
var templateFS embed.FS

const (
	emailVerificationTTL    = 24 * time.Hour
	passwordResetEmailTTL   = time.Hour
	inviteEmailTTL          = 24 * time.Hour
	emailChangeEmailTTL     = 24 * time.Hour
	accountDeletionEmailTTL = 30 * 24 * time.Hour
	adminApprovalEmailTTL   = 24 * time.Hour
)

// Mailer handles sending auth-related emails.
type Mailer struct {
	settings   *settings.SettingsStore
	templates  *EmailTemplateStore
	outbox     *EmailOutboxStore
	deliverRaw func(to, subject, templateName, htmlBody, textBody string) error
}

// NewMailer creates a mailer backed by the SettingsStore.
func NewMailer(s *settings.SettingsStore) *Mailer {
	return &Mailer{settings: s, templates: NewEmailTemplateStore(nil)}
}

// NewMailerWithDB creates a mailer that persists outgoing email jobs to the DB.
func NewMailerWithDB(db *database.DB, s *settings.SettingsStore, outboxEncryptionKey ...string) *Mailer {
	key := ""
	if len(outboxEncryptionKey) > 0 {
		key = outboxEncryptionKey[0]
	}
	m := &Mailer{
		settings:  s,
		templates: NewEmailTemplateStore(db),
	}
	if key != "" {
		m.outbox = NewEmailOutboxStore(db, key)
	}
	return m
}

func (m *Mailer) SetDeliverRawForTest(fn func(to, subject, templateName, htmlBody, textBody string) error) {
	m.deliverRaw = fn
}

// IsConfigured returns true if SMTP is complete enough to send email.
func (m *Mailer) IsConfigured() bool {
	return m != nil && m.settings.EmailDeliveryConfigured()
}

func (m *Mailer) HasOutbox() bool {
	return m != nil && m.outbox != nil
}

func (m *Mailer) GetEmailTemplate(ctx context.Context, key EmailTemplateKey) (EmailTemplate, error) {
	return m.templates.GetEffective(ctx, key)
}

func (m *Mailer) SaveEmailTemplate(ctx context.Context, key EmailTemplateKey, input EmailTemplateOverride, adminID string) error {
	return m.templates.SaveOverride(ctx, key, input, adminID)
}

func (m *Mailer) ResetEmailTemplate(ctx context.Context, key EmailTemplateKey) error {
	return m.templates.ResetOverride(ctx, key)
}

func (m *Mailer) PreviewEmailTemplate(ctx context.Context, key EmailTemplateKey, payload map[string]string) (RenderedEmail, error) {
	return m.templates.Render(ctx, key, payload)
}

func (m *Mailer) PreviewEmailTemplateOverride(key EmailTemplateKey, input EmailTemplateOverride, payload map[string]string) (RenderedEmail, error) {
	return RenderEmailTemplateOverride(key, input, payload)
}

func (m *Mailer) SamplePayload(key EmailTemplateKey) map[string]string {
	return EmailTemplateSamplePayload(key, m.settings.Get("app.name").Value, m.settings.Get("app.url").Value)
}

func (m *Mailer) SendTemplateTestEmail(ctx context.Context, key EmailTemplateKey, to, username string) error {
	payload := m.SamplePayload(key)
	payload["Username"] = username
	rendered, err := m.templates.Render(ctx, key, payload)
	if err != nil {
		return err
	}
	return m.sendRaw(to, rendered.Subject, string(key), rendered.HTMLBody, rendered.TextBody)
}

// SendVerificationEmail sends an email verification link asynchronously.
func (m *Mailer) SendVerificationEmail(userID, to, username, token string) {
	frontendURL := m.settings.Get("app.url").Value
	m.sendOrEnqueue(userID, to, EmailTemplateVerification, time.Now().Add(emailVerificationTTL), map[string]string{
		"AppName":   m.settings.Get("app.name").Value,
		"AppURL":    frontendURL,
		"Username":  username,
		"ActionURL": fmt.Sprintf("%s/verify-email?token=%s", frontendURL, token),
	})
}

// SendResetEmail sends a password reset link asynchronously.
func (m *Mailer) SendResetEmail(userID, to, username, token string) {
	frontendURL := m.settings.Get("app.url").Value
	m.sendOrEnqueue(userID, to, EmailTemplatePasswordReset, time.Now().Add(passwordResetEmailTTL), map[string]string{
		"AppName":   m.settings.Get("app.name").Value,
		"AppURL":    frontendURL,
		"Username":  username,
		"ActionURL": fmt.Sprintf("%s/reset-password?token=%s", frontendURL, token),
	})
}

// SendInviteEmail sends an invite email with a reset-password link so the
// invited user can set their initial password. Called asynchronously.
func (m *Mailer) SendInviteEmail(userID, to, username, token string) {
	frontendURL := m.settings.Get("app.url").Value
	m.sendOrEnqueue(userID, to, EmailTemplateInviteUser, time.Now().Add(inviteEmailTTL), map[string]string{
		"AppName":   m.settings.Get("app.name").Value,
		"AppURL":    frontendURL,
		"Username":  username,
		"ActionURL": fmt.Sprintf("%s/reset-password?token=%s", frontendURL, token),
	})
}

// SendChangeEmailVerification sends a verification link for an email change request.
// The link is sent to the NEW email address (not the old one).
// Called synchronously — the caller wraps in a goroutine.
func (m *Mailer) SendChangeEmailVerification(userID, to, username, token string) {
	frontendURL := m.settings.Get("app.url").Value
	m.sendOrEnqueue(userID, to, EmailTemplateChangeEmail, time.Now().Add(emailChangeEmailTTL), map[string]string{
		"AppName":   m.settings.Get("app.name").Value,
		"AppURL":    frontendURL,
		"Username":  username,
		"ActionURL": fmt.Sprintf("%s/confirm-email-change?token=%s", frontendURL, token),
	})
}

// SendDeletionEmail sends an account deletion confirmation with a cancel link.
// Called asynchronously via goroutine by the caller.
func (m *Mailer) SendDeletionEmail(userID, to, username, cancelToken string) {
	frontendURL := m.settings.Get("app.url").Value
	m.sendOrEnqueue(userID, to, EmailTemplateAccountDeletion, time.Now().Add(accountDeletionEmailTTL), map[string]string{
		"AppName":   m.settings.Get("app.name").Value,
		"AppURL":    frontendURL,
		"Username":  username,
		"ActionURL": fmt.Sprintf("%s/cancel-deletion?token=%s", frontendURL, cancelToken),
	})
}

// SendAdminApprovalNeeded notifies an admin that a new registration is pending approval.
func (m *Mailer) SendAdminApprovalNeeded(userID, to, newUsername, newEmail string) {
	frontendURL := m.settings.Get("app.url").Value
	m.sendOrEnqueue(userID, to, EmailTemplateAdminApprovalNeeded, time.Now().Add(adminApprovalEmailTTL), map[string]string{
		"AppName":  m.settings.Get("app.name").Value,
		"AppURL":   frontendURL,
		"Username": newUsername,
		"Email":    newEmail,
		"AdminURL": fmt.Sprintf("%s/admin/users", frontendURL),
	})
}

func (m *Mailer) sendOrEnqueue(userID, to string, templateKey EmailTemplateKey, expiresAt time.Time, payload map[string]string) {
	if !m.IsConfigured() {
		slog.Warn("email not queued: SMTP not configured", "to", to, "template", templateKey)
		return
	}
	if m.outbox != nil {
		if _, err := m.outbox.Enqueue(context.Background(), EmailJob{
			UserID:      userID,
			TemplateKey: templateKey,
			Recipient:   to,
			Payload:     payload,
			ExpiresAt:   expiresAt,
		}); err != nil {
			slog.Error("enqueue email failed", "to", to, "template", templateKey, "error", err)
		}
		return
	}
	go m.sendTemplate(to, templateKey, payload)
}

func (m *Mailer) sendTemplate(to string, templateKey EmailTemplateKey, payload map[string]string) error {
	if !m.IsConfigured() {
		return fmt.Errorf("SMTP not configured")
	}
	rendered, err := m.templates.Render(context.Background(), templateKey, payload)
	if err != nil {
		return err
	}
	return m.sendRaw(to, rendered.Subject, string(templateKey), rendered.HTMLBody, rendered.TextBody)
}

// SendTestEmail sends a test email synchronously and returns any error
// so the caller (admin endpoint) can report it to the frontend.
func (m *Mailer) SendTestEmail(to, username string) error {
	if !m.IsConfigured() {
		return fmt.Errorf("SMTP not configured")
	}
	appName := m.settings.Get("app.name").Value
	frontendURL := m.settings.Get("app.url").Value
	rendered, err := m.templates.Render(context.Background(), EmailTemplateTest, map[string]string{
		"AppName":  appName,
		"AppURL":   frontendURL,
		"Username": username,
	})
	if err != nil {
		return err
	}
	return m.sendRaw(to, rendered.Subject, string(EmailTemplateTest), rendered.HTMLBody, rendered.TextBody)
}

// sendRaw creates an SMTP client and dispatches the message.
// Retries up to 2 times on transient dial/send errors (1s, 3s backoff).
func (m *Mailer) sendRaw(to, subject, templateName, htmlBody, textBody string) error {
	if m.deliverRaw != nil {
		return m.deliverRaw(to, subject, templateName, htmlBody, textBody)
	}

	cfg, ok := m.settings.EmailDeliveryConfig()
	if !ok {
		return fmt.Errorf("SMTP not configured")
	}

	msg := gomail.NewMsg()
	if err := msg.From(cfg.From); err != nil {
		return fmt.Errorf("set from: %w", err)
	}
	if err := msg.To(to); err != nil {
		return fmt.Errorf("set to: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(gomail.TypeTextHTML, htmlBody)
	msg.AddAlternativeString(gomail.TypeTextPlain, textBody)

	// Build client options based on TLS mode.
	opts := []gomail.Option{
		gomail.WithPort(cfg.Port),
		gomail.WithTimeout(5 * time.Second),
	}

	// Only enable SMTP auth when credentials are provided.
	// Relay servers without auth would reject an unsolicited PLAIN attempt.
	if cfg.HasAuth {
		opts = append(opts,
			gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
			gomail.WithUsername(cfg.Username),
			gomail.WithPassword(cfg.Password),
		)
	}

	switch cfg.TLSMode {
	case "ssl":
		// Implicit SSL/TLS (port 465) — direct TLS connection, no STARTTLS negotiation.
		opts = append(opts, gomail.WithSSL())
	case "none":
		opts = append(opts, gomail.WithTLSPortPolicy(gomail.NoTLS))
	default: // "starttls"
		opts = append(opts, gomail.WithTLSPortPolicy(gomail.TLSMandatory))
	}

	const maxAttempts = 3
	backoff := 1 * time.Second
	var lastErr error

	for attempt := range maxAttempts {
		client, err := gomail.NewClient(cfg.Host, opts...)
		if err != nil {
			return fmt.Errorf("create client: %w", err)
		}
		if err := client.DialAndSend(msg); err != nil {
			lastErr = err
			if attempt < maxAttempts-1 {
				slog.Warn("email send failed, retrying",
					"template", templateName, "to", to,
					"attempt", attempt+1, "backoff", backoff, "error", err)
				time.Sleep(backoff)
				backoff *= 3
				continue
			}
		} else {
			return nil
		}
	}
	return fmt.Errorf("dial and send (after %d attempts): %w", maxAttempts, lastErr)
}
