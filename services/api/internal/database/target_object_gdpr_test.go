package database

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func TestGDPRDeletesPrivateTargetObjectsAndCollections(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)
	otherID := insertObjectAccessTestUser(t, db)
	scope := createTargetObjectAccessScope(t, db, userID, viewerID)

	place := createTargetPlaceFixture(t, db, userID, "gdpr place", "private", nil, `[8,49]`)
	region := createTargetRegionFixture(t, db, userID, "gdpr region", "private", nil, `[[[8,49],[8.01,49],[8.01,49.01],[8,49.01],[8,49]]]`)
	collection := createTargetCollectionFixture(t, db, userID, "gdpr collection", "private", nil)
	addTargetCollectionMemberFixture(t, db, userID, collection.ID, "place", place.ID, "poi")
	addTargetCollectionMemberFixture(t, db, userID, collection.ID, "region", region.ID, "area")

	remainingSharedPlaceID := insertTargetPlaceWithScopeFixture(t, db, otherID, scope.ID)

	if _, err := db.Pool.Exec(ctx, `UPDATE users SET deleted_at = NOW() WHERE id = $1`, userID); err != nil {
		t.Fatalf("mark user deleted: %v", err)
	}
	if err := db.DeleteUserData(ctx, userID, nil); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
	}

	assertTargetObjectGDPRCount(t, db, "target_places owned by deleted user", `SELECT COUNT(*) FROM target_places WHERE id = $1`, place.ID)
	assertTargetObjectGDPRCount(t, db, "target_regions owned by deleted user", `SELECT COUNT(*) FROM target_regions WHERE id = $1`, region.ID)
	assertTargetObjectGDPRCount(t, db, "target_collections owned by deleted user", `SELECT COUNT(*) FROM target_collections WHERE id = $1`, collection.ID)
	assertTargetObjectGDPRCount(t, db, "target_collection_members deleted collection", `SELECT COUNT(*) FROM target_collection_members WHERE collection_id = $1`, collection.ID)
	assertTargetObjectGDPRCount(t, db, "target_collection_members deleted members", `SELECT COUNT(*) FROM target_collection_members WHERE member_id IN ($1, $2)`, place.ID, region.ID)
	assertTargetObjectGDPRCount(t, db, "object_access_scopes owned by deleted user", `SELECT COUNT(*) FROM object_access_scopes WHERE id = $1`, scope.ID)

	assertTargetAccessState(t, db, "target_places", remainingSharedPlaceID, "private", false)
	assertTargetObjectProjectionAccessState(t, db, "place", remainingSharedPlaceID, "private", false)
}

