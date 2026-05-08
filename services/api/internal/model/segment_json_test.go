package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestSegmentAllowedPredicatesJSONPreservesEmptyArray(t *testing.T) {
	raw, err := json.Marshal(Segment{AllowedPredicates: []sport.Predicate{}})
	if err != nil {
		t.Fatalf("marshal segment: %v", err)
	}
	if !strings.Contains(string(raw), `"allowed_predicates":[]`) {
		t.Fatalf("Segment JSON = %s, want explicit empty allowed_predicates array", raw)
	}
}

func TestSegmentAllowedPredicatesJSONPreservesNull(t *testing.T) {
	raw, err := json.Marshal(Segment{})
	if err != nil {
		t.Fatalf("marshal segment: %v", err)
	}
	if !strings.Contains(string(raw), `"allowed_predicates":null`) {
		t.Fatalf("Segment JSON = %s, want explicit null allowed_predicates", raw)
	}
}
