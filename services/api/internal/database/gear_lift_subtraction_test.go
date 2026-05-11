package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
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
		INSERT INTO gear (id, user_id, name, type, scope)
		VALUES ($1, $2, 'Lift Net Test Gear', 'bike', 'bike')`,
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
	return fx.insertActivityForGear(t, fx.gearID, distanceM, elapsedS, ascentM, descentM, liftDistanceM, liftAscentM, liftDescentM)
}

func (fx gearLiftFixture) insertActivityForGear(t *testing.T, gearID string, distanceM, elapsedS, ascentM, descentM float64, liftDistanceM, liftAscentM, liftDescentM *float64) string {
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
		fx.userID, activityID, gearID); err != nil {
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

func TestGearLiftSubtraction_SummaryUsesNetLiftTotals(t *testing.T) {
	fx := newGearLiftFixture(t)

	fx.insertActivity(t, 10000, 1800, 900, 600, ptrFloat64(2000), ptrFloat64(250), ptrFloat64(120))

	summary, err := fx.db.GetGearSummary(context.Background(), fx.userID, GearSummaryFilter{})
	if err != nil {
		t.Fatalf("GetGearSummary: %v", err)
	}
	if summary.TotalDistanceM != 8000 {
		t.Fatalf("summary total_distance_m = %.0f, want 8000", summary.TotalDistanceM)
	}
	if summary.TotalElevationGainM != 650 {
		t.Fatalf("summary total_elevation_gain_m = %.0f, want 650", summary.TotalElevationGainM)
	}
	if summary.TotalElevationLossM != 480 {
		t.Fatalf("summary total_elevation_loss_m = %.0f, want 480", summary.TotalElevationLossM)
	}
}

func TestGearLiftSubtraction_MergeGearUsesNetLiftTotals(t *testing.T) {
	fx := newGearLiftFixture(t)
	ctx := context.Background()

	secondaryID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO gear (id, user_id, name, type, scope)
		VALUES ($1, $2, 'Lift Merge Secondary', 'bike', 'bike')`,
		secondaryID, fx.userID); err != nil {
		t.Fatalf("insert secondary gear: %v", err)
	}
	fx.insertActivityForGear(t, secondaryID, 10000, 1800, 900, 600, ptrFloat64(2000), ptrFloat64(250), ptrFloat64(120))

	if err := fx.db.MergeGear(ctx, fx.userID, fx.gearID, secondaryID); err != nil {
		t.Fatalf("MergeGear: %v", err)
	}

	var totalDistance, totalElevationGain, totalElevationLoss float64
	if err := fx.pool.QueryRow(ctx, `
		SELECT total_distance_m, total_elevation_gain_m, total_elevation_loss_m
		FROM gear WHERE id = $1`, fx.gearID).Scan(&totalDistance, &totalElevationGain, &totalElevationLoss); err != nil {
		t.Fatalf("query primary gear totals: %v", err)
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

func TestGearLiftSubtraction_UpdateActivityExcludeRefreshesGearTotals(t *testing.T) {
	fx := newGearLiftFixture(t)
	ctx := context.Background()
	activityID := fx.insertActivity(t, 10000, 1800, 900, 600, ptrFloat64(2000), ptrFloat64(250), ptrFloat64(120))

	if err := fx.db.UpdateGearStatsOwnerVerified(ctx, fx.gearID); err != nil {
		t.Fatalf("initial UpdateGearStatsOwnerVerified: %v", err)
	}

	exclude := true
	if _, err := fx.db.UpdateActivity(ctx, activityID, fx.userID, &model.ActivityUpdate{ExcludeLiftSubtraction: &exclude}); err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}

	var totalDistance, totalElevationGain, totalElevationLoss float64
	if err := fx.pool.QueryRow(ctx, `
		SELECT total_distance_m, total_elevation_gain_m, total_elevation_loss_m
		FROM gear WHERE id = $1`, fx.gearID).Scan(&totalDistance, &totalElevationGain, &totalElevationLoss); err != nil {
		t.Fatalf("query gear totals: %v", err)
	}
	if totalDistance != 10000 {
		t.Fatalf("total_distance_m = %.0f, want 10000", totalDistance)
	}
	if totalElevationGain != 900 {
		t.Fatalf("total_elevation_gain_m = %.0f, want 900", totalElevationGain)
	}
	if totalElevationLoss != 600 {
		t.Fatalf("total_elevation_loss_m = %.0f, want 600", totalElevationLoss)
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

func TestRecalculateActivityLifts_RefreshesGearTotalsAfterReset(t *testing.T) {
	fx := newGearLiftFixture(t)
	ctx := context.Background()
	activityID := fx.insertActivity(t, 10000, 1800, 900, 600, ptrFloat64(2000), ptrFloat64(250), ptrFloat64(120))

	if err := fx.db.UpdateGearStatsOwnerVerified(ctx, fx.gearID); err != nil {
		t.Fatalf("initial UpdateGearStatsOwnerVerified: %v", err)
	}

	if err := fx.db.RecalculateActivityLifts(ctx, fx.userID, activityID); err != nil {
		t.Fatalf("RecalculateActivityLifts: %v", err)
	}

	var liftDistance, totalDistance, totalElevationGain, totalElevationLoss float64
	if err := fx.pool.QueryRow(ctx, `
		SELECT COALESCE(a.lift_distance_m, 0), g.total_distance_m,
		       g.total_elevation_gain_m, g.total_elevation_loss_m
		FROM activities a
		JOIN activity_gear_assignments aga ON aga.activity_id = a.id
		JOIN gear g ON g.id = aga.gear_id
		WHERE a.id = $1`, activityID).Scan(
		&liftDistance, &totalDistance, &totalElevationGain, &totalElevationLoss,
	); err != nil {
		t.Fatalf("query activity and gear totals: %v", err)
	}
	if liftDistance != 0 {
		t.Fatalf("lift_distance_m = %.0f, want 0", liftDistance)
	}
	if totalDistance != 10000 {
		t.Fatalf("total_distance_m = %.0f, want refreshed gross 10000", totalDistance)
	}
	if totalElevationGain != 900 {
		t.Fatalf("total_elevation_gain_m = %.0f, want refreshed gross 900", totalElevationGain)
	}
	if totalElevationLoss != 600 {
		t.Fatalf("total_elevation_loss_m = %.0f, want refreshed gross 600", totalElevationLoss)
	}
}

func TestResetSoftRejectedLiftCandidates_RecomputesAggregatesAndGearTotals(t *testing.T) {
	fx := newGearLiftFixture(t)
	ctx := context.Background()
	activityID := fx.insertActivity(t, 10000, 1800, 900, 600, nil, nil, nil)
	insertRejectedConfirmedLiftCandidate(t, fx.pool, activityID, 2000, 250, 120)

	if err := fx.db.RecomputeLiftStats(ctx, fx.userID, activityID); err != nil {
		t.Fatalf("initial RecomputeLiftStats: %v", err)
	}

	cleared, activities, err := fx.db.ResetSoftRejectedLiftCandidates(ctx, fx.userID)
	if err != nil {
		t.Fatalf("ResetSoftRejectedLiftCandidates: %v", err)
	}
	if cleared != 1 {
		t.Fatalf("cleared=%d, want 1", cleared)
	}
	if activities != 1 {
		t.Fatalf("activities=%d, want 1", activities)
	}

	var rejected bool
	var liftDistance, totalDistance, totalElevationGain, totalElevationLoss float64
	if err := fx.pool.QueryRow(ctx, `
		SELECT alc.rejected_by_user, COALESCE(a.lift_distance_m, 0),
		       g.total_distance_m, g.total_elevation_gain_m, g.total_elevation_loss_m
		FROM activity_lift_candidates alc
		JOIN activities a ON a.id = alc.activity_id
		JOIN activity_gear_assignments aga ON aga.activity_id = a.id
		JOIN gear g ON g.id = aga.gear_id
		WHERE a.id = $1`, activityID).Scan(
		&rejected, &liftDistance, &totalDistance, &totalElevationGain, &totalElevationLoss,
	); err != nil {
		t.Fatalf("query reset state: %v", err)
	}
	if rejected {
		t.Fatal("rejected_by_user = true after reset, want false")
	}
	if liftDistance != 2000 {
		t.Fatalf("lift_distance_m = %.0f, want 2000", liftDistance)
	}
	if totalDistance != 8000 {
		t.Fatalf("total_distance_m = %.0f, want net 8000", totalDistance)
	}
	if totalElevationGain != 650 {
		t.Fatalf("total_elevation_gain_m = %.0f, want net 650", totalElevationGain)
	}
	if totalElevationLoss != 480 {
		t.Fatalf("total_elevation_loss_m = %.0f, want net 480", totalElevationLoss)
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

func insertRejectedConfirmedLiftCandidate(t *testing.T, pool *pgxpool.Pool, activityID string, distanceM, ascentM, descentM float64) {
	t.Helper()
	ctx := context.Background()
	startedAt := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `
			INSERT INTO activity_lift_candidates
				(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
				 confidence, confirmed, source, rejected_by_user)
			VALUES ($1, $2, $3, 300, $4, $5, $6, 'high', true, 'heuristic', true)`,
		activityID, startedAt, startedAt.Add(5*time.Minute), ascentM, descentM, distanceM); err != nil {
		t.Fatalf("insert rejected lift candidate: %v", err)
	}
}
