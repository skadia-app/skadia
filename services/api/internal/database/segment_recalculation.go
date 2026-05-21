package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

var ErrSegmentRecalculationObsolete = errors.New("segment recalculation obsolete")

type SegmentRecalculationScope struct {
	SegmentID       string
	OwnerUserID     string
	IncludeAllUsers bool
	AdminOverride   bool
	OperationID     string
	FromAt          *time.Time
	UntilAt         *time.Time
	IdempotencyKey  string
}

type SegmentEffortSupersedeResult struct {
	Superseded int
	Inserted   int
}

type SegmentRecalculationActivityClaim struct {
	RunID      string
	SegmentID  string
	ActivityID string
	UserID     string
	ClaimToken string
	Generation int64
	FromAt     *time.Time
	UntilAt    *time.Time
}

type supersededEffortRef struct {
	id               string
	matchIdentityKey string
	startedAt        time.Time
}

func (db *DB) SeedSegmentRecalculationRun(ctx context.Context, actorUserID string, scope SegmentRecalculationScope, reason string, metadata map[string]any) (string, int, error) {
	if strings.TrimSpace(scope.SegmentID) == "" {
		return "", 0, fmt.Errorf("%w: segment_id is required", ErrSegmentValidation)
	}
	if strings.TrimSpace(scope.OperationID) == "" {
		return "", 0, fmt.Errorf("%w: operation_id is required", ErrSegmentValidation)
	}
	if scope.UntilAt != nil && scope.FromAt != nil && !scope.UntilAt.After(*scope.FromAt) {
		return "", 0, fmt.Errorf("%w: recalculation interval must be empty or increasing", ErrSegmentValidation)
	}
	if strings.TrimSpace(reason) == "" {
		reason = "segment recalculation"
	}
	idempotencyKey := strings.TrimSpace(scope.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = segmentRecalculationIdempotencyKey(scope)
	}

	var existingID string
	var existingTotal int
	err := db.Pool.QueryRow(ctx, `
		SELECT id, total_activities
		FROM segment_recalculation_runs
		WHERE idempotency_key = $1`, idempotencyKey).Scan(&existingID, &existingTotal)
	if err == nil {
		return existingID, existingTotal, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", 0, fmt.Errorf("lookup existing segment recalculation run: %w", err)
	}

	if metadata == nil {
		metadata = map[string]any{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", 0, fmt.Errorf("marshal recalculation metadata: %w", err)
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("begin segment recalculation seed: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	runID, total, err := seedSegmentRecalculationRunTx(ctx, tx, actorUserID, scope, reason, idempotencyKey, metadataJSON)
	if err != nil {
		return "", 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", 0, fmt.Errorf("commit segment recalculation seed: %w", err)
	}
	return runID, total, nil
}

func seedSegmentRecalculationRunTx(ctx context.Context, tx pgx.Tx, actorUserID string, scope SegmentRecalculationScope, reason, idempotencyKey string, metadataJSON []byte) (string, int, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('segment_recalc:' || $1, 0))`, scope.SegmentID); err != nil {
		return "", 0, fmt.Errorf("lock segment recalculation seed: %w", err)
	}

	var existingID string
	var existingTotal int
	err := tx.QueryRow(ctx, `
		SELECT id, total_activities
		FROM segment_recalculation_runs
		WHERE idempotency_key = $1`, idempotencyKey).Scan(&existingID, &existingTotal)
	if err == nil {
		return existingID, existingTotal, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", 0, fmt.Errorf("lookup existing segment recalculation run: %w", err)
	}

	var segmentOwner, visibility string
	if err := tx.QueryRow(ctx, `
		SELECT created_by, visibility
		FROM segments
		WHERE id = $1
		FOR UPDATE`, scope.SegmentID).Scan(&segmentOwner, &visibility); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, ErrSegmentNotFound
		}
		return "", 0, fmt.Errorf("load segment recalculation scope: %w", err)
	}
	ownerUserID := strings.TrimSpace(scope.OwnerUserID)
	if ownerUserID == "" {
		ownerUserID = segmentOwner
	}
	if scope.IncludeAllUsers && visibility != "public" {
		var actorIsAdmin bool
		if err := tx.QueryRow(ctx, `SELECT COALESCE(is_admin, false) FROM users WHERE id = $1`, actorUserID).Scan(&actorIsAdmin); err != nil {
			return "", 0, fmt.Errorf("load recalculation actor admin state: %w", err)
		}
		if !scope.AdminOverride || !actorIsAdmin {
			return "", 0, fmt.Errorf("%w: private segments cannot schedule all-user recalculation without admin override", ErrSegmentNotAuthorized)
		}
	}

	var generation int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO segment_recalculation_segment_state (segment_id, generation)
		VALUES ($1, 1)
		ON CONFLICT (segment_id) DO UPDATE
			SET generation = segment_recalculation_segment_state.generation + 1,
			    updated_at = NOW()
		RETURNING generation
	`, scope.SegmentID).Scan(&generation); err != nil {
		return "", 0, fmt.Errorf("advance segment recalculation generation: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE segment_recalculation_runs
		SET status = 'obsolete',
		    completed_at = NOW()
		WHERE segment_id = $1
		  AND status IN ('queued', 'running')
	`, scope.SegmentID); err != nil {
		return "", 0, fmt.Errorf("obsolete previous segment recalculation runs: %w", err)
	}

	runID := stableUUID("segment-recalculation-run", scope.SegmentID, idempotencyKey).String()
	if _, err := tx.Exec(ctx, `
		INSERT INTO segment_recalculation_runs (
			id, segment_id, actor_user_id, operation_key_hash, idempotency_key,
			generation, reason, requested_from_at, requested_until_at, metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, COALESCE($10::jsonb, '{}'::jsonb))
	`, runID, scope.SegmentID, actorUserID, stableOperationHash(scope.OperationID, "operation"), idempotencyKey, generation, reason, scope.FromAt, scope.UntilAt, metadataJSON); err != nil {
		return "", 0, fmt.Errorf("insert segment recalculation run: %w", err)
	}

	if _, err := tx.Exec(ctx, seedSegmentRecalculationActivitiesSQL, runID, scope.SegmentID, scope.FromAt, scope.UntilAt, scope.IncludeAllUsers, ownerUserID); err != nil {
		return "", 0, fmt.Errorf("seed segment recalculation activities: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		UPDATE segment_recalculation_runs
		SET total_activities = (
			SELECT count(*)
			FROM segment_recalculation_run_activities
			WHERE run_id = $1
		)
		WHERE id = $1
		RETURNING total_activities
	`, runID).Scan(&existingTotal); err != nil {
		return "", 0, fmt.Errorf("update segment recalculation activity count: %w", err)
	}
	if existingTotal == 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE segment_recalculation_runs
			SET status = 'completed',
			    completed_at = NOW()
			WHERE id = $1
		`, runID); err != nil {
			return "", 0, fmt.Errorf("complete empty segment recalculation run: %w", err)
		}
	}
	return runID, existingTotal, nil
}

func segmentRecalculationIdempotencyKey(scope SegmentRecalculationScope) string {
	from := "unbounded"
	if scope.FromAt != nil {
		from = scope.FromAt.UTC().Format(time.RFC3339Nano)
	}
	until := "unbounded"
	if scope.UntilAt != nil {
		until = scope.UntilAt.UTC().Format(time.RFC3339Nano)
	}
	return "segment:" + scope.SegmentID +
		":operation:" + stableOperationHash(scope.OperationID, "recalc") +
		":from:" + from +
		":until:" + until +
		":policy:segment_recalculation_v1"
}

const seedSegmentRecalculationActivitiesSQL = `
WITH eligible_activity AS (
    SELECT a.id,
           a.user_id,
           a.started_at,
           atp.activity_trait_bucket_key
    FROM activities a
    JOIN activity_trait_projection atp
      ON atp.sport_type = a.sport_type
     AND atp.sub_type = COALESCE(NULLIF(a.sub_type, ''), '_')
     AND atp.is_indoor = COALESCE(a.is_indoor, false)
     AND atp.is_virtual = COALESCE(a.is_virtual, false)
     AND atp.is_electric = COALESCE(a.is_electric, false)
    WHERE ($3::timestamptz IS NULL OR COALESCE(a.ended_at, a.started_at) > $3)
      AND ($4::timestamptz IS NULL OR a.started_at < $4)
      AND ($5::boolean OR a.user_id = $6::uuid)
),
candidate_activity AS (
    SELECT DISTINCT ea.id, ea.user_id
    FROM eligible_activity ea
    JOIN activity_tracks at ON at.activity_id = ea.id
    JOIN LATERAL (
        SELECT id, track
        FROM segment_versions
        WHERE segment_id = $2
          AND matching_contract_key = 'course_gps_v1'
          AND matching_contract_version = 1
          AND validity_basis <> 'rollback_retired'
          AND (valid_from_at IS NULL OR valid_from_at <= ea.started_at)
          AND (valid_until_at IS NULL OR ea.started_at < valid_until_at)
        ORDER BY COALESCE(valid_from_at, '-infinity'::timestamptz) DESC,
                 version_number DESC
        LIMIT 1
    ) sv ON true
    JOIN target_course_eligibility_projection cep
      ON cep.target_kind = 'segment'
     AND cep.target_id = $2
     AND cep.target_version_id = sv.id
     AND cep.activity_trait_bucket_key = ea.activity_trait_bucket_key
     AND cep.match_allowed = true
    WHERE at.track_full IS NOT NULL
      AND sv.track IS NOT NULL
      AND at.track_full && ST_Expand(sv.track, 0.002)
      AND ST_DWithin(at.track_full::geography, sv.track::geography, 100)

    UNION

    SELECT DISTINCT ea.id, ea.user_id
    FROM eligible_activity ea
    JOIN activity_tracks at ON at.activity_id = ea.id
    JOIN segments s ON s.id = $2 AND s.course_kind = 'checkpoint_sequence'
    JOIN LATERAL (
        SELECT id
        FROM segment_versions
        WHERE segment_id = $2
          AND matching_contract_key = 'checkpoint_sequence_v1'
          AND matching_contract_version = 1
          AND validity_basis <> 'rollback_retired'
          AND (valid_from_at IS NULL OR valid_from_at <= ea.started_at)
          AND (valid_until_at IS NULL OR ea.started_at < valid_until_at)
        ORDER BY COALESCE(valid_from_at, '-infinity'::timestamptz) DESC,
                 version_number DESC
        LIMIT 1
    ) sv ON true
    JOIN target_course_eligibility_projection cep
      ON cep.target_kind = 'segment'
     AND cep.target_id = $2
     AND cep.target_version_id = sv.id
     AND cep.activity_trait_bucket_key = ea.activity_trait_bucket_key
     AND cep.match_allowed = true
    WHERE at.track_full IS NOT NULL
      AND NOT EXISTS (
          SELECT 1
          FROM course_version_controls c
          WHERE c.segment_version_id = sv.id
            AND c.required = true
            AND NOT (
                (
                    c.geometry_shape IN ('point', 'circle')
                    AND ST_DWithin(
                        at.track_full::geography,
                        c.geom::geography,
                        GREATEST(COALESCE(c.hit_radius_m, c.area_radius_m, 0), 1)
                    )
                )
                OR (
                    c.geometry_shape IN ('polygon', 'multipolygon')
                    AND ST_Intersects(at.track_full, c.geom)
                )
            )
      )

    UNION

    SELECT se.activity_id, se.user_id
    FROM segment_efforts se
    JOIN activities a ON a.id = se.activity_id
    WHERE se.segment_id = $2
      AND se.superseded_at IS NULL
      AND ($3::timestamptz IS NULL OR se.started_at >= $3)
      AND ($4::timestamptz IS NULL OR se.started_at < $4)
      AND ($5::boolean OR se.user_id = $6::uuid)
)
INSERT INTO segment_recalculation_run_activities (run_id, activity_id, user_id)
SELECT $1, id, user_id
FROM candidate_activity
ON CONFLICT (run_id, activity_id) DO NOTHING`

func (db *DB) ClaimNextSegmentRecalculationActivity(ctx context.Context, workerID string, ttl time.Duration) (*SegmentRecalculationActivityClaim, error) {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	claimTTL := fmt.Sprintf("%.0f seconds", ttl.Seconds())
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin segment recalculation claim: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var claim SegmentRecalculationActivityClaim
	err = tx.QueryRow(ctx, `
		WITH picked AS (
			SELECT ra.run_id,
			       r.segment_id,
			       ra.activity_id,
			       ra.user_id,
			       r.generation,
			       r.requested_from_at,
			       r.requested_until_at
			FROM segment_recalculation_run_activities ra
			JOIN segment_recalculation_runs r ON r.id = ra.run_id
			WHERE r.status IN ('queued', 'running')
			  AND (
			  	ra.status = 'queued'
			  	OR (ra.status = 'running' AND ra.claimed_until < NOW())
			  )
			ORDER BY r.created_at, ra.activity_id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		),
		updated AS (
			UPDATE segment_recalculation_run_activities ra
			SET status = 'running',
			    attempt_count = attempt_count + 1,
			    claim_token = gen_random_uuid(),
			    claimed_at = NOW(),
			    claimed_until = NOW() + $1::interval,
			    claimed_by = $2
			FROM picked
			WHERE ra.run_id = picked.run_id
			  AND ra.activity_id = picked.activity_id
			RETURNING ra.run_id, picked.segment_id, ra.activity_id, ra.user_id,
			          ra.claim_token, picked.generation, picked.requested_from_at, picked.requested_until_at
		)
		SELECT run_id, segment_id, activity_id, user_id, claim_token::text,
		       generation, requested_from_at, requested_until_at
		FROM updated
	`, claimTTL, workerID).Scan(&claim.RunID, &claim.SegmentID, &claim.ActivityID, &claim.UserID, &claim.ClaimToken, &claim.Generation, &claim.FromAt, &claim.UntilAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tx.Commit(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("claim segment recalculation activity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE segment_recalculation_runs
		SET status = 'running',
		    started_at = COALESCE(started_at, NOW()),
		    claimed_at = NOW(),
		    claimed_by = $2
		WHERE id = $1
		  AND status = 'queued'
	`, claim.RunID, workerID); err != nil {
		return nil, fmt.Errorf("mark segment recalculation run running: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit segment recalculation claim: %w", err)
	}
	return &claim, nil
}

func ValidateSegmentRecalculationActivityClaimTx(ctx context.Context, tx pgx.Tx, claim SegmentRecalculationActivityClaim) error {
	var runStatus, activityStatus string
	var runGeneration, currentGeneration int64
	err := tx.QueryRow(ctx, `
		SELECT r.status, ra.status, r.generation, COALESCE(s.generation, 0)
		FROM segment_recalculation_run_activities ra
		JOIN segment_recalculation_runs r ON r.id = ra.run_id
		LEFT JOIN segment_recalculation_segment_state s ON s.segment_id = r.segment_id
		WHERE ra.run_id = $1
		  AND ra.activity_id = $2
		  AND ra.claim_token = $3::uuid
		FOR UPDATE OF r, ra
	`, claim.RunID, claim.ActivityID, claim.ClaimToken).Scan(&runStatus, &activityStatus, &runGeneration, &currentGeneration)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: stale segment recalculation activity claim", ErrSegmentValidation)
	}
	if err != nil {
		return fmt.Errorf("validate segment recalculation activity claim: %w", err)
	}
	if runStatus != "running" || activityStatus != "running" {
		return fmt.Errorf("%w: stale segment recalculation activity claim", ErrSegmentValidation)
	}
	if claim.Generation != 0 && claim.Generation != runGeneration {
		return fmt.Errorf("%w: stale segment recalculation generation", ErrSegmentRecalculationObsolete)
	}
	if runGeneration != currentGeneration {
		return fmt.Errorf("%w: obsolete segment recalculation generation", ErrSegmentRecalculationObsolete)
	}
	return nil
}

