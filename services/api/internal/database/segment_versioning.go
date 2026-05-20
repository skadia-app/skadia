package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const segmentVersionRevisionSchemaKey = "segment_version_revision_v1"

type SegmentVersionSnapshot struct {
	SegmentID         string                          `json:"segment_id"`
	Segment           SegmentMetadataSnapshotRow      `json:"segment"`
	Versions          []SegmentVersionSnapshotRow     `json:"versions"`
	EquivalenceGroups []SegmentEquivalenceSnapshotRow `json:"equivalence_groups"`
}

type SegmentMetadataSnapshotRow struct {
	ID                string          `json:"id"`
	CreatedBy         string          `json:"created_by"`
	Name              string          `json:"name"`
	SegmentType       string          `json:"segment_type"`
	SportGroup        string          `json:"sport_group"`
	AllowedPredicates json.RawMessage `json:"allowed_predicates,omitempty"`
	Bidirectional     bool            `json:"bidirectional"`
	Visibility        string          `json:"visibility"`
	ElevationKind     string          `json:"elevation_kind"`
	TerrainType       *string         `json:"terrain_type,omitempty"`
	ClimbType         *string         `json:"climb_type,omitempty"`
	SegmentTypeMeta   json.RawMessage `json:"segment_type_meta"`
}

type SegmentVersionSnapshotRow struct {
	ID                              string          `json:"id"`
	VersionNumber                   int             `json:"version_number"`
	ValidFromAt                     *time.Time      `json:"valid_from_at,omitempty"`
	ValidUntilAt                    *time.Time      `json:"valid_until_at,omitempty"`
	ValidityBasis                   string          `json:"validity_basis"`
	MatchingContractKey             string          `json:"matching_contract_key"`
	MatchingContractVersion         int             `json:"matching_contract_version"`
	TrackGeoJSON                    string          `json:"track_geojson"`
	StartPointGeoJSON               string          `json:"start_point_geojson"`
	EndPointGeoJSON                 string          `json:"end_point_geojson"`
	EligibilitySegmentType          string          `json:"eligibility_segment_type"`
	EligibilitySportGroup           string          `json:"eligibility_sport_group"`
	EligibilityAllowedPredicates    json.RawMessage `json:"eligibility_allowed_predicates,omitempty"`
	EligibilityAllowedPredicatesSet bool            `json:"eligibility_allowed_predicates_set"`
	EligibilityBidirectional        bool            `json:"eligibility_bidirectional"`
	EligibilityVisibility           string          `json:"eligibility_visibility"`
	EligibilityElevationKind        string          `json:"eligibility_elevation_kind"`
}

type SegmentEquivalenceSnapshotRow struct {
	GroupID            string          `json:"group_id"`
	SegmentVersionID   string          `json:"segment_version_id"`
	LeaderboardSurface string          `json:"leaderboard_surface"`
	RetiredAt          *time.Time      `json:"retired_at,omitempty"`
	JoinReason         string          `json:"join_reason"`
	ChangeKey          string          `json:"change_key"`
	PolicyKey          string          `json:"policy_key"`
	PolicyVersion      int             `json:"policy_version"`
	EquivalenceKind    string          `json:"equivalence_kind"`
	Metadata           json.RawMessage `json:"metadata"`
}

