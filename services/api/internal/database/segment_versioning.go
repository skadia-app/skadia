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
	CourseKind        string          `json:"course_kind"`
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
	ID                              string                    `json:"id"`
	VersionNumber                   int                       `json:"version_number"`
	ValidFromAt                     *time.Time                `json:"valid_from_at,omitempty"`
	ValidUntilAt                    *time.Time                `json:"valid_until_at,omitempty"`
	ValidityBasis                   string                    `json:"validity_basis"`
	MatchingContractKey             string                    `json:"matching_contract_key"`
	MatchingContractVersion         int                       `json:"matching_contract_version"`
	TrackGeoJSON                    string                    `json:"track_geojson"`
	StartPointGeoJSON               string                    `json:"start_point_geojson"`
	EndPointGeoJSON                 string                    `json:"end_point_geojson"`
	EligibilitySegmentType          string                    `json:"eligibility_segment_type"`
	EligibilitySportGroup           string                    `json:"eligibility_sport_group"`
	EligibilityAllowedPredicates    json.RawMessage           `json:"eligibility_allowed_predicates,omitempty"`
	EligibilityAllowedPredicatesSet bool                      `json:"eligibility_allowed_predicates_set"`
	EligibilityBidirectional        bool                      `json:"eligibility_bidirectional"`
	EligibilityVisibility           string                    `json:"eligibility_visibility"`
	EligibilityElevationKind        string                    `json:"eligibility_elevation_kind"`
	Controls                        []model.CheckpointControl `json:"controls,omitempty"`
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
		SELECT id, created_by, name, segment_type, course_kind, sport_group,
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
		&snapshot.Segment.CourseKind,
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
	for i := range snapshot.Versions {
		if snapshot.Versions[i].MatchingContractKey != CheckpointSequenceV1Key {
			continue
		}
		controls, err := getCheckpointControlsTx(ctx, tx, snapshot.Versions[i].ID)
		if err != nil {
			return snapshot, err
		}
		snapshot.Versions[i].Controls = controls
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
	beforeHash, err := segmentVersionSnapshotHash(before)
	if err != nil {
		return nil, err
	}
	oldVersion, ok := findSnapshotVersion(before, versionID)
	if !ok {
		return nil, ErrSegmentNotFound
	}
	inputHash, err := segmentVersionOperationInputHash("validity_update", map[string]any{
		"version_id":           versionID,
		"valid_from_at":        update.ValidFromAt,
		"valid_until_at":       update.ValidUntilAt,
		"recalculate":          update.Recalculate,
		"equivalence_decision": update.EquivalenceDecision,
		"reason":               update.Reason,
	})
	if err != nil {
		return nil, err
	}
	if existing, err := loadExistingSegmentVersionRevisionTx(ctx, tx, segmentID, "validity_update", update.OperationID, inputHash, beforeHash, &versionID); err != nil {
		return nil, err
	} else if existing != nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit existing segment validity revision: %w", err)
		}
		return existing, nil
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
	isCheckpoint, err := segmentIsCheckpointCourseTx(ctx, tx, segmentID)
	if err != nil {
		return nil, err
	}
	if isCheckpoint {
		if err := rebuildCheckpointCourseProjectionTx(ctx, tx, segmentID); err != nil {
			return nil, err
		}
	}
	if err := applyEquivalenceDecisionTx(ctx, tx, segmentID, versionID, "", actorUserID, update.OperationID, update.EquivalenceDecision, update.Reason, nil); err != nil {
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
	revision, err := insertSegmentVersionRevisionTx(ctx, tx, segmentID, actorUserID, "validity_update", update.Reason, before, after, affectedFrom, affectedUntil, recalcRunID, nil, update.OperationID, inputHash, &versionID, beforeHash)
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
	beforeHash, err := segmentVersionSnapshotHash(before)
	if err != nil {
		return nil, err
	}
	oldVersion, ok := findSnapshotVersion(before, versionID)
	if !ok {
		return nil, ErrSegmentNotFound
	}
	if oldVersion.MatchingContractKey == CheckpointSequenceV1Key {
		return nil, fmt.Errorf("%w: checkpoint course controls require a checkpoint version operation", ErrSegmentValidation)
	}
	inputHash, err := segmentVersionOperationInputHash("geometry_update", map[string]any{
		"version_id":           versionID,
		"track_geojson":        update.TrackGeoJSON,
		"start_point_geojson":  update.StartPointGeoJSON,
		"end_point_geojson":    update.EndPointGeoJSON,
		"equivalence_decision": update.EquivalenceDecision,
		"equivalence_evidence": update.EquivalenceEvidence,
		"recalculate":          update.Recalculate,
		"reason":               update.Reason,
	})
	if err != nil {
		return nil, err
	}
	if existing, err := loadExistingSegmentVersionRevisionTx(ctx, tx, segmentID, "geometry_update", update.OperationID, inputHash, beforeHash, &versionID); err != nil {
		return nil, err
	} else if existing != nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit existing segment geometry revision: %w", err)
		}
		return existing, nil
	}
	boundary := time.Now().UTC()
	affectedFrom := &boundary
	affectedUntil := oldVersion.ValidUntilAt
	if err := validateSegmentGeometryUpdateTx(ctx, tx, update); err != nil {
		return nil, err
	}

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
	trustedEvidence := map[string]any(nil)
	equivalenceDecision := strings.TrimSpace(update.EquivalenceDecision)
	if equivalenceDecision == "" || equivalenceDecision == "continue_group" {
		trustedEvidence, err = trustedSegmentVersionCompatibilityEvidenceTx(ctx, tx, segmentID, versionID, newVersionID, actorUserID)
		if err != nil {
			return nil, err
		}
	}
	if err := applyEquivalenceDecisionTx(ctx, tx, segmentID, newVersionID, versionID, actorUserID, update.OperationID, update.EquivalenceDecision, update.Reason, trustedEvidence); err != nil {
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
	revision, err := insertSegmentVersionRevisionTx(ctx, tx, segmentID, actorUserID, "geometry_update", update.Reason, before, after, affectedFrom, affectedUntil, recalcRunID, nil, update.OperationID, inputHash, &versionID, beforeHash)
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
	operationID := "rollback:" + revisionID
	before, err := LoadSegmentVersionSnapshotTx(ctx, tx, segmentID)
	if err != nil {
		return nil, err
	}
	beforeHash, err := segmentVersionSnapshotHash(before)
	if err != nil {
		return nil, err
	}
	inputHash, err := segmentVersionOperationInputHash("rollback_revision", map[string]any{
		"revision_id": revisionID,
		"reason":      reason,
	})
	if err != nil {
		return nil, err
	}
	if existing, err := loadExistingSegmentVersionRevisionTx(ctx, tx, segmentID, "rollback_revision", operationID, inputHash, beforeHash, nil); err != nil {
		return nil, err
	} else if existing != nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit existing segment rollback revision: %w", err)
		}
		return existing, nil
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
	isCheckpoint, err := segmentIsCheckpointCourseTx(ctx, tx, segmentID)
	if err != nil {
		return nil, err
	}
	if isCheckpoint {
		if err := rebuildCheckpointCourseProjectionTx(ctx, tx, segmentID); err != nil {
			return nil, err
		}
	}
	recalcRunID, err := createSegmentRecalculationRunTx(ctx, tx, segmentID, actorUserID, operationID, reason, affectedFrom, affectedUntil, true)
	if err != nil {
		return nil, err
	}
	after, err := LoadSegmentVersionSnapshotTx(ctx, tx, segmentID)
	if err != nil {
		return nil, err
	}
	rollbackOf := revisionID
	revision, err := insertSegmentVersionRevisionTx(ctx, tx, segmentID, actorUserID, "rollback_revision", reason, before, after, affectedFrom, affectedUntil, recalcRunID, &rollbackOf, operationID, inputHash, nil, beforeHash)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit segment revision rollback: %w", err)
	}
	return revision, nil
}

