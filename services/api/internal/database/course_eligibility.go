package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

const (
	courseEligibilityPolicyKey      = "course_eligibility"
	courseEligibilityPolicyVersion  = 1
	courseEligibilityAdapterKey     = "legacy_segment_adapter_v1"
	courseEligibilityAdapterVersion = 1
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
	if err := rebuildActivityTraitProjectionTx(ctx, tx); err != nil {
		return err
	}

	var (
		segmentType   string
		sportGroup    string
		rawPredicates []byte
		bidirectional bool
		visibility    string
		elevationKind string
	)
	err := tx.QueryRow(ctx, `
		SELECT segment_type, sport_group, allowed_predicates, bidirectional, visibility, elevation_kind
		FROM segments
		WHERE id = $1
	`, segmentID).Scan(&segmentType, &sportGroup, &rawPredicates, &bidirectional, &visibility, &elevationKind)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("load segment for course eligibility projection: %w", err)
	}
	allowedPredicates, err := decodeSegmentPredicates(rawPredicates)
	if err != nil {
		return fmt.Errorf("decode segment predicates for course eligibility projection: %w", err)
	}
	allowedPredicatesSet := rawPredicates != nil

	rows, err := tx.Query(ctx, `
		SELECT id
		FROM segment_versions
		WHERE segment_id = $1
		  AND matching_contract_key = 'course_gps_v1'
		  AND matching_contract_version = 1
		ORDER BY version_number, id
	`, segmentID)
	if err != nil {
		return fmt.Errorf("load segment versions for course eligibility projection: %w", err)
	}
	versionIDs := []string{}
	for rows.Next() {
		var versionID string
		if err := rows.Scan(&versionID); err != nil {
			rows.Close()
			return fmt.Errorf("scan segment version for course eligibility projection: %w", err)
		}
		versionIDs = append(versionIDs, versionID)
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

	for _, versionID := range versionIDs {
		projectedRows := sport.CourseEligibilityRowsForSegment(sport.CourseEligibilityInput{
			TargetKind:           "segment",
			TargetID:             segmentID,
			TargetVersionID:      versionID,
			SegmentType:          segmentType,
			SportGroup:           sportGroup,
			AllowedPredicates:    allowedPredicates,
			AllowedPredicatesSet: allowedPredicatesSet,
			Bidirectional:        bidirectional,
			Visibility:           visibility,
			ElevationKind:        elevationKind,
			AdapterKey:           courseEligibilityAdapterKey,
			AdapterVersion:       courseEligibilityAdapterVersion,
		})
		for _, row := range projectedRows {
			template, err := json.Marshal(row.LeaderboardTemplate)
			if err != nil {
				return fmt.Errorf("marshal leaderboard partition template: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO target_course_eligibility_projection (
					target_kind, target_id, target_version_id, matching_contract_key, matching_contract_version,
					activity_trait_bucket_key, policy_key, policy_version, source_hash, compatibility_preset,
					activity_group, match_allowed, completion_allowed, rank_allowed, leaderboard_partition_template
				)
				VALUES ('segment', $1, $2, 'course_gps_v1', 1,
				        $3, $4, $5, $6, $7,
				        $8, $9, $10, $11, $12::jsonb)
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
			`, segmentID, versionID, row.ActivityTraitBucketKey, courseEligibilityPolicyKey, courseEligibilityPolicyVersion,
				row.SourceHash, row.CompatibilityPreset, row.ActivityGroup, row.MatchAllowed, row.CompletionAllowed,
				row.RankAllowed, template); err != nil {
				return fmt.Errorf("insert course eligibility projection row: %w", err)
			}
		}
	}
	return nil
}

func (db *DB) RebuildAllCourseEligibilityProjections(ctx context.Context) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin full course eligibility projection rebuild: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := rebuildActivityTraitProjectionTx(ctx, tx); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM segments ORDER BY id`)
	if err != nil {
		return fmt.Errorf("list segments for course eligibility projection rebuild: %w", err)
	}
	segmentIDs := []string{}
	for rows.Next() {
		var segmentID string
		if err := rows.Scan(&segmentID); err != nil {
			rows.Close()
			return fmt.Errorf("scan segment id for course eligibility projection rebuild: %w", err)
		}
		segmentIDs = append(segmentIDs, segmentID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list segments for course eligibility projection rebuild: %w", err)
	}
	for _, segmentID := range segmentIDs {
		if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, segmentID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit full course eligibility projection rebuild: %w", err)
	}
	return nil
}

func (db *DB) VerifyCourseEligibilityProjectionCoverage(ctx context.Context) error {
	var missing int
	if err := db.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM segments s
		JOIN segment_versions sv ON sv.segment_id = s.id
		 AND sv.matching_contract_key = 'course_gps_v1'
		 AND sv.matching_contract_version = 1
		WHERE (
			(s.allowed_predicates IS NULL AND s.segment_type NOT IN ('wintersport', 'wassersport'))
			OR jsonb_array_length(s.allowed_predicates) > 0
		)
		AND NOT EXISTS (
			SELECT 1
			FROM target_course_eligibility_projection cep
			WHERE cep.target_kind = 'segment'
			  AND cep.target_id = s.id
			  AND cep.target_version_id = sv.id
			  AND cep.match_allowed = true
		)
	`).Scan(&missing); err != nil {
		return fmt.Errorf("verify course eligibility projection coverage: %w", err)
	}
	if missing > 0 {
		return fmt.Errorf("course eligibility projection missing %d matchable segment version(s)", missing)
	}
	return nil
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
