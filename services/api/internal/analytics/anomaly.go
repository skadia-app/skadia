package analytics

import (
	"context"
	"encoding/json"
	"math"
	"sort"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

const (
	anomaliesCacheKey  = "anomalies"
	minDataPoints      = 14
	windowDays         = 30
	zscoreThreshold    = 2.0 // absolute value; anomaly when |z| >= 2
)

// AnomalyDetail holds per-metric detail for a single anomaly day.
type AnomalyDetail struct {
	Metric       string  `json:"metric"`
	Value        float64 `json:"value"`
	ZScore       float64 `json:"zscore"`
	BaselineMean float64 `json:"baseline_mean"`
	BaselineStd  float64 `json:"baseline_std"`
}

// AnomalyEntry represents a day with at least one detected anomaly.
type AnomalyEntry struct {
	Date            string          `json:"date"`
	Severity        string          `json:"severity"`
	AffectedMetrics []string        `json:"affected_metrics"`
	Details         []AnomalyDetail `json:"details"`
}

// metricQuery describes a single metric's SQL query.
type metricQuery struct {
	name  string
	query string
}

var anomalyMetrics = []metricQuery{
	{
		name: "resting_hr",
		query: `
			SELECT measured_at_utc::date AS date,
			       (extra_data->>'resting_hr')::float AS value
			FROM health_snapshots
			WHERE user_id = $1
			  AND measured_at_utc > NOW() - interval '90 days'
			  AND extra_data->>'resting_hr' IS NOT NULL
			ORDER BY measured_at_utc::date
		`,
	},
	{
		name: "hrv_rmssd",
		query: `
			SELECT measured_at_utc::date AS date, rmssd_ms AS value
			FROM health_snapshots
			WHERE user_id = $1
			  AND measured_at_utc > NOW() - interval '90 days'
			  AND rmssd_ms IS NOT NULL
			ORDER BY measured_at_utc::date
		`,
	},
	{
		name: "sleep_score",
		query: `
			SELECT calendar_date AS date, score_overall::float AS value
			FROM health_sleep
			WHERE user_id = $1
			  AND calendar_date > CURRENT_DATE - 90
			  AND score_overall IS NOT NULL
			ORDER BY calendar_date
		`,
	},
	{
		name: "spo2",
		query: `
			SELECT measured_at_utc::date AS date, AVG(spo2_pct) AS value
			FROM health_spo2
			WHERE user_id = $1
			  AND measured_at_utc > NOW() - interval '90 days'
			GROUP BY 1
			ORDER BY 1
		`,
	},
}

// DetectAnomalies erkennt Anomalien via Rolling Z-Score über HRV, Resting HR,
// Sleep und SpO2. Flaggt Werte mit |z| >= 2.0.
// Schreibt das Ergebnis in analytics_cache (key: "anomalies").
//
// Graceful skip: weniger als 14 Datenpunkte pro Metrik → Metrik überspringen.
func DetectAnomalies(ctx context.Context, db *database.DB, userID string) error {
	// date -> AnomalyEntry
	allAnomalies := make(map[string]*AnomalyEntry)

	sevRank := map[string]int{"low": 0, "medium": 1, "high": 2}

	for _, mq := range anomalyMetrics {
		daily, err := fetchDailyValues(ctx, db, mq.query, userID)
		if err != nil || len(daily) < minDataPoints {
			continue
		}

		dates := sortedKeys(daily)
		values := make([]float64, len(dates))
		for i, d := range dates {
			values[i] = daily[d]
		}

		for i := range values {
			windowStart := i - windowDays
			if windowStart < 0 {
				windowStart = 0
			}
			window := values[windowStart:i]
			if len(window) < 7 {
				continue
			}

			baselineMean, baselineStd := sampleMeanStd(window)
			if baselineStd == 0 {
				continue
			}

			zscore := (values[i] - baselineMean) / baselineStd
			absZ := math.Abs(zscore)
			if absZ < zscoreThreshold {
				continue
			}

			severity := "low"
			if absZ >= 3.0 {
				severity = "high"
			} else if absZ >= 2.5 {
				severity = "medium"
			}

			dt := dates[i]
			entry, exists := allAnomalies[dt]
			if !exists {
				entry = &AnomalyEntry{
					Date:     dt,
					Severity: severity,
				}
				allAnomalies[dt] = entry
			} else if sevRank[severity] > sevRank[entry.Severity] {
				entry.Severity = severity
			}

			entry.AffectedMetrics = append(entry.AffectedMetrics, mq.name)
			entry.Details = append(entry.Details, AnomalyDetail{
				Metric:       mq.name,
				Value:        roundN(values[i], 2),
				ZScore:       roundN(zscore, 3),
				BaselineMean: roundN(baselineMean, 2),
				BaselineStd:  roundN(baselineStd, 2),
			})
		}
	}

	// Sort by date
	result := make([]AnomalyEntry, 0, len(allAnomalies))
	for _, e := range allAnomalies {
		result = append(result, *e)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Date < result[j].Date
	})

	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return db.UpsertAnalyticsCache(ctx, userID, anomaliesCacheKey, data)
}

// fetchDailyValues führt eine Metrik-Abfrage aus und gibt eine date->value Map zurück.
// Die Abfrage muss zwei Spalten zurückgeben: date (date/string) und value (float64).
// Beim gleichen Datum gewinnt der letzte Wert (Deduplizierung).
func fetchDailyValues(ctx context.Context, db *database.DB, query, userID string) (map[string]float64, error) {
	rows, err := db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	daily := make(map[string]float64)
	for rows.Next() {
		var dateStr string
		var val float64
		if err := rows.Scan(&dateStr, &val); err != nil {
			continue
		}
		daily[dateStr] = val
	}
	return daily, rows.Err()
}

// sortedKeys gibt die Keys einer map[string]float64 sortiert zurück.
func sortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sampleMeanStd berechnet Mittelwert und Sample-Standardabweichung (ddof=1).
func sampleMeanStd(vals []float64) (mean, std float64) {
	n := float64(len(vals))
	if n == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	mean = sum / n
	if n < 2 {
		return mean, 0
	}
	var variance float64
	for _, v := range vals {
		d := v - mean
		variance += d * d
	}
	variance /= n - 1 // sample std (ddof=1)
	std = math.Sqrt(variance)
	return
}

// roundN rundet auf n Dezimalstellen.
func roundN(v float64, decimals int) float64 {
	factor := math.Pow(10, float64(decimals))
	return math.Round(v*factor) / factor
}
