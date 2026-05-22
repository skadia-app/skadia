package database

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
)

func TestDeleteUserDataRemovesRoutePublicationEventsForPurgedSubjects(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &DB{Pool: pool}

	userID := uuid.NewString()
	routeID := uuid.NewString()
	segmentID := uuid.NewString()
	versionID := uuid.NewString()
	snapshotID := uuid.NewString()
	publicationID := uuid.NewString()
	actorOnlyEventID := uuid.NewString()

	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source, deleted_at)
		VALUES ($1, $2, $3, true, 'test', NOW() - INTERVAL '31 days')
	`, userID, "gdpr_route_"+userID[:8], "gdpr_route_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO planned_routes (
			id, user_id, name, sport_type, profile, geometry, waypoints, segments, route_schema_version
		)
		VALUES (
			$1, $2, 'Route publication purge fixture', 'hiking', 'outdoor',
			ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
			'[]'::jsonb, '[]'::jsonb, 1
		)
	`, routeID, userID); err != nil {
		t.Fatalf("insert route: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type, visibility, elevation_kind)
		VALUES ($1, $2, 'GDPR route course', 'bike', 'standard', 'private', 'flat')
	`, segmentID, userID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_versions (
			id, segment_id, version_number, track, valid_from, created_by,
			valid_from_at, validity_basis
		)
		VALUES (
			$1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
			CURRENT_DATE, $3, NULL, 'unknown_start'
		)
	`, versionID, segmentID, userID); err != nil {
		t.Fatalf("insert segment version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO planned_route_snapshots (
			id, route_id, user_id, source_route_version, route_name, route_description,
			sport_type, sub_type, profile, route_schema_version, geometry,
			waypoints, segments, geometry_hash, snapshot_hash
		)
		VALUES (
			$1, $2, $3, 1, 'Snapshot fixture', 'Fixture',
			'cycling', 'mountain', 'default', 1,
			ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
			'[]'::jsonb, '[]'::jsonb, 'geometry-hash', 'snapshot-hash'
		)
	`, snapshotID, routeID, userID); err != nil {
		t.Fatalf("insert route snapshot: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO route_course_publications (
			id, route_id, route_snapshot_id, course_segment_id, course_segment_version_id,
			publish_policy, equivalence_decision, visibility_snapshot,
			actor_user_id, operation_id, operation_input_hash, observed_route_version
		)
		VALUES (
			$1, $2, $3, $4, $5,
			'new_course', 'new_group', 'public',
			$6, 'gdpr-publication', 'hash', 1
		)
	`, publicationID, routeID, snapshotID, segmentID, versionID, userID); err != nil {
		t.Fatalf("insert route publication: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO route_publication_events (
			id, actor_user_id, operation, operation_id, operation_input_hash,
			subject_kind, subject_id, after_snapshot_hash
		)
		VALUES
			(gen_random_uuid(), $1, 'snapshot', 'gdpr-route-event', 'hash', 'planned_route', $2, 'after-route'),
			(gen_random_uuid(), $1, 'snapshot', 'gdpr-snapshot-event', 'hash', 'planned_route_snapshot', $3, 'after-snapshot'),
			(gen_random_uuid(), $1, 'publish_course', 'gdpr-publication-event', 'hash', 'route_course_publication', $4, 'after-publication'),
			($5, $1, 'relate_route', 'gdpr-actor-only-event', 'hash', 'route_relationship', gen_random_uuid(), 'after-actor')
	`, userID, routeID, snapshotID, publicationID, actorOnlyEventID); err != nil {
		t.Fatalf("insert route publication events: %v", err)
	}

	if err := db.DeleteUserData(ctx, userID, nil); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
	}

	var subjectEventCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM route_publication_events
		WHERE subject_id = ANY($1::uuid[])
	`, []string{routeID, snapshotID, publicationID}).Scan(&subjectEventCount); err != nil {
		t.Fatalf("count subject events: %v", err)
	}
	if subjectEventCount != 0 {
		t.Fatalf("subject-bound route publication events remaining = %d, want 0", subjectEventCount)
	}

	var actorUserID *string
	if err := pool.QueryRow(ctx, `
		SELECT actor_user_id::text FROM route_publication_events WHERE id = $1
	`, actorOnlyEventID).Scan(&actorUserID); err != nil {
		t.Fatalf("query actor-only event: %v", err)
	}
	if actorUserID != nil {
		t.Fatalf("actor-only event actor_user_id = %q, want NULL", *actorUserID)
	}
}
