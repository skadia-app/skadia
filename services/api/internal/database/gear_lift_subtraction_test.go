package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

type gearLiftFixture struct {
	db     *DB
	pool   *pgxpool.Pool
	userID string
	gearID string
}

func newGearLiftFixture(t *testing.T) gearLiftFixture {
	t.Helper()
	ctx := context.Background()
	dsn := testdb.RequireSafeDSN(t)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')`,
		userID, "gear-lift-"+userID[:8], "gear-lift-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	gearID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO gear (id, user_id, name, type)
		VALUES ($1, $2, 'Lift Net Test Gear', 'bike')`,
		gearID, userID); err != nil {
		t.Fatalf("insert gear: %v", err)
	}

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id = $1`, userID)
	})

	return gearLiftFixture{db: &DB{Pool: pool}, pool: pool, userID: userID, gearID: gearID}
}

func (fx gearLiftFixture) insertActivity(t *testing.T, distanceM, elapsedS, ascentM, descentM float64, liftDistanceM, liftAscentM, liftDescentM *float64) string {
	t.Helper()
	ctx := context.Background()
	activityID := uuid.NewString()
	startedAt := time.Now().UTC().Truncate(time.Second)
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activities
			(id, user_id, sport_type, started_at, ended_at, file_hash,
			 distance_m, elapsed_time_s, elevation_gain_m, elevation_loss_m,
			 lift_distance_m, lift_ascent_m, lift_descent_m,
			 exclude_lift_subtraction, is_group_secondary)
		VALUES ($1, $2, 'cycling', $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, false, false)`,
		activityID, fx.userID, startedAt, startedAt.Add(time.Duration(elapsedS)*time.Second),
		uuid.NewString(), distanceM, int(elapsedS), ascentM, descentM, liftDistanceM, liftAscentM, liftDescentM); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_gear_assignments (user_id, activity_id, gear_id, role_key, source)
		VALUES ($1, $2, $3, 'primary', 'user')`,
		fx.userID, activityID, fx.gearID); err != nil {
		t.Fatalf("insert activity gear assignment: %v", err)
	}
	return activityID
}

func ptrFloat64(v float64) *float64 {
	return &v
}

func TestGearLiftSubtraction_UpdateGearStatsNetsLiftTotals(t *testing.T) {
	fx := newGearLiftFixture(t)
	ctx := context.Background()

	fx.insertActivity(t, 20000, 3600, 800, 1200, ptrFloat64(4000), ptrFloat64(600), ptrFloat64(350))
	fx.insertActivity(t, 15000, 2700, 500, 700, nil, nil, nil)

	if err := fx.db.UpdateGearStatsOwnerVerified(ctx, fx.gearID); err != nil {
		t.Fatalf("UpdateGearStatsOwnerVerified: %v", err)
	}

	var totalDistance, totalElevationGain, totalElevationLoss float64
	var totalTime int64
	var totalActivities int
	if err := fx.pool.QueryRow(ctx, `
		SELECT total_distance_m, total_time_s, total_elevation_gain_m, total_elevation_loss_m, total_activities
		FROM gear WHERE id = $1`, fx.gearID).Scan(
		&totalDistance, &totalTime, &totalElevationGain, &totalElevationLoss, &totalActivities,
	); err != nil {
		t.Fatalf("query gear totals: %v", err)
	}

	if totalDistance != 31000 {
		t.Fatalf("total_distance_m = %.0f, want 31000", totalDistance)
	}
	if totalElevationGain != 700 {
		t.Fatalf("total_elevation_gain_m = %.0f, want 700", totalElevationGain)
	}
	if totalElevationLoss != 1550 {
		t.Fatalf("total_elevation_loss_m = %.0f, want 1550", totalElevationLoss)
	}
	if totalTime != 6300 {
		t.Fatalf("total_time_s = %d, want 6300", totalTime)
	}
	if totalActivities != 2 {
		t.Fatalf("total_activities = %d, want 2", totalActivities)
	}
}

