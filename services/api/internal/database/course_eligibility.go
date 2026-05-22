package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

const (
	courseEligibilityPolicyKey      = "course_eligibility"
	courseEligibilityPolicyVersion  = 1
	courseEligibilityAdapterKey     = "legacy_segment_adapter_v1"
	courseEligibilityAdapterVersion = 1
	courseEligibilityBatchSize      = 1000
	courseEligibilityCopyChunkSize  = 100000
)

func (db *DB) RebuildActivityTraitProjection(ctx context.Context) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin activity trait projection rebuild: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := rebuildActivityTraitProjectionTx(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit activity trait projection rebuild: %w", err)
	}
	return nil
}

func (db *DB) RebuildSegmentCourseEligibilityProjection(ctx context.Context, segmentID string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin course eligibility projection rebuild: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, segmentID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit course eligibility projection rebuild: %w", err)
	}
	return nil
}

func (db *DB) RebuildSegmentCourseEligibilityProjectionTx(ctx context.Context, tx pgx.Tx, segmentID string) error {
	return db.rebuildSegmentCourseEligibilityProjectionTx(ctx, tx, segmentID, true)
}

func syncActiveSegmentVersionEligibilitySnapshotTx(ctx context.Context, tx pgx.Tx, segmentID string) error {
	_, err := tx.Exec(ctx, `
		WITH current_segment AS (
			SELECT segment_type,
			       sport_group,
			       allowed_predicates,
			       (allowed_predicates IS NOT NULL) AS allowed_predicates_set,
			       bidirectional,
			       COALESCE(visibility, 'public') AS visibility,
			       elevation_kind
			FROM segments
			WHERE id = $1
		),
		active_version AS (
			SELECT id
			FROM segment_versions
			WHERE segment_id = $1
			  AND matching_contract_key = 'course_gps_v1'
			  AND matching_contract_version = 1
			  AND validity_basis <> 'rollback_retired'
			  AND (valid_from_at IS NULL OR valid_from_at <= NOW())
			  AND (valid_until_at IS NULL OR NOW() < valid_until_at)
			ORDER BY COALESCE(valid_from_at, '-infinity'::timestamptz) DESC,
			         version_number DESC
			LIMIT 1
		)
		UPDATE segment_versions sv
		SET eligibility_segment_type = cs.segment_type,
		    eligibility_sport_group = cs.sport_group,
		    eligibility_allowed_predicates = cs.allowed_predicates,
		    eligibility_allowed_predicates_set = cs.allowed_predicates_set,
		    eligibility_bidirectional = cs.bidirectional,
		    eligibility_visibility = cs.visibility,
		    eligibility_elevation_kind = cs.elevation_kind
		FROM current_segment cs, active_version av
		WHERE sv.id = av.id
	`, segmentID)
	if err != nil {
		return fmt.Errorf("sync active segment version eligibility snapshot: %w", err)
	}
	return nil
}

type segmentCourseEligibilitySnapshot struct {
	VersionID               string
	MatchingContractKey     string
	MatchingContractVersion int
	SegmentType             string
	SportGroup              string
	RawPredicates           []byte
	AllowedPredicatesSet    bool
	Bidirectional           bool
	Visibility              string
	ElevationKind           string
}