func (db *DB) ListSegmentVersionRevisions(ctx context.Context, segmentID, actorUserID string, isAdmin bool, limit, offset int) ([]model.SegmentVersionRevision, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin list segment revisions: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := authorizeSegmentOwnerTx(ctx, tx, segmentID, actorUserID, isAdmin); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, segment_id, operation, reason,
		       snapshot_schema_key, snapshot_schema_version,
		       affected_from_at, affected_until_at,
		       recalculation_run_id, rollback_of_revision_id,
		       before_snapshot, after_snapshot,
		       before_snapshot_sha256, after_snapshot_sha256, created_at
		FROM segment_version_revisions
		WHERE segment_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, segmentID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list segment revisions: %w", err)
	}
	defer rows.Close()
	revisions, err := scanSegmentVersionRevisions(rows)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit list segment revisions: %w", err)
	}
	return revisions, nil
}

func (db *DB) GetSegmentVersionRevision(ctx context.Context, segmentID, revisionID, actorUserID string, isAdmin bool) (*model.SegmentVersionRevision, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin get segment revision: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := authorizeSegmentOwnerTx(ctx, tx, segmentID, actorUserID, isAdmin); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, segment_id, operation, reason,
		       snapshot_schema_key, snapshot_schema_version,
		       affected_from_at, affected_until_at,
		       recalculation_run_id, rollback_of_revision_id,
		       before_snapshot, after_snapshot,
		       before_snapshot_sha256, after_snapshot_sha256, created_at
		FROM segment_version_revisions
		WHERE segment_id = $1 AND id = $2
	`, segmentID, revisionID)
	if err != nil {
		return nil, fmt.Errorf("get segment revision: %w", err)
	}
	defer rows.Close()
	revisions, err := scanSegmentVersionRevisions(rows)
	if err != nil {
		return nil, err
	}
	if len(revisions) == 0 {
		return nil, ErrSegmentNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit get segment revision: %w", err)
	}
	return &revisions[0], nil
}

func scanSegmentVersionRevisions(rows pgx.Rows) ([]model.SegmentVersionRevision, error) {
	var revisions []model.SegmentVersionRevision
	for rows.Next() {
		var revision model.SegmentVersionRevision
		var beforeRaw, afterRaw []byte
		if err := rows.Scan(
			&revision.ID, &revision.SegmentID, &revision.Operation, &revision.Reason,
			&revision.SnapshotSchemaKey, &revision.SnapshotSchemaVersion,
			&revision.AffectedFromAt, &revision.AffectedUntilAt,
			&revision.RecalculationRunID, &revision.RollbackOfRevisionID,
			&beforeRaw, &afterRaw,
			&revision.BeforeSnapshotSHA256, &revision.AfterSnapshotSHA256, &revision.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan segment revision: %w", err)
		}
		revision.BeforeSnapshot = cloneRawMessage(beforeRaw)
		revision.AfterSnapshot = cloneRawMessage(afterRaw)
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("segment revision rows: %w", err)
	}
	if revisions == nil {
		revisions = []model.SegmentVersionRevision{}
	}
	return revisions, nil
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
			  AND validity_basis <> 'rollback_retired'
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
			  AND validity_basis <> 'rollback_retired'
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
	if err := applyEquivalenceDecisionTx(ctx, tx, segmentID, versionID, "", actorUserID, operationID, "new_group", "reverse", nil); err != nil {
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
	beforeHash, err := segmentVersionSnapshotHash(before)
	if err != nil {
		return "", "", err
	}
	inputHash, err := segmentVersionOperationInputHash("reverse", map[string]any{"operation": "reverse"})
	if err != nil {
		return "", "", err
	}
	if _, err := insertSegmentVersionRevisionTx(ctx, tx, segmentID, actorUserID, "reverse", "reverse", before, after, &boundary, nil, recalcRunID, nil, operationID, inputHash, nil, beforeHash); err != nil {
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

func applyEquivalenceDecisionTx(ctx context.Context, tx pgx.Tx, segmentID, versionID, continueFromVersionID, actorUserID, operationID, decision, reason string, evidence map[string]any) error {
	decision = strings.TrimSpace(decision)
	if decision == "" {
		decision = "continue_group"
	}
	var groupID string
	var joinSourceVersionID string
	if decision == "continue_group" {
		if strings.TrimSpace(continueFromVersionID) != "" {
			if err := validateTrustedSegmentVersionCompatibilityEvidence(segmentID, continueFromVersionID, versionID, actorUserID, evidence); err != nil {
				return err
			}
		}
		var currentGroupID string
		err := tx.QueryRow(ctx, `
			SELECT group_id
			FROM segment_leaderboard_equivalence_group_versions
			WHERE segment_version_id = $1
			  AND leaderboard_surface = 'default'
			  AND retired_at IS NULL
			LIMIT 1
		`, versionID).Scan(&currentGroupID)
		if err == nil && currentGroupID != "" && strings.TrimSpace(continueFromVersionID) == "" {
			return nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("load current equivalence group: %w", err)
		}
		if strings.TrimSpace(continueFromVersionID) != "" {
			var sourceGroupKind string
			err = tx.QueryRow(ctx, `
				SELECT gv.group_id, g.equivalence_kind
				FROM segment_leaderboard_equivalence_group_versions gv
				JOIN segment_leaderboard_equivalence_groups g ON g.id = gv.group_id
				WHERE gv.segment_version_id = $1
				  AND gv.leaderboard_surface = 'default'
				  AND gv.retired_at IS NULL
				LIMIT 1
			`, continueFromVersionID).Scan(&groupID, &sourceGroupKind)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("%w: source version has no active equivalence group to continue", ErrSegmentValidation)
				}
				return fmt.Errorf("load source equivalence group: %w", err)
			}
			if sourceGroupKind == "version_specific" {
				joinSourceVersionID = continueFromVersionID
				groupID = ""
			}
		} else {
			decision = "version_specific"
		}
	}
	if decision != "continue_group" && decision != "new_group" && decision != "version_specific" {
		return fmt.Errorf("%w: unsupported equivalence decision %q", ErrSegmentValidation, decision)
	}
	if groupID == "" {
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
	}
	if _, err := tx.Exec(ctx, `
		UPDATE segment_leaderboard_equivalence_group_versions
		SET retired_at = GREATEST(NOW(), joined_at + interval '1 microsecond')
		WHERE segment_version_id = ANY($1::uuid[])
		  AND leaderboard_surface = 'default'
		  AND retired_at IS NULL
	`, equivalenceRetireVersionIDs(versionID, joinSourceVersionID)); err != nil {
		return fmt.Errorf("retire previous equivalence membership: %w", err)
	}
	if joinSourceVersionID != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO segment_leaderboard_equivalence_group_versions (
				group_id, segment_version_id, leaderboard_surface, join_reason
			)
			VALUES ($1, $2, 'default', $3)
			ON CONFLICT (group_id, segment_version_id, leaderboard_surface) DO UPDATE
				SET retired_at = NULL,
				    join_reason = EXCLUDED.join_reason
		`, groupID, joinSourceVersionID, decision); err != nil {
			return fmt.Errorf("insert source equivalence membership: %w", err)
		}
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
	if err := UpdateSegmentEffortRanksTx(ctx, tx, segmentID); err != nil {
		return fmt.Errorf("refresh segment effort ranks after equivalence change: %w", err)
	}
	return nil
}

