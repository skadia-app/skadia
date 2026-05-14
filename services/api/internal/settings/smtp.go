package settings

import (
	"fmt"
	"strconv"
	"strings"
)

var smtpDeliveryKeys = []string{
	"smtp.host",
	"smtp.port",
	"smtp.username",
	"smtp.password",
	"smtp.from",
	"smtp.tls",
}

type SMTPDeliveryConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	TLSMode  string
	HasAuth  bool
}

func parseSMTPDeliveryConfig(values map[string]string) (SMTPDeliveryConfig, error) {
	var missing []string
	cfg := SMTPDeliveryConfig{
		Host:     strings.TrimSpace(values["smtp.host"]),
		Username: strings.TrimSpace(values["smtp.username"]),
		Password: values["smtp.password"],
		From:     strings.TrimSpace(values["smtp.from"]),
		TLSMode:  strings.TrimSpace(values["smtp.tls"]),
	}
	if cfg.TLSMode == "" {
		cfg.TLSMode = "starttls"
	}
	if cfg.Host == "" {
		missing = append(missing, "smtp.host")
	}
	if cfg.From == "" {
		missing = append(missing, "smtp.from")
	}

	portRaw := strings.TrimSpace(values["smtp.port"])
	if portRaw == "" {
		missing = append(missing, "smtp.port")
	} else if port, err := strconv.Atoi(portRaw); err != nil || port < 1 || port > 65535 {
		return SMTPDeliveryConfig{}, fmt.Errorf("smtp.port must be between 1 and 65535: %w", ErrValidation)
	} else {
		cfg.Port = port
	}

	if len(missing) > 0 {
		return SMTPDeliveryConfig{}, fmt.Errorf("SMTP delivery requires %s: %w", strings.Join(missing, ", "), ErrValidation)
	}

	if (cfg.Username == "") != (cfg.Password == "") {
		return SMTPDeliveryConfig{}, fmt.Errorf("smtp.username and smtp.password must be configured together: %w", ErrValidation)
	}
	cfg.HasAuth = cfg.Username != "" && cfg.Password != ""

	return cfg, nil
}

func validateSMTPDeliverySettings(values map[string]string) error {
	_, err := parseSMTPDeliveryConfig(values)
	return err
}

// EmailDeliveryConfigured reports whether settings are complete enough to send
// auth emails. It deliberately accepts unauthenticated SMTP relays: username
// and password must be either both present or both absent.
func (s *SettingsStore) EmailDeliveryConfigured() bool {
	if s == nil {
		return false
	}

	values := make(map[string]string, len(smtpDeliveryKeys))
	for _, key := range smtpDeliveryKeys {
		values[key] = s.Get(key).Value
	}
	return validateSMTPDeliverySettings(values) == nil
}

func (s *SettingsStore) EmailDeliveryConfig() (SMTPDeliveryConfig, bool) {
	if s == nil {
		return SMTPDeliveryConfig{}, false
	}
	values := make(map[string]string, len(smtpDeliveryKeys))
	for _, key := range smtpDeliveryKeys {
		values[key] = s.Get(key).Value
	}
	cfg, err := parseSMTPDeliveryConfig(values)
	return cfg, err == nil
}