func (db *DB) rebuildSegmentCourseEligibilityProjectionTx(ctx context.Context, tx pgx.Tx, segmentID string, seedActivityTraits bool) error {
	if seedActivityTraits {
		if err := rebuildActivityTraitProjectionTx(ctx, tx); err != nil {
			return err
		}
	}

	var segmentExists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM segments WHERE id = $1)`, segmentID).Scan(&segmentExists)
	if err != nil {
		return fmt.Errorf("check segment for course eligibility projection: %w", err)
	}
	if !segmentExists {
		return nil
	}

	rows, err := tx.Query(ctx, `
		SELECT id,
		       matching_contract_key,
		       matching_contract_version,
		       eligibility_segment_type,
		       eligibility_sport_group,
		       eligibility_allowed_predicates,
		       eligibility_allowed_predicates_set,
		       eligibility_bidirectional,
		       eligibility_visibility,
		       eligibility_elevation_kind
		FROM segment_versions
		WHERE segment_id = $1
		  AND (
		      (matching_contract_key = 'course_gps_v1' AND matching_contract_version = 1)
		      OR (matching_contract_key = 'checkpoint_sequence_v1' AND matching_contract_version = 1)
		  )
		  AND validity_basis <> 'rollback_retired'
		ORDER BY version_number, id
	`, segmentID)
	if err != nil {
		return fmt.Errorf("load segment versions for course eligibility projection: %w", err)
	}
	snapshots := []segmentCourseEligibilitySnapshot{}
	for rows.Next() {
		var snapshot segmentCourseEligibilitySnapshot
		if err := rows.Scan(&snapshot.VersionID, &snapshot.MatchingContractKey, &snapshot.MatchingContractVersion,
			&snapshot.SegmentType, &snapshot.SportGroup, &snapshot.RawPredicates,
			&snapshot.AllowedPredicatesSet, &snapshot.Bidirectional, &snapshot.Visibility, &snapshot.ElevationKind); err != nil {
			rows.Close()
			return fmt.Errorf("scan segment version for course eligibility projection: %w", err)
		}
		snapshots = append(snapshots, snapshot)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load segment versions for course eligibility projection: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM target_course_eligibility_projection
		WHERE target_kind = 'segment' AND target_id = $1
	`, segmentID); err != nil {
		return fmt.Errorf("delete course eligibility projection rows: %w", err)
	}

	for _, snapshot := range snapshots {
		rawPredicates := snapshot.RawPredicates
		if !snapshot.AllowedPredicatesSet {
			rawPredicates = nil
		}
		allowedPredicates, err := decodeSegmentPredicates(rawPredicates)
		if err != nil {
			return fmt.Errorf("decode segment version predicates for course eligibility projection: %w", err)
		}
		projectedRows := sport.CourseEligibilityRowsForSegment(sport.CourseEligibilityInput{
			TargetKind:           "segment",
			TargetID:             segmentID,
			TargetVersionID:      snapshot.VersionID,
			SegmentType:          snapshot.SegmentType,
			SportGroup:           snapshot.SportGroup,
			AllowedPredicates:    allowedPredicates,
			AllowedPredicatesSet: snapshot.AllowedPredicatesSet,
			Bidirectional:        snapshot.Bidirectional,
			Visibility:           snapshot.Visibility,
			ElevationKind:        snapshot.ElevationKind,
			AdapterKey:           courseEligibilityAdapterKey,
			AdapterVersion:       courseEligibilityAdapterVersion,
		})
		if len(projectedRows) == 0 {
			continue
		}

		batch := &pgx.Batch{}
		for _, row := range projectedRows {
			template, err := json.Marshal(row.LeaderboardTemplate)
			if err != nil {
				return fmt.Errorf("marshal leaderboard partition template: %w", err)
			}
			queueCourseEligibilityProjectionInsert(batch, segmentID, snapshot, row, template)
		}
		if err := execCourseEligibilityProjectionBatch(ctx, tx, batch, len(projectedRows)); err != nil {
			return err
		}
	}
	return nil
}

func queueCourseEligibilityProjectionInsert(batch *pgx.Batch, segmentID string, snapshot segmentCourseEligibilitySnapshot, row sport.CourseEligibilityProjectionRow, template []byte) {
	batch.Queue(`
		INSERT INTO target_course_eligibility_projection (
			target_kind, target_id, target_version_id, matching_contract_key, matching_contract_version,
			activity_trait_bucket_key, policy_key, policy_version, source_hash, compatibility_preset,
			activity_group, match_allowed, completion_allowed, rank_allowed, leaderboard_partition_template
		)
		VALUES ('segment', $1, $2, $3, $4,
		        $5, $6, $7, $8, $9,
		        $10, $11, $12, $13, $14::jsonb)
		ON CONFLICT (target_kind, target_id, target_version_id, activity_trait_bucket_key, policy_key, policy_version)
		DO UPDATE SET
			matching_contract_key = EXCLUDED.matching_contract_key,
			matching_contract_version = EXCLUDED.matching_contract_version,
			source_hash = EXCLUDED.source_hash,
			generated_at = NOW(),
			compatibility_preset = EXCLUDED.compatibility_preset,
			activity_group = EXCLUDED.activity_group,
			match_allowed = EXCLUDED.match_allowed,
			completion_allowed = EXCLUDED.completion_allowed,
			rank_allowed = EXCLUDED.rank_allowed,
			leaderboard_partition_template = EXCLUDED.leaderboard_partition_template
	`, segmentID, snapshot.VersionID, snapshot.MatchingContractKey, snapshot.MatchingContractVersion,
		row.ActivityTraitBucketKey, courseEligibilityPolicyKey, courseEligibilityPolicyVersion,
		row.SourceHash, row.CompatibilityPreset, row.ActivityGroup, row.MatchAllowed, row.CompletionAllowed,
		row.RankAllowed, template)
}