func trustedSegmentVersionCompatibilityEvidenceTx(ctx context.Context, tx pgx.Tx, segmentID, targetVersionID, newVersionID, actorUserID string) (map[string]any, error) {
	var targetVersionNumber, newVersionNumber int
	var targetContractKey, newContractKey string
	var targetContractVersion, newContractVersion int
	var targetLengthM, lengthM, startDistanceM, endDistanceM, reverseStartDistanceM, reverseEndDistanceM float64
	if err := tx.QueryRow(ctx, `
		WITH target AS (
			SELECT id, segment_id, version_number, track, start_point, end_point,
			       matching_contract_key, matching_contract_version
			FROM segment_versions
			WHERE id = $2 AND segment_id = $1
		),
		candidate AS (
			SELECT id, segment_id, version_number, track, start_point, end_point,
			       matching_contract_key, matching_contract_version
			FROM segment_versions
			WHERE id = $3 AND segment_id = $1
		)
		SELECT target.version_number,
		       candidate.version_number,
		       target.matching_contract_key,
		       target.matching_contract_version,
		       candidate.matching_contract_key,
		       candidate.matching_contract_version,
		       ST_Length(target.track::geography),
		       ST_Length(candidate.track::geography),
		       ST_Distance(ST_StartPoint(target.track)::geography, ST_StartPoint(candidate.track)::geography),
		       ST_Distance(ST_EndPoint(target.track)::geography, ST_EndPoint(candidate.track)::geography),
		       ST_Distance(ST_StartPoint(target.track)::geography, ST_EndPoint(candidate.track)::geography),
		       ST_Distance(ST_EndPoint(target.track)::geography, ST_StartPoint(candidate.track)::geography)
		FROM target, candidate
	`, segmentID, targetVersionID, newVersionID).Scan(
		&targetVersionNumber, &newVersionNumber,
		&targetContractKey, &targetContractVersion,
		&newContractKey, &newContractVersion,
		&targetLengthM, &lengthM,
		&startDistanceM, &endDistanceM,
		&reverseStartDistanceM, &reverseEndDistanceM,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: compatibility versions must belong to the segment", ErrSegmentValidation)
		}
		return nil, fmt.Errorf("build segment version compatibility evidence: %w", err)
	}
	if targetContractKey != newContractKey || targetContractVersion != newContractVersion {
		return nil, fmt.Errorf("%w: cannot continue equivalence group across matching contracts", ErrSegmentValidation)
	}
	direction := "forward"
	if reverseStartDistanceM+reverseEndDistanceM < startDistanceM+endDistanceM {
		direction = "reverse"
	}
	lengthDeltaM := lengthM - targetLengthM
	lengthDeltaRatio := 0.0
	if targetLengthM > 0 {
		lengthDeltaRatio = lengthDeltaM / targetLengthM
	}
	return map[string]any{
		"source":                    "server",
		"policy_key":                "segment_version_compatibility",
		"policy_version":            1,
		"actor_user_id":             actorUserID,
		"target_version_id":         targetVersionID,
		"new_version_id":            newVersionID,
		"target_version_number":     targetVersionNumber,
		"new_version_number":        newVersionNumber,
		"matching_contract_key":     newContractKey,
		"matching_contract_version": newContractVersion,
		"target_length_m":           targetLengthM,
		"length_m":                  lengthM,
		"length_delta_m":            lengthDeltaM,
		"length_delta_ratio":        lengthDeltaRatio,
		"start_distance_m":          startDistanceM,
		"end_distance_m":            endDistanceM,
		"reverse_start_distance_m":  reverseStartDistanceM,
		"reverse_end_distance_m":    reverseEndDistanceM,
		"direction":                 direction,
		"compatible":                true,
	}, nil
}

