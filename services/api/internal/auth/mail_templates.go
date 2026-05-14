package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"strings"
	texttemplate "text/template"
	"time"

	"sort"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5"
)

type EmailTemplateKey string

const (
	EmailTemplateVerification        EmailTemplateKey = "verify_email"
	EmailTemplatePasswordReset       EmailTemplateKey = "reset_password"
	EmailTemplateInviteUser          EmailTemplateKey = "invite_user"
	EmailTemplateChangeEmail         EmailTemplateKey = "change_email"
	EmailTemplateAccountDeletion     EmailTemplateKey = "account_deletion"
	EmailTemplateAdminApprovalNeeded EmailTemplateKey = "admin_approval_needed"
	EmailTemplateTest                EmailTemplateKey = "test_email"
)

var (
	ErrEmailTemplateUnknown    = errors.New("unknown email template")
	ErrEmailTemplateValidation = errors.New("email template validation error")
	ErrEmailTemplateDisabled   = errors.New("email template disabled")
)

const maxEmailTemplateFieldBytes = 64 * 1024

type EmailTemplateDefinition struct {
	Key            EmailTemplateKey
	DefaultSubject string
	BaseName       string
	Variables      []string
}

type EmailTemplate struct {
	Key       EmailTemplateKey `json:"key"`
	Subject   string           `json:"subject"`
	HTML      string           `json:"html"`
	Text      string           `json:"text"`
	Enabled   bool             `json:"enabled"`
	Source    string           `json:"source"`
	UpdatedAt *time.Time       `json:"updated_at,omitempty"`
}

type EmailTemplateOverride struct {
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
	Enabled bool   `json:"enabled"`
}

type RenderedEmail struct {
	Subject  string `json:"subject"`
	HTMLBody string `json:"html"`
	TextBody string `json:"text"`
}

type EmailTemplateStore struct {
	db *database.DB
}

func NewEmailTemplateStore(db *database.DB) *EmailTemplateStore {
	return &EmailTemplateStore{db: db}
}

func ParseEmailTemplateKey(raw string) (EmailTemplateKey, error) {
	key := EmailTemplateKey(raw)
	if _, ok := emailTemplateRegistry[key]; !ok {
		return "", fmt.Errorf("%w: %s", ErrEmailTemplateUnknown, raw)
	}
	return key, nil
}