func CompleteSegmentRecalculationActivityTx(ctx context.Context, tx pgx.Tx, claim SegmentRecalculationActivityClaim, result SegmentEffortSupersedeResult) error {
	tag, err := tx.Exec(ctx, `
		UPDATE segment_recalculation_run_activities
		SET status = 'completed',
		    processed_at = NOW(),
		    claimed_until = NULL
		WHERE run_id = $1
		  AND activity_id = $2
		  AND claim_token = $3::uuid
		  AND status = 'running'
	`, claim.RunID, claim.ActivityID, claim.ClaimToken)
	if err != nil {
		return fmt.Errorf("complete segment recalculation activity: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: stale segment recalculation activity claim", ErrSegmentValidation)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE segment_recalculation_runs
		SET processed_activities = processed_activities + 1,
		    superseded_efforts = superseded_efforts + $2,
		    inserted_efforts = inserted_efforts + $3
		WHERE id = $1
	`, claim.RunID, result.Superseded, result.Inserted); err != nil {
		return fmt.Errorf("update segment recalculation counters: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE segment_recalculation_runs r
		SET status = CASE
		    	WHEN failed_activities > 0 THEN 'failed'
		    	ELSE 'completed'
		    END,
		    completed_at = NOW()
		WHERE r.id = $1
		  AND NOT EXISTS (
		  	SELECT 1
		  	FROM segment_recalculation_run_activities ra
		  	WHERE ra.run_id = r.id
		  	  AND ra.status IN ('queued', 'running')
		  )
	`, claim.RunID); err != nil {
		return fmt.Errorf("complete segment recalculation run if done: %w", err)
	}
	return nil
}

