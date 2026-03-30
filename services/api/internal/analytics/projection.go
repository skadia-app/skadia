package analytics

import (
	"context"
	"encoding/json"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

const (
	projectionCacheKey    = "projection"
	projectionMinDays     = 14
	projectionDays        = 28
	plateauSlopeThreshold = 0.05 // CTL/Tag
)

// ProjectionResult holds the 28-day CTL extrapolation output.
type ProjectionResult struct {
	Slope           *float64 `json:"slope"`
	CurrentCTL      *float64 `json:"current_ctl"`
	ProjectedCTL28d *float64 `json:"projected_ctl_28d"`
	Trend           *string  `json:"trend"`
	DataPoints      int      `json:"data_points"`
}

// ComputeProjection extrapoliert den 28-Tage-CTL-Trend via linearer Regression
// und schreibt das Ergebnis in analytics_cache (key: "projection").
//
// Lädt die letzten 90 PMC-Tage, fittet eine Gerade auf CTL und extrapoliert
// 28 Tage in die Zukunft.
//
// Graceful skip: weniger als 14 CTL-Datenpunkte → return nil.
func ComputeProjection(ctx context.Context, db *database.DB, userID string) error {
	rows, err := db.Pool.Query(ctx, `
		SELECT date, ctl FROM pmc_daily
		WHERE user_id = $1
		  AND date >= CURRENT_DATE - interval '90 days'
		  AND ctl IS NOT NULL
		ORDER BY date ASC
	`, userID)
	if err != nil {
		return nil // graceful skip
	}
	defer rows.Close()

	var ctlValues []float64
	for rows.Next() {
		var dateIgnored any
		var ctl float64
		if err := rows.Scan(&dateIgnored, &ctl); err != nil {
			continue
		}
		ctlValues = append(ctlValues, ctl)
	}
	rows.Close()

	if len(ctlValues) < projectionMinDays {
		// Not enough data — write empty projection and return
		return writeEmptyProjection(ctx, db, userID)
	}

	slope, intercept := linearRegression(ctlValues)
	currentCTL := ctlValues[len(ctlValues)-1]

	// Extrapolation: aktuelle Position = len-1, + 28 Tage
	xFuture := float64(len(ctlValues)-1) + projectionDays
	projectedCTL := slope*xFuture + intercept

	trend := "plateau"
	if slope >= plateauSlopeThreshold {
		trend = "aufbau"
	} else if slope <= -plateauSlopeThreshold {
		trend = "abbau"
	}

	slopeR := roundN(slope, 4)
	currentR := roundN(currentCTL, 1)
	projectedR := roundN(projectedCTL, 1)

	result := ProjectionResult{
		Slope:           &slopeR,
		CurrentCTL:      &currentR,
		ProjectedCTL28d: &projectedR,
		Trend:           &trend,
		DataPoints:      len(ctlValues),
	}

	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return db.UpsertAnalyticsCache(ctx, userID, projectionCacheKey, data)
}

// linearRegression berechnet Steigung (slope) und Achsenabschnitt (intercept)
// via Ordinary Least Squares für y = slope*x + intercept.
// x sind implizit die Indizes 0, 1, ..., n-1.
func linearRegression(y []float64) (slope, intercept float64) {
	n := float64(len(y))
	if n < 2 {
		return 0, y[0]
	}

	// Mittelwerte
	var sumX, sumY float64
	for i, yi := range y {
		sumX += float64(i)
		sumY += yi
	}
	meanX := sumX / n
	meanY := sumY / n

	// Steigung: Σ(xi - x̄)(yi - ȳ) / Σ(xi - x̄)²
	var num, den float64
	for i, yi := range y {
		dx := float64(i) - meanX
		num += dx * (yi - meanY)
		den += dx * dx
	}
	if den == 0 {
		return 0, meanY
	}
	slope = num / den
	intercept = meanY - slope*meanX
	return
}

// writeEmptyProjection schreibt einen leeren Projektions-Cache-Eintrag.
func writeEmptyProjection(ctx context.Context, db *database.DB, userID string) error {
	result := ProjectionResult{DataPoints: 0}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return db.UpsertAnalyticsCache(ctx, userID, projectionCacheKey, data)
}
