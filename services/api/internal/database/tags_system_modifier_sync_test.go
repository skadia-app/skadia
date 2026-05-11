package database

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// tagSystemModifierSyncTestDB sets up an isolated user (system tags seeded)
// + a child user the per-test workload can use as a "different owner". Both
// users get cleaned up via t.Cleanup so tests stay independent across runs.
func tagSystemModifierSyncTestDB(t *testing.T) (*DB, string, string) {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	db := &DB{Pool: pool}
	primary := seedSyncTestUser(t, db, "primary")
	other := seedSyncTestUser(t, db, "other")
	return db, primary, other
}

func seedSyncTestUser(t *testing.T, db *DB, label string) string {
	t.Helper()
	ctx := context.Background()
	userID := uuid.NewString()
	suffix := userID[:8]
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')`,
		userID, "tagsync-"+label+"-"+suffix, "tagsync-"+label+"-"+suffix+"@example.com"); err != nil {
		t.Fatalf("insert user %s: %v", label, err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.Pool.Exec(bg, `DELETE FROM activity_tag_removals WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)`, userID)
		_, _ = db.Pool.Exec(bg, `DELETE FROM activity_tags WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)`, userID)
		_, _ = db.Pool.Exec(bg, `DELETE FROM activities WHERE user_id = $1`, userID)
		_, _ = db.Pool.Exec(bg, `UPDATE tags SET is_system = false WHERE user_id = $1`, userID)
		_, _ = db.Pool.Exec(bg, `DELETE FROM tags WHERE user_id = $1`, userID)
		_, _ = db.Pool.Exec(bg, `DELETE FROM users WHERE id = $1`, userID)
	})
	if err := db.SeedSystemDefaults(ctx, userID); err != nil {
		t.Fatalf("SeedSystemDefaults %s: %v", label, err)
	}
	seedSystemModifierTagsForTest(t, db, userID)
	return userID
}

func seedSystemModifierTagsForTest(t *testing.T, db *DB, userID string) {
	t.Helper()
	ctx := context.Background()
	for _, sync := range systemModifierTagSyncs() {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description)
			VALUES ($1, $2, $3, '#FFFFFF', 'system', true, false, $4)
			ON CONFLICT (user_id, name) DO NOTHING`,
			userID, sync.SystemTagName, sync.Color, sync.Description); err != nil {
			t.Fatalf("seed system modifier tag %s: %v", sync.SystemTagName, err)
		}
	}
}

func insertSyncTestActivity(t *testing.T, db *DB, userID, sport string) string {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	act, err := db.InsertActivity(context.Background(), userID, &model.ActivityData{
		SportType: sport,
		StartedAt: now,
		EndedAt:   now.Add(15 * time.Minute),
		FileHash:  uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("insert activity %s/%s: %v", userID, sport, err)
	}
	return act.ID
}

func mustSystemTagID(t *testing.T, db *DB, userID, name string) string {
	t.Helper()
	id, err := db.GetSystemTagID(context.Background(), userID, name)
	if err != nil {
		t.Fatalf("get system tag %s: %v", name, err)
	}
	return id
}

type activityTagState struct {
	IsIndoor   bool
	IsCommute  bool
	IsVirtual  bool
	IsElectric bool
	Tags       []string
}

func loadActivityTagState(t *testing.T, db *DB, activityID string) activityTagState {
	t.Helper()
	var st activityTagState
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT COALESCE(is_indoor, false), COALESCE(is_commute, false),
		        COALESCE(is_virtual, false), COALESCE(is_electric, false),
		        COALESCE(tags, '{}')
		 FROM activities WHERE id = $1`, activityID,
	).Scan(&st.IsIndoor, &st.IsCommute, &st.IsVirtual, &st.IsElectric, &st.Tags); err != nil {
		t.Fatalf("load activity state: %v", err)
	}
	sort.Strings(st.Tags)
	return st
}

func hasJunctionRow(t *testing.T, db *DB, activityID, tagID string) bool {
	t.Helper()
	var exists bool
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM activity_tags WHERE activity_id = $1 AND tag_id = $2)`,
		activityID, tagID,
	).Scan(&exists); err != nil {
		t.Fatalf("hasJunctionRow: %v", err)
	}
	return exists
}

func tagActivityCount(t *testing.T, db *DB, tagID string) int {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT activity_count FROM tags WHERE id = $1`, tagID,
	).Scan(&count); err != nil {
		t.Fatalf("tagActivityCount: %v", err)
	}
	return count
}

