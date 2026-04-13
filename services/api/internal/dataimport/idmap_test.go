package dataimport

import (
	"testing"

	"github.com/google/uuid"
)

func TestIDMap_SourceUserMapsToTarget(t *testing.T) {
	src := uuid.New().String()
	tgt := uuid.New().String()
	m := NewIDMap(src, tgt)

	got := m.Get(src)
	if got != tgt {
		t.Errorf("Get(sourceUserID) = %q; want %q", got, tgt)
	}
}

func TestIDMap_EmptyStringReturnsEmpty(t *testing.T) {
	m := NewIDMap(uuid.New().String(), uuid.New().String())

	got := m.Get("")
	if got != "" {
		t.Errorf("Get(\"\") = %q; want empty string", got)
	}
}

func TestIDMap_GeneratesNewUUID(t *testing.T) {
	m := NewIDMap(uuid.New().String(), uuid.New().String())
	oldID := uuid.New().String()

	first := m.Get(oldID)
	if first == "" {
		t.Fatal("Get returned empty string for unknown ID")
	}
	if first == oldID {
		t.Errorf("Get returned the original ID unchanged; want a new UUID")
	}
	// Idempotenz: gleicher old → gleicher new
	second := m.Get(oldID)
	if first != second {
		t.Errorf("Get not idempotent: first=%q second=%q", first, second)
	}
}

func TestIDMap_SetOverrides(t *testing.T) {
	m := NewIDMap(uuid.New().String(), uuid.New().String())
	oldID := uuid.New().String()
	explicit := uuid.New().String()

	m.Set(oldID, explicit)

	got := m.Get(oldID)
	if got != explicit {
		t.Errorf("Get after Set = %q; want %q", got, explicit)
	}
}

func TestIDMap_GetIfExists(t *testing.T) {
	src := uuid.New().String()
	tgt := uuid.New().String()
	m := NewIDMap(src, tgt)

	// sourceUserID always resolves
	got, ok := m.GetIfExists(src)
	if !ok || got != tgt {
		t.Errorf("GetIfExists(sourceUserID) = (%q, %v); want (%q, true)", got, ok, tgt)
	}

	// Unknown ID → not found
	unknown := uuid.New().String()
	got, ok = m.GetIfExists(unknown)
	if ok {
		t.Errorf("GetIfExists(unknown) returned ok=true with value %q", got)
	}

	// After Get, it exists
	m.Get(unknown)
	got, ok = m.GetIfExists(unknown)
	if !ok {
		t.Errorf("GetIfExists after Get returned ok=false")
	}
	if got == "" {
		t.Errorf("GetIfExists after Get returned empty value")
	}

	// After Set, explicit value is returned
	setID := uuid.New().String()
	explicit := uuid.New().String()
	m.Set(setID, explicit)
	got, ok = m.GetIfExists(setID)
	if !ok || got != explicit {
		t.Errorf("GetIfExists after Set = (%q, %v); want (%q, true)", got, ok, explicit)
	}
}
