package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestSuggestRoutes_Schema_DescriptorsAlwaysArray ensures that variant_descriptors
// is always an array in the JSON response (never null, never missing).
// Guards against Go's default nil-slice-to-null JSON encoding after the
// removal of the omitempty tag.
func TestSuggestRoutes_Schema_DescriptorsAlwaysArray(t *testing.T) {
	setup := newTestRoutingHandler(t)

	const body = `{"start":[12.10,48.99],"target_distance_m":10000,"profile":"hiking"}`
	w := postSuggestRoutes(setup.Handler, body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal top-level: %v", err)
	}
	var suggestions []map[string]json.RawMessage
	if err := json.Unmarshal(raw["suggestions"], &suggestions); err != nil {
		t.Fatalf("unmarshal suggestions: %v", err)
	}

	// Skip trivial-pass when the fixture produces no suggestions —
	// we need at least one to prove the [] vs null invariant.
	if len(suggestions) == 0 {
		t.Skip("test grid returned no suggestions; descriptor invariant not exercisable")
	}

	for i, s := range suggestions {
		desc, ok := s["variant_descriptors"]
		if !ok {
			t.Errorf("suggestion[%d]: variant_descriptors field missing", i)
			continue
		}
		trimmed := strings.TrimSpace(string(desc))
		if trimmed == "null" {
			t.Errorf("suggestion[%d]: variant_descriptors is null, expected []", i)
		}
		if !strings.HasPrefix(trimmed, "[") {
			t.Errorf("suggestion[%d]: variant_descriptors is not an array, got %q", i, trimmed)
		}
	}
}

// TestSuggestRoutes_NoVariantIDsField ensures that a request body without
// variant_ids succeeds cleanly and that the response contains no used_fallback
// field after contract shrink.
func TestSuggestRoutes_NoVariantIDsField(t *testing.T) {
	setup := newTestRoutingHandler(t)

	const body = `{"start":[12.10,48.99],"target_distance_m":10000,"profile":"hiking"}`
	w := postSuggestRoutes(setup.Handler, body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 without variant_ids, got %d: %s", w.Code, w.Body.String())
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, present := raw["used_fallback"]; present {
		t.Errorf("used_fallback key must not be present in response after contract shrink")
	}
}