func TestAssignActivitiesToTagSystemModifierIndoor(t *testing.T) {
	db, userID, _ := tagSystemModifierSyncTestDB(t)
	ctx := context.Background()
	activityID := insertSyncTestActivity(t, db, userID, "running")
	indoorTagID := mustSystemTagID(t, db, userID, "Indoor")

	accepted, err := db.AssignActivitiesToTag(ctx, indoorTagID, userID, []string{activityID})
	if err != nil {
		t.Fatalf("AssignActivitiesToTag: %v", err)
	}
	if len(accepted) != 1 || accepted[0] != activityID {
		t.Fatalf("accepted = %v, want [%s]", accepted, activityID)
	}

	st := loadActivityTagState(t, db, activityID)
	if !st.IsIndoor {
		t.Fatalf("is_indoor = false, want true")
	}
	if !containsString(st.Tags, "Indoor") {
		t.Fatalf("tags = %v, want to include Indoor", st.Tags)
	}
	if !hasJunctionRow(t, db, activityID, indoorTagID) {
		t.Fatalf("activity_tags missing Indoor junction row")
	}
	if got := tagActivityCount(t, db, indoorTagID); got != 1 {
		t.Fatalf("activity_count = %d, want 1", got)
	}
}

func TestAssignActivitiesToTagSystemModifierTagIndoorUnassignClearsState(t *testing.T) {
	db, userID, _ := tagSystemModifierSyncTestDB(t)
	ctx := context.Background()
	activityID := insertSyncTestActivity(t, db, userID, "running")
	indoorTagID := mustSystemTagID(t, db, userID, "Indoor")

	if _, err := db.AssignActivitiesToTag(ctx, indoorTagID, userID, []string{activityID}); err != nil {
		t.Fatalf("seed assign: %v", err)
	}

	accepted, err := db.UnassignActivitiesFromTag(ctx, indoorTagID, userID, []string{activityID})
	if err != nil {
		t.Fatalf("UnassignActivitiesFromTag: %v", err)
	}
	if len(accepted) != 1 {
		t.Fatalf("accepted = %v, want 1 element", accepted)
	}

	st := loadActivityTagState(t, db, activityID)
	if st.IsIndoor {
		t.Fatalf("is_indoor = true, want false after unassign")
	}
	if containsString(st.Tags, "Indoor") {
		t.Fatalf("tags = %v, must not contain Indoor", st.Tags)
	}
	if hasJunctionRow(t, db, activityID, indoorTagID) {
		t.Fatalf("activity_tags still has Indoor junction row")
	}
	if got := tagActivityCount(t, db, indoorTagID); got != 0 {
		t.Fatalf("activity_count = %d, want 0", got)
	}
}

func TestAssignActivitiesToTagSystemModifierTagVirtuellRoundTrip(t *testing.T) {
	db, userID, _ := tagSystemModifierSyncTestDB(t)
	ctx := context.Background()
	// Cycling is fine; virtuell is sport-agnostic.
	activityID := insertSyncTestActivity(t, db, userID, "cycling")
	virtualTagID := mustSystemTagID(t, db, userID, "virtuell")

	if _, err := db.AssignActivitiesToTag(ctx, virtualTagID, userID, []string{activityID}); err != nil {
		t.Fatalf("assign virtuell: %v", err)
	}
	st := loadActivityTagState(t, db, activityID)
	if !st.IsVirtual {
		t.Fatalf("is_virtual = false, want true")
	}
	if !containsString(st.Tags, "virtuell") {
		t.Fatalf("tags = %v, want to include virtuell", st.Tags)
	}
	if !hasJunctionRow(t, db, activityID, virtualTagID) {
		t.Fatalf("activity_tags missing virtuell junction row after assign")
	}

	// Unassign — must repair without tripping the prevent_virtual_tag_removal trigger.
	if _, err := db.UnassignActivitiesFromTag(ctx, virtualTagID, userID, []string{activityID}); err != nil {
		t.Fatalf("unassign virtuell: %v", err)
	}
	st = loadActivityTagState(t, db, activityID)
	if st.IsVirtual {
		t.Fatalf("is_virtual = true, want false after unassign")
	}
	if containsString(st.Tags, "virtuell") {
		t.Fatalf("tags = %v, must not contain virtuell after unassign", st.Tags)
	}
	if hasJunctionRow(t, db, activityID, virtualTagID) {
		t.Fatalf("activity_tags still has virtuell junction row")
	}
}

func TestAssignActivitiesToTagSystemModifierEBikeOnCycling(t *testing.T) {
	db, userID, _ := tagSystemModifierSyncTestDB(t)
	ctx := context.Background()
	activityID := insertSyncTestActivity(t, db, userID, "cycling")
	ebikeTagID := mustSystemTagID(t, db, userID, "E-Bike")

	accepted, err := db.AssignActivitiesToTag(ctx, ebikeTagID, userID, []string{activityID})
	if err != nil {
		t.Fatalf("AssignActivitiesToTag: %v", err)
	}
	if len(accepted) != 1 || accepted[0] != activityID {
		t.Fatalf("accepted = %v, want [%s]", accepted, activityID)
	}
	st := loadActivityTagState(t, db, activityID)
	if !st.IsElectric {
		t.Fatalf("is_electric = false, want true on cycling activity")
	}
	if !containsString(st.Tags, "E-Bike") {
		t.Fatalf("tags = %v, want to include E-Bike", st.Tags)
	}
	if !hasJunctionRow(t, db, activityID, ebikeTagID) {
		t.Fatalf("activity_tags missing E-Bike junction row on cycling activity")
	}
}