func execCourseEligibilityProjectionBatch(ctx context.Context, tx pgx.Tx, batch *pgx.Batch, queued int) error {
	results := tx.SendBatch(ctx, batch)
	for range queued {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return fmt.Errorf("insert course eligibility projection row: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("close course eligibility projection batch: %w", err)
	}
	return nil
}

type segmentCourseEligibilityBackfillSnapshot struct {
	SegmentID string
	Snapshot  segmentCourseEligibilitySnapshot
}

func (db *DB) RebuildAllCourseEligibilityProjections(ctx context.Context) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin full course eligibility projection rebuild: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('skadia_course_eligibility_projection_rebuild'))`); err != nil {
		return fmt.Errorf("lock full course eligibility projection rebuild: %w", err)
	}
	if err := rebuildActivityTraitProjectionTx(ctx, tx); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `
		SELECT segment_id,
		       id,
		       matching_contract_key,
		       matching_contract_version,
		       eligibility_segment_type,
		       eligibility_sport_group,
		       eligibility_allowed_predicates,
		       eligibility_allowed_predicates_set,
		       eligibility_bidirectional,
		       eligibility_visibility,
		       eligibility_elevation_kind
		FROM segment_versions
		WHERE (
		      (matching_contract_key = 'course_gps_v1' AND matching_contract_version = 1)
		      OR (matching_contract_key = 'checkpoint_sequence_v1' AND matching_contract_version = 1)
		)
		  AND validity_basis <> 'rollback_retired'
		ORDER BY segment_id, version_number, id
	`)
	if err != nil {
		return fmt.Errorf("list segment versions for course eligibility projection rebuild: %w", err)
	}
	snapshots := []segmentCourseEligibilityBackfillSnapshot{}
	for rows.Next() {
		var backfillSnapshot segmentCourseEligibilityBackfillSnapshot
		if err := rows.Scan(&backfillSnapshot.SegmentID, &backfillSnapshot.Snapshot.VersionID,
			&backfillSnapshot.Snapshot.MatchingContractKey, &backfillSnapshot.Snapshot.MatchingContractVersion,
			&backfillSnapshot.Snapshot.SegmentType, &backfillSnapshot.Snapshot.SportGroup,
			&backfillSnapshot.Snapshot.RawPredicates, &backfillSnapshot.Snapshot.AllowedPredicatesSet,
			&backfillSnapshot.Snapshot.Bidirectional, &backfillSnapshot.Snapshot.Visibility,
			&backfillSnapshot.Snapshot.ElevationKind); err != nil {
			rows.Close()
			return fmt.Errorf("scan segment version for course eligibility projection rebuild: %w", err)
		}
		snapshots = append(snapshots, backfillSnapshot)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list segment versions for course eligibility projection rebuild: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM target_course_eligibility_projection WHERE target_kind = 'segment'`); err != nil {
		return fmt.Errorf("delete course eligibility projection rows: %w", err)
	}

	copyRows := make([][]any, 0, courseEligibilityCopyChunkSize)
	flush := func() error {
		if len(copyRows) == 0 {
			return nil
		}
		copied, err := tx.CopyFrom(ctx, pgx.Identifier{"target_course_eligibility_projection"}, []string{
			"target_kind",
			"target_id",
			"target_version_id",
			"matching_contract_key",
			"matching_contract_version",
			"activity_trait_bucket_key",
			"policy_key",
			"policy_version",
			"source_hash",
			"compatibility_preset",
			"activity_group",
			"match_allowed",
			"completion_allowed",
			"rank_allowed",
			"leaderboard_partition_template",
		}, pgx.CopyFromRows(copyRows))
		if err != nil {
			return fmt.Errorf("copy course eligibility projection rows: %w", err)
		}
		if copied != int64(len(copyRows)) {
			return fmt.Errorf("copy course eligibility projection rows: copied %d of %d", copied, len(copyRows))
		}
		copyRows = copyRows[:0]
		return nil
	}
	for _, backfillSnapshot := range snapshots {
		snapshot := backfillSnapshot.Snapshot
		rawPredicates := snapshot.RawPredicates
		if !snapshot.AllowedPredicatesSet {
			rawPredicates = nil
		}
		allowedPredicates, err := decodeSegmentPredicates(rawPredicates)
		if err != nil {
			return fmt.Errorf("decode segment version predicates for course eligibility projection: %w", err)
		}
		projectedRows := sport.CourseEligibilityRowsForSegment(sport.CourseEligibilityInput{
			TargetKind:           "segment",
			TargetID:             backfillSnapshot.SegmentID,
			TargetVersionID:      snapshot.VersionID,
			SegmentType:          snapshot.SegmentType,
			SportGroup:           snapshot.SportGroup,
			AllowedPredicates:    allowedPredicates,
			AllowedPredicatesSet: snapshot.AllowedPredicatesSet,
			Bidirectional:        snapshot.Bidirectional,
			Visibility:           snapshot.Visibility,
			ElevationKind:        snapshot.ElevationKind,
			AdapterKey:           courseEligibilityAdapterKey,
			AdapterVersion:       courseEligibilityAdapterVersion,
		})
		for _, row := range projectedRows {
			template, err := json.Marshal(row.LeaderboardTemplate)
			if err != nil {
				return fmt.Errorf("marshal leaderboard partition template: %w", err)
			}
			copyRows = append(copyRows, []any{
				"segment",
				backfillSnapshot.SegmentID,
				snapshot.VersionID,
				snapshot.MatchingContractKey,
				snapshot.MatchingContractVersion,
				row.ActivityTraitBucketKey,
				courseEligibilityPolicyKey,
				courseEligibilityPolicyVersion,
				row.SourceHash,
				row.CompatibilityPreset,
				row.ActivityGroup,
				row.MatchAllowed,
				row.CompletionAllowed,
				row.RankAllowed,
				template,
			})
			if len(copyRows) >= courseEligibilityCopyChunkSize {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit full course eligibility projection rebuild: %w", err)
	}
	return nil
}

func (db *DB) BackfillAndVerifyCourseEligibilityProjections(ctx context.Context) error {
	if err := db.RebuildAllCourseEligibilityProjections(ctx); err != nil {
		return err
	}
	if err := db.VerifyCourseEligibilityProjectionCoverage(ctx); err != nil {
		return err
	}
	return nil
}

func (db *DB) EnsureCourseEligibilityProjections(ctx context.Context) error {
	seeded, err := db.courseEligibilityProjectionSeeded(ctx)
	if err != nil {
		return err
	}
	if seeded {
		return nil
	}
	return db.RebuildAllCourseEligibilityProjections(ctx)
}

func (db *DB) courseEligibilityProjectionSeeded(ctx context.Context) (bool, error) {
	var traitRows int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM activity_trait_projection`).Scan(&traitRows); err != nil {
		return false, fmt.Errorf("check activity trait projection seed: %w", err)
	}
	if traitRows < len(sport.ActivityTraitBuckets()) {
		return false, nil
	}

	var hasCourseRows bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM target_course_eligibility_projection
			WHERE target_kind = 'segment'
			LIMIT 1
		)
	`).Scan(&hasCourseRows); err != nil {
		return false, fmt.Errorf("check course eligibility projection seed: %w", err)
	}
	return hasCourseRows, nil
}