func LoadSegmentVersionSnapshotTx(ctx context.Context, tx pgx.Tx, segmentID string) (SegmentVersionSnapshot, error) {
	var snapshot SegmentVersionSnapshot
	snapshot.SegmentID = segmentID

	var allowedPredicates, segmentTypeMeta []byte
	if err := tx.QueryRow(ctx, `
		SELECT id, created_by, name, segment_type, sport_group,
		       COALESCE(allowed_predicates, 'null'::jsonb),
		       bidirectional, COALESCE(visibility, 'public'), elevation_kind,
		       terrain_type, climb_type, COALESCE(segment_type_meta, '{}'::jsonb)
		FROM segments
		WHERE id = $1
	`, segmentID).Scan(
		&snapshot.Segment.ID,
		&snapshot.Segment.CreatedBy,
		&snapshot.Segment.Name,
		&snapshot.Segment.SegmentType,
		&snapshot.Segment.SportGroup,
		&allowedPredicates,
		&snapshot.Segment.Bidirectional,
		&snapshot.Segment.Visibility,
		&snapshot.Segment.ElevationKind,
		&snapshot.Segment.TerrainType,
		&snapshot.Segment.ClimbType,
		&segmentTypeMeta,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return snapshot, ErrSegmentNotFound
		}
		return snapshot, fmt.Errorf("load segment snapshot metadata: %w", err)
	}
	snapshot.Segment.AllowedPredicates = cloneRawMessage(allowedPredicates)
	snapshot.Segment.SegmentTypeMeta = cloneRawMessage(segmentTypeMeta)

	rows, err := tx.Query(ctx, `
		SELECT id, version_number, valid_from_at, valid_until_at, validity_basis,
		       matching_contract_key, matching_contract_version,
		       ST_AsGeoJSON(track),
		       ST_AsGeoJSON(COALESCE(start_point::geometry, ST_StartPoint(track))),
		       ST_AsGeoJSON(COALESCE(end_point::geometry, ST_EndPoint(track))),
		       eligibility_segment_type, eligibility_sport_group,
		       COALESCE(eligibility_allowed_predicates, 'null'::jsonb),
		       eligibility_allowed_predicates_set, eligibility_bidirectional,
		       eligibility_visibility, eligibility_elevation_kind
		FROM segment_versions
		WHERE segment_id = $1
		ORDER BY version_number, id
	`, segmentID)
	if err != nil {
		return snapshot, fmt.Errorf("load segment version snapshot rows: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var row SegmentVersionSnapshotRow
		var predicates []byte
		if err := rows.Scan(
			&row.ID,
			&row.VersionNumber,
			&row.ValidFromAt,
			&row.ValidUntilAt,
			&row.ValidityBasis,
			&row.MatchingContractKey,
			&row.MatchingContractVersion,
			&row.TrackGeoJSON,
			&row.StartPointGeoJSON,
			&row.EndPointGeoJSON,
			&row.EligibilitySegmentType,
			&row.EligibilitySportGroup,
			&predicates,
			&row.EligibilityAllowedPredicatesSet,
			&row.EligibilityBidirectional,
			&row.EligibilityVisibility,
			&row.EligibilityElevationKind,
		); err != nil {
			return snapshot, fmt.Errorf("scan segment version snapshot row: %w", err)
		}
		row.EligibilityAllowedPredicates = cloneRawMessage(predicates)
		snapshot.Versions = append(snapshot.Versions, row)
	}
	if err := rows.Err(); err != nil {
		return snapshot, fmt.Errorf("segment version snapshot rows: %w", err)
	}

	groupRows, err := tx.Query(ctx, `
		SELECT gv.group_id, gv.segment_version_id, gv.leaderboard_surface,
		       gv.retired_at, gv.join_reason,
		       g.change_key, g.policy_key, g.policy_version, g.equivalence_kind,
		       COALESCE(g.metadata, '{}'::jsonb)
		FROM segment_leaderboard_equivalence_group_versions gv
		JOIN segment_leaderboard_equivalence_groups g ON g.id = gv.group_id
		WHERE g.segment_id = $1
		ORDER BY gv.segment_version_id, gv.leaderboard_surface, gv.retired_at NULLS FIRST, gv.group_id
	`, segmentID)
	if err != nil {
		return snapshot, fmt.Errorf("load segment equivalence snapshot rows: %w", err)
	}
	defer groupRows.Close()
	for groupRows.Next() {
		var row SegmentEquivalenceSnapshotRow
		var metadata []byte
		if err := groupRows.Scan(
			&row.GroupID,
			&row.SegmentVersionID,
			&row.LeaderboardSurface,
			&row.RetiredAt,
			&row.JoinReason,
			&row.ChangeKey,
			&row.PolicyKey,
			&row.PolicyVersion,
			&row.EquivalenceKind,
			&metadata,
		); err != nil {
			return snapshot, fmt.Errorf("scan segment equivalence snapshot row: %w", err)
		}
		row.Metadata = cloneRawMessage(metadata)
		snapshot.EquivalenceGroups = append(snapshot.EquivalenceGroups, row)
	}
	if err := groupRows.Err(); err != nil {
		return snapshot, fmt.Errorf("segment equivalence snapshot rows: %w", err)
	}
	return snapshot, nil
}