func EmailTemplateKeys() []EmailTemplateKey {
	keys := make([]EmailTemplateKey, 0, len(emailTemplateRegistry))
	for key := range emailTemplateRegistry {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func EmailTemplateSamplePayload(key EmailTemplateKey, appName, appURL string) map[string]string {
	payload := map[string]string{
		"AppName":   appName,
		"AppURL":    appURL,
		"Username":  "max",
		"ActionURL": strings.TrimRight(appURL, "/") + "/example-email-action?token=sample",
		"Email":     "user@example.test",
		"AdminURL":  strings.TrimRight(appURL, "/") + "/admin/users",
	}
	if def, ok := emailTemplateRegistry[key]; ok {
		filtered := make(map[string]string, len(def.Variables))
		for _, variable := range def.Variables {
			filtered[variable] = payload[variable]
		}
		return filtered
	}
	return payload
}

var emailTemplateRegistry = map[EmailTemplateKey]EmailTemplateDefinition{
	EmailTemplateVerification: {
		Key:            EmailTemplateVerification,
		DefaultSubject: "[{{.AppName}}] Email verifizieren",
		BaseName:       "verify_email",
		Variables:      []string{"AppName", "AppURL", "Username", "ActionURL"},
	},
	EmailTemplatePasswordReset: {
		Key:            EmailTemplatePasswordReset,
		DefaultSubject: "[{{.AppName}}] Passwort zurücksetzen",
		BaseName:       "reset_password",
		Variables:      []string{"AppName", "AppURL", "Username", "ActionURL"},
	},
	EmailTemplateInviteUser: {
		Key:            EmailTemplateInviteUser,
		DefaultSubject: "[{{.AppName}}] Einladung — Konto einrichten",
		BaseName:       "invite_user",
		Variables:      []string{"AppName", "AppURL", "Username", "ActionURL"},
	},
	EmailTemplateChangeEmail: {
		Key:            EmailTemplateChangeEmail,
		DefaultSubject: "[{{.AppName}}] Email-Adresse bestätigen",
		BaseName:       "change_email",
		Variables:      []string{"AppName", "AppURL", "Username", "ActionURL"},
	},
	EmailTemplateAccountDeletion: {
		Key:            EmailTemplateAccountDeletion,
		DefaultSubject: "[{{.AppName}}] Account-Löschung bestätigt",
		BaseName:       "account_deletion",
		Variables:      []string{"AppName", "AppURL", "Username", "ActionURL"},
	},
	EmailTemplateAdminApprovalNeeded: {
		Key:            EmailTemplateAdminApprovalNeeded,
		DefaultSubject: "[{{.AppName}}] Neue Registrierung wartet auf Freigabe",
		BaseName:       "admin_approval_needed",
		Variables:      []string{"AppName", "AppURL", "Username", "Email", "AdminURL"},
	},
	EmailTemplateTest: {
		Key:            EmailTemplateTest,
		DefaultSubject: "[{{.AppName}}] SMTP-Test",
		BaseName:       "test_email",
		Variables:      []string{"AppName", "AppURL", "Username"},
	},
}

func (s *EmailTemplateStore) GetEffective(ctx context.Context, key EmailTemplateKey) (EmailTemplate, error) {
	def, ok := emailTemplateRegistry[key]
	if !ok {
		return EmailTemplate{}, fmt.Errorf("%w: %s", ErrEmailTemplateUnknown, key)
	}

	if s != nil && s.db != nil {
		var tpl EmailTemplate
		var updatedAt time.Time
		err := s.db.Pool.QueryRow(ctx,
			`SELECT key, subject, html_body, text_body, enabled, updated_at
			   FROM email_templates
			  WHERE key = $1`,
			string(key),
		).Scan(&tpl.Key, &tpl.Subject, &tpl.HTML, &tpl.Text, &tpl.Enabled, &updatedAt)
		if err == nil {
			tpl.Source = "db"
			tpl.UpdatedAt = &updatedAt
			return tpl, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return EmailTemplate{}, fmt.Errorf("load email template %s: %w", key, err)
		}
	}

	htmlBody, err := templateFS.ReadFile("templates/" + def.BaseName + ".html")
	if err != nil {
		return EmailTemplate{}, fmt.Errorf("read default html template %s: %w", key, err)
	}
	textBody, err := templateFS.ReadFile("templates/" + def.BaseName + ".txt")
	if err != nil {
		return EmailTemplate{}, fmt.Errorf("read default text template %s: %w", key, err)
	}
	return EmailTemplate{
		Key:     key,
		Subject: def.DefaultSubject,
		HTML:    string(htmlBody),
		Text:    string(textBody),
		Enabled: true,
		Source:  "default",
	}, nil
}

func (s *EmailTemplateStore) SaveOverride(ctx context.Context, key EmailTemplateKey, input EmailTemplateOverride, adminID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("email template store has no db")
	}
	if err := validateEmailTemplateOverride(key, input); err != nil {
		return err
	}

	var adminIDParam any
	if adminID != "" {
		adminIDParam = adminID
	}
	_, err := s.db.Pool.Exec(ctx,
		`INSERT INTO email_templates (key, subject, html_body, text_body, enabled, updated_by, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())
		 ON CONFLICT (key) DO UPDATE
		   SET subject = EXCLUDED.subject,
		       html_body = EXCLUDED.html_body,
		       text_body = EXCLUDED.text_body,
		       enabled = EXCLUDED.enabled,
		       updated_by = EXCLUDED.updated_by,
		       updated_at = EXCLUDED.updated_at`,
		string(key), input.Subject, input.HTML, input.Text, input.Enabled, adminIDParam,
	)
	if err != nil {
		return fmt.Errorf("save email template %s: %w", key, err)
	}
	return nil
}

func (s *EmailTemplateStore) ResetOverride(ctx context.Context, key EmailTemplateKey) error {
	if _, ok := emailTemplateRegistry[key]; !ok {
		return fmt.Errorf("%w: %s", ErrEmailTemplateUnknown, key)
	}
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM email_templates WHERE key = $1`, string(key))
	return err
}

func (s *EmailTemplateStore) Render(ctx context.Context, key EmailTemplateKey, payload map[string]string) (RenderedEmail, error) {
	tpl, err := s.GetEffective(ctx, key)
	if err != nil {
		return RenderedEmail{}, err
	}
	if !tpl.Enabled {
		return RenderedEmail{}, fmt.Errorf("%w: %s", ErrEmailTemplateDisabled, key)
	}

	subject, err := renderTextTemplate(string(key)+".subject", tpl.Subject, payload)
	if err != nil {
		return RenderedEmail{}, err
	}
	htmlBody, err := renderHTMLTemplate(string(key)+".html", tpl.HTML, payload)
	if err != nil {
		return RenderedEmail{}, err
	}
	textBody, err := renderTextTemplate(string(key)+".txt", tpl.Text, payload)
	if err != nil {
		return RenderedEmail{}, err
	}
	return RenderedEmail{Subject: subject, HTMLBody: htmlBody, TextBody: textBody}, nil
}

func RenderEmailTemplateOverride(key EmailTemplateKey, input EmailTemplateOverride, payload map[string]string) (RenderedEmail, error) {
	if err := validateEmailTemplateOverride(key, input); err != nil {
		return RenderedEmail{}, err
	}
	subject, err := renderTextTemplate(string(key)+".subject", input.Subject, payload)
	if err != nil {
		return RenderedEmail{}, err
	}
	htmlBody, err := renderHTMLTemplate(string(key)+".html", input.HTML, payload)
	if err != nil {
		return RenderedEmail{}, err
	}
	textBody, err := renderTextTemplate(string(key)+".txt", input.Text, payload)
	if err != nil {
		return RenderedEmail{}, err
	}
	return RenderedEmail{Subject: subject, HTMLBody: htmlBody, TextBody: textBody}, nil
}

func validateEmailTemplate(key EmailTemplateKey, subject, htmlBody, textBody string) error {
	def, ok := emailTemplateRegistry[key]
	if !ok {
		return fmt.Errorf("%w: %s", ErrEmailTemplateUnknown, key)
	}
	if strings.TrimSpace(subject) == "" || strings.TrimSpace(htmlBody) == "" || strings.TrimSpace(textBody) == "" {
		return fmt.Errorf("%w: subject, html and text must not be empty", ErrEmailTemplateValidation)
	}
	if len(subject) > maxEmailTemplateFieldBytes || len(htmlBody) > maxEmailTemplateFieldBytes || len(textBody) > maxEmailTemplateFieldBytes {
		return fmt.Errorf("%w: subject, html and text must be at most %d bytes each", ErrEmailTemplateValidation, maxEmailTemplateFieldBytes)
	}
	payload := make(map[string]string, len(def.Variables))
	for _, variable := range def.Variables {
		payload[variable] = "sample"
	}
	if _, err := renderTextTemplate(string(key)+".subject", subject, payload); err != nil {
		return fmt.Errorf("%w: subject: %v", ErrEmailTemplateValidation, err)
	}
	if _, err := renderHTMLTemplate(string(key)+".html", htmlBody, payload); err != nil {
		return fmt.Errorf("%w: html: %v", ErrEmailTemplateValidation, err)
	}
	if _, err := renderTextTemplate(string(key)+".txt", textBody, payload); err != nil {
		return fmt.Errorf("%w: text: %v", ErrEmailTemplateValidation, err)
	}
	return nil
}

func validateEmailTemplateOverride(key EmailTemplateKey, input EmailTemplateOverride) error {
	if !input.Enabled {
		return fmt.Errorf("%w: mandatory auth email templates cannot be disabled", ErrEmailTemplateValidation)
	}
	return validateEmailTemplate(key, input.Subject, input.HTML, input.Text)
}

func renderTextTemplate(name, body string, payload map[string]string) (string, error) {
	t, err := texttemplate.New(name).Option("missingkey=error").Parse(body)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, payload); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderHTMLTemplate(name, body string, payload map[string]string) (string, error) {
	t, err := template.New(name).Option("missingkey=error").Parse(body)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, payload); err != nil {
		return "", err
	}
	return buf.String(), nil
}