func validateTrustedSegmentVersionCompatibilityEvidence(segmentID, targetVersionID, newVersionID, actorUserID string, evidence map[string]any) error {
	if len(evidence) == 0 {
		return fmt.Errorf("%w: trusted server compatibility evidence is required when a new geometry continues an existing leaderboard group", ErrSegmentValidation)
	}
	required := []string{
		"source", "actor_user_id", "target_version_id", "new_version_id",
		"matching_contract_key", "matching_contract_version", "target_length_m",
		"length_m", "length_delta_ratio", "start_distance_m", "end_distance_m",
		"direction", "compatible",
	}
	for _, key := range required {
		if _, ok := evidence[key]; !ok {
			return fmt.Errorf("%w: trusted compatibility evidence missing %s", ErrSegmentValidation, key)
		}
	}
	if fmt.Sprint(evidence["source"]) != "server" ||
		fmt.Sprint(evidence["actor_user_id"]) != actorUserID ||
		fmt.Sprint(evidence["target_version_id"]) != targetVersionID ||
		fmt.Sprint(evidence["new_version_id"]) != newVersionID {
		return fmt.Errorf("%w: compatibility evidence is not trusted for this equivalence operation", ErrSegmentValidation)
	}
	if fmt.Sprint(evidence["matching_contract_key"]) == "" || fmt.Sprint(evidence["direction"]) == "" {
		return fmt.Errorf("%w: trusted compatibility evidence is incomplete", ErrSegmentValidation)
	}
	_ = segmentID
	return nil
}

