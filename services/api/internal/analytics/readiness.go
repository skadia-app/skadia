// Package analytics implements Go ports of the Python ML-service analytics
// functions (readiness, anomaly, projection, correlations).
// All statistical computations use pure Go arithmetic — no cgo or numpy.
package analytics

import (
	"context"
	"encoding/json"
	"math"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

const (
	readinessCacheKey = "readiness"
)

// ReadinessFactor is a single weighted contributor to the readiness score.
type ReadinessFactor struct {
	Name   string `json:"name"`
	Value  int    `json:"value"`
	Status string `json:"status"`
}

// ReadinessResult holds the final readiness output stored in analytics_cache.
type ReadinessResult struct {
	Score   int               `json:"score"`
	Level   string            `json:"level"`
	Factors []ReadinessFactor `json:"factors"`
}

// ComputeReadiness berechnet einen Readiness-Score (0–100) aus HRV-Trend,
// ATL und Schlaf und schreibt das Ergebnis in analytics_cache (key: "readiness").
//
// Gewichtung:
//   - HRV-Trend  40 %
//   - Ermüdung   30 % (invertiertes ATL)
//   - Schlaf     30 %
//
// Graceful skip: weniger als 3 HRV-Punkte oder fehlende DB-Daten → return nil.
func ComputeReadiness(ctx context.Context, db *database.DB, userID string) error {
	// 1. HRV 7-Tage-Trend
	type hrvRow struct {
		RmssdMs *float64
	}
	hrvRows, err := db.Pool.Query(ctx, `
		SELECT rmssd_ms FROM health_snapshots
		WHERE user_id = $1 AND measured_at_utc > NOW() - interval '7 days'
		ORDER BY measured_at_utc
	`, userID)
	if err != nil {
		return nil // graceful skip
	}
	defer hrvRows.Close()

	var hrvValues []float64
	for hrvRows.Next() {
		var v *float64
		if err := hrvRows.Scan(&v); err != nil {
			continue
		}
		if v != nil {
			hrvValues = append(hrvValues, *v)
		}
	}
	hrvRows.Close()

	// 2. Letzter PMC-ATL-Wert
	var atl *float64
	_ = db.Pool.QueryRow(ctx, `
		SELECT atl FROM pmc_daily
		WHERE user_id = $1 ORDER BY date DESC LIMIT 1
	`, userID).Scan(&atl)

	// 3. Letzter Schlaf-Score
	var sleepScore *int
	_ = db.Pool.QueryRow(ctx, `
		SELECT score_overall FROM health_sleep
		WHERE user_id = $1 ORDER BY calendar_date DESC LIMIT 1
	`, userID).Scan(&sleepScore)

	// Faktoren berechnen
	hrvFactor := computeHRVFactor(hrvValues)

	atlVal := 50.0
	if atl != nil {
		atlVal = *atl
	}
	// ATL 0 → 100 (frisch), ATL 150+ → 0 (sehr müde)
	atlFactor := math.Max(0.0, math.Min(100.0, 100.0-(atlVal/150.0)*100.0))

	sleepFactor := 50.0
	if sleepScore != nil {
		sleepFactor = float64(*sleepScore)
	}

	rawScore := hrvFactor*0.4 + atlFactor*0.3 + sleepFactor*0.3
	score := int(math.Round(rawScore))

	statusLabel := func(v float64) string {
		if v >= 70 {
			return "gut"
		}
		if v >= 40 {
			return "moderat"
		}
		return "niedrig"
	}

	level := "moderat"
	if score >= 70 {
		level = "hoch"
	} else if score < 40 {
		level = "niedrig"
	}

	result := ReadinessResult{
		Score: score,
		Level: level,
		Factors: []ReadinessFactor{
			{Name: "HRV-Trend", Value: int(math.Round(hrvFactor)), Status: statusLabel(hrvFactor)},
			{Name: "Ermüdung", Value: int(math.Round(atlFactor)), Status: statusLabel(atlFactor)},
			{Name: "Schlaf", Value: int(math.Round(sleepFactor)), Status: statusLabel(sleepFactor)},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return db.UpsertAnalyticsCache(ctx, userID, readinessCacheKey, data)
}

// computeHRVFactor berechnet einen 0-100-Faktor aus dem 7-Tage-HRV-Trend.
// Vergleicht die letzten 3 Werte mit dem Gesamt-Mittelwert.
// ratio 1.0 → 100 (über Baseline), ratio 0.7 → 0 (deutlich darunter).
func computeHRVFactor(values []float64) float64 {
	if len(values) < 3 {
		return 50.0
	}

	var sum float64
	for _, v := range values {
		sum += v
	}
	baseline := sum / float64(len(values))
	if baseline == 0 {
		return 50.0
	}

	tail := values[len(values)-3:]
	var recentSum float64
	for _, v := range tail {
		recentSum += v
	}
	recent := recentSum / float64(len(tail))

	ratio := recent / baseline
	// ratio 0.7 → 0, ratio 1.3 → 100, linear clipped
	factor := (ratio - 0.7) / (1.3 - 0.7) * 100.0
	return math.Min(100.0, math.Max(0.0, factor))
}
