package analytics

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

const (
	correlationsCacheKey = "correlations"
	minPairs             = 10
	maxLagDays           = 7
	topN                 = 20
)

// CorrelationEntry represents a single feature-pair correlation result.
type CorrelationEntry struct {
	FeatureA       string   `json:"feature_a"`
	FeatureB       string   `json:"feature_b"`
	LagDays        int      `json:"lag_days"`
	RPearson       float64  `json:"r_pearson"`
	RSpearman      *float64 `json:"r_spearman"`
	AbsR           float64  `json:"abs_r"`
	N              int      `json:"n"`
	Interpretation string   `json:"interpretation"`
}

// CorrelationsResult is the top-level output stored in analytics_cache.
type CorrelationsResult struct {
	Correlations        []CorrelationEntry `json:"correlations"`
	TotalPairsEvaluated int                `json:"total_pairs_evaluated"`
}

// metricPair defines the two series to correlate.
type metricPair struct {
	a string
	b string
}

var correlationPairs = []metricPair{
	{"hrv_rmssd", "tss"},
	{"hrv_rmssd", "atl"},
	{"sleep_score", "tss"},
	{"sleep_score", "hrv_rmssd"},
}

// ComputeCorrelations berechnet Pearson + Spearman zwischen Metriken mit Lag-Analyse
// (0–7 Tage) und schreibt das Ergebnis in analytics_cache (key: "correlations").
//
// Nutzt die Top-20 Paare (sortiert nach |r_pearson|).
// Graceful skip pro Metrik-Paar: weniger als 10 überlappende Tage → überspringen.
func ComputeCorrelations(ctx context.Context, db *database.DB, userID string) error {
	// Fetch all needed time series (180 days)
	hrv, err := fetchSeriesHRV(ctx, db, userID)
	if err != nil {
		hrv = map[string]float64{}
	}
	tss, err := fetchSeriesTSS(ctx, db, userID)
	if err != nil {
		tss = map[string]float64{}
	}
	atl, err := fetchSeriesATL(ctx, db, userID)
	if err != nil {
		atl = map[string]float64{}
	}
	sleepScore, err := fetchSeriesSleepScore(ctx, db, userID)
	if err != nil {
		sleepScore = map[string]float64{}
	}

	series := map[string]map[string]float64{
		"hrv_rmssd":   hrv,
		"tss":         tss,
		"atl":         atl,
		"sleep_score": sleepScore,
	}

	// Collect all dates across all series
	dateSet := make(map[string]struct{})
	for _, s := range series {
		for d := range s {
			dateSet[d] = struct{}{}
		}
	}
	allDates := make([]string, 0, len(dateSet))
	for d := range dateSet {
		allDates = append(allDates, d)
	}
	sort.Strings(allDates)

	var allCorrelations []CorrelationEntry

	for lag := 0; lag <= maxLagDays; lag++ {
		for _, pair := range correlationPairs {
			seriesA := series[pair.a]
			seriesB := series[pair.b]

			var xs, ys []float64
			for _, dt := range allDates {
				valA, okA := seriesA[dt]
				if !okA {
					continue
				}
				laggedDt := shiftDateString(dt, lag)
				valB, okB := seriesB[laggedDt]
				if !okB {
					continue
				}
				xs = append(xs, valA)
				ys = append(ys, valB)
			}

			if len(xs) < minPairs {
				continue
			}

			rPearson, ok := pearsonR(xs, ys)
			if !ok {
				continue
			}

			var rSpearman *float64
			if rs, ok := spearmanR(xs, ys); ok {
				rSpearman = &rs
			}

			allCorrelations = append(allCorrelations, CorrelationEntry{
				FeatureA:       pair.a,
				FeatureB:       pair.b,
				LagDays:        lag,
				RPearson:       roundN(rPearson, 3),
				RSpearman:      roundSpearman(rSpearman),
				AbsR:           roundN(math.Abs(rPearson), 3),
				N:              len(xs),
				Interpretation: interpretR(rPearson, false),
			})
		}
	}

	// Sort by abs_r descending, take top-N
	sort.Slice(allCorrelations, func(i, j int) bool {
		return allCorrelations[i].AbsR > allCorrelations[j].AbsR
	})
	totalEvaluated := len(allCorrelations)
	if len(allCorrelations) > topN {
		allCorrelations = allCorrelations[:topN]
	}

	result := CorrelationsResult{
		Correlations:        allCorrelations,
		TotalPairsEvaluated: totalEvaluated,
	}

	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return db.UpsertAnalyticsCache(ctx, userID, correlationsCacheKey, data)
}

// --- Series fetchers ---

