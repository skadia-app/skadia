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
	if !m.IsConfigured() {
		slog.Warn("email not sent: SMTP not configured", "to", to, "template", "admin_approval_needed")
		return
	}

	var htmlBuf, textBuf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&htmlBuf, "templates/admin_approval_needed.html", data); err != nil {
		slog.Error("render email HTML template failed", "template", "admin_approval_needed", "error", err)
		return
	}
	if err := tmpl.ExecuteTemplate(&textBuf, "templates/admin_approval_needed.txt", data); err != nil {
		slog.Error("render email text template failed", "template", "admin_approval_needed", "error", err)
		return
	}

	msg := gomail.NewMsg()
	if err := msg.From(m.from); err != nil {
		slog.Error("set email from failed", "error", err)
		return
	}
	if err := msg.To(to); err != nil {
		slog.Error("set email to failed", "error", err)
		return
	}
	msg.Subject(subject)
	msg.SetBodyString(gomail.TypeTextHTML, htmlBuf.String())
	msg.AddAlternativeString(gomail.TypeTextPlain, textBuf.String())

	var tlsPolicy gomail.TLSPolicy
	switch m.tlsMode {
	case "ssl":
		tlsPolicy = gomail.TLSMandatory
	case "none":
		tlsPolicy = gomail.NoTLS
	default: // "starttls"
		tlsPolicy = gomail.TLSOpportunistic
	}

	client, err := gomail.NewClient(m.host,
		gomail.WithPort(m.port),
		gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
		gomail.WithUsername(m.username),
		gomail.WithPassword(m.password),
		gomail.WithTLSPortPolicy(tlsPolicy),
		gomail.WithTimeout(10*time.Second),
	)
	if err != nil {
		slog.Error("create mail client failed", "error", err)
		return
	}
	if err := client.DialAndSend(msg); err != nil {
		slog.Error("send email failed", "to", to, "template", "admin_approval_needed", "error", err)
		return
	}
	slog.Info("email sent", "to", to, "template", "admin_approval_needed")
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

	msg := gomail.NewMsg()
	if err := msg.From(m.from); err != nil {
		slog.Error("set email from failed", "error", err)
		return
	}
	if err := msg.To(to); err != nil {
		slog.Error("set email to failed", "error", err)
		return
	}
	msg.Subject(subject)
	msg.SetBodyString(gomail.TypeTextHTML, htmlBuf.String())
	msg.AddAlternativeString(gomail.TypeTextPlain, textBuf.String())

	var tlsPolicy gomail.TLSPolicy
	switch m.tlsMode {
	case "ssl":
		tlsPolicy = gomail.TLSMandatory
	case "none":
		tlsPolicy = gomail.NoTLS
	default: // "starttls"
		tlsPolicy = gomail.TLSOpportunistic
	}

	client, err := gomail.NewClient(m.host,
		gomail.WithPort(m.port),
		gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
		gomail.WithUsername(m.username),
		gomail.WithPassword(m.password),
		gomail.WithTLSPortPolicy(tlsPolicy),
		gomail.WithTimeout(10*time.Second),
	)
	if err != nil {
		slog.Error("create mail client failed", "error", err)
		return
	}
	if err := client.DialAndSend(msg); err != nil {
		slog.Error("send email failed", "to", to, "template", templateName, "error", err)
		return
	}
	slog.Info("email sent", "to", to, "template", templateName)
}