func TestGDPRDeletesTargetOverlayRows(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	otherID := insertObjectAccessTestUser(t, db)
	place := createTargetPlaceFixture(t, db, userID, "gdpr overlay place", "private", nil, `[8,49]`)
	otherPlace := createTargetPlaceFixture(t, db, otherID, "gdpr other overlay place", "public", nil, `[8.01,49.01]`)

	reportID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO target_reports (
			id, overlay_subject_kind, subject_id, author_user_id, report_type,
			visibility, attribution_mode, description, observed_at, moderation_state
		)
		VALUES ($1, 'place', $2, $3, 'condition', 'private', 'named', 'owner report', NOW(), 'visible')
	`, reportID, place.ID, userID); err != nil {
		t.Fatalf("insert target report fixture: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO target_report_status_events (report_id, actor_user_id, status, reason)
		VALUES ($1, $2, 'open', 'created')
	`, reportID, userID); err != nil {
		t.Fatalf("insert target report status fixture: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO target_activity_observations (
			overlay_subject_kind, subject_id, user_id, observation_type,
			source_type, source_id, source_occurrence_key, visibility,
			visibility_snapshot, occurred_at
		)
		VALUES ('place', $1, $2, 'visit', 'manual', 'visit-1', 'once', 'private', 'private', NOW())
	`, place.ID, userID); err != nil {
		t.Fatalf("insert target observation fixture: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO target_user_content (
			overlay_subject_kind, subject_id, user_id, content_kind, payload, visibility
		)
		VALUES ('place', $1, $2, 'comment', '{"text":"fixture"}'::jsonb, 'private')
	`, place.ID, userID); err != nil {
		t.Fatalf("insert target user content fixture: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO target_overlay_summary_projection (
			overlay_subject_kind, subject_id, visibility_scope, public_open_report_count
		)
		VALUES ('place', $1, 'public', 1)
		`, place.ID); err != nil {
		t.Fatalf("insert target overlay summary fixture: %v", err)
	}
	otherReportID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO target_reports (
			id, overlay_subject_kind, subject_id, author_user_id, report_type,
			visibility, attribution_mode, description, observed_at, moderation_state
		)
		VALUES ($1, 'place', $2, $3, 'condition', 'public', 'named', 'cross target report', NOW(), 'visible')
	`, otherReportID, otherPlace.ID, userID); err != nil {
		t.Fatalf("insert cross target report fixture: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO target_report_status_events (report_id, actor_user_id, status, reason)
		VALUES ($1, $2, 'open', 'created')
	`, otherReportID, userID); err != nil {
		t.Fatalf("insert cross target report status fixture: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO target_overlay_summary_projection (
			overlay_subject_kind, subject_id, visibility_scope, public_open_report_count
		)
		VALUES ('place', $1, 'public', 1)
	`, otherPlace.ID); err != nil {
		t.Fatalf("insert cross target overlay summary fixture: %v", err)
	}

	if _, err := db.Pool.Exec(ctx, `UPDATE users SET deleted_at = NOW() WHERE id = $1`, userID); err != nil {
		t.Fatalf("mark user deleted: %v", err)
	}
	if err := db.DeleteUserData(ctx, userID, nil); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
	}

	assertTargetObjectGDPRCount(t, db, "target_activity_observations", `SELECT COUNT(*) FROM target_activity_observations WHERE subject_id = $1 OR user_id = $2`, place.ID, userID)
	assertTargetObjectGDPRCount(t, db, "target_reports", `SELECT COUNT(*) FROM target_reports WHERE subject_id = $1 OR author_user_id = $2`, place.ID, userID)
	assertTargetObjectGDPRCount(t, db, "target_report_status_events", `SELECT COUNT(*) FROM target_report_status_events WHERE report_id = $1 OR actor_user_id = $2`, reportID, userID)
	assertTargetObjectGDPRCount(t, db, "target_user_content", `SELECT COUNT(*) FROM target_user_content WHERE subject_id = $1 OR user_id = $2`, place.ID, userID)
	assertTargetObjectGDPRCount(t, db, "target_overlay_summary_projection", `SELECT COUNT(*) FROM target_overlay_summary_projection WHERE subject_id = $1`, place.ID)
	assertTargetObjectGDPRCount(t, db, "cross target overlay reports", `SELECT COUNT(*) FROM target_reports WHERE id = $1`, otherReportID)
	assertTargetObjectGDPRCount(t, db, "cross target stale overlay summary", `SELECT COUNT(*) FROM target_overlay_summary_projection WHERE subject_id = $1 AND public_open_report_count <> 0`, otherPlace.ID)
}

func TestGDPRAnonymizesTargetChangeEvents(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, userID, "gdpr audit place", "private", nil, `[8,49]`)
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin target audit fixture: %v", err)
	}
	if err := writeTargetChangeEventTx(ctx, tx, "place", place.ID, objectAccessPtrString("place"), &place.ID, nil, userID, "update_place", nil, place); err != nil {
		tx.Rollback(ctx) //nolint:errcheck
		t.Fatalf("write target audit fixture: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit target audit fixture: %v", err)
	}

	var beforeEvents int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM target_change_events
		WHERE subject_type = 'place' AND subject_id = $1 AND actor_user_id = $2
	`, place.ID, userID).Scan(&beforeEvents); err != nil {
		t.Fatalf("count target events before purge: %v", err)
	}
	if beforeEvents == 0 {
		t.Fatal("fixture did not create a target_change_events row")
	}

	if _, err := db.Pool.Exec(ctx, `UPDATE users SET deleted_at = NOW() WHERE id = $1`, userID); err != nil {
		t.Fatalf("mark user deleted: %v", err)
	}
	if err := db.DeleteUserData(ctx, userID, nil); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
	}

	assertTargetObjectGDPRCount(t, db, "target_change_events actor", `SELECT COUNT(*) FROM target_change_events WHERE actor_user_id = $1`, userID)
	assertTargetObjectGDPRCount(t, db, "target_revision_snapshots creator", `SELECT COUNT(*) FROM target_revision_snapshots WHERE created_by = $1`, userID)
	assertTargetObjectGDPRCount(t, db, "owner-private snapshots for deleted place", `
		SELECT COUNT(*)
		FROM target_revision_snapshots
		WHERE subject_type = 'place' AND subject_id = $1 AND redaction_policy = 'owner_private'
	`, place.ID)

	var remainingEvents int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM target_change_events
		WHERE subject_type = 'place' AND subject_id = $1
	`, place.ID).Scan(&remainingEvents); err != nil {
		t.Fatalf("count target events after purge: %v", err)
	}
	if remainingEvents == 0 {
		t.Fatal("GDPR purge deleted target change history instead of anonymizing it")
	}
}