func (db *DB) UpdateSegmentVersionValidity(ctx context.Context, segmentID, versionID, actorUserID string, isAdmin bool, update model.SegmentVersionValidityUpdate) (*model.SegmentVersionRevision, error) {
	if strings.TrimSpace(update.OperationID) == "" {
		return nil, fmt.Errorf("%w: operation_id is required", ErrSegmentValidation)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin segment validity update: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := authorizeSegmentOwnerTx(ctx, tx, segmentID, actorUserID, isAdmin); err != nil {
		return nil, err
	}
	before, err := LoadSegmentVersionSnapshotTx(ctx, tx, segmentID)
	if err != nil {
		return nil, err
	}
	oldVersion, ok := findSnapshotVersion(before, versionID)
	if !ok {
		return nil, ErrSegmentNotFound
	}
	affectedFrom, affectedUntil := affectedInterval(oldVersion.ValidFromAt, oldVersion.ValidUntilAt, update.ValidFromAt, update.ValidUntilAt)
	if !update.Recalculate && intervalTouchesNowOrPast(affectedFrom) {
		return nil, fmt.Errorf("%w: recalculation is required for current or historical validity changes", ErrSegmentValidation)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE segment_versions
		SET valid_from_at = $3,
		    valid_until_at = $4,
		    validity_basis = 'editor_asserted'
		WHERE segment_id = $1 AND id = $2
	`, segmentID, versionID, update.ValidFromAt, update.ValidUntilAt); err != nil {
		return nil, segmentVersioningPgError("update segment version validity", err)
	}
	if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, segmentID); err != nil {
		return nil, err
	}
	if err := applyEquivalenceDecisionTx(ctx, tx, segmentID, versionID, actorUserID, update.OperationID, update.EquivalenceDecision, update.Reason, nil); err != nil {
		return nil, err
	}
	recalcRunID, err := createSegmentRecalculationRunTx(ctx, tx, segmentID, actorUserID, update.OperationID, update.Reason, affectedFrom, affectedUntil, update.Recalculate || intervalTouchesNowOrPast(affectedFrom))
	if err != nil {
		return nil, err
	}
	after, err := LoadSegmentVersionSnapshotTx(ctx, tx, segmentID)
	if err != nil {
		return nil, err
	}
	revision, err := insertSegmentVersionRevisionTx(ctx, tx, segmentID, actorUserID, "validity_update", update.Reason, before, after, affectedFrom, affectedUntil, recalcRunID, nil, update.OperationID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit segment validity update: %w", err)
	}
	return revision, nil
}

func (db *DB) UpdateSegmentVersionGeometry(ctx context.Context, segmentID, versionID, actorUserID string, isAdmin bool, update model.SegmentGeometryUpdate) (*model.SegmentVersionRevision, error) {
	if strings.TrimSpace(update.OperationID) == "" {
		return nil, fmt.Errorf("%w: operation_id is required", ErrSegmentValidation)
	}
	if len(update.TrackGeoJSON) == 0 {
		return nil, fmt.Errorf("%w: track_geojson is required", ErrSegmentValidation)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin segment geometry update: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := authorizeSegmentOwnerTx(ctx, tx, segmentID, actorUserID, isAdmin); err != nil {
		return nil, err
	}
	before, err := LoadSegmentVersionSnapshotTx(ctx, tx, segmentID)
	if err != nil {
		return nil, err
	}
	oldVersion, ok := findSnapshotVersion(before, versionID)
	if !ok {
		return nil, ErrSegmentNotFound
	}
	boundary := time.Now().UTC()
	affectedFrom := &boundary
	affectedUntil := oldVersion.ValidUntilAt

	var newVersionID string
	if err := tx.QueryRow(ctx, `
		WITH next_version AS (
			SELECT COALESCE(MAX(version_number), 0) + 1 AS number
			FROM segment_versions
			WHERE segment_id = $1
		),
		closed AS (
			UPDATE segment_versions
			SET valid_until_at = $4
			WHERE id = $2
			  AND segment_id = $1
			  AND (valid_until_at IS NULL OR valid_until_at > $4)
			RETURNING id
		),
		g AS (
			SELECT ST_SetSRID(ST_GeomFromGeoJSON($3), 4326) AS geom
		)
		INSERT INTO segment_versions (
			segment_id, version_number, track, start_point, end_point,
			valid_from, created_by, valid_from_at, valid_until_at, validity_basis,
			matching_contract_key, matching_contract_version, published_at
		)
		SELECT $1, next_version.number, g.geom, ST_StartPoint(g.geom), ST_EndPoint(g.geom),
		       CURRENT_DATE, $5, $4, $6, 'editor_asserted',
		       'course_gps_v1', 1, $4
		FROM next_version, g, closed
		RETURNING id
	`, segmentID, versionID, string(update.TrackGeoJSON), boundary, actorUserID, affectedUntil).Scan(&newVersionID); err != nil {
		return nil, segmentVersioningPgError("insert segment geometry version", err)
	}
	if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, segmentID); err != nil {
		return nil, err
	}
	if err := applyEquivalenceDecisionTx(ctx, tx, segmentID, newVersionID, actorUserID, update.OperationID, update.EquivalenceDecision, update.Reason, update.EquivalenceEvidence); err != nil {
		return nil, err
	}
	recalcRunID, err := createSegmentRecalculationRunTx(ctx, tx, segmentID, actorUserID, update.OperationID, update.Reason, affectedFrom, affectedUntil, true)
	if err != nil {
		return nil, err
	}
	after, err := LoadSegmentVersionSnapshotTx(ctx, tx, segmentID)
	if err != nil {
		return nil, err
	}
	revision, err := insertSegmentVersionRevisionTx(ctx, tx, segmentID, actorUserID, "geometry_update", update.Reason, before, after, affectedFrom, affectedUntil, recalcRunID, nil, update.OperationID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit segment geometry update: %w", err)
	}
	return revision, nil
}

func (db *DB) RollbackSegmentVersionRevision(ctx context.Context, segmentID, revisionID, actorUserID string, isAdmin bool, reason string) (*model.SegmentVersionRevision, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin segment revision rollback: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := authorizeSegmentOwnerTx(ctx, tx, segmentID, actorUserID, isAdmin); err != nil {
		return nil, err
	}
	before, err := LoadSegmentVersionSnapshotTx(ctx, tx, segmentID)
	if err != nil {
		return nil, err
	}

	var storedSegmentID string
	var beforeRaw []byte
	var affectedFrom, affectedUntil *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT segment_id, before_snapshot, affected_from_at, affected_until_at
		FROM segment_version_revisions
		WHERE id = $1
	`, revisionID).Scan(&storedSegmentID, &beforeRaw, &affectedFrom, &affectedUntil); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSegmentNotFound
		}
		return nil, fmt.Errorf("load segment revision for rollback: %w", err)
	}
	if storedSegmentID != segmentID {
		return nil, fmt.Errorf("%w: revision belongs to a different segment", ErrSegmentValidation)
	}
	var restore SegmentVersionSnapshot
	if err := json.Unmarshal(beforeRaw, &restore); err != nil {
		return nil, fmt.Errorf("decode rollback snapshot: %w", err)
	}
	if err := restoreSegmentSnapshotTx(ctx, tx, restore); err != nil {
		return nil, err
	}
	if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, segmentID); err != nil {
		return nil, err
	}
	recalcRunID, err := createSegmentRecalculationRunTx(ctx, tx, segmentID, actorUserID, "rollback:"+revisionID, reason, affectedFrom, affectedUntil, true)
	if err != nil {
		return nil, err
	}
	after, err := LoadSegmentVersionSnapshotTx(ctx, tx, segmentID)
	if err != nil {
		return nil, err
	}
	rollbackOf := revisionID
	revision, err := insertSegmentVersionRevisionTx(ctx, tx, segmentID, actorUserID, "rollback_revision", reason, before, after, affectedFrom, affectedUntil, recalcRunID, &rollbackOf, "rollback:"+revisionID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit segment revision rollback: %w", err)
	}
	return revision, nil
}

