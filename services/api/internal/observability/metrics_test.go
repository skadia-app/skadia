package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewMetricsHandlerServesCustomRegistry(t *testing.T) {
	registry := prometheus.NewRegistry()
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "skadia_observability_test_total",
		Help: "Test counter exposed through the metrics handler.",
	})
	registry.MustRegister(counter)
	counter.Inc()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	NewMetricsHandler(registry).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "skadia_observability_test_total") {
		t.Fatalf("response does not contain test metric:\n%s", rec.Body.String())
	}
}