func TestAssignActivitiesToTagSystemModifierEBikeOnRunningRejected(t *testing.T) {
	db, userID, _ := tagSystemModifierSyncTestDB(t)
	ctx := context.Background()
	runningID := insertSyncTestActivity(t, db, userID, "running")
	ebikeTagID := mustSystemTagID(t, db, userID, "E-Bike")

	accepted, err := db.AssignActivitiesToTag(ctx, ebikeTagID, userID, []string{runningID})
	if err != nil {
		t.Fatalf("AssignActivitiesToTag: %v", err)
	}
	if len(accepted) != 0 {
		t.Fatalf("accepted = %v, want empty (cycling-only filter)", accepted)
	}
	st := loadActivityTagState(t, db, runningID)
	if st.IsElectric {
		t.Fatalf("is_electric = true, want false on running activity")
	}
	if containsString(st.Tags, "E-Bike") {
		t.Fatalf("tags = %v, must not contain E-Bike on running activity", st.Tags)
	}
	if hasJunctionRow(t, db, runningID, ebikeTagID) {
		t.Fatalf("activity_tags has E-Bike junction row on running activity")
	}
}

func TestAssignActivitiesToTagSystemModifierEBikeMixedSports(t *testing.T) {
	db, userID, _ := tagSystemModifierSyncTestDB(t)
	ctx := context.Background()
	cyclingID := insertSyncTestActivity(t, db, userID, "cycling")
	runningID := insertSyncTestActivity(t, db, userID, "running")
	ebikeTagID := mustSystemTagID(t, db, userID, "E-Bike")

	accepted, err := db.AssignActivitiesToTag(ctx, ebikeTagID, userID, []string{cyclingID, runningID})
	if err != nil {
		t.Fatalf("AssignActivitiesToTag: %v", err)
	}
	if len(accepted) != 1 || accepted[0] != cyclingID {
		t.Fatalf("accepted = %v, want [%s] (only cycling)", accepted, cyclingID)
	}
	if st := loadActivityTagState(t, db, cyclingID); !st.IsElectric {
		t.Fatalf("cycling is_electric = false, want true")
	}
	if !hasJunctionRow(t, db, cyclingID, ebikeTagID) {
		t.Fatalf("activity_tags missing E-Bike junction row for cycling activity in mixed batch")
	}
	if st := loadActivityTagState(t, db, runningID); st.IsElectric {
		t.Fatalf("running is_electric = true, want false")
	}
	if hasJunctionRow(t, db, runningID, ebikeTagID) {
		t.Fatalf("activity_tags has E-Bike junction row for running activity in mixed batch")
	}
}

func TestAssignActivitiesToTagSystemModifierCrossUserSameNameTag(t *testing.T) {
	// "other" user has their own manual "Indoor" tag that is_system=false.
	// When primary calls assign with a tagID that does not belong to primary,
	// the function must reject the lookup outright (tag not found for primary)
	// and never touch other's activity.
	db, primaryID, otherID := tagSystemModifierSyncTestDB(t)
	ctx := context.Background()

	// Other-user activity to verify it is never touched.
	otherActivityID := insertSyncTestActivity(t, db, otherID, "running")
	// Other-user has system Indoor seeded, but pretend they renamed it to a
	// non-system manual tag for the sake of the test scenario:
	if _, err := db.Pool.Exec(ctx, `
		UPDATE tags SET is_system = false, category = 'custom'
		WHERE user_id = $1 AND name = 'Indoor'`, otherID); err != nil {
		t.Fatalf("demote other user Indoor: %v", err)
	}

	// Primary's Indoor tag (system).
	primaryIndoorID := mustSystemTagID(t, db, primaryID, "Indoor")

	// Primary tries to assign with an other-user activity — must be filtered out.
	accepted, err := db.AssignActivitiesToTag(ctx, primaryIndoorID, primaryID, []string{otherActivityID})
	if err != nil {
		t.Fatalf("AssignActivitiesToTag: %v", err)
	}
	if len(accepted) != 0 {
		t.Fatalf("accepted = %v, want empty (cross-user activity)", accepted)
	}
	if st := loadActivityTagState(t, db, otherActivityID); st.IsIndoor {
		t.Fatalf("other-user activity is_indoor = true, must remain false")
	}
}

