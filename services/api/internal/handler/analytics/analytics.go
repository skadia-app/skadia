package analytics

import (
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/ml"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	DB        *database.DB
	Publisher *event.Publisher // optional — nil when NATS is unavailable
	ML        *ml.Client       // optional — nil when ML service is unconfigured
}

func NewHandler(db *database.DB, pub *event.Publisher) *Handler {
	return &Handler{DB: db, Publisher: pub}
}

// NewHandlerWithML creates a Handler with an ML client attached.
func NewHandlerWithML(db *database.DB, pub *event.Publisher, mlClient *ml.Client) *Handler {
	return &Handler{DB: db, Publisher: pub, ML: mlClient}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	// Training Tab
	r.Get("/pmc", h.GetPMC)
	r.Get("/year-progression", h.GetYearProgression)
	r.Get("/training-volume", h.GetTrainingVolume)
	r.Get("/zone-distribution", h.GetZoneDistribution)
	r.Get("/personal-records", h.GetPersonalRecords)

	// Health Tab
	r.Get("/hrv", h.GetHRV)
	r.Get("/hrv/correlation", h.GetHRVCorrelation)
	r.Get("/hrv/dfa-alpha1", h.GetHRVDFAAlpha1)
	r.Get("/hrv/recovery", h.GetHRVRecovery)
	r.Get("/sleep", h.GetSleep)
	r.Get("/weight", h.GetWeight)
	r.Get("/health-metrics", h.GetHealthMetrics)
	r.Get("/activity-glucose", h.GetActivityGlucose)

	// Stats Tab
	r.Get("/lifetime-stats", h.GetLifetimeStats)
	r.Get("/eddington", h.GetEddington)
	r.Get("/distributions", h.GetDistributions)
	r.Get("/yearly-overview", h.GetYearlyOverview)
	r.Get("/streaks", h.GetStreaks)

	// Insights Tab
	r.Get("/insights/readiness", h.GetInsightsReadiness)
	r.Get("/insights/warnings", h.GetInsightsWarnings)
	r.Get("/insights/projection", h.GetInsightsProjection)
	r.Get("/insights/clusters", h.GetInsightsClusters)
	r.Get("/insights/correlations", h.GetInsightsCorrelations)
	r.Post("/insights/refresh", h.RefreshInsights)

	// Route similarity
	r.Get("/similar-routes", h.GetSimilarRoutes)

	// ML Features (WP3)
	r.Get("/changepoints", h.GetChangepoints)
	r.Get("/hrv-extended", h.GetHRVExtended)
	r.Get("/features", h.GetActivityFeatures)
	r.Get("/readiness-v2", h.GetReadinessV2)
	r.Get("/anomalies-v2", h.GetAnomaliesV2)
	r.Get("/correlations-v2", h.GetCorrelationsV2)

	return r
}
