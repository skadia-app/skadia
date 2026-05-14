package tools

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type ExternalReadPolicy string

const (
	ExternalReadPolicyManualConfirm ExternalReadPolicy = "manual_confirm"
)

type ExternalReadRequirement struct {
	Policy  ExternalReadPolicy
	Purpose string
	TTL     time.Duration
}

type ExternalReadConsentRequest struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	ToolName  string    `json:"tool_name"`
	Purpose   string    `json:"purpose"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ExternalReadConsentManager struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]*externalReadConsentEntry
}

type externalReadConsentEntry struct {
	request   ExternalReadConsentRequest
	userID    string
	sessionID string
	argsHash  string
	confirmed bool
	used      bool
}

func NewExternalReadConsentManager(ttl time.Duration) *ExternalReadConsentManager {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &ExternalReadConsentManager{
		ttl:     ttl,
		entries: make(map[string]*externalReadConsentEntry),
	}
}

func (m *ExternalReadConsentManager) Request(ctx context.Context, req agentExternalReadConsentRequest) (ExternalReadConsentRequest, error) {
	if err := ctx.Err(); err != nil {
		return ExternalReadConsentRequest{}, err
	}
	if m == nil {
		return ExternalReadConsentRequest{}, fmt.Errorf("external read consent manager is not configured")
	}
	token, err := newExternalReadConsentToken()
	if err != nil {
		return ExternalReadConsentRequest{}, err
	}
	ttl := req.TTL
	if ttl <= 0 {
		ttl = m.ttl
	}
	request := ExternalReadConsentRequest{
		ID:        token[:16],
		Token:     token,
		ToolName:  req.ToolName,
		Purpose:   req.Purpose,
		ExpiresAt: time.Now().Add(ttl).UTC(),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[token] = &externalReadConsentEntry{
		request:   request,
		userID:    req.UserID,
		sessionID: req.SessionID,
		argsHash:  hashExternalReadArgs(req.Args),
	}
	return request, nil
}

func (m *ExternalReadConsentManager) Confirm(userID, token string) bool {
	if m == nil || token == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.entries[token]
	if entry == nil || entry.userID != userID || entry.used || time.Now().After(entry.request.ExpiresAt) {
		return false
	}
	entry.confirmed = true
	return true
}

func (m *ExternalReadConsentManager) Consume(ctx context.Context, req agentExternalReadConsentRequest, token string) bool {
	if err := ctx.Err(); err != nil || m == nil || token == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.entries[token]
	if entry == nil || entry.used || !entry.confirmed || time.Now().After(entry.request.ExpiresAt) {
		return false
	}
	if entry.userID != req.UserID || entry.sessionID != req.SessionID || entry.request.ToolName != req.ToolName {
		return false
	}
	if entry.argsHash != hashExternalReadArgs(req.Args) {
		return false
	}
	entry.used = true
	delete(m.entries, token)
	return true
}

type agentExternalReadConsentRequest struct {
	UserID    string
	SessionID string
	ToolName  string
	Purpose   string
	TTL       time.Duration
	Args      json.RawMessage
}

func externalReadConsentToken(args json.RawMessage) string {
	var in map[string]any
	if err := json.Unmarshal(args, &in); err != nil {
		return ""
	}
	for _, key := range []string{"consent_token", "external_consent_token"} {
		if raw, ok := in[key].(string); ok {
			return raw
		}
	}
	return ""
}

func stripExternalReadConsentArgs(args json.RawMessage) json.RawMessage {
	var in map[string]any
	if err := json.Unmarshal(args, &in); err != nil {
		return append(json.RawMessage(nil), args...)
	}
	delete(in, "consent_token")
	delete(in, "external_consent_token")
	delete(in, "confirmed")
	delete(in, "user_confirmed")
	raw, err := json.Marshal(in)
	if err != nil {
		return append(json.RawMessage(nil), args...)
	}
	return raw
}

func hashExternalReadArgs(args json.RawMessage) string {
	clean := stripExternalReadConsentArgs(args)
	sum := sha256.Sum256(clean)
	return hex.EncodeToString(sum[:])
}

func newExternalReadConsentToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("create external read consent token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