func (db *DB) ReverseActiveSegmentVersionTx(ctx context.Context, tx pgx.Tx, segmentID, actorUserID, operationID string) (string, string, error) {
	before, err := LoadSegmentVersionSnapshotTx(ctx, tx, segmentID)
	if err != nil {
		return "", "", err
	}
	boundary := time.Now().UTC()
	var versionID string
	if err := tx.QueryRow(ctx, `
		WITH boundary AS (
			SELECT $3::timestamptz AS at
		),
		next_version AS (
			SELECT COALESCE(MAX(version_number), 0) + 1 AS number
			FROM segment_versions
			WHERE segment_id = $1
		),
		active AS (
			SELECT id, segment_id, version_number, track, start_point, end_point
			FROM segment_versions, boundary
			WHERE segment_id = $1
			  AND matching_contract_key = 'course_gps_v1'
			  AND matching_contract_version = 1
			  AND (valid_from_at IS NULL OR valid_from_at <= boundary.at)
			  AND (valid_until_at IS NULL OR boundary.at < valid_until_at)
			ORDER BY COALESCE(valid_from_at, '-infinity'::timestamptz) DESC,
			         version_number DESC
			LIMIT 1
		),
		next_future AS (
			SELECT MIN(valid_from_at) AS at
			FROM segment_versions, boundary
			WHERE segment_id = $1
			  AND matching_contract_key = 'course_gps_v1'
			  AND matching_contract_version = 1
			  AND valid_from_at > boundary.at
		),
		closed AS (
			UPDATE segment_versions sv
			SET valid_until_at = boundary.at
			FROM active, boundary
			WHERE sv.id = active.id
			  AND (sv.valid_until_at IS NULL OR sv.valid_until_at > boundary.at)
			RETURNING sv.id
		)
		INSERT INTO segment_versions (
			segment_id, version_number, track, start_point, end_point,
			valid_from, created_by, valid_from_at, valid_until_at, validity_basis,
			matching_contract_key, matching_contract_version, published_at
		)
		SELECT active.segment_id,
		       next_version.number,
		       ST_Reverse(active.track),
		       active.end_point,
		       active.start_point,
		       CURRENT_DATE,
		       $2,
		       boundary.at,
		       next_future.at,
		       'editor_asserted',
		       'course_gps_v1',
		       1,
		       boundary.at
		FROM active, next_version, next_future, boundary, closed
		RETURNING id
	`, segmentID, actorUserID, boundary).Scan(&versionID); err != nil {
		return "", "", fmt.Errorf("insert reversed segment version: %w", err)
	}
	if err := applyEquivalenceDecisionTx(ctx, tx, segmentID, versionID, actorUserID, operationID, "new_group", "reverse", nil); err != nil {
		return "", "", err
	}
	if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, segmentID); err != nil {
		return "", "", fmt.Errorf("rebuild course eligibility projection: %w", err)
	}
	recalcRunID, err := createSegmentRecalculationRunTx(ctx, tx, segmentID, actorUserID, operationID, "reverse", &boundary, nil, true)
	if err != nil {
		return "", "", err
	}
	after, err := LoadSegmentVersionSnapshotTx(ctx, tx, segmentID)
	if err != nil {
		return "", "", err
	}
	if _, err := insertSegmentVersionRevisionTx(ctx, tx, segmentID, actorUserID, "reverse", "reverse", before, after, &boundary, nil, recalcRunID, nil, operationID); err != nil {
		return "", "", err
	}
	groupID, err := activeLeaderboardEquivalenceGroupTx(ctx, tx, segmentID, versionID)
	if err != nil {
		return "", "", err
	}
	return versionID, groupID, nil
}

