//go:build gate

package handler_test

// workshop_gate_test.go — Phase 4a empirical gate: validates the hypothesis that
// recording-group pairs share substantial GPS track overlap before investing in
// Phase 4d UI work.
//
// Run:
//   TEST_DATABASE_URL="postgres://..." \
//     go test -tags gate ./internal/handler/ -v -run TestWorkshopGate -timeout 5m

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"sort"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// groupPair represents one pair of activities from the same recording group.
type groupPair struct {
	GroupID    string
	ActivityA  string
	ActivityB  string
}

func TestWorkshopGate(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping workshop gate test")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to test DB: %v", err)
	}
	defer pool.Close()

	db := &database.DB{Pool: pool}

	// ── Step 1: collect all group pairs ──────────────────────────────────────
	rows, err := pool.Query(ctx, `
		SELECT rg.id, a1.activity_id, a2.activity_id
		FROM recording_groups rg
		JOIN recording_group_members a1 ON a1.group_id = rg.id
		JOIN recording_group_members a2 ON a2.group_id = rg.id
		                               AND a2.activity_id > a1.activity_id
		ORDER BY rg.created_at DESC, a1.activity_id
	`)
	if err != nil {
		t.Fatalf("query group pairs: %v", err)
	}
	var allPairs []groupPair
	for rows.Next() {
		var p groupPair
		if err := rows.Scan(&p.GroupID, &p.ActivityA, &p.ActivityB); err != nil {
			t.Fatalf("scan pair: %v", err)
		}
		allPairs = append(allPairs, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate pairs: %v", err)
	}

	poolSize := len(allPairs)
	t.Logf("Pool size: %d group-pairs", poolSize)
	if poolSize == 0 {
		t.Fatal("no group pairs found in DB — cannot run gate")
	}

	// ── Step 2: shuffle for random sampling ──────────────────────────────────
	rand.Shuffle(len(allPairs), func(i, j int) {
		allPairs[i], allPairs[j] = allPairs[j], allPairs[i]
	})

	// ── Step 3: score up to 10 pairs ─────────────────────────────────────────
	const wantScored = 10
	const radius = 50.0 // metres, matches CheckTier2

	var overlaps []float64
	var skipped int

	for _, pair := range allPairs {
		if len(overlaps) >= wantScored {
			break
		}

		streamsA, err := db.GetStreams(ctx, pair.ActivityA)
		if err != nil {
			t.Logf("SKIP pair (%s, %s): load streams A: %v", pair.ActivityA, pair.ActivityB, err)
			skipped++
			continue
		}
		streamsB, err := db.GetStreams(ctx, pair.ActivityB)
		if err != nil {
			t.Logf("SKIP pair (%s, %s): load streams B: %v", pair.ActivityA, pair.ActivityB, err)
			skipped++
			continue
		}

		trackA := extractLatLng(streamsA)
		trackB := extractLatLng(streamsB)

		if len(trackA) < 10 {
			t.Logf("SKIP pair (%s, %s): activity A has only %d GPS points", pair.ActivityA, pair.ActivityB, len(trackA))
			skipped++
			continue
		}
		if len(trackB) < 10 {
			t.Logf("SKIP pair (%s, %s): activity B has only %d GPS points", pair.ActivityA, pair.ActivityB, len(trackB))
			skipped++
			continue
		}

		overlap := importer.GPSOverlapPercent(trackA, trackB, radius)
		t.Logf("PAIR group=%s actA=%s actB=%s overlap=%.1f%%", pair.GroupID, pair.ActivityA, pair.ActivityB, overlap)
		overlaps = append(overlaps, overlap)
	}

	sampleSize := len(overlaps)
	t.Logf("Scored %d pairs (%d skipped, pool=%d)", sampleSize, skipped, poolSize)

	if sampleSize == 0 {
		t.Fatal("STOP: no pairs could be scored — check if GPS streams exist in the dev DB")
	}

	// ── Step 4: compute statistics ────────────────────────────────────────────
	var sum float64
	var aboveThreshold int
	const threshold = 75.0

	for _, v := range overlaps {
		sum += v
		if v >= threshold {
			aboveThreshold++
		}
	}
	avg := sum / float64(sampleSize)
	fractionAbove := float64(aboveThreshold) / float64(sampleSize)

	confirmed := avg > 70 && fractionAbove >= 0.6

	// sorted desc for readability
	sorted := make([]float64, len(overlaps))
	copy(sorted, overlaps)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] > sorted[j] })

	t.Logf("── Workshop Gate Results ──────────────────────────────────")
	t.Logf("Sample size:             %d / pool %d", sampleSize, poolSize)
	t.Logf("Threshold:               %.0f%%", threshold)
	t.Logf("avg_overlap:             %.2f%%", avg)
	t.Logf("above_threshold_count:   %d / %d", aboveThreshold, sampleSize)
	t.Logf("fraction_above_threshold: %.2f", fractionAbove)
	t.Logf("confirmed:               %v", confirmed)
	t.Logf("Overlaps (sorted desc):  %s", formatOverlaps(sorted))
	t.Logf("──────────────────────────────────────────────────────────")

	if confirmed {
		t.Logf("DECISION: Hypothesis CONFIRMED — Phase 4d may proceed.")
	} else {
		t.Logf("DECISION: Hypothesis NOT CONFIRMED — Phase 4d should be deferred pending investigation.")
	}

	// The gate is informational: we do not t.Fatal on a refuted hypothesis.
	// Log the result clearly so the CI / human runner can act on it.
}

// extractLatLng converts model.StreamResponse slices to LatLng slices for GPSOverlapPercent.
// Mirrors the unexported extractLatLng in importer/duplicate_detector.go.
func extractLatLng(streams []model.StreamResponse) []importer.LatLng {
	out := make([]importer.LatLng, 0, len(streams))
	for _, s := range streams {
		if s.Lat != nil && s.Lng != nil {
			out = append(out, importer.LatLng{Lat: *s.Lat, Lng: *s.Lng})
		}
	}
	return out
}

// formatOverlaps formats a slice of float64 as "[v1, v2, ...]".
func formatOverlaps(vals []float64) string {
	if len(vals) == 0 {
		return "[]"
	}
	s := "["
	for i, v := range vals {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf("%.1f", v)
	}
	s += "]"
	return s
}
