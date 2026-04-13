package auth

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"time"

	gomail "github.com/wneessen/go-mail"

	"github.com/skadia-app/skadia/services/api/internal/config"
)

//go:embed templates/*.html templates/*.txt
var templateFS embed.FS

var tmpl *template.Template

func init() {
	tmpl = template.Must(template.ParseFS(templateFS, "templates/*.html", "templates/*.txt"))
}

// Mailer handles sending auth-related emails.
type Mailer struct {
	host        string
	port        int
	username    string
	password    string
	from        string
	appName     string
	frontendURL string // for email action links
	tlsMode     string // starttls | ssl | none
}

// NewMailer creates a mailer from config.
func NewMailer(cfg *config.Config) *Mailer {
	return &Mailer{
		host:        cfg.SMTPHost,
		port:        cfg.SMTPPort,
		username:    cfg.SMTPUsername,
		password:    cfg.SMTPPassword,
		from:        cfg.SMTPFrom,
		appName:     cfg.AppName,
		frontendURL: cfg.FrontendURL,
		tlsMode:     cfg.SMTPTLSMode,
	}
}

// IsConfigured returns true if SMTP is configured.
func (m *Mailer) IsConfigured() bool {
	return m.host != ""
}

type emailData struct {
	AppName   string
	AppURL    string
	Username  string
	ActionURL string
}

// adminApprovalData is the template data for the admin_approval_needed email.
type adminApprovalData struct {
	AppName  string
	AppURL   string
	Username string
	Email    string
	AdminURL string
}

// SendVerificationEmail sends an email verification link asynchronously.
func (m *Mailer) SendVerificationEmail(to, username, token string) {
	data := emailData{
		AppName:   m.appName,
		AppURL:    m.frontendURL,
		Username:  username,
		ActionURL: fmt.Sprintf("%s/verify-email?token=%s", m.frontendURL, token),
	}
	go m.send(to, fmt.Sprintf("[%s] Email verifizieren", m.appName), "verify_email", data)
}

// SendResetEmail sends a password reset link asynchronously.
func (m *Mailer) SendResetEmail(to, username, token string) {
	data := emailData{
		AppName:   m.appName,
		AppURL:    m.frontendURL,
		Username:  username,
		ActionURL: fmt.Sprintf("%s/reset-password?token=%s", m.frontendURL, token),
	}
	go m.send(to, fmt.Sprintf("[%s] Passwort zurücksetzen", m.appName), "reset_password", data)
}

// SendInviteEmail sends an invite email with a reset-password link so the
// invited user can set their initial password. Called asynchronously.
func (m *Mailer) SendInviteEmail(to, username, token string) {
	data := emailData{
		AppName:   m.appName,
		AppURL:    m.frontendURL,
		Username:  username,
		ActionURL: fmt.Sprintf("%s/reset-password?token=%s", m.frontendURL, token),
	}
	go m.send(to, fmt.Sprintf("[%s] Einladung — Konto einrichten", m.appName), "invite_user", data)
}

// SendChangeEmailVerification sends a verification link for an email change request.
// The link is sent to the NEW email address (not the old one).
// Called synchronously — the caller wraps in a goroutine.
func (m *Mailer) SendChangeEmailVerification(to, username, token string) {
	data := emailData{
		AppName:   m.appName,
		AppURL:    m.frontendURL,
		Username:  username,
		ActionURL: fmt.Sprintf("%s/confirm-email-change?token=%s", m.frontendURL, token),
	}
	m.send(to, fmt.Sprintf("[%s] Email-Adresse bestätigen", m.appName), "change_email", data)
}

// SendDeletionEmail sends an account deletion confirmation with a cancel link.
// Called asynchronously via goroutine by the caller.
func (m *Mailer) SendDeletionEmail(to, username, cancelToken string) {
	data := emailData{
		AppName:   m.appName,
		AppURL:    m.frontendURL,
		Username:  username,
		ActionURL: fmt.Sprintf("%s/cancel-deletion?token=%s", m.frontendURL, cancelToken),
	}
	m.send(to, fmt.Sprintf("[%s] Account-Löschung bestätigt", m.appName), "account_deletion", data)
}