func (db *DB) SkipObsoleteSegmentRecalculationActivity(ctx context.Context, claim SegmentRecalculationActivityClaim) error {
	var updated int
	err := db.Pool.QueryRow(ctx, `
		WITH activity_update AS (
			UPDATE segment_recalculation_run_activities
			SET status = 'skipped',
			    error_code = 'obsolete_generation',
			    error_message = NULL,
			    processed_at = NOW(),
			    claimed_until = NULL
			WHERE run_id = $1
			  AND activity_id = $2
			  AND claim_token = $3::uuid
			  AND status = 'running'
			RETURNING run_id
		)
		SELECT count(*) FROM activity_update
	`, claim.RunID, claim.ActivityID, claim.ClaimToken).Scan(&updated)
	if err != nil {
		return fmt.Errorf("skip obsolete segment recalculation activity: %w", err)
	}
	if updated != 1 {
		return fmt.Errorf("%w: stale segment recalculation activity claim", ErrSegmentValidation)
	}
	return nil
}

func (db *DB) FailSegmentRecalculationActivity(ctx context.Context, claim SegmentRecalculationActivityClaim, errText string) error {
	var updated int
	err := db.Pool.QueryRow(ctx, `
		WITH activity_update AS (
			UPDATE segment_recalculation_run_activities
			SET status = 'failed',
			    error_code = 'activity_failed',
			    error_message = NULLIF($4, ''),
			    processed_at = NOW(),
			    claimed_until = NULL
			WHERE run_id = $1
			  AND activity_id = $2
			  AND claim_token = $3::uuid
			  AND status = 'running'
			RETURNING run_id
		),
		counter_update AS (
			UPDATE segment_recalculation_runs r
			SET failed_activities = failed_activities + 1
			FROM activity_update au
			WHERE r.id = au.run_id
			RETURNING r.id
		),
		finalize AS (
			UPDATE segment_recalculation_runs r
			SET status = 'failed',
			    completed_at = NOW()
			FROM counter_update cu
			WHERE r.id = cu.id
			  AND NOT EXISTS (
				SELECT 1
				FROM segment_recalculation_run_activities ra
				WHERE ra.run_id = r.id
				  AND ra.status IN ('queued', 'running')
			  )
			RETURNING r.id
		)
		SELECT count(*) FROM activity_update
	`, claim.RunID, claim.ActivityID, claim.ClaimToken, sanitizedSegmentRecalculationErrorMessage(errText)).Scan(&updated)
	if err != nil {
		return fmt.Errorf("fail segment recalculation activity: %w", err)
	}
	if updated != 1 {
		return fmt.Errorf("%w: stale segment recalculation activity claim", ErrSegmentValidation)
	}
	return nil
}

