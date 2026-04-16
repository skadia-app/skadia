package dataimport

import (
	"sync"

	"github.com/google/uuid"
)

// IDMap trackt old→new UUID Mappings für Cross-Server-Import.
// Concurrent-safe via sync.RWMutex.
type IDMap struct {
	mu           sync.RWMutex
	m            map[string]string
	sourceUserID string
	targetUserID string
}

// NewIDMap creates an IDMap that will always map sourceUserID to targetUserID.
func NewIDMap(sourceUserID, targetUserID string) *IDMap {
	return &IDMap{
		m:            make(map[string]string),
		sourceUserID: sourceUserID,
		targetUserID: targetUserID,
	}
}

// Get returns the mapped ID for oldID, generating a new UUID if unseen.
// Empty string returns empty string (NULL FK passthrough).
// sourceUserID always maps to targetUserID.
func (m *IDMap) Get(oldID string) string {
	if oldID == "" {
		return ""
	}
	if oldID == m.sourceUserID {
		return m.targetUserID
	}

	m.mu.RLock()
	newID, ok := m.m[oldID]
	m.mu.RUnlock()
	if ok {
		return newID
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Double-check after acquiring write lock.
	if newID, ok = m.m[oldID]; ok {
		return newID
	}
	newID = uuid.New().String()
	m.m[oldID] = newID
	return newID
}

// Set explicitly maps oldID to newID (e.g. for cmMergeID, ShadowFS file IDs).
func (m *IDMap) Set(oldID, newID string) {
	m.mu.Lock()
	m.m[oldID] = newID
	m.mu.Unlock()
}

// GetIfExists returns the mapping for oldID only if it already exists.
// sourceUserID always resolves to (targetUserID, true).
func (m *IDMap) GetIfExists(oldID string) (string, bool) {
	if oldID == m.sourceUserID {
		return m.targetUserID, true
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	newID, ok := m.m[oldID]
	return newID, ok
}

// NewIDSet returns all mapped new IDs as a boolean set.
// This is a snapshot — further Set/Get calls after this point are not reflected.
func (m *IDMap) NewIDSet() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]bool, len(m.m)+1)
	for _, newID := range m.m {
		out[newID] = true
	}
	out[m.targetUserID] = true
	return out
}