// SendAdminApprovalNeeded notifies an admin that a new registration is pending approval.
func (m *Mailer) SendAdminApprovalNeeded(to, newUsername, newEmail string) {
	data := adminApprovalData{
		AppName:  m.appName,
		AppURL:   m.frontendURL,
		Username: newUsername,
		Email:    newEmail,
		AdminURL: fmt.Sprintf("%s/admin/users", m.frontendURL),
	}
	go m.sendApproval(to, fmt.Sprintf("[%s] Neue Registrierung wartet auf Freigabe", m.appName), data)
}

func (m *Mailer) sendApproval(to, subject string, data adminApprovalData) {
	templateName := "admin_approval_needed"
	if !m.IsConfigured() {
		slog.Warn("email not sent: SMTP not configured", "to", to, "template", templateName)
		return
	}

	var htmlBuf, textBuf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&htmlBuf, "templates/"+templateName+".html", data); err != nil {
		slog.Error("render email HTML template failed", "template", templateName, "error", err)
		return
	}
	if err := tmpl.ExecuteTemplate(&textBuf, "templates/"+templateName+".txt", data); err != nil {
		slog.Error("render email text template failed", "template", templateName, "error", err)
		return
	}

	if err := m.sendRaw(to, subject, templateName, htmlBuf.String(), textBuf.String()); err != nil {
		slog.Error("send email failed", "to", to, "template", templateName, "error", err)
		return
	}
	slog.Info("email sent", "to", to, "template", templateName)
}

func (m *Mailer) send(to, subject, templateName string, data emailData) {
	if !m.IsConfigured() {
		slog.Warn("email not sent: SMTP not configured", "to", to, "template", templateName)
		return
	}

	var htmlBuf, textBuf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&htmlBuf, "templates/"+templateName+".html", data); err != nil {
		slog.Error("render email HTML template failed", "template", templateName, "error", err)
		return
	}
	if err := tmpl.ExecuteTemplate(&textBuf, "templates/"+templateName+".txt", data); err != nil {
		slog.Error("render email text template failed", "template", templateName, "error", err)
		return
	}

	if err := m.sendRaw(to, subject, templateName, htmlBuf.String(), textBuf.String()); err != nil {
		slog.Error("send email failed", "to", to, "template", templateName, "error", err)
		return
	}
	slog.Info("email sent", "to", to, "template", templateName)
}

// SendTestEmail sends a test email synchronously and returns any error
// so the caller (admin endpoint) can report it to the frontend.
func (m *Mailer) SendTestEmail(to, username string) error {
	if !m.IsConfigured() {
		return fmt.Errorf("SMTP not configured")
	}
	data := emailData{
		AppName:  m.appName,
		AppURL:   m.frontendURL,
		Username: username,
	}
	var htmlBuf, textBuf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&htmlBuf, "templates/test_email.html", data); err != nil {
		return fmt.Errorf("render HTML template: %w", err)
	}
	if err := tmpl.ExecuteTemplate(&textBuf, "templates/test_email.txt", data); err != nil {
		return fmt.Errorf("render text template: %w", err)
	}
	return m.sendRaw(to, fmt.Sprintf("[%s] SMTP-Test", m.appName), "test_email", htmlBuf.String(), textBuf.String())
}

// sendRaw creates an SMTP client and dispatches the message.
// Retries up to 2 times on transient dial/send errors (1s, 3s backoff).
func (m *Mailer) sendRaw(to, subject, templateName, htmlBody, textBody string) error {
	msg := gomail.NewMsg()
	if err := msg.From(m.from); err != nil {
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
		gomail.WithPort(m.port),
		gomail.WithTimeout(5 * time.Second),
	}

	// Only enable SMTP auth when credentials are provided.
	// Relay servers without auth would reject an unsolicited PLAIN attempt.
	if m.username != "" || m.password != "" {
		opts = append(opts,
			gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
			gomail.WithUsername(m.username),
			gomail.WithPassword(m.password),
		)
	}

	switch m.tlsMode {
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
		client, err := gomail.NewClient(m.host, opts...)
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
