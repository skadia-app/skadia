// Standalone read-only tool: scan all DB activities through the new lift
// detector and emit a CSV report for false-positive hunting.
//
// Usage:
//
//	DATABASE_URL=postgres://skadia@localhost:5432/skadia \
//	  go run ./cmd/lift_detect_db_scan > scan.csv
//
//	# or with explicit DB URL flag:
//	go run ./cmd/lift_detect_db_scan -db-url postgres://... > scan.csv
//
//	# sample a subset (skip the per-stream loop after N activities):
//	go run ./cmd/lift_detect_db_scan -limit 50 > scan.csv
//
// Writes NOTHING to the DB. No migrations required. Uses the same heuristic
// detector entry-point as the production lift_enrich pipeline (DetectHeuristic
// on a lifts.StreamPoint slice), minus the OSM snap stage. Sport gating
// mirrors lifts.IsLiftRelevant.
//
// Output is a CSV with one row per eligible activity (-min-lifts/-skip-zero
// trim the result). For false-positive hunting, filter rows with n_lifts > 0
// and cross-reference titles + start coordinates against a ground-truth list.
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

func main() {
	dbURL := flag.String("db-url", os.Getenv("DATABASE_URL"), "PostgreSQL connection string")
	limit := flag.Int("limit", 0, "process only the first N eligible activities (0 = all)")
	sportFilter := flag.String("sport", "", "filter to one sport_type (empty = all eligible)")
	minLifts := flag.Int("min-lifts", 0, "only emit rows with >= N detected lifts (0 = emit all eligible)")
	skipZero := flag.Bool("skip-zero", false, "shortcut for -min-lifts=1 (emit only activities with detections)")
	progressEvery := flag.Int("progress", 25, "log a progress line every N activities (stderr)")
	flag.Parse()

	if *dbURL == "" {
		fmt.Fprintln(os.Stderr, "need -db-url or DATABASE_URL")
		os.Exit(2)
	}
	if *skipZero && *minLifts < 1 {
		*minLifts = 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := pgxpool.New(ctx, *dbURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pgxpool.New:", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Header: human-readable, FP-triage friendly.
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()
	if err := w.Write([]string{
		"activity_id", "user_id", "sport_type", "sub_type",
		"is_indoor", "is_electric", "is_virtual",
		"started_at", "start_lat", "start_lng", "title",
		"n_samples", "n_lifts", "n_high", "n_medium", "n_low", "n_bidir",
		"total_ascent_m", "total_descent_m", "total_duration_s",
	}); err != nil {
		fmt.Fprintln(os.Stderr, "csv header:", err)
		os.Exit(1)
	}

	// Eligibility: same predicate as production (IsLiftRelevant).
	q := `
		SELECT id, user_id, sport_type, COALESCE(sub_type,''),
		       COALESCE(is_indoor,false), COALESCE(is_electric,false), COALESCE(is_virtual,false),
		       started_at,
		       ST_Y(start_location::geometry) AS start_lat,
		       ST_X(start_location::geometry) AS start_lng,
		       COALESCE(title,'')
		FROM activities
		WHERE start_location IS NOT NULL
	`
	if *sportFilter != "" {
		q += fmt.Sprintf(" AND sport_type = '%s'", strings.ReplaceAll(*sportFilter, "'", ""))
	}
	q += " ORDER BY started_at DESC"
	if *limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", *limit*4) // over-fetch; eligibility filter trims later
	}

	rows, err := pool.Query(ctx, q)
	if err != nil {
		fmt.Fprintln(os.Stderr, "activities query:", err)
		os.Exit(1)
	}

	type cand struct {
		activityID string
		userID     string
		sportType  string
		subType    string
		isIndoor   bool
		isElectric bool
		isVirtual  bool
		startedAt  time.Time
		lat, lng   float64
		title      string
	}
	var batch []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(
			&c.activityID, &c.userID, &c.sportType, &c.subType,
			&c.isIndoor, &c.isElectric, &c.isVirtual,
			&c.startedAt, &c.lat, &c.lng, &c.title,
		); err != nil {
			fmt.Fprintln(os.Stderr, "scan activity:", err)
			os.Exit(1)
		}
		if !lifts.IsLiftRelevant(c.sportType, c.subType, c.isIndoor, c.isElectric, c.isVirtual) {
			continue
		}
		batch = append(batch, c)
		if *limit > 0 && len(batch) >= *limit {
			break
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "activities rows:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "eligible activities to scan: %d\n", len(batch))

	emitted := 0
	for i, c := range batch {
		if (*progressEvery > 0) && (i%(*progressEvery) == 0) {
			fmt.Fprintf(os.Stderr, "[%d/%d] %s sport=%s lifts-emitted=%d\n",
				i+1, len(batch), c.activityID, c.sportType, emitted)
		}

		pts, err := loadStreamPoints(ctx, pool, c.activityID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%d/%d] %s load-stream error: %v\n", i+1, len(batch), c.activityID, err)
			continue
		}
		if len(pts) < 30 { // detector needs at least a couple of valid windows
			continue
		}

		cfg := lifts.DefaultDetectorConfig(c.sportType)
		rides := lifts.DetectHeuristic(pts, cfg)
		if len(rides) < *minLifts {
			continue
		}

		nHigh, nMedium, nLow, nBidir := 0, 0, 0, 0
		var totalAsc, totalDesc float64
		var totalDur int
		for _, r := range rides {
			switch r.Confidence {
			case "high":
				nHigh++
			case "medium":
				nMedium++
			case "low":
				nLow++
			case "bidir":
				nBidir++
			}
			totalAsc += r.AscentM
			totalDesc += r.DescentM
			totalDur += r.DurationS
		}

		if err := w.Write([]string{
			c.activityID, c.userID, c.sportType, c.subType,
			boolStr(c.isIndoor), boolStr(c.isElectric), boolStr(c.isVirtual),
			c.startedAt.UTC().Format(time.RFC3339),
			strconv.FormatFloat(c.lat, 'f', 5, 64),
			strconv.FormatFloat(c.lng, 'f', 5, 64),
			c.title,
			strconv.Itoa(len(pts)),
			strconv.Itoa(len(rides)),
			strconv.Itoa(nHigh), strconv.Itoa(nMedium), strconv.Itoa(nLow), strconv.Itoa(nBidir),
			strconv.FormatFloat(totalAsc, 'f', 1, 64),
			strconv.FormatFloat(totalDesc, 'f', 1, 64),
			strconv.Itoa(totalDur),
		}); err != nil {
			fmt.Fprintln(os.Stderr, "csv write:", err)
			os.Exit(1)
		}
		w.Flush()
		emitted++
	}
	fmt.Fprintf(os.Stderr, "scan complete: scanned=%d emitted=%d\n", len(batch), emitted)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// loadStreamPoints mirrors enrichment.getEnrichmentStream but converts directly
// to lifts.StreamPoint (skipping the EnrichmentStreamPoint intermediate, which
// is package-private).
func loadStreamPoints(ctx context.Context, pool *pgxpool.Pool, activityID string) ([]lifts.StreamPoint, error) {
	rows, err := pool.Query(ctx, `
		SELECT time, lat, lng, altitude_m, speed_ms, distance_m
		FROM activity_streams
		WHERE activity_id = $1
		ORDER BY time ASC
	`, activityID)
	if err != nil {
		return nil, fmt.Errorf("stream query: %w", err)
	}
	defer rows.Close()
	var pts []lifts.StreamPoint
	for rows.Next() {
		var p lifts.StreamPoint
		if err := rows.Scan(&p.Time, &p.Lat, &p.Lng, &p.AltitudeM, &p.SpeedMs, &p.DistanceM); err != nil {
			if err == pgx.ErrNoRows {
				return nil, nil
			}
			return nil, fmt.Errorf("stream scan: %w", err)
		}
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("stream rows: %w", err)
	}
	return pts, nil
}
