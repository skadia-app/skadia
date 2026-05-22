package database_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

func analyticsStatsPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func insertAnalyticsStatsUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	userID := uuid.NewString()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "analytics_stats_"+userID[:8], "analytics_stats_"+userID[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})
	return userID
}

type analyticsStatsActivity struct {
	DistanceM              float64
	ElevationGainM         float64
	ElevationLossM         float64
	MovingTimeS            int
	ElapsedTimeS           int
	TotalCalories          int
	LiftCount              int
	LiftDistanceM          float64
	LiftAscentM            float64
	LiftDescentM           float64
	ExcludeLiftSubtraction bool
	IsGroupSecondary       bool
	VisitedCountryCodes    []string
}

func insertAnalyticsStatsActivity(t *testing.T, pool *pgxpool.Pool, userID string, startedAt time.Time, act analyticsStatsActivity) {
	t.Helper()
	visitedCountryCodes := act.VisitedCountryCodes
	if visitedCountryCodes == nil {
		visitedCountryCodes = []string{}
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO activities
			(id, user_id, sport_type, started_at, ended_at, file_hash,
			 distance_m, elevation_gain_m, elevation_loss_m, moving_time_s, elapsed_time_s,
			 total_calories, lift_count, lift_distance_m, lift_ascent_m,
			 lift_descent_m, exclude_lift_subtraction, is_group_secondary, visited_country_codes)
			VALUES ($1, $2, 'skiing', $3, $4, $5,
			        $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		`, uuid.NewString(), userID, startedAt, startedAt.Add(time.Hour), uuid.NewString(),
		act.DistanceM, act.ElevationGainM, act.ElevationLossM, act.MovingTimeS, act.ElapsedTimeS,
		act.TotalCalories, act.LiftCount, act.LiftDistanceM, act.LiftAscentM,
		act.LiftDescentM, act.ExcludeLiftSubtraction, act.IsGroupSecondary, visitedCountryCodes,
	); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
}

func TestGetLifetimeStatsLiftNetAggregation(t *testing.T) {
	tests := []struct {
		name string
		act  analyticsStatsActivity
		want database.LifetimeStats
	}{
		{
			name: "activity without lifts stays raw",
			act: analyticsStatsActivity{
				DistanceM:      12_000,
				ElevationGainM: 700,
				ElevationLossM: 650,
				MovingTimeS:    3_600,
				ElapsedTimeS:   4_000,
				TotalCalories:  850,
			},
			want: database.LifetimeStats{
				TotalActivities:     1,
				TotalDistanceM:      12_000,
				TotalElevationM:     700,
				TotalElevationLossM: 650,
				TotalMovingTimeS:    3_600,
				TotalElapsedTimeS:   4_000,
				TotalCalories:       850,
				ActiveDays:          1,
				LiftRideCount:       0,
				LiftAscentM:         0,
				LiftDescentM:        0,
				LiftDistanceM:       0,
			},
		},
		{
			name: "confirmed lifts subtract from lifetime totals",
			act: analyticsStatsActivity{
				DistanceM:              20_000,
				ElevationGainM:         900,
				ElevationLossM:         1_200,
				MovingTimeS:            5_000,
				ElapsedTimeS:           5_700,
				TotalCalories:          1_100,
				LiftCount:              2,
				LiftDistanceM:          3_500,
				LiftAscentM:            400,
				LiftDescentM:           50,
				ExcludeLiftSubtraction: false,
			},
			want: database.LifetimeStats{
				TotalActivities:     1,
				TotalDistanceM:      16_500,
				TotalElevationM:     500,
				TotalElevationLossM: 1_150,
				TotalMovingTimeS:    5_000,
				TotalElapsedTimeS:   5_700,
				TotalCalories:       1_100,
				ActiveDays:          1,
				LiftRideCount:       2,
				LiftAscentM:         400,
				LiftDescentM:        50,
				LiftDistanceM:       3_500,
			},
		},
		{
			name: "lift subtraction opt-out keeps raw totals but exposes lift aggregates",
			act: analyticsStatsActivity{
				DistanceM:              20_000,
				ElevationGainM:         900,
				ElevationLossM:         1_200,
				MovingTimeS:            5_000,
				ElapsedTimeS:           5_700,
				TotalCalories:          1_100,
				LiftCount:              2,
				LiftDistanceM:          3_500,
				LiftAscentM:            400,
				LiftDescentM:           50,
				ExcludeLiftSubtraction: true,
			},
			want: database.LifetimeStats{
				TotalActivities:     1,
				TotalDistanceM:      20_000,
				TotalElevationM:     900,
				TotalElevationLossM: 1_200,
				TotalMovingTimeS:    5_000,
				TotalElapsedTimeS:   5_700,
				TotalCalories:       1_100,
				ActiveDays:          1,
				LiftRideCount:       2,
				LiftAscentM:         400,
				LiftDescentM:        50,
				LiftDistanceM:       3_500,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := analyticsStatsPool(t)
			userID := insertAnalyticsStatsUser(t, pool)
			insertAnalyticsStatsActivity(t, pool, userID, time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC), tt.act)

			got, err := (&database.DB{Pool: pool}).GetLifetimeStats(context.Background(), userID, "", nil)
			if err != nil {
				t.Fatalf("GetLifetimeStats: %v", err)
			}

			assertLifetimeStats(t, *got, tt.want)
		})
	}
}

func TestGetEddingtonDataLiftNetAggregation(t *testing.T) {
	pool := analyticsStatsPool(t)
	userID := insertAnalyticsStatsUser(t, pool)

	insertAnalyticsStatsActivity(t, pool, userID, time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC), analyticsStatsActivity{
		DistanceM:      10_000,
		ElevationGainM: 800,
		ElevationLossM: 700,
		MovingTimeS:    3_600,
		ElapsedTimeS:   4_000,
		LiftCount:      1,
		LiftDistanceM:  2_000,
		LiftAscentM:    300,
		LiftDescentM:   200,
	})
	insertAnalyticsStatsActivity(t, pool, userID, time.Date(2026, 1, 3, 8, 0, 0, 0, time.UTC), analyticsStatsActivity{
		DistanceM:              5_000,
		ElevationGainM:         1_200,
		ElevationLossM:         900,
		MovingTimeS:            2_600,
		ElapsedTimeS:           3_000,
		LiftCount:              1,
		LiftDistanceM:          3_000,
		LiftAscentM:            1_000,
		LiftDescentM:           800,
		ExcludeLiftSubtraction: true,
	})

	db := &database.DB{Pool: pool}
	km, err := db.GetEddingtonData(context.Background(), userID, "km", "", nil)
	if err != nil {
		t.Fatalf("GetEddingtonData km: %v", err)
	}
	elevation, err := db.GetEddingtonData(context.Background(), userID, "elevation", "", nil)
	if err != nil {
		t.Fatalf("GetEddingtonData elevation: %v", err)
	}
	descent, err := db.GetEddingtonData(context.Background(), userID, "descent", "", nil)
	if err != nil {
		t.Fatalf("GetEddingtonData descent: %v", err)
	}

	assertFloatSlice(t, "km", km, []float64{8, 5})
	assertFloatSlice(t, "elevation", elevation, []float64{1_200, 500})
	assertFloatSlice(t, "descent", descent, []float64{900, 500})
}

func TestGetLifetimeStatsCountriesUseVisitedCountryCodesWithoutDuplicatingTotals(t *testing.T) {
	pool := analyticsStatsPool(t)
	userID := insertAnalyticsStatsUser(t, pool)

	insertAnalyticsStatsActivity(t, pool, userID, time.Date(2026, 1, 6, 8, 0, 0, 0, time.UTC), analyticsStatsActivity{
		DistanceM:           10_000,
		ElevationGainM:      500,
		ElevationLossM:      450,
		MovingTimeS:         3_000,
		ElapsedTimeS:        3_600,
		TotalCalories:       700,
		VisitedCountryCodes: []string{"CH", "IT"},
	})
	insertAnalyticsStatsActivity(t, pool, userID, time.Date(2026, 1, 7, 8, 0, 0, 0, time.UTC), analyticsStatsActivity{
		DistanceM:           5_000,
		ElevationGainM:      200,
		ElevationLossM:      250,
		MovingTimeS:         1_800,
		ElapsedTimeS:        2_000,
		TotalCalories:       300,
		VisitedCountryCodes: []string{"IT"},
	})
	insertAnalyticsStatsActivity(t, pool, userID, time.Date(2026, 1, 8, 8, 0, 0, 0, time.UTC), analyticsStatsActivity{
		DistanceM:           50_000,
		VisitedCountryCodes: []string{"FR"},
		IsGroupSecondary:    true,
	})

	got, err := (&database.DB{Pool: pool}).GetLifetimeStats(context.Background(), userID, "", nil)
	if err != nil {
		t.Fatalf("GetLifetimeStats: %v", err)
	}

	if got.Countries != 2 {
		t.Fatalf("Countries = %d, want 2", got.Countries)
	}
	if got.TotalActivities != 2 {
		t.Fatalf("TotalActivities = %d, want 2", got.TotalActivities)
	}
	assertFloat64(t, "TotalDistanceM", got.TotalDistanceM, 15_000)
	assertFloat64(t, "TotalElevationM", got.TotalElevationM, 700)
}

func TestGetDistributionLiftNetAggregationAndGroupSecondaryExcluded(t *testing.T) {
	pool := analyticsStatsPool(t)
	userID := insertAnalyticsStatsUser(t, pool)
	base := time.Date(2026, 1, 4, 8, 0, 0, 0, time.UTC)

	insertAnalyticsStatsActivity(t, pool, userID, base, analyticsStatsActivity{
		DistanceM:      20_000,
		ElevationGainM: 900,
		ElevationLossM: 1_200,
		MovingTimeS:    5_000,
		LiftDistanceM:  3_500,
		LiftAscentM:    400,
		LiftDescentM:   50,
	})
	insertAnalyticsStatsActivity(t, pool, userID, base.Add(2*time.Hour), analyticsStatsActivity{
		DistanceM:              8_000,
		ElevationGainM:         600,
		ElevationLossM:         500,
		MovingTimeS:            2_000,
		LiftDistanceM:          2_000,
		LiftAscentM:            200,
		LiftDescentM:           100,
		ExcludeLiftSubtraction: true,
	})
	insertAnalyticsStatsActivity(t, pool, userID, base.Add(4*time.Hour), analyticsStatsActivity{
		DistanceM:        50_000,
		ElevationGainM:   5_000,
		ElevationLossM:   6_000,
		MovingTimeS:      10_000,
		LiftDistanceM:    20_000,
		LiftAscentM:      2_000,
		LiftDescentM:     3_000,
		IsGroupSecondary: true,
	})

	db := &database.DB{Pool: pool}
	rows, err := db.GetDistribution(context.Background(), userID, "sport", "distance_m", "", nil)
	if err != nil {
		t.Fatalf("GetDistribution distance_m: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("distance rows len = %d, want 1", len(rows))
	}
	assertFloat64(t, "distance value", rows[0].Value, 24_500)
	if rows[0].Count != 2 {
		t.Fatalf("distance count = %d, want 2 (secondary row leaked)", rows[0].Count)
	}

	rows, err = db.GetDistribution(context.Background(), userID, "sport", "elevation_gain_m", "", nil)
	if err != nil {
		t.Fatalf("GetDistribution elevation_gain_m: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("gain rows len = %d, want 1", len(rows))
	}
	assertFloat64(t, "gain value", rows[0].Value, 1_100)

	rows, err = db.GetDistribution(context.Background(), userID, "sport", "elevation_loss_m", "", nil)
	if err != nil {
		t.Fatalf("GetDistribution elevation_loss_m: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("loss rows len = %d, want 1", len(rows))
	}
	assertFloat64(t, "loss value", rows[0].Value, 1_650)
}

func TestGetDistributionDistanceBucketUsesNetDistance(t *testing.T) {
	pool := analyticsStatsPool(t)
	userID := insertAnalyticsStatsUser(t, pool)
	base := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)

	insertAnalyticsStatsActivity(t, pool, userID, base, analyticsStatsActivity{
		DistanceM:     6_000,
		LiftDistanceM: 2_000,
	})
	insertAnalyticsStatsActivity(t, pool, userID, base.Add(time.Hour), analyticsStatsActivity{
		DistanceM:              6_000,
		LiftDistanceM:          2_000,
		ExcludeLiftSubtraction: true,
	})

	rows, err := (&database.DB{Pool: pool}).GetDistribution(context.Background(), userID, "distance_bucket", "count", "", nil)
	if err != nil {
		t.Fatalf("GetDistribution distance_bucket: %v", err)
	}

	buckets := map[string]int64{}
	for _, row := range rows {
		buckets[row.Label] = row.Count
	}
	if buckets["<5 km"] != 1 {
		t.Fatalf("<5 km count = %d, want 1 from net 4 km activity", buckets["<5 km"])
	}
	if buckets["5–10 km"] != 1 {
		t.Fatalf("5–10 km count = %d, want 1 from opt-out raw 6 km activity", buckets["5–10 km"])
	}
}

// TestGetLifetimeStats_GroupSecondaryExcluded verifies F2: is_group_secondary rows
// must be excluded from lifetime totals so merged fusion activities are not double-counted.
func TestGetLifetimeStats_GroupSecondaryExcluded(t *testing.T) {
	pool := analyticsStatsPool(t)
	userID := insertAnalyticsStatsUser(t, pool)

	base := time.Date(2026, 2, 1, 8, 0, 0, 0, time.UTC)

	// Primary activity — should count.
	insertAnalyticsStatsActivity(t, pool, userID, base, analyticsStatsActivity{
		DistanceM:      15_000,
		ElevationGainM: 500,
		ElevationLossM: 700,
		MovingTimeS:    3_600,
		ElapsedTimeS:   4_000,
		TotalCalories:  800,
		LiftCount:      1,
		LiftDistanceM:  2_000,
		LiftAscentM:    200,
		LiftDescentM:   30,
	})
	// Secondary activity — must be excluded entirely.
	insertAnalyticsStatsActivity(t, pool, userID, base.Add(2*time.Hour), analyticsStatsActivity{
		DistanceM:        8_000,
		ElevationGainM:   300,
		ElevationLossM:   400,
		MovingTimeS:      1_800,
		ElapsedTimeS:     2_000,
		TotalCalories:    400,
		LiftDistanceM:    1_000,
		LiftAscentM:      100,
		IsGroupSecondary: true,
	})

	got, err := (&database.DB{Pool: pool}).GetLifetimeStats(context.Background(), userID, "", nil)
	if err != nil {
		t.Fatalf("GetLifetimeStats: %v", err)
	}

	// Only the primary row counts. Net distance = 15000 - 2000 = 13000.
	want := database.LifetimeStats{
		TotalActivities:     1,
		TotalDistanceM:      13_000,
		TotalElevationM:     300,
		TotalElevationLossM: 670,
		TotalMovingTimeS:    3_600,
		TotalElapsedTimeS:   4_000,
		TotalCalories:       800,
		ActiveDays:          1,
		LiftRideCount:       1,
		LiftAscentM:         200,
		LiftDescentM:        30,
		LiftDistanceM:       2_000,
	}
	assertLifetimeStats(t, *got, want)
}

// TestGetLifetimeStats_PerRowFloor verifies F15: per-row GREATEST(0,...) prevents
// a single over-large lift value from producing a negative contribution to the SUM.
func TestGetLifetimeStats_PerRowFloor(t *testing.T) {
	pool := analyticsStatsPool(t)
	userID := insertAnalyticsStatsUser(t, pool)

	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	// Row where lift exceeds raw (should clamp to 0 before summing).
	insertAnalyticsStatsActivity(t, pool, userID, base, analyticsStatsActivity{
		DistanceM:      1_000,
		ElevationGainM: 100,
		ElevationLossM: 80,
		LiftDistanceM:  5_000, // lift > raw — per-row floor must clamp to 0
		LiftAscentM:    500,
		LiftDescentM:   300,
	})
	// Normal row.
	insertAnalyticsStatsActivity(t, pool, userID, base.Add(2*time.Hour), analyticsStatsActivity{
		DistanceM:      10_000,
		ElevationGainM: 400,
		ElevationLossM: 500,
		LiftDistanceM:  2_000,
		LiftAscentM:    150,
		LiftDescentM:   100,
	})

	got, err := (&database.DB{Pool: pool}).GetLifetimeStats(context.Background(), userID, "", nil)
	if err != nil {
		t.Fatalf("GetLifetimeStats: %v", err)
	}

	// Row 1: GREATEST(0, 1000-5000)=0 distance, GREATEST(0, 100-500)=0 elevation.
	// Row 2: GREATEST(0, 10000-2000)=8000 distance, GREATEST(0, 400-150)=250 elevation.
	if got.TotalDistanceM < 0 {
		t.Errorf("TotalDistanceM = %.0f, must be non-negative", got.TotalDistanceM)
	}
	assertFloat64(t, "TotalDistanceM", got.TotalDistanceM, 8_000)
	assertFloat64(t, "TotalElevationM", got.TotalElevationM, 250)
	assertFloat64(t, "TotalElevationLossM", got.TotalElevationLossM, 400)
}

// TestGetEddingtonData_GroupSecondaryExcluded verifies F2: secondary rows must not
// appear in the per-day aggregation used for Eddington number calculation.
func TestGetEddingtonData_GroupSecondaryExcluded(t *testing.T) {
	pool := analyticsStatsPool(t)
	userID := insertAnalyticsStatsUser(t, pool)

	day := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)

	// Primary: net 8 km on day 1.
	insertAnalyticsStatsActivity(t, pool, userID, day, analyticsStatsActivity{
		DistanceM:      10_000,
		ElevationGainM: 400,
		ElevationLossM: 500,
		LiftDistanceM:  2_000,
		LiftAscentM:    100,
		LiftDescentM:   80,
	})
	// Secondary on same day: must not add to the day total.
	insertAnalyticsStatsActivity(t, pool, userID, day.Add(time.Hour), analyticsStatsActivity{
		DistanceM:        20_000,
		ElevationGainM:   800,
		ElevationLossM:   900,
		LiftDistanceM:    3_000,
		LiftAscentM:      300,
		LiftDescentM:     250,
		IsGroupSecondary: true,
	})

	db := &database.DB{Pool: pool}
	km, err := db.GetEddingtonData(context.Background(), userID, "km", "", nil)
	if err != nil {
		t.Fatalf("GetEddingtonData km: %v", err)
	}
	// Expect exactly one day value = 8 km (10000 - 2000 = 8000 → 8.0 km).
	if len(km) != 1 {
		t.Fatalf("km len = %d, want 1 (secondary row leaked)", len(km))
	}
	assertFloat64(t, "km[0]", km[0], 8.0)

	elevation, err := db.GetEddingtonData(context.Background(), userID, "elevation", "", nil)
	if err != nil {
		t.Fatalf("GetEddingtonData elevation: %v", err)
	}
	if len(elevation) != 1 {
		t.Fatalf("elevation len = %d, want 1 (secondary row leaked)", len(elevation))
	}
	assertFloat64(t, "elevation[0]", elevation[0], 300)

	descent, err := db.GetEddingtonData(context.Background(), userID, "descent", "", nil)
	if err != nil {
		t.Fatalf("GetEddingtonData descent: %v", err)
	}
	if len(descent) != 1 {
		t.Fatalf("descent len = %d, want 1 (secondary row leaked)", len(descent))
	}
	assertFloat64(t, "descent[0]", descent[0], 420)
}

// TestGetEddingtonData_PerRowFloor verifies F15: the Eddington per-day aggregation
// uses per-row GREATEST(0,...) so an over-large lift on one row does not produce a
// negative day contribution when summed with other rows on the same day.
func TestGetEddingtonData_PerRowFloor(t *testing.T) {
	pool := analyticsStatsPool(t)
	userID := insertAnalyticsStatsUser(t, pool)

	day := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)

	// Row where lift > raw (clamps to 0 per-row).
	insertAnalyticsStatsActivity(t, pool, userID, day, analyticsStatsActivity{
		DistanceM:      500,
		ElevationGainM: 50,
		ElevationLossM: 40,
		LiftDistanceM:  5_000,
		LiftAscentM:    600,
		LiftDescentM:   500,
	})
	// Normal row on same day.
	insertAnalyticsStatsActivity(t, pool, userID, day.Add(time.Hour), analyticsStatsActivity{
		DistanceM:      8_000,
		ElevationGainM: 300,
		ElevationLossM: 350,
		LiftDistanceM:  1_000,
		LiftAscentM:    100,
		LiftDescentM:   70,
	})

	db := &database.DB{Pool: pool}
	km, err := db.GetEddingtonData(context.Background(), userID, "km", "", nil)
	if err != nil {
		t.Fatalf("GetEddingtonData km: %v", err)
	}
	// Per-row: row1 → 0, row2 → 7000. Sum → 7.0 km.
	// Sum-then-floor (old form): GREATEST(0, (500+8000)-(5000+1000)) = GREATEST(0, 2500) = 2.5 km — different.
	if len(km) != 1 {
		t.Fatalf("km len = %d, want 1", len(km))
	}
	assertFloat64(t, "km[0]", km[0], 7.0)

	elevation, err := db.GetEddingtonData(context.Background(), userID, "elevation", "", nil)
	if err != nil {
		t.Fatalf("GetEddingtonData elevation: %v", err)
	}
	if len(elevation) != 1 {
		t.Fatalf("elevation len = %d, want 1", len(elevation))
	}
	// Per-row: row1 → 0, row2 → 200. Sum → 200.
	assertFloat64(t, "elevation[0]", elevation[0], 200)

	descent, err := db.GetEddingtonData(context.Background(), userID, "descent", "", nil)
	if err != nil {
		t.Fatalf("GetEddingtonData descent: %v", err)
	}
	if len(descent) != 1 {
		t.Fatalf("descent len = %d, want 1", len(descent))
	}
	assertFloat64(t, "descent[0]", descent[0], 280)
}

func assertLifetimeStats(t *testing.T, got, want database.LifetimeStats) {
	t.Helper()
	if got.TotalActivities != want.TotalActivities {
		t.Errorf("TotalActivities = %d, want %d", got.TotalActivities, want.TotalActivities)
	}
	if got.TotalMovingTimeS != want.TotalMovingTimeS {
		t.Errorf("TotalMovingTimeS = %d, want %d", got.TotalMovingTimeS, want.TotalMovingTimeS)
	}
	if got.TotalElapsedTimeS != want.TotalElapsedTimeS {
		t.Errorf("TotalElapsedTimeS = %d, want %d", got.TotalElapsedTimeS, want.TotalElapsedTimeS)
	}
	if got.TotalCalories != want.TotalCalories {
		t.Errorf("TotalCalories = %d, want %d", got.TotalCalories, want.TotalCalories)
	}
	if got.ActiveDays != want.ActiveDays {
		t.Errorf("ActiveDays = %d, want %d", got.ActiveDays, want.ActiveDays)
	}
	if got.LiftRideCount != want.LiftRideCount {
		t.Errorf("LiftRideCount = %d, want %d", got.LiftRideCount, want.LiftRideCount)
	}
	assertFloat64(t, "TotalDistanceM", got.TotalDistanceM, want.TotalDistanceM)
	assertFloat64(t, "TotalElevationM", got.TotalElevationM, want.TotalElevationM)
	assertFloat64(t, "TotalElevationLossM", got.TotalElevationLossM, want.TotalElevationLossM)
	assertFloat64(t, "LiftAscentM", got.LiftAscentM, want.LiftAscentM)
	assertFloat64(t, "LiftDescentM", got.LiftDescentM, want.LiftDescentM)
	assertFloat64(t, "LiftDistanceM", got.LiftDistanceM, want.LiftDistanceM)
}

func assertFloat64(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.001 {
		t.Errorf("%s = %.3f, want %.3f", name, got, want)
	}
}

func assertFloatSlice(t *testing.T, name string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s len = %d, want %d (%v)", name, len(got), len(want), got)
	}
	for i := range got {
		assertFloat64(t, name, got[i], want[i])
	}
}