func sanitizedSegmentRecalculationErrorMessage(errText string) string {
	if strings.TrimSpace(errText) == "" {
		return "activity recalculation failed"
	}
	return "activity recalculation failed"
}

func (db *DB) ListActiveSegmentRecalculationJobs(ctx context.Context, userID string) ([]BackgroundJob, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT r.id,
		       COALESCE(r.actor_user_id, $1::uuid)::text AS user_id,
		       'segment_recalculation' AS job_type,
		       r.segment_id::text AS entity_id,
		       COALESCE(NULLIF(r.metadata->>'entity_name', ''), s.name) AS entity_name,
		       CASE WHEN r.status = 'queued' THEN 'running' ELSE r.status END AS status,
		       'recalculation' AS phase,
		       LEAST(r.total_activities, r.processed_activities + r.failed_activities)::bigint AS progress_current,
		       r.total_activities::bigint AS progress_total,
		       COALESCE(r.started_at, r.created_at) AS started_at,
		       r.completed_at AS finished_at,
		       NULL::text AS error
		FROM segment_recalculation_runs r
		JOIN segments s ON s.id = r.segment_id
		WHERE r.actor_user_id = $1
		  AND r.metadata->>'visibility' = 'user_visible'
		  AND (
		        r.status IN ('queued', 'running')
		        OR (r.status IN ('completed', 'failed')
		            AND r.completed_at > NOW() - INTERVAL '5 minutes')
		      )
		ORDER BY COALESCE(r.started_at, r.created_at) DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list active segment recalculation jobs: %w", err)
	}
	defer rows.Close()

	jobs := []BackgroundJob{}
	for rows.Next() {
		var job BackgroundJob
		var entityID, entityName string
		if err := rows.Scan(
			&job.ID,
			&job.UserID,
			&job.JobType,
			&entityID,
			&entityName,
			&job.Status,
			&job.Phase,
			&job.ProgressCurrent,
			&job.ProgressTotal,
			&job.StartedAt,
			&job.FinishedAt,
			&job.Error,
		); err != nil {
			return nil, fmt.Errorf("scan active segment recalculation job: %w", err)
		}
		job.EntityID = &entityID
		job.EntityName = &entityName
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list active segment recalculation jobs: %w", err)
	}
	return jobs, nil
}

