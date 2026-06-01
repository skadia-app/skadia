package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// TestRefreshInsights_TriggersInvalidate verifies that RefreshInsights
// publishes exactly one analytics-invalidate event per request (Phase 6.3).
// The Go-analytics goroutine is short-circuited by pre-acquiring the per-user
// refreshLock, so the test does not need a live DB.
func TestRefreshInsights_TriggersInvalidate(t *testing.T) {
	const uid = "00000000-0000-0000-0000-0000000000aa"

	// Pre-acquire the per-user refresh lock so RefreshInsights skips the
	// goroutine that would otherwise call goanalytics.* with a nil DB.
	refreshMu.Lock()
	refreshLock[uid] = struct{}{}
	refreshMu.Unlock()
	t.Cleanup(func() {
		refreshMu.Lock()
		delete(refreshLock, uid)
		refreshMu.Unlock()
	})

	fake := &event.FakeInvalidator{}
	h := &Handler{Invalidator: fake}

	r := httptest.NewRequest(http.MethodPost, "/analytics/insights/refresh", nil)
	r = r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.RefreshInsights(rr, r)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status %d, want 202; body: %s", rr.Code, rr.Body.String())
	}
	if got, want := len(fake.Calls), 1; got != want {
		t.Fatalf("FakeInvalidator.Calls = %d, want %d", got, want)
	}
	if fake.Calls[0] != uid {
		t.Errorf("FakeInvalidator.Calls[0] = %q, want %q", fake.Calls[0], uid)
	}
}

func TestBuildAPIWarningIncludesMachineReadableFields(t *testing.T) {
	warning := buildAPIWarning(anomalyEntry{
		Date:     "2026-05-31",
		Severity: "high",
	}, anomalyDetail{
		Metric:       "sleep_score",
		Value:        41,
		ZScore:       -2.4,
		BaselineMean: 72,
		BaselineStd:  10,
	})

	if warning.Code != "analytics.warning.anomaly" {
		t.Fatalf("Code = %q, want stable warning code", warning.Code)
	}
	if warning.MessageKey != "analytics.warning.anomaly.message" {
		t.Fatalf("MessageKey = %q, want message key", warning.MessageKey)
	}
	if warning.MessageParams["metric"] != "sleep_score" || warning.MessageParams["date"] != "2026-05-31" {
		t.Fatalf("MessageParams = %#v, want metric and date codes", warning.MessageParams)
	}
	if strings.Contains(warning.MessageKey, "Anomalie") || strings.Contains(warning.ExplanationKey, "Baseline") {
		t.Fatalf("i18n keys contain localized text: message=%q explanation=%q", warning.MessageKey, warning.ExplanationKey)
	}

	data, err := json.Marshal(warning)
	if err != nil {
		t.Fatalf("Marshal warning: %v", err)
	}
	payload := string(data)
	for _, want := range []string{`"code":"analytics.warning.anomaly"`, `"message_key":"analytics.warning.anomaly.message"`, `"message_params"`, `"explanation_key":"analytics.warning.anomaly.explanation"`} {
		if !strings.Contains(payload, want) {
			t.Fatalf("serialized warning missing %s: %s", want, payload)
		}
	}
}

func TestBuildAPIProjectionIncludesMachineReadableExplanation(t *testing.T) {
	slope := 0.12
	trend := "aufbau"
	trendKey := "building"
	projection := buildAPIProjection(75, 80, "2026-06-28", &trend, &trendKey, &slope, 42)

	if projection.Code != "analytics.projection.ctl" {
		t.Fatalf("Code = %q, want stable projection code", projection.Code)
	}
	if projection.ExplanationKey != "analytics.projection.ctl_trend" {
		t.Fatalf("ExplanationKey = %q, want explanation key", projection.ExplanationKey)
	}
	if projection.ExplanationParams["trend"] != "building" {
		t.Fatalf("trend param = %#v, want normalized stable code", projection.ExplanationParams["trend"])
	}
	if strings.Contains(projection.ExplanationKey, "Trend") || strings.Contains(projection.ExplanationKey, "Datenpunkten") {
		t.Fatalf("ExplanationKey contains localized text: %q", projection.ExplanationKey)
	}

	data, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("Marshal projection: %v", err)
	}
	payload := string(data)
	for _, want := range []string{`"code":"analytics.projection.ctl"`, `"explanation_key":"analytics.projection.ctl_trend"`, `"explanation_params"`, `"trend":"building"`} {
		if !strings.Contains(payload, want) {
			t.Fatalf("serialized projection missing %s: %s", want, payload)
		}
	}
}

// TestRefreshInsights_NilInvalidatorOK ensures the handler does not panic when
// no invalidator is configured (NATS unavailable scenario).
func TestRefreshInsights_NilInvalidatorOK(t *testing.T) {
	const uid = "00000000-0000-0000-0000-0000000000bb"

	refreshMu.Lock()
	refreshLock[uid] = struct{}{}
	refreshMu.Unlock()
	t.Cleanup(func() {
		refreshMu.Lock()
		delete(refreshLock, uid)
		refreshMu.Unlock()
	})

	h := &Handler{Invalidator: nil}

	r := httptest.NewRequest(http.MethodPost, "/analytics/insights/refresh", nil)
	r = r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.RefreshInsights(rr, r)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status %d, want 202; body: %s", rr.Code, rr.Body.String())
	}
}