func (db *DB) VerifyCourseEligibilityProjectionCoverage(ctx context.Context) error {
	for _, bucket := range sport.ActivityTraitBuckets() {
		var exists bool
		if err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM activity_trait_projection
				WHERE activity_trait_bucket_key = $1
			)
		`, bucket.Key).Scan(&exists); err != nil {
			return fmt.Errorf("verify activity trait projection coverage: %w", err)
		}
		if !exists {
			return fmt.Errorf("activity trait projection missing generated bucket %s", bucket.Key)
		}
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT sv.segment_id,
		       sv.id,
		       sv.eligibility_segment_type,
		       sv.eligibility_sport_group,
		       sv.eligibility_allowed_predicates,
		       sv.eligibility_allowed_predicates_set,
		       sv.eligibility_bidirectional,
		       sv.eligibility_visibility,
		       sv.eligibility_elevation_kind
		FROM segment_versions sv
		WHERE sv.matching_contract_key = 'course_gps_v1'
		  AND sv.matching_contract_version = 1
		  AND sv.validity_basis <> 'rollback_retired'
		ORDER BY sv.segment_id, sv.version_number, sv.id
	`)
	if err != nil {
		return fmt.Errorf("verify course eligibility projection coverage: %w", err)
	}
	defer rows.Close()

	mismatches := 0
	mismatchSamples := []string{}
	for rows.Next() {
		var (
			segmentID     string
			versionID     string
			segmentType   string
			sportGroup    string
			rawPredicates []byte
			predicatesSet bool
			bidirectional bool
			visibility    string
			elevationKind string
		)
		if err := rows.Scan(&segmentID, &versionID, &segmentType, &sportGroup, &rawPredicates,
			&predicatesSet, &bidirectional, &visibility, &elevationKind); err != nil {
			return fmt.Errorf("scan course eligibility coverage row: %w", err)
		}
		if !predicatesSet {
			rawPredicates = nil
		}
		allowedPredicates, err := decodeSegmentPredicates(rawPredicates)
		if err != nil {
			return fmt.Errorf("decode course eligibility coverage predicates: %w", err)
		}
		expected := sport.CourseEligibilityRowsForSegment(sport.CourseEligibilityInput{
			TargetKind:           "segment",
			TargetID:             segmentID,
			TargetVersionID:      versionID,
			SegmentType:          segmentType,
			SportGroup:           sportGroup,
			AllowedPredicates:    allowedPredicates,
			AllowedPredicatesSet: predicatesSet,
			Bidirectional:        bidirectional,
			Visibility:           visibility,
			ElevationKind:        elevationKind,
			AdapterKey:           courseEligibilityAdapterKey,
			AdapterVersion:       courseEligibilityAdapterVersion,
		})
		if err := db.verifyCourseEligibilityProjectionSet(ctx, segmentID, versionID, expected); err != nil {
			mismatches++
			if len(mismatchSamples) < 5 {
				mismatchSamples = append(mismatchSamples, fmt.Sprintf("%s/%s: %v", segmentID, versionID, err))
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("verify course eligibility projection coverage: %w", err)
	}
	if mismatches > 0 {
		return fmt.Errorf("course eligibility projection mismatch for %d segment version(s), samples=%s", mismatches, strings.Join(mismatchSamples, ","))
	}
	return nil
}

type courseEligibilityProjectionComparable struct {
	ActivityTraitBucketKey string
	PolicyKey              string
	PolicyVersion          int
	SourceHash             string
	CompatibilityPreset    string
	ActivityGroup          string
	MatchAllowed           bool
	CompletionAllowed      bool
	RankAllowed            bool
}

func (db *DB) verifyCourseEligibilityProjectionSet(ctx context.Context, segmentID, versionID string, expected []sport.CourseEligibilityProjectionRow) error {
	actualRows, err := db.Pool.Query(ctx, `
		SELECT activity_trait_bucket_key, policy_key, policy_version, source_hash,
		       compatibility_preset, activity_group, match_allowed, completion_allowed, rank_allowed
		FROM target_course_eligibility_projection
		WHERE target_kind = 'segment'
		  AND target_id = $1
		  AND target_version_id = $2
	`, segmentID, versionID)
	if err != nil {
		return fmt.Errorf("load projection rows: %w", err)
	}
	defer actualRows.Close()

	actual := map[string]courseEligibilityProjectionComparable{}
	for actualRows.Next() {
		var row courseEligibilityProjectionComparable
		if err := actualRows.Scan(&row.ActivityTraitBucketKey, &row.PolicyKey, &row.PolicyVersion, &row.SourceHash,
			&row.CompatibilityPreset, &row.ActivityGroup, &row.MatchAllowed, &row.CompletionAllowed, &row.RankAllowed); err != nil {
			return fmt.Errorf("scan projection row: %w", err)
		}
		actual[courseEligibilityProjectionKey(row.ActivityTraitBucketKey, row.PolicyKey, row.PolicyVersion)] = row
	}
	if err := actualRows.Err(); err != nil {
		return fmt.Errorf("load projection rows: %w", err)
	}

	expectedMap := map[string]courseEligibilityProjectionComparable{}
	for _, row := range expected {
		comparable := courseEligibilityProjectionComparable{
			ActivityTraitBucketKey: row.ActivityTraitBucketKey,
			PolicyKey:              courseEligibilityPolicyKey,
			PolicyVersion:          courseEligibilityPolicyVersion,
			SourceHash:             row.SourceHash,
			CompatibilityPreset:    row.CompatibilityPreset,
			ActivityGroup:          row.ActivityGroup,
			MatchAllowed:           row.MatchAllowed,
			CompletionAllowed:      row.CompletionAllowed,
			RankAllowed:            row.RankAllowed,
		}
		expectedMap[courseEligibilityProjectionKey(comparable.ActivityTraitBucketKey, comparable.PolicyKey, comparable.PolicyVersion)] = comparable
	}

	if len(actual) != len(expectedMap) {
		return fmt.Errorf("row count=%d want=%d", len(actual), len(expectedMap))
	}
	for key, want := range expectedMap {
		got, ok := actual[key]
		if !ok {
			return fmt.Errorf("missing row %s", key)
		}
		if got != want {
			return fmt.Errorf("row %s = %+v want %+v", key, got, want)
		}
	}
	return nil
}

func courseEligibilityProjectionKey(activityTraitBucketKey, policyKey string, policyVersion int) string {
	return fmt.Sprintf("%s|%s|%d", activityTraitBucketKey, policyKey, policyVersion)
}

func rebuildActivityTraitProjectionTx(ctx context.Context, tx pgx.Tx) error {
	for _, bucket := range sport.ActivityTraitBuckets() {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_trait_projection (
				activity_trait_bucket_key, trait_policy_key, trait_policy_version, source_hash,
				sport_type, sub_type, is_indoor, is_virtual, is_electric,
				movement_families, propulsion_modes, equipment_families,
				activity_form, environment_state
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			ON CONFLICT (activity_trait_bucket_key) DO UPDATE SET
				trait_policy_key = EXCLUDED.trait_policy_key,
				trait_policy_version = EXCLUDED.trait_policy_version,
				source_hash = EXCLUDED.source_hash,
				generated_at = NOW(),
				sport_type = EXCLUDED.sport_type,
				sub_type = EXCLUDED.sub_type,
				is_indoor = EXCLUDED.is_indoor,
				is_virtual = EXCLUDED.is_virtual,
				is_electric = EXCLUDED.is_electric,
				movement_families = EXCLUDED.movement_families,
				propulsion_modes = EXCLUDED.propulsion_modes,
				equipment_families = EXCLUDED.equipment_families,
				activity_form = EXCLUDED.activity_form,
				environment_state = EXCLUDED.environment_state
		`, bucket.Key, bucket.TraitPolicyKey, bucket.TraitPolicyVersion, bucket.SourceHash,
			bucket.SportType, bucket.SubType, bucket.IsIndoor, bucket.IsVirtual, bucket.IsElectric,
			nonNilTextArray(bucket.MovementFamilies), nonNilTextArray(bucket.PropulsionModes), nonNilTextArray(bucket.EquipmentFamilies),
			bucket.ActivityForm, bucket.EnvironmentState); err != nil {
			return fmt.Errorf("upsert activity trait projection %s: %w", bucket.Key, err)
		}
	}
	return nil
}

func nonNilTextArray(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