func TestRecomputeLiftStats_AutoRefreshesGearTotals(t *testing.T) {
	fx := newGearLiftFixture(t)
	ctx := context.Background()
	activityID := fx.insertActivity(t, 10000, 1800, 900, 600, nil, nil, nil)

	if err := fx.db.UpdateGearStatsOwnerVerified(ctx, fx.gearID); err != nil {
		t.Fatalf("initial UpdateGearStatsOwnerVerified: %v", err)
	}
	insertConfirmedLiftCandidate(t, fx.pool, activityID, 2000, 250, 120)

	if err := fx.db.recomputeLiftStats(ctx, fx.userID, activityID); err != nil {
		t.Fatalf("recomputeLiftStats: %v", err)
	}

	var totalDistance, totalElevationGain, totalElevationLoss float64
	if err := fx.pool.QueryRow(ctx, `
		SELECT total_distance_m, total_elevation_gain_m, total_elevation_loss_m
		FROM gear WHERE id = $1`, fx.gearID).Scan(&totalDistance, &totalElevationGain, &totalElevationLoss); err != nil {
		t.Fatalf("query gear totals: %v", err)
	}
	if totalDistance != 8000 {
		t.Fatalf("total_distance_m = %.0f, want 8000", totalDistance)
	}
	if totalElevationGain != 650 {
		t.Fatalf("total_elevation_gain_m = %.0f, want 650", totalElevationGain)
	}
	if totalElevationLoss != 480 {
		t.Fatalf("total_elevation_loss_m = %.0f, want 480", totalElevationLoss)
	}
}

func TestRecomputeLiftStats_SkipGearRefreshLeavesGearTotalsUntouched(t *testing.T) {
	fx := newGearLiftFixture(t)
	ctx := context.Background()
	activityID := fx.insertActivity(t, 10000, 1800, 900, 600, nil, nil, nil)

	if err := fx.db.UpdateGearStatsOwnerVerified(ctx, fx.gearID); err != nil {
		t.Fatalf("initial UpdateGearStatsOwnerVerified: %v", err)
	}
	insertConfirmedLiftCandidate(t, fx.pool, activityID, 2000, 250, 120)

	if err := fx.db.recomputeLiftStats(ctx, fx.userID, activityID, skipGearRefresh()); err != nil {
		t.Fatalf("recomputeLiftStats: %v", err)
	}

	var liftDistance, totalDistance float64
	if err := fx.pool.QueryRow(ctx, `
		SELECT a.lift_distance_m, g.total_distance_m
		FROM activities a
		JOIN activity_gear_assignments aga ON aga.activity_id = a.id AND aga.role_key = 'primary'
		JOIN gear g ON g.id = aga.gear_id
		WHERE a.id = $1`, activityID).Scan(&liftDistance, &totalDistance); err != nil {
		t.Fatalf("query lift and gear totals: %v", err)
	}
	if liftDistance != 2000 {
		t.Fatalf("lift_distance_m = %.0f, want 2000", liftDistance)
	}
	if totalDistance != 10000 {
		t.Fatalf("total_distance_m = %.0f, want unchanged raw 10000", totalDistance)
	}
}

func TestRecomputeLiftStats_RejectsForeignActivity(t *testing.T) {
	fx := newGearLiftFixture(t)
	ctx := context.Background()
	foreignUserID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')`,
		foreignUserID, "gear-lift-foreign-"+foreignUserID[:8], "gear-lift-foreign-"+foreignUserID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert foreign user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = fx.pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, foreignUserID)
	})
	foreignActivityID := uuid.NewString()
	startedAt := time.Now().UTC().Truncate(time.Second)
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash, distance_m, elevation_gain_m)
		VALUES ($1, $2, 'cycling', $3, $4, $5, 10000, 900)`,
		foreignActivityID, foreignUserID, startedAt, startedAt.Add(30*time.Minute), uuid.NewString()); err != nil {
		t.Fatalf("insert foreign activity: %v", err)
	}
	insertConfirmedLiftCandidate(t, fx.pool, foreignActivityID, 2000, 250, 120)

	if err := fx.db.recomputeLiftStats(ctx, fx.userID, foreignActivityID); err != nil {
		t.Fatalf("recomputeLiftStats: %v", err)
	}

	var liftCount int
	var liftDistanceIsNull bool
	if err := fx.pool.QueryRow(ctx, `
		SELECT COALESCE(lift_count, 0), lift_distance_m IS NULL
		FROM activities WHERE id = $1`, foreignActivityID).Scan(&liftCount, &liftDistanceIsNull); err != nil {
		t.Fatalf("query foreign activity: %v", err)
	}
	if liftCount != 0 {
		t.Fatalf("foreign lift_count = %d, want 0", liftCount)
	}
	if !liftDistanceIsNull {
		t.Fatal("foreign lift_distance_m is not nil")
	}
}

func insertConfirmedLiftCandidate(t *testing.T, pool *pgxpool.Pool, activityID string, distanceM, ascentM, descentM float64) {
	t.Helper()
	ctx := context.Background()
	startedAt := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `
			INSERT INTO activity_lift_candidates
				(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
				 confidence, confirmed, source)
			VALUES ($1, $2, $3, 300, $4, $5, $6, 'high', true, 'heuristic')`,
		activityID, startedAt, startedAt.Add(5*time.Minute), ascentM, descentM, distanceM); err != nil {
		t.Fatalf("insert lift candidate: %v", err)
	}
}