func equivalenceRetireVersionIDs(versionID, sourceVersionID string) []string {
	if sourceVersionID == "" {
		return []string{versionID}
	}
	return []string{versionID, sourceVersionID}
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
	if strings.TrimSpace(reason) == "" {
		reason = "segment version changed"
	}
	var ownerUserID, visibility, segmentName string
	if err := tx.QueryRow(ctx, `
		SELECT created_by, visibility, name
		FROM segments
		WHERE id = $1
		FOR UPDATE
	`, segmentID).Scan(&ownerUserID, &visibility, &segmentName); err != nil {
		return nil, fmt.Errorf("load segment recalculation scope: %w", err)
	}
	scope := SegmentRecalculationScope{
		SegmentID:       segmentID,
		OwnerUserID:     ownerUserID,
		IncludeAllUsers: visibility == "public",
		OperationID:     operationID,
		FromAt:          fromAt,
		UntilAt:         untilAt,
	}
	metadata, err := json.Marshal(map[string]any{
		"source":      "segment_version_workflow",
		"visibility":  "user_visible",
		"entity_name": segmentName,
	})
	if err != nil {
		return nil, err
	}
	idempotencyKey := segmentRecalculationIdempotencyKey(scope)
	runID, _, err := seedSegmentRecalculationRunTx(ctx, tx, actorUserID, scope, reason, idempotencyKey, metadata)
	if err != nil {
		return nil, err
	}
	return &runID, nil
}