// SupersedeSegmentEffortsForActivitySegments replaces active efforts for an
// activity/segment set without deleting history. Old active rows are marked
// inactive first so the partial active match-identity index cannot reject the
// replacement rows.
func (db *DB) SupersedeSegmentEffortsForActivitySegments(ctx context.Context, activityID, userID string, segmentIDs []string, efforts []*model.SegmentEffort, runID *string, fromAt, untilAt *time.Time) (SegmentEffortSupersedeResult, error) {
	return db.SupersedeSegmentEffortsForActivitySegmentsWithCheckpoint(ctx, activityID, userID, segmentIDs, efforts, nil, runID, fromAt, untilAt)
}

func (db *DB) SupersedeSegmentEffortsForActivitySegmentsWithCheckpoint(ctx context.Context, activityID, userID string, segmentIDs []string, efforts []*model.SegmentEffort, checkpointMatches []model.CheckpointMatchedEffort, runID *string, fromAt, untilAt *time.Time) (SegmentEffortSupersedeResult, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return SegmentEffortSupersedeResult{}, fmt.Errorf("begin supersede segment efforts: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	result, err := SupersedeSegmentEffortsForActivitySegmentsWithCheckpointTx(ctx, tx, activityID, userID, segmentIDs, efforts, checkpointMatches, runID, fromAt, untilAt)
	if err != nil {
		return SegmentEffortSupersedeResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SegmentEffortSupersedeResult{}, fmt.Errorf("commit supersede segment efforts: %w", err)
	}
	return result, nil
}

func SupersedeSegmentEffortsForActivitySegmentsTx(ctx context.Context, tx pgx.Tx, activityID, userID string, segmentIDs []string, efforts []*model.SegmentEffort, runID *string, fromAt, untilAt *time.Time) (SegmentEffortSupersedeResult, error) {
	return SupersedeSegmentEffortsForActivitySegmentsWithCheckpointTx(ctx, tx, activityID, userID, segmentIDs, efforts, nil, runID, fromAt, untilAt)
}

func SupersedeSegmentEffortsForActivitySegmentsWithCheckpointTx(ctx context.Context, tx pgx.Tx, activityID, userID string, segmentIDs []string, efforts []*model.SegmentEffort, checkpointMatches []model.CheckpointMatchedEffort, runID *string, fromAt, untilAt *time.Time) (SegmentEffortSupersedeResult, error) {
	if len(segmentIDs) == 0 {
		return SegmentEffortSupersedeResult{}, nil
	}
	if err := LockSegmentEffortReplacementTx(ctx, tx, activityID); err != nil {
		return SegmentEffortSupersedeResult{}, err
	}

	segmentSet := make(map[string]struct{}, len(segmentIDs))
	for _, segmentID := range segmentIDs {
		segmentSet[segmentID] = struct{}{}
	}
	filteredEfforts := make([]*model.SegmentEffort, 0, len(efforts))
	for _, effort := range efforts {
		if effort == nil {
			continue
		}
		if _, ok := segmentSet[effort.SegmentID]; !ok {
			continue
		}
		if !timeInOptionalRange(effort.StartedAt, fromAt, untilAt) {
			continue
		}
		effort.CreatedByRecalculationRunID = runID
		effort.SupersededAt = nil
		effort.SupersededReason = nil
		effort.SupersededByRecalculationRunID = nil
		effort.SupersededByEffortID = nil
		filteredEfforts = append(filteredEfforts, effort)
	}
	filteredCheckpoint := make([]model.CheckpointMatchedEffort, 0, len(checkpointMatches))
	for _, match := range checkpointMatches {
		if match.Effort == nil {
			continue
		}
		if _, ok := segmentSet[match.Effort.SegmentID]; !ok {
			continue
		}
		if !timeInOptionalRange(match.Effort.StartedAt, fromAt, untilAt) {
			continue
		}
		match.Effort.CreatedByRecalculationRunID = runID
		match.Effort.SupersededAt = nil
		match.Effort.SupersededReason = nil
		match.Effort.SupersededByRecalculationRunID = nil
		match.Effort.SupersededByEffortID = nil
		filteredCheckpoint = append(filteredCheckpoint, match)
	}

	superseded, err := markActiveSegmentEffortsSupersededTx(ctx, tx, activityID, userID, segmentIDs, runID, fromAt, untilAt)
	if err != nil {
		return SegmentEffortSupersedeResult{}, err
	}
	inserted, err := insertSegmentEffortsStrictTx(ctx, tx, filteredEfforts)
	if err != nil {
		return SegmentEffortSupersedeResult{}, err
	}
	if err := linkSupersededReplacementEffortsTx(ctx, tx, superseded, inserted, runID); err != nil {
		return SegmentEffortSupersedeResult{}, err
	}
	if err := InsertCheckpointMatchedEffortsTx(ctx, tx, filteredCheckpoint); err != nil {
		return SegmentEffortSupersedeResult{}, err
	}
	return SegmentEffortSupersedeResult{Superseded: len(superseded), Inserted: len(inserted) + len(filteredCheckpoint)}, nil
}

func timeInOptionalRange(value time.Time, fromAt, untilAt *time.Time) bool {
	if fromAt != nil && value.Before(*fromAt) {
		return false
	}
	if untilAt != nil && !value.Before(*untilAt) {
		return false
	}
	return true
}

func markActiveSegmentEffortsSupersededTx(ctx context.Context, tx pgx.Tx, activityID, userID string, segmentIDs []string, runID *string, fromAt, untilAt *time.Time) ([]supersededEffortRef, error) {
	reason := "admin_correction"
	if runID != nil {
		reason = "recalculated_removed"
	}
	rows, err := tx.Query(ctx, `
		UPDATE segment_efforts
		SET superseded_at = NOW(),
		    superseded_reason = $4,
		    superseded_by_recalculation_run_id = $5,
		    superseded_by_effort_id = NULL
		WHERE activity_id = $1
		  AND user_id = $2
		  AND segment_id = ANY($3::uuid[])
		  AND superseded_at IS NULL
		  AND ($6::timestamptz IS NULL OR started_at >= $6)
		  AND ($7::timestamptz IS NULL OR started_at < $7)
		RETURNING id, COALESCE(match_identity_key, ''), started_at`,
		activityID, userID, segmentIDs, reason, runID, fromAt, untilAt)
	if err != nil {
		return nil, fmt.Errorf("supersede active segment efforts: %w", err)
	}
	defer rows.Close()

	var refs []supersededEffortRef
	for rows.Next() {
		var ref supersededEffortRef
		if err := rows.Scan(&ref.id, &ref.matchIdentityKey, &ref.startedAt); err != nil {
			return nil, fmt.Errorf("scan superseded segment effort: %w", err)
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("supersede active segment efforts: %w", err)
	}
	return refs, nil
}

type insertedEffortRef struct {
	id               string
	matchIdentityKey string
	startedAt        time.Time
}

func insertSegmentEffortsStrictTx(ctx context.Context, tx pgx.Tx, efforts []*model.SegmentEffort) ([]insertedEffortRef, error) {
	inserted := make([]insertedEffortRef, 0, len(efforts))
	for _, effort := range efforts {
		var ref insertedEffortRef
		err := tx.QueryRow(ctx, `
			INSERT INTO segment_efforts (
				segment_id, segment_version_id, activity_id, user_id,
				started_at, elapsed_time_s, is_reverse,
				sport_type, sub_type, gear_id,
				avg_speed_ms, avg_heart_rate, avg_power_w,
				distance_m, elevation_gain_m,
				matching_contract_key, matching_contract_version,
				match_source_type, match_source_id, match_identity_key,
				occurrence_index, occurrence_hash, superseded_by_effort_id,
				superseded_at, superseded_reason, superseded_by_recalculation_run_id,
				created_by_recalculation_run_id, leaderboard_equivalence_group_id,
				leaderboard_sport_type, leaderboard_sub_type,
				leaderboard_equipment_class, leaderboard_assist_class,
				leaderboard_indoor_policy_state, leaderboard_virtual_policy_state,
				leaderboard_electric_policy_state, leaderboard_direction,
				leaderboard_version_policy_key,
				leaderboard_contract_key, leaderboard_contract_version,
				activity_trait_bucket_key, eligibility_policy_key,
				eligibility_policy_version, eligibility_source_hash,
				in_leaderboard
			) VALUES (
				$1, $2, $3, $4,
				$5, $6, $7,
				$8, $9, $10,
				$11, $12, $13,
				$14, $15,
				$16, $17,
				$18, $19, $20,
				$21, $22, $23,
				$24, $25, $26,
				$27, $28,
				$29, $30,
				$31, $32,
				$33, $34,
				$35, $36,
				$37,
				$38, $39,
				$40, $41,
				$42, $43,
				$44
			)
			RETURNING id, COALESCE(match_identity_key, ''), started_at`,
			effort.SegmentID, effort.SegmentVersionID, effort.ActivityID, effort.UserID,
			effort.StartedAt, effort.ElapsedTimeS, effort.IsReverse, effort.SportType, effort.SubType,
			effort.GearID, effort.AvgSpeedMs, effort.AvgHeartRate, effort.AvgPowerW,
			effort.DistanceM, effort.ElevationGainM,
			nullableString(effort.MatchingContractKey), nullableInt(effort.MatchingContractVersion),
			nullableString(effort.MatchSourceType), nullableString(effort.MatchSourceID), nullableString(effort.MatchIdentityKey),
			effort.OccurrenceIndex, effort.OccurrenceHash, effort.SupersededByEffortID,
			effort.SupersededAt, effort.SupersededReason, effort.SupersededByRecalculationRunID,
			effort.CreatedByRecalculationRunID, nullableString(effort.LeaderboardEquivalenceGroupID),
			nullableString(effort.LeaderboardSportType), nullableString(effort.LeaderboardSubType),
			nullableString(effort.LeaderboardEquipmentClass), nullableString(effort.LeaderboardAssistClass),
			nullableString(effort.LeaderboardIndoorPolicyState), nullableString(effort.LeaderboardVirtualPolicyState),
			nullableString(effort.LeaderboardElectricPolicyState), nullableString(effort.LeaderboardDirection),
			nullableString(effort.LeaderboardVersionPolicyKey),
			nullableString(effort.LeaderboardContractKey), nullableInt(effort.LeaderboardContractVersion),
			nullableString(effort.ActivityTraitBucketKey), segmentEffortEligibilityPolicyKey(effort),
			segmentEffortEligibilityPolicyVersion(effort), nullableString(effort.EligibilitySourceHash),
			segmentEffortInLeaderboard(effort),
		).Scan(&ref.id, &ref.matchIdentityKey, &ref.startedAt)
		if err != nil {
			return nil, fmt.Errorf("strict insert segment effort: %w", err)
		}
		inserted = append(inserted, ref)
	}
	return inserted, nil
}

func linkSupersededReplacementEffortsTx(ctx context.Context, tx pgx.Tx, superseded []supersededEffortRef, inserted []insertedEffortRef, runID *string) error {
	if len(superseded) == 0 || len(inserted) == 0 {
		return nil
	}
	replacements := make(map[string]string, len(inserted))
	for _, ref := range inserted {
		if ref.matchIdentityKey == "" {
			continue
		}
		replacements[segmentReplacementKey(ref.matchIdentityKey, ref.startedAt)] = ref.id
	}
	reason := "admin_correction"
	if runID != nil {
		reason = "recalculated_replaced"
	}
	for _, ref := range superseded {
		if ref.matchIdentityKey == "" {
			continue
		}
		replacementID, ok := replacements[segmentReplacementKey(ref.matchIdentityKey, ref.startedAt)]
		if !ok {
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE segment_efforts
			SET superseded_by_effort_id = $2,
			    superseded_reason = $3
			WHERE id = $1`, ref.id, replacementID, reason); err != nil {
			return fmt.Errorf("link superseded segment effort replacement: %w", err)
		}
	}
	return nil
}

func segmentReplacementKey(matchIdentityKey string, startedAt time.Time) string {
	return matchIdentityKey + "\x00" + startedAt.UTC().Format(time.RFC3339Nano)
}
