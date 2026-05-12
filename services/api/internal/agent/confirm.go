package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type PendingConfirm struct {
	UserID     string
	ActionID   string
	SessionID  string
	ToolCallID string
	Tool       string
	Args       json.RawMessage
	Summary    string
	ExpiresAt  time.Time
	Metadata   json.RawMessage
	respCh     chan ConfirmResponse
	resolved   atomic.Bool
}

type ConfirmResponse struct {
	Confirmed     bool
	ArgsOverrides json.RawMessage
	Source        string
	Metadata      json.RawMessage
}

type ConfirmationManager struct {
	mu         sync.Mutex
	pending    map[string]*PendingConfirm
	actions    map[string]*PendingConfirm
	resolved   map[string]ConfirmResponse
	ttl        time.Duration
	gcInterval time.Duration
	gcOnce     sync.Once
}

func NewConfirmationManager(ttl time.Duration) *ConfirmationManager {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	gcInterval := 30 * time.Second
	if ttl < gcInterval {
		gcInterval = ttl / 2
		if gcInterval <= 0 {
			gcInterval = ttl
		}
	}
	return &ConfirmationManager{
		pending:    make(map[string]*PendingConfirm),
		actions:    make(map[string]*PendingConfirm),
		resolved:   make(map[string]ConfirmResponse),
		ttl:        ttl,
		gcInterval: gcInterval,
	}
}

func (m *ConfirmationManager) Register(ctx context.Context, p *PendingConfirm) (<-chan ConfirmResponse, error) {
	if p == nil {
		return nil, fmt.Errorf("pending confirmation is nil")
	}
	if p.SessionID == "" || p.ToolCallID == "" {
		return nil, fmt.Errorf("pending confirmation requires session_id and tool_call_id")
	}
	key := confirmKey(p.SessionID, p.ToolCallID)
	if p.ExpiresAt.IsZero() {
		p.ExpiresAt = time.Now().Add(m.ttl)
	}
	if p.respCh == nil {
		p.respCh = make(chan ConfirmResponse, 1)
	}

	m.mu.Lock()
	if _, exists := m.pending[key]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("confirmation already pending")
	}
	m.pending[key] = p
	if p.ActionID != "" {
		m.actions[p.ActionID] = p
	}
	m.mu.Unlock()

	m.gcOnce.Do(func() { go m.gcLoop(ctx) })
	return p.respCh, nil
}

func (m *ConfirmationManager) Resolve(sessionID, toolCallID string, resp ConfirmResponse) bool {
	key := confirmKey(sessionID, toolCallID)
	m.mu.Lock()
	p, ok := m.pending[key]
	m.mu.Unlock()
	if !ok {
		return false
	}
	if !p.resolved.CompareAndSwap(false, true) {
		return false
	}
	if len(resp.Metadata) == 0 {
		resp.Metadata = p.Metadata
	}
	select {
	case p.respCh <- resp:
	default:
	}
	m.mu.Lock()
	delete(m.pending, key)
	if p.ActionID != "" {
		delete(m.actions, p.ActionID)
		m.resolved[p.ActionID] = resp
	}
	m.mu.Unlock()
	return true
}

func (m *ConfirmationManager) PendingAction(actionID string) (*PendingConfirm, bool) {
	if m == nil || actionID == "" {
		return nil, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.actions[actionID]
	return p, ok
}

func (m *ConfirmationManager) ConfirmedAction(actionID string) (ConfirmResponse, bool) {
	if m == nil || actionID == "" {
		return ConfirmResponse{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	resp, ok := m.resolved[actionID]
	return resp, ok && resp.Confirmed
}

func (m *ConfirmationManager) ResolvedAction(actionID string) (ConfirmResponse, bool) {
	if m == nil || actionID == "" {
		return ConfirmResponse{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	resp, ok := m.resolved[actionID]
	return resp, ok
}

func (m *ConfirmationManager) gcLoop(ctx context.Context) {
	ticker := time.NewTicker(m.gcInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.resolveExpired(now)
		}
	}
}

func (m *ConfirmationManager) resolveExpired(now time.Time) {
	m.mu.Lock()
	expired := make([]*PendingConfirm, 0)
	for _, p := range m.pending {
		if !p.ExpiresAt.After(now) {
			expired = append(expired, p)
		}
	}
	m.mu.Unlock()

	for _, p := range expired {
		m.Resolve(p.SessionID, p.ToolCallID, ConfirmResponse{
			Confirmed: false,
			Source:    "timeout",
		})
	}
}

func confirmKey(sessionID, toolCallID string) string {
	return sessionID + ":" + toolCallID
}