func authorizeSegmentOwnerTx(ctx context.Context, tx pgx.Tx, segmentID, actorUserID string, isAdmin bool) error {
	var ownerID string
	if err := tx.QueryRow(ctx, `SELECT created_by FROM segments WHERE id = $1 FOR UPDATE`, segmentID).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSegmentNotFound
		}
		return fmt.Errorf("lock segment: %w", err)
	}
	if !isAdmin && ownerID != actorUserID {
		return ErrSegmentNotAuthorized
	}
	return nil
}

func applyEquivalenceDecisionTx(ctx context.Context, tx pgx.Tx, segmentID, versionID, actorUserID, operationID, decision, reason string, evidence map[string]any) error {
	decision = strings.TrimSpace(decision)
	if decision == "" {
		decision = "continue_group"
	}
	if decision == "continue_group" && len(evidence) == 0 {
		var existing string
		err := tx.QueryRow(ctx, `
			SELECT group_id
			FROM segment_leaderboard_equivalence_group_versions
			WHERE segment_version_id = $1
			  AND leaderboard_surface = 'default'
			  AND retired_at IS NULL
			LIMIT 1
		`, versionID).Scan(&existing)
		if err == nil && existing != "" {
			return nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("load current equivalence group: %w", err)
		}
		decision = "version_specific"
	}

	equivalenceKind := "same_course"
	if decision == "version_specific" {
		equivalenceKind = "version_specific"
	}
	changeKey := "operation:" + stableOperationHash(operationID, decision) + ":" + decision
	metadata, err := json.Marshal(map[string]any{
		"operation_id_hash": stableOperationHash(operationID, "metadata"),
		"decision":          decision,
		"evidence":          evidence,
	})
	if err != nil {
		return err
	}
	var groupID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO segment_leaderboard_equivalence_groups (
			segment_id, change_key, policy_key, policy_version, equivalence_kind,
			created_by, reason, metadata
		)
		VALUES ($1, $2, 'segment_leaderboard_equivalence', 1, $3, $4, NULLIF($5, ''), $6)
		ON CONFLICT (segment_id, policy_key, policy_version, change_key) DO UPDATE
			SET reason = COALESCE(EXCLUDED.reason, segment_leaderboard_equivalence_groups.reason),
			    metadata = EXCLUDED.metadata
		RETURNING id
	`, segmentID, changeKey, equivalenceKind, actorUserID, reason, metadata).Scan(&groupID); err != nil {
		return fmt.Errorf("upsert segment leaderboard equivalence group: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE segment_leaderboard_equivalence_group_versions
		SET retired_at = GREATEST(NOW(), joined_at + interval '1 microsecond')
		WHERE segment_version_id = $1
		  AND leaderboard_surface = 'default'
		  AND retired_at IS NULL
	`, versionID); err != nil {
		return fmt.Errorf("retire previous equivalence membership: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO segment_leaderboard_equivalence_group_versions (
			group_id, segment_version_id, leaderboard_surface, join_reason
		)
		VALUES ($1, $2, 'default', $3)
		ON CONFLICT (group_id, segment_version_id, leaderboard_surface) DO UPDATE
			SET retired_at = NULL,
			    join_reason = EXCLUDED.join_reason
	`, groupID, versionID, decision); err != nil {
		return fmt.Errorf("insert equivalence membership: %w", err)
	}
	return nil
}

func activeLeaderboardEquivalenceGroupTx(ctx context.Context, tx pgx.Tx, segmentID, versionID string) (string, error) {
	var groupID string
	if err := tx.QueryRow(ctx, `
		SELECT gv.group_id
		FROM segment_leaderboard_equivalence_group_versions gv
		JOIN segment_leaderboard_equivalence_groups g ON g.id = gv.group_id
		WHERE gv.segment_version_id = $1
		  AND gv.leaderboard_surface = 'default'
		  AND gv.retired_at IS NULL
		  AND g.segment_id = $2
	`, versionID, segmentID).Scan(&groupID); err != nil {
		return "", fmt.Errorf("load active leaderboard equivalence group: %w", err)
	}
	return groupID, nil
}

func createSegmentRecalculationRunTx(ctx context.Context, tx pgx.Tx, segmentID, actorUserID, operationID, reason string, fromAt, untilAt *time.Time, required bool) (*string, error) {
	if !required {
		return nil, nil
	}
	runID := stableUUID("segment-recalculation-run", segmentID, operationID).String()
	idempotencyKey := "segment:" + segmentID + ":operation:" + stableOperationHash(operationID, "recalc")
	if strings.TrimSpace(reason) == "" {
		reason = "segment version changed"
	}
	var generation int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO segment_recalculation_segment_state (segment_id, generation)
		VALUES ($1, 1)
		ON CONFLICT (segment_id) DO UPDATE
			SET generation = segment_recalculation_segment_state.generation + 1,
			    updated_at = NOW()
		RETURNING generation
	`, segmentID).Scan(&generation); err != nil {
		return nil, fmt.Errorf("advance segment recalculation generation: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO segment_recalculation_runs (
			id, segment_id, actor_user_id, operation_key_hash, idempotency_key,
			generation, reason, requested_from_at, requested_until_at, metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, '{}'::jsonb)
		ON CONFLICT (idempotency_key) DO NOTHING
	`, runID, segmentID, actorUserID, stableOperationHash(operationID, "operation"), idempotencyKey, generation, reason, fromAt, untilAt); err != nil {
		return nil, fmt.Errorf("insert segment recalculation run: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO segment_recalculation_run_activities (run_id, activity_id, user_id)
		SELECT $1, a.id, a.user_id
		FROM activities a
		JOIN segments s ON s.id = $2
		WHERE a.user_id = s.created_by
		  AND ($3::timestamptz IS NULL OR a.started_at >= $3)
		  AND ($4::timestamptz IS NULL OR a.started_at < $4)
		ON CONFLICT (run_id, activity_id) DO NOTHING
	`, runID, segmentID, fromAt, untilAt); err != nil {
		return nil, fmt.Errorf("seed segment recalculation activities: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE segment_recalculation_runs
		SET total_activities = (
			SELECT count(*)
			FROM segment_recalculation_run_activities
			WHERE run_id = $1
		)
		WHERE id = $1
	`, runID); err != nil {
		return nil, fmt.Errorf("update segment recalculation activity count: %w", err)
	}
	return &runID, nil
}

func insertSegmentVersionRevisionTx(ctx context.Context, tx pgx.Tx, segmentID, actorUserID, operation, reason string, before, after SegmentVersionSnapshot, affectedFrom, affectedUntil *time.Time, recalcRunID *string, rollbackOfRevisionID *string, operationID string) (*model.SegmentVersionRevision, error) {
	beforeRaw, beforeHash, err := canonicalSnapshotJSON(before)
	if err != nil {
		return nil, err
	}
	afterRaw, afterHash, err := canonicalSnapshotJSON(after)
	if err != nil {
		return nil, err
	}
	revisionID := stableUUID("segment-version-revision", segmentID, operation, operationID).String()
	var revision model.SegmentVersionRevision
	err = tx.QueryRow(ctx, `
		INSERT INTO segment_version_revisions (
			id, segment_id, actor_user_id, operation, reason,
			snapshot_schema_key, snapshot_schema_version,
			before_snapshot, after_snapshot, before_snapshot_sha256, after_snapshot_sha256,
			affected_from_at, affected_until_at, recalculation_run_id, rollback_of_revision_id
		)
		VALUES (
			$1, $2, $3, $4, NULLIF($5, ''),
			$6, 1,
			$7, $8, $9, $10,
			$11, $12, $13, $14
		)
		ON CONFLICT (id) DO UPDATE
			SET reason = COALESCE(EXCLUDED.reason, segment_version_revisions.reason)
		RETURNING id, segment_id, operation, reason, snapshot_schema_key,
		          snapshot_schema_version, affected_from_at, affected_until_at,
		          recalculation_run_id, rollback_of_revision_id,
		          before_snapshot, after_snapshot, before_snapshot_sha256,
		          after_snapshot_sha256, created_at
	`, revisionID, segmentID, actorUserID, operation, reason, segmentVersionRevisionSchemaKey,
		beforeRaw, afterRaw, beforeHash, afterHash, affectedFrom, affectedUntil, recalcRunID, rollbackOfRevisionID).Scan(
		&revision.ID,
		&revision.SegmentID,
		&revision.Operation,
		&revision.Reason,
		&revision.SnapshotSchemaKey,
		&revision.SnapshotSchemaVersion,
		&revision.AffectedFromAt,
		&revision.AffectedUntilAt,
		&revision.RecalculationRunID,
		&revision.RollbackOfRevisionID,
		&revision.BeforeSnapshot,
		&revision.AfterSnapshot,
		&revision.BeforeSnapshotSHA256,
		&revision.AfterSnapshotSHA256,
		&revision.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert segment version revision: %w", err)
	}
	return &revision, nil
}

func restoreSegmentSnapshotTx(ctx context.Context, tx pgx.Tx, snapshot SegmentVersionSnapshot) error {
	_, err := tx.Exec(ctx, `
		UPDATE segments
		SET segment_type = $2,
		    sport_group = $3,
		    allowed_predicates = $4,
		    bidirectional = $5,
		    visibility = $6,
		    elevation_kind = $7,
		    terrain_type = $8,
		    climb_type = $9,
		    segment_type_meta = $10
		WHERE id = $1
	`, snapshot.Segment.ID, snapshot.Segment.SegmentType, snapshot.Segment.SportGroup,
		nullableRawJSON(snapshot.Segment.AllowedPredicates), snapshot.Segment.Bidirectional,
		snapshot.Segment.Visibility, snapshot.Segment.ElevationKind, snapshot.Segment.TerrainType,
		snapshot.Segment.ClimbType, nullableRawJSON(snapshot.Segment.SegmentTypeMeta))
	if err != nil {
		return fmt.Errorf("restore segment metadata: %w", err)
	}
	for _, version := range snapshot.Versions {
		if _, err := tx.Exec(ctx, `
			UPDATE segment_versions
			SET valid_from_at = $2,
			    valid_until_at = $3,
			    validity_basis = $4,
			    track = ST_SetSRID(ST_GeomFromGeoJSON($5), 4326),
			    start_point = ST_SetSRID(ST_GeomFromGeoJSON($6), 4326),
			    end_point = ST_SetSRID(ST_GeomFromGeoJSON($7), 4326),
			    eligibility_segment_type = $8,
			    eligibility_sport_group = $9,
			    eligibility_allowed_predicates = $10,
			    eligibility_allowed_predicates_set = $11,
			    eligibility_bidirectional = $12,
			    eligibility_visibility = $13,
			    eligibility_elevation_kind = $14
			WHERE id = $1
		`, version.ID, version.ValidFromAt, version.ValidUntilAt, version.ValidityBasis,
			version.TrackGeoJSON, version.StartPointGeoJSON, version.EndPointGeoJSON,
			version.EligibilitySegmentType, version.EligibilitySportGroup,
			nullableRawJSON(version.EligibilityAllowedPredicates),
			version.EligibilityAllowedPredicatesSet, version.EligibilityBidirectional,
			version.EligibilityVisibility, version.EligibilityElevationKind); err != nil {
			return fmt.Errorf("restore segment version %s: %w", version.ID, err)
		}
	}
	return nil
}

func findSnapshotVersion(snapshot SegmentVersionSnapshot, versionID string) (SegmentVersionSnapshotRow, bool) {
	for _, version := range snapshot.Versions {
		if version.ID == versionID {
			return version, true
		}
	}
	return SegmentVersionSnapshotRow{}, false
}

func affectedInterval(oldFrom, oldUntil, newFrom, newUntil *time.Time) (*time.Time, *time.Time) {
	return minLowerBound(oldFrom, newFrom), maxUpperBound(oldUntil, newUntil)
}

func minLowerBound(a, b *time.Time) *time.Time {
	if a == nil || b == nil {
		return nil
	}
	if a.Before(*b) {
		return a
	}
	return b
}

func maxUpperBound(a, b *time.Time) *time.Time {
	if a == nil || b == nil {
		return nil
	}
	if a.After(*b) {
		return a
	}
	return b
}

func intervalTouchesNowOrPast(from *time.Time) bool {
	return from == nil || !from.After(time.Now().UTC())
}

func canonicalSnapshotJSON(snapshot SegmentVersionSnapshot) ([]byte, string, error) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:]), nil
}

func stableUUID(parts ...string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(strings.Join(parts, "\x00")))
}

func stableOperationHash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func cloneRawMessage(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return json.RawMessage(out)
}

func nullableRawJSON(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return []byte(raw)
}

func segmentVersioningPgError(action string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23P01", "23514":
			return fmt.Errorf("%w: %s: %s", ErrSegmentValidation, action, pgErr.Message)
		}
	}
	return fmt.Errorf("%s: %w", action, err)
}
