package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeGearDefaultSportsForRequestRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "tuple", values: []string{"cycling:mountain"}, want: "top-level sport_type"},
		{name: "unknown sport", values: []string{"not_a_sport"}, want: "not allowed"},
		{name: "non selectable parent", values: []string{"fitness_equipment"}, want: "not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/gear", nil)
			got, ok := normalizeGearDefaultSportsForRequest(rec, req, tt.values)
			if ok || got != nil {
				t.Fatalf("normalizeGearDefaultSportsForRequest() = %#v, %v; want nil, false", got, ok)
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.want) {
				t.Fatalf("body %q missing %q", rec.Body.String(), tt.want)
			}
		})
	}
}

func TestNormalizeGearDefaultSportsForRequestNormalizesValidValues(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gear", nil)
	got, ok := normalizeGearDefaultSportsForRequest(rec, req, []string{" cycling ", "cycling"})
	if !ok {
		t.Fatalf("normalizeGearDefaultSportsForRequest() returned false; body=%s", rec.Body.String())
	}
	if len(got) != 1 || got[0] != "cycling" {
		t.Fatalf("normalized sports = %#v, want []string{cycling}", got)
	}
}