func insertSegmentVersionRevisionTx(ctx context.Context, tx pgx.Tx, segmentID, actorUserID, operation, reason string, before, after SegmentVersionSnapshot, affectedFrom, affectedUntil *time.Time, recalcRunID *string, rollbackOfRevisionID *string, operationID, operationInputHash string, targetVersionID *string, expectedBeforeSnapshotSHA256 string) (*model.SegmentVersionRevision, error) {
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
			affected_from_at, affected_until_at, recalculation_run_id, rollback_of_revision_id,
			operation_input_hash, target_version_id, expected_before_snapshot_sha256
		)
		VALUES (
			$1, $2, $3, $4, NULLIF($5, ''),
			$6, 1,
			$7, $8, $9, $10,
			$11, $12, $13, $14,
			NULLIF($15, ''), $16, NULLIF($17, '')
		)
		ON CONFLICT (id) DO UPDATE
			SET reason = COALESCE(EXCLUDED.reason, segment_version_revisions.reason)
		RETURNING id, segment_id, operation, reason, snapshot_schema_key,
		          snapshot_schema_version, affected_from_at, affected_until_at,
		          recalculation_run_id, rollback_of_revision_id,
		          before_snapshot, after_snapshot, before_snapshot_sha256,
		          after_snapshot_sha256, created_at
	`, revisionID, segmentID, actorUserID, operation, reason, segmentVersionRevisionSchemaKey,
		beforeRaw, afterRaw, beforeHash, afterHash, affectedFrom, affectedUntil, recalcRunID, rollbackOfRevisionID,
		operationInputHash, targetVersionID, expectedBeforeSnapshotSHA256).Scan(
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

func loadExistingSegmentVersionRevisionTx(ctx context.Context, tx pgx.Tx, segmentID, operation, operationID, expectedOperationInputHash, expectedBeforeSnapshotSHA256 string, targetVersionID *string) (*model.SegmentVersionRevision, error) {
	revisionID := stableUUID("segment-version-revision", segmentID, operation, operationID).String()
	rows, err := tx.Query(ctx, `
		SELECT id, segment_id, operation, reason,
		       snapshot_schema_key, snapshot_schema_version,
		       affected_from_at, affected_until_at,
		       recalculation_run_id, rollback_of_revision_id,
		       before_snapshot, after_snapshot,
		       before_snapshot_sha256, after_snapshot_sha256, created_at,
		       COALESCE(operation_input_hash, ''),
		       COALESCE(target_version_id::text, ''),
		       COALESCE(expected_before_snapshot_sha256, '')
		FROM segment_version_revisions
		WHERE id = $1 AND segment_id = $2
	`, revisionID, segmentID)
	if err != nil {
		return nil, fmt.Errorf("load existing segment version revision: %w", err)
	}
	defer rows.Close()
	var revision model.SegmentVersionRevision
	var beforeRaw, afterRaw []byte
	var existingOperationInputHash, existingTargetVersionID, existingExpectedBeforeHash string
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("load existing segment version revision: %w", err)
		}
		return nil, nil
	}
	if err := rows.Scan(
		&revision.ID, &revision.SegmentID, &revision.Operation, &revision.Reason,
		&revision.SnapshotSchemaKey, &revision.SnapshotSchemaVersion,
		&revision.AffectedFromAt, &revision.AffectedUntilAt,
		&revision.RecalculationRunID, &revision.RollbackOfRevisionID,
		&beforeRaw, &afterRaw,
		&revision.BeforeSnapshotSHA256, &revision.AfterSnapshotSHA256, &revision.CreatedAt,
		&existingOperationInputHash, &existingTargetVersionID, &existingExpectedBeforeHash,
	); err != nil {
		return nil, fmt.Errorf("scan existing segment revision: %w", err)
	}
	if rows.Next() {
		return nil, fmt.Errorf("%w: duplicate segment revision operation id", ErrSegmentValidation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load existing segment version revision: %w", err)
	}
	revision.BeforeSnapshot = cloneRawMessage(beforeRaw)
	revision.AfterSnapshot = cloneRawMessage(afterRaw)
	if expectedOperationInputHash != "" && existingOperationInputHash != "" && existingOperationInputHash != expectedOperationInputHash {
		return nil, fmt.Errorf("%w: operation_id was already used with different input", ErrSegmentValidation)
	}
	if targetVersionID != nil && existingTargetVersionID != "" && existingTargetVersionID != *targetVersionID {
		return nil, fmt.Errorf("%w: operation_id was already used for a different target version", ErrSegmentValidation)
	}
	if expectedBeforeSnapshotSHA256 != "" && existingExpectedBeforeHash != "" && existingExpectedBeforeHash != expectedBeforeSnapshotSHA256 {
		return nil, fmt.Errorf("%w: operation_id replay no longer matches current segment state", ErrSegmentValidation)
	}
	return &revision, nil
}

func restoreSegmentSnapshotTx(ctx context.Context, tx pgx.Tx, snapshot SegmentVersionSnapshot) error {
	_, err := tx.Exec(ctx, `
		UPDATE segments
		SET segment_type = $2,
		    course_kind = $3,
		    sport_group = $4,
		    allowed_predicates = $5,
		    bidirectional = $6,
		    visibility = $7,
		    elevation_kind = $8,
		    terrain_type = $9,
		    climb_type = $10,
		    segment_type_meta = $11
		WHERE id = $1
	`, snapshot.Segment.ID, snapshot.Segment.SegmentType, snapshot.Segment.CourseKind, snapshot.Segment.SportGroup,
		nullableRawJSON(snapshot.Segment.AllowedPredicates), snapshot.Segment.Bidirectional,
		snapshot.Segment.Visibility, snapshot.Segment.ElevationKind, snapshot.Segment.TerrainType,
		snapshot.Segment.ClimbType, nullableRawJSON(snapshot.Segment.SegmentTypeMeta))
	if err != nil {
		return fmt.Errorf("restore segment metadata: %w", err)
	}
	if err := retireVersionsAbsentFromSnapshotTx(ctx, tx, snapshot); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM course_version_controls WHERE segment_id = $1`, snapshot.SegmentID); err != nil {
		return fmt.Errorf("clear checkpoint controls before restore: %w", err)
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
			    eligibility_elevation_kind = $14,
			    matching_contract_key = $15,
			    matching_contract_version = $16
			WHERE id = $1
		`, version.ID, version.ValidFromAt, version.ValidUntilAt, version.ValidityBasis,
			version.TrackGeoJSON, version.StartPointGeoJSON, version.EndPointGeoJSON,
			version.EligibilitySegmentType, version.EligibilitySportGroup,
			nullableRawJSON(version.EligibilityAllowedPredicates),
			version.EligibilityAllowedPredicatesSet, version.EligibilityBidirectional,
			version.EligibilityVisibility, version.EligibilityElevationKind,
			version.MatchingContractKey, version.MatchingContractVersion); err != nil {
			return fmt.Errorf("restore segment version %s: %w", version.ID, err)
		}
		for _, control := range version.Controls {
			if _, err := tx.Exec(ctx, `
				INSERT INTO course_version_controls (
					id, segment_id, segment_version_id, sequence_index, role, primitive,
					geometry_shape, label, geom, hit_radius_m, area_radius_m, required
				)
				VALUES (
					COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6,
					$7, $8, ST_SetSRID(ST_GeomFromGeoJSON($9), 4326), $10, $11, $12
				)
			`, control.ID, snapshot.SegmentID, version.ID, control.SequenceIndex, control.Role,
				control.Primitive, control.GeometryShape, control.Label, string(control.Geometry),
				control.HitRadiusM, control.AreaRadiusM, control.Required); err != nil {
				return fmt.Errorf("restore checkpoint control %s/%d: %w", version.ID, control.SequenceIndex, err)
			}
		}
	}
	if err := restoreSegmentEquivalenceSnapshotTx(ctx, tx, snapshot); err != nil {
		return err
	}
	return nil
}

func retireVersionsAbsentFromSnapshotTx(ctx context.Context, tx pgx.Tx, snapshot SegmentVersionSnapshot) error {
	versionIDs := make([]string, 0, len(snapshot.Versions))
	for _, version := range snapshot.Versions {
		versionIDs = append(versionIDs, version.ID)
	}
	_, err := tx.Exec(ctx, `
		UPDATE segment_versions
		SET validity_basis = 'rollback_retired'
		WHERE segment_id = $1
		  AND NOT (id = ANY($2::uuid[]))
	`, snapshot.SegmentID, versionIDs)
	if err != nil {
		return fmt.Errorf("retire segment versions absent from rollback snapshot: %w", err)
	}
	return nil
}

func restoreSegmentEquivalenceSnapshotTx(ctx context.Context, tx pgx.Tx, snapshot SegmentVersionSnapshot) error {
	if _, err := tx.Exec(ctx, `
		UPDATE segment_leaderboard_equivalence_group_versions gv
		SET retired_at = GREATEST(NOW(), gv.joined_at + interval '1 microsecond')
		FROM segment_leaderboard_equivalence_groups g
		WHERE g.id = gv.group_id
		  AND g.segment_id = $1
		  AND gv.retired_at IS NULL
	`, snapshot.SegmentID); err != nil {
		return fmt.Errorf("retire segment equivalence memberships for rollback: %w", err)
	}
	for _, group := range snapshot.EquivalenceGroups {
		metadata := nullableRawJSON(group.Metadata)
		if metadata == nil {
			metadata = []byte(`{}`)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO segment_leaderboard_equivalence_groups (
				id, segment_id, change_key, policy_key, policy_version, equivalence_kind, reason, metadata
			)
			VALUES ($1, $2, $3, $4, $5, $6, NULL, COALESCE($7::jsonb, '{}'::jsonb))
			ON CONFLICT (id) DO UPDATE
				SET change_key = EXCLUDED.change_key,
				    policy_key = EXCLUDED.policy_key,
				    policy_version = EXCLUDED.policy_version,
				    equivalence_kind = EXCLUDED.equivalence_kind,
				    metadata = EXCLUDED.metadata
		`, group.GroupID, snapshot.SegmentID, group.ChangeKey, group.PolicyKey, group.PolicyVersion, group.EquivalenceKind, metadata); err != nil {
			return fmt.Errorf("restore segment equivalence group %s: %w", group.GroupID, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO segment_leaderboard_equivalence_group_versions (
				group_id, segment_version_id, leaderboard_surface, joined_at, retired_at, join_reason
			)
			VALUES (
				$1, $2, $3,
				CASE WHEN $4::timestamptz IS NULL THEN NOW() ELSE $4::timestamptz - interval '1 microsecond' END,
				$4, $5
			)
			ON CONFLICT (group_id, segment_version_id, leaderboard_surface) DO UPDATE
				SET retired_at = EXCLUDED.retired_at,
				    join_reason = EXCLUDED.join_reason
		`, group.GroupID, group.SegmentVersionID, group.LeaderboardSurface, group.RetiredAt, group.JoinReason); err != nil {
			return fmt.Errorf("restore segment equivalence membership %s/%s: %w", group.GroupID, group.SegmentVersionID, err)
		}
	}
	return nil
}

func validateSegmentGeometryUpdateTx(ctx context.Context, tx pgx.Tx, update model.SegmentGeometryUpdate) error {
	var valid bool
	err := tx.QueryRow(ctx, `
		WITH g AS (
			SELECT ST_SetSRID(ST_GeomFromGeoJSON($1), 4326) AS track
		)
		SELECT ST_GeometryType(track) = 'ST_LineString'
		   AND ST_NPoints(track) >= 2
		FROM g
	`, string(update.TrackGeoJSON)).Scan(&valid)
	if err != nil {
		return segmentVersioningPgError("validate segment geometry", err)
	}
	if !valid {
		return fmt.Errorf("%w: track_geojson must be a LineString with at least two points", ErrSegmentValidation)
	}
	if len(update.StartPointGeoJSON) > 0 {
		if err := validateGeometryEndpointTx(ctx, tx, update.TrackGeoJSON, update.StartPointGeoJSON, true); err != nil {
			return err
		}
	}
	if len(update.EndPointGeoJSON) > 0 {
		if err := validateGeometryEndpointTx(ctx, tx, update.TrackGeoJSON, update.EndPointGeoJSON, false); err != nil {
			return err
		}
	}
	return nil
}

func validateGeometryEndpointTx(ctx context.Context, tx pgx.Tx, trackRaw, pointRaw json.RawMessage, start bool) error {
	endpointSQL := "ST_EndPoint(track)"
	label := "end_point_geojson"
	if start {
		endpointSQL = "ST_StartPoint(track)"
		label = "start_point_geojson"
	}
	var matches bool
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		WITH g AS (
			SELECT ST_SetSRID(ST_GeomFromGeoJSON($1), 4326) AS track,
			       ST_SetSRID(ST_GeomFromGeoJSON($2), 4326) AS point
		)
		SELECT ST_GeometryType(point) = 'ST_Point'
		   AND ST_DWithin(%s::geography, point::geography, 0.01)
		FROM g
	`, endpointSQL), string(trackRaw), string(pointRaw)).Scan(&matches)
	if err != nil {
		return segmentVersioningPgError("validate segment geometry endpoint", err)
	}
	if !matches {
		return fmt.Errorf("%w: %s must match the track endpoint", ErrSegmentValidation, label)
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

func segmentVersionSnapshotHash(snapshot SegmentVersionSnapshot) (string, error) {
	_, hash, err := canonicalSnapshotJSON(snapshot)
	return hash, err
}

func segmentVersionOperationInputHash(operation string, payload any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal segment version operation input: %w", err)
	}
	return stableOperationHash(operation, string(raw)), nil
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
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case "23P01", "23514":
			return fmt.Errorf("%w: %s: %s", ErrSegmentValidation, action, pgErr.Message)
		}
	}
	return fmt.Errorf("%s: %w", action, err)
}