func fetchSeriesHRV(ctx context.Context, db *database.DB, userID string) (map[string]float64, error) {
	return fetchSeriesSQL(ctx, db, userID, `
		SELECT measured_at_utc::date AS date, AVG(rmssd_ms) AS value
		FROM health_snapshots
		WHERE user_id = $1 AND measured_at_utc > NOW() - interval '180 days'
		  AND rmssd_ms IS NOT NULL
		GROUP BY 1 ORDER BY 1
	`)
}

func fetchSeriesTSS(ctx context.Context, db *database.DB, userID string) (map[string]float64, error) {
	return fetchSeriesSQL(ctx, db, userID, `
		SELECT date, tss AS value FROM pmc_daily
		WHERE user_id = $1 AND date > CURRENT_DATE - 180 AND tss IS NOT NULL
		ORDER BY date
	`)
}

func fetchSeriesATL(ctx context.Context, db *database.DB, userID string) (map[string]float64, error) {
	return fetchSeriesSQL(ctx, db, userID, `
		SELECT date, atl AS value FROM pmc_daily
		WHERE user_id = $1 AND date > CURRENT_DATE - 180 AND atl IS NOT NULL
		ORDER BY date
	`)
}

func fetchSeriesSleepScore(ctx context.Context, db *database.DB, userID string) (map[string]float64, error) {
	return fetchSeriesSQL(ctx, db, userID, `
		SELECT calendar_date AS date, score_overall::float AS value
		FROM health_sleep
		WHERE user_id = $1 AND calendar_date > CURRENT_DATE - 180
		  AND score_overall IS NOT NULL
		ORDER BY calendar_date
	`)
}

func fetchSeriesSQL(ctx context.Context, db *database.DB, userID, query string) (map[string]float64, error) {
	rows, err := db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]float64)
	for rows.Next() {
		var dateStr string
		var val float64
		if err := rows.Scan(&dateStr, &val); err != nil {
			continue
		}
		m[dateStr] = val
	}
	return m, rows.Err()
}

// --- Statistical functions ---

// pearsonR berechnet den Pearson-Korrelationskoeffizienten via manuelle Formel.
func pearsonR(xs, ys []float64) (float64, bool) {
	n := len(xs)
	if n < 3 {
		return 0, false
	}

	var sumX, sumY float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
	}
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)

	var num, denX, denY float64
	for i := range xs {
		dx := xs[i] - meanX
		dy := ys[i] - meanY
		num += dx * dy
		denX += dx * dx
		denY += dy * dy
	}
	den := math.Sqrt(denX * denY)
	if den == 0 {
		return 0, false
	}
	r := num / den
	if math.IsNaN(r) {
		return 0, false
	}
	return r, true
}

// spearmanR berechnet den Spearman-Rangkorrelationskoeffizienten
// als Pearson auf den Rängen.
func spearmanR(xs, ys []float64) (float64, bool) {
	if len(xs) < 3 {
		return 0, false
	}
	rankX := rankData(xs)
	rankY := rankData(ys)
	return pearsonR(rankX, rankY)
}

// rankData weist jedem Element seinen Rang zu (1-basiert, mittlerer Rang bei Gleichstand).
func rankData(vals []float64) []float64 {
	n := len(vals)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool {
		return vals[idx[a]] < vals[idx[b]]
	})

	ranks := make([]float64, n)
	for i := 0; i < n; {
		j := i + 1
		for j < n && vals[idx[j]] == vals[idx[i]] {
			j++
		}
		// Mittlerer Rang für Gleichstandsgruppe
		avgRank := float64(i+j+1) / 2.0 // 1-basiert: (i+1 + j) / 2
		for k := i; k < j; k++ {
			ranks[idx[k]] = avgRank
		}
		i = j
	}
	return ranks
}

// interpretR gibt eine textuelle Interpretation des Pearson-r zurück.
func interpretR(r float64, inverse bool) string {
	absR := math.Abs(r)

	var strength string
	switch {
	case absR < 0.1:
		strength = "keine"
	case absR < 0.3:
		strength = "schwache"
	case absR < 0.5:
		strength = "moderate"
	case absR < 0.7:
		strength = "starke"
	default:
		strength = "sehr starke"
	}

	effectiveR := r
	if inverse {
		effectiveR = -r
	}
	direction := "positive"
	if effectiveR < 0 {
		direction = "negative"
	}
	return strength + " " + direction + " Korrelation"
}

// shiftDateString addiert lag Tage zu einem "2006-01-02" Datumsstring.
func shiftDateString(dt string, lag int) string {
	t, err := time.Parse("2006-01-02", dt)
	if err != nil {
		return dt
	}
	return t.AddDate(0, 0, lag).Format("2006-01-02")
}

// roundSpearman rundet einen optionalen Spearman-Koeffizienten.
func roundSpearman(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := roundN(*p, 3)
	return &v
}