func TestAssignActivitiesToTagSystemModifierRejectsCrossUserTagID(t *testing.T) {
	// Primary calls AssignActivitiesToTag with another user's tag id and a
	// primary-owned activity. resolveOwnedTagForSync filters by id+user_id,
	// so the lookup must return an error and primary's activity must stay
	// untouched — no junction row, no modifier flag flip, no array merge.
	db, primaryID, otherID := tagSystemModifierSyncTestDB(t)
	ctx := context.Background()

	primaryActivityID := insertSyncTestActivity(t, db, primaryID, "running")
	otherIndoorID := mustSystemTagID(t, db, otherID, "Indoor")

	accepted, err := db.AssignActivitiesToTag(ctx, otherIndoorID, primaryID, []string{primaryActivityID})
	if err == nil {
		t.Fatalf("AssignActivitiesToTag with cross-user tagID returned no error; accepted=%v", accepted)
	}
	if len(accepted) != 0 {
		t.Fatalf("accepted = %v on cross-user tagID, want empty", accepted)
	}
	if st := loadActivityTagState(t, db, primaryActivityID); st.IsIndoor {
		t.Fatalf("primary activity is_indoor = true after cross-user tagID assign")
	}
	if hasJunctionRow(t, db, primaryActivityID, otherIndoorID) {
		t.Fatalf("activity_tags links primary activity to other user's tag id")
	}
}

func TestAssignActivitiesToTagSilentlyDropsCrossUserActivityIDs(t *testing.T) {
	db, primaryID, otherID := tagSystemModifierSyncTestDB(t)
	ctx := context.Background()

	primaryActivityID := insertSyncTestActivity(t, db, primaryID, "running")
	otherActivityID := insertSyncTestActivity(t, db, otherID, "running")
	indoorTagID := mustSystemTagID(t, db, primaryID, "Indoor")

	accepted, err := db.AssignActivitiesToTag(ctx, indoorTagID, primaryID, []string{primaryActivityID, otherActivityID})
	if err != nil {
		t.Fatalf("AssignActivitiesToTag: %v", err)
	}
	if len(accepted) != 1 || accepted[0] != primaryActivityID {
		t.Fatalf("accepted = %v, want [%s]", accepted, primaryActivityID)
	}
	if st := loadActivityTagState(t, db, otherActivityID); st.IsIndoor {
		t.Fatalf("other-user activity is_indoor = true, must remain false")
	}
	// Verify no junction row was written referencing primary's tagID + other's activity.
	if hasJunctionRow(t, db, otherActivityID, indoorTagID) {
		t.Fatalf("activity_tags contains primary tag for other user's activity")
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestUnassignActivitiesFromTagEBikeSkipsNonCyclingActivities(t *testing.T) {
	// Setup: cycling A1 with is_electric=true, running A2 with is_electric=false.
	// Unassign E-Bike with [A1, A2]. Only A1 should be in accepted; A2 must be
	// untouched (no modifier change, no junction row, no removal row).
	db, userID, _ := tagSystemModifierSyncTestDB(t)
	ctx := context.Background()

	cyclingID := insertSyncTestActivity(t, db, userID, "cycling")
	runningID := insertSyncTestActivity(t, db, userID, "running")
	ebikeTagID := mustSystemTagID(t, db, userID, "E-Bike")

	// Assign E-Bike to the cycling activity first so there is state to unassign.
	if _, err := db.AssignActivitiesToTag(ctx, ebikeTagID, userID, []string{cyclingID}); err != nil {
		t.Fatalf("seed assign cycling: %v", err)
	}
	if st := loadActivityTagState(t, db, cyclingID); !st.IsElectric {
		t.Fatalf("cycling is_electric = false after seed assign, want true")
	}

	accepted, err := db.UnassignActivitiesFromTag(ctx, ebikeTagID, userID, []string{cyclingID, runningID})
	if err != nil {
		t.Fatalf("UnassignActivitiesFromTag: %v", err)
	}

	// Only the cycling activity should be in accepted.
	if len(accepted) != 1 || accepted[0] != cyclingID {
		t.Fatalf("accepted = %v, want [%s] (cycling only)", accepted, cyclingID)
	}

	// Cycling activity must have is_electric cleared.
	if st := loadActivityTagState(t, db, cyclingID); st.IsElectric {
		t.Fatalf("cycling is_electric = true after unassign, want false")
	}

	// Running activity must be completely untouched.
	if st := loadActivityTagState(t, db, runningID); st.IsElectric {
		t.Fatalf("running is_electric = true, must remain false (never had E-Bike)")
	}
	if hasJunctionRow(t, db, runningID, ebikeTagID) {
		t.Fatalf("running activity has E-Bike junction row, must have none")
	}
}