func TestGDPRDeletesPrivateTargetSnapshotsBySubjectOwner(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	adminID := insertObjectAccessTestUser(t, db)
	place := createTargetPlaceFixture(t, db, userID, "gdpr admin snapshot place", "private", nil, `[8,49]`)
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin target audit fixture: %v", err)
	}
	if err := writeTargetChangeEventTx(ctx, tx, "place", place.ID, objectAccessPtrString("place"), &place.ID, nil, adminID, "admin_update_place", nil, place); err != nil {
		tx.Rollback(ctx) //nolint:errcheck
		t.Fatalf("write target audit fixture: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit target audit fixture: %v", err)
	}

	if _, err := db.Pool.Exec(ctx, `UPDATE users SET deleted_at = NOW() WHERE id = $1`, userID); err != nil {
		t.Fatalf("mark user deleted: %v", err)
	}
	if err := db.DeleteUserData(ctx, userID, nil); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
	}

	assertTargetObjectGDPRCount(t, db, "owner-private snapshots for deleted-user private place written by admin", `
		SELECT COUNT(*)
		FROM target_revision_snapshots
		WHERE subject_type = 'place' AND subject_id = $1 AND redaction_policy = 'owner_private'
	`, place.ID)
}

func TestGDPRRemovesGovernanceRolesAndBindings(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	ownerID := insertObjectAccessTestUser(t, db)
	place := createTargetPlaceFixture(t, db, ownerID, "governance gdpr place", "public", nil, `[8,49]`)
	scope := createGovernanceScopeFixture(t, db, ownerID, "place", place.ID)
	userRole := addGovernanceRoleFixture(t, db, ownerID, scope.ID, userID, []string{"edit_metadata"})
	if _, err := db.AddGovernanceActorRole(ctx, ownerID, scope.ID, model.GovernanceActorRoleCreate{
		ActorType:    "external_party_credit",
		ActorRef:     objectAccessPtrString("credit:gdpr-safe"),
		Role:         "sponsor",
		PublicCredit: "listed",
	}); err != nil {
		t.Fatalf("AddGovernanceActorRole external credit: %v", err)
	}
	addGovernanceBindingFixture(t, db, ownerID, scope.ID, "place", place.ID, "metadata")

	if _, err := db.Pool.Exec(ctx, `UPDATE users SET deleted_at = NOW() WHERE id = $1`, userID); err != nil {
		t.Fatalf("mark user deleted: %v", err)
	}
	if err := db.DeleteUserData(ctx, userID, nil); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
	}

	assertTargetObjectGDPRCount(t, db, "governance role for deleted user", `SELECT COUNT(*) FROM governance_actor_roles WHERE id = $1`, userRole.ID)
	assertTargetObjectGDPRCount(t, db, "governance projection for deleted user", `SELECT COUNT(*) FROM governance_actor_target_projection WHERE actor_user_id = $1`, userID)

	var externalCredits int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM governance_actor_roles
		WHERE scope_id = $1 AND actor_type = 'external_party_credit' AND actor_user_id IS NULL
	`, scope.ID).Scan(&externalCredits); err != nil {
		t.Fatalf("count external credit governance roles: %v", err)
	}
	if externalCredits != 1 {
		t.Fatalf("external credit governance roles = %d, want 1", externalCredits)
	}
}

func insertTargetPlaceWithScopeFixture(t *testing.T, db *DB, ownerID, scopeID string) string {
	t.Helper()
	placeID := uuid.NewString()
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO target_places (
			id, owner_user_id, name, place_kind, geom,
			visibility, access_scope_id, origin_key, created_by
		)
		VALUES ($1, $2, 'remaining shared place', 'custom', ST_SetSRID(ST_MakePoint(8.1, 49.1), 4326),
			'shared', $3, 'manual', $2)
	`, placeID, ownerID, scopeID); err != nil {
		t.Fatalf("insert shared target place fixture: %v", err)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin shared target place projection rebuild: %v", err)
	}
	defer tx.Rollback(ctx)
	if err := rebuildTargetPlaceProjectionTx(ctx, tx, placeID); err != nil {
		t.Fatalf("rebuild shared target place projection: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit shared target place projection rebuild: %v", err)
	}
	return placeID
}

func assertTargetObjectProjectionAccessState(t *testing.T, db *DB, objectKind, objectID, wantVisibility string, wantScope bool) {
	t.Helper()
	var visibility string
	var hasScope bool
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT visibility, access_scope_id IS NOT NULL
		FROM target_object_search_projection
		WHERE object_kind = $1 AND object_id = $2
	`, objectKind, objectID).Scan(&visibility, &hasScope); err != nil {
		t.Fatalf("query target object projection access state: %v", err)
	}
	if visibility != wantVisibility || hasScope != wantScope {
		t.Fatalf("target object projection access state = %s/%v, want %s/%v", visibility, hasScope, wantVisibility, wantScope)
	}
}

func assertTargetObjectGDPRCount(t *testing.T, db *DB, name, query string, args ...any) {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", name, err)
	}
	if count != 0 {
		t.Fatalf("%s count = %d, want 0", name, count)
	}
}
