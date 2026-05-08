// CLI tool: re-enrich lift candidates for all eligible activities.
//
// Usage:
//
//	lift_recompute_all --user-id <uuid>    re-enriches one user
//	lift_recompute_all --all-users         re-enriches all users with EnableLiftDetection=true
//	lift_recompute_all --ack-backfill-only sets lifts_acknowledged_at=updated_at for activities
//	                                       whose heuristic candidates are all resolved
//
// The tool deletes non-rejected candidates before re-running enrichStreamLifts
// (the enrichment path already does this, but the explicit delete is belt-and-
// suspenders against double-counting in the confirm-gating aggregate).
//
// Soft-rejects (rejected_by_user=true) survive re-enrichment: enrichStreamLifts
// filters on NOT rejected_by_user, so they are never deleted.
//
// After all activities are processed the tool calls UpdateGearStats once per
// affected gear to bring gear totals in sync without redundant per-activity
// calls (--all-users only).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
)

func main() {
	userIDFlag := flag.String("user-id", "", "re-enrich a single user (UUID)")
	allUsers := flag.Bool("all-users", false, "re-enrich all users with EnableLiftDetection=true")
	ackOnly := flag.Bool("ack-backfill-only", false, "only run the acknowledged-at backfill UPDATE, no re-enrichment")
	dbURL := flag.String("db-url", os.Getenv("DATABASE_URL"), "PostgreSQL connection string")
	flag.Parse()

	if *dbURL == "" {
		fmt.Fprintln(os.Stderr, "need --db-url or DATABASE_URL")
		os.Exit(2)
	}
	if !*ackOnly && *userIDFlag == "" && !*allUsers {
		fmt.Fprintln(os.Stderr, "need --user-id, --all-users, or --ack-backfill-only")
		flag.Usage()
		os.Exit(2)
	}
	if *userIDFlag != "" && *allUsers {
		fmt.Fprintln(os.Stderr, "--user-id and --all-users are mutually exclusive")
		os.Exit(2)
	}

	ctx := context.Background()
	db, err := database.New(ctx, *dbURL)
	if err != nil {
		slog.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Pool.Close()

	if *ackOnly {
		n, err := runAckBackfill(ctx, db.Pool)
		if err != nil {
			slog.Error("ack backfill failed", "error", err)
			os.Exit(1)
		}
		slog.Info("ack backfill complete", "updated", n)
		return
	}

	var userIDs []string
	if *userIDFlag != "" {
		userIDs = []string{*userIDFlag}
	} else {
		ids, err := listUsersWithLiftDetection(ctx, db.Pool)
		if err != nil {
			slog.Error("list users with lift detection", "error", err)
			os.Exit(1)
		}
		userIDs = ids
		slog.Info("found users with lift detection enabled", "count", len(userIDs))
	}

	var (
		totalEnriched  int
		totalErrors    int
		totalDeleted   int
		affectedGears  = make(map[string]struct{})
	)

	for _, userID := range userIDs {
		activities, err := listLiftEligibleActivities(ctx, db.Pool, userID)
		if err != nil {
			slog.Error("list activities", "user_id", userID, "error", err)
			totalErrors++
			continue
		}

		for _, act := range activities {
			// Count and delete non-rejected candidates before re-enrichment.
			// enrichStreamLifts does this too, but explicit pre-deletion ensures
			// the backfill is idempotent even if a previous run was interrupted
			// mid-flight (partial state).
			deleted, err := deleteNonRejectedCandidates(ctx, db.Pool, act.ID, userID)
			if err != nil {
				slog.Warn("pre-delete candidates failed", "activity_id", act.ID, "error", err)
				totalErrors++
				continue
			}
			totalDeleted += deleted

			settings := enrichment.StreamEnrichmentSettings{
				EnableLiftDetection: true,
			}
			if err := enrichment.RunStreamEnrichment(ctx, db, userID, act.ID, settings); err != nil {
				slog.Warn("re-enrich failed", "activity_id", act.ID, "error", err)
				totalErrors++
				continue
			}
			slog.Info("re-enriched", "activity_id", act.ID, "user_id", userID)
			totalEnriched++

			// Collect gear IDs so we can refresh stats once per gear after all
			// activities are processed (avoids N redundant UPDATE calls).
			if act.GearID != "" {
				affectedGears[act.GearID] = struct{}{}
			}
		}
	}

	// Refresh gear stats once per affected gear.
	gearErrors := 0
	for gearID := range affectedGears {
		if err := db.UpdateGearStats(ctx, gearID); err != nil {
			slog.Warn("UpdateGearStats failed", "gear_id", gearID, "error", err)
			gearErrors++
		}
	}

	// Run the acknowledged-at backfill after re-enrichment on --all-users path.
	if *allUsers {
		n, err := runAckBackfill(ctx, db.Pool)
		if err != nil {
			slog.Warn("ack backfill failed", "error", err)
		} else {
			slog.Info("ack backfill complete", "updated", n)
		}
	}

	slog.Info("backfill complete",
		"re_enriched", totalEnriched,
		"pre_deleted_candidates", totalDeleted,
		"errors", totalErrors,
		"gear_updates", len(affectedGears),
		"gear_errors", gearErrors,
	)

	if totalErrors > 0 || gearErrors > 0 {
		os.Exit(1)
	}
}

// liftEligibleActivity is the minimal activity projection needed by the backfill.
type liftEligibleActivity struct {
	ID     string
	GearID string
}

// listUsersWithLiftDetection returns user IDs that have enable_lift_detection=true
// in their enrichment_settings row.
func listUsersWithLiftDetection(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT user_id::text
		FROM user_enrichment_settings
		WHERE enable_lift_detection = true
	`)
	if err != nil {
		return nil, fmt.Errorf("listUsersWithLiftDetection: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("listUsersWithLiftDetection scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// listLiftEligibleActivities returns activities for a user that are either
// lift-sport activities or have lift_count > 0 (were previously classified as
// containing lifts).
func listLiftEligibleActivities(ctx context.Context, pool *pgxpool.Pool, userID string) ([]liftEligibleActivity, error) {
	liftSports := []string{
		"alpine_skiing",
		"snowboarding",
		"mtb_downhill",
		"ski",
		"ski_touring",
	}
	rows, err := pool.Query(ctx, `
		SELECT id::text, COALESCE(gear_id::text, '')
		FROM activities
		WHERE user_id = $1
		  AND (
		        sport_type = ANY($2)
		        OR lift_count > 0
		      )
		ORDER BY started_at DESC
	`, userID, liftSports)
	if err != nil {
		return nil, fmt.Errorf("listLiftEligibleActivities: %w", err)
	}
	defer rows.Close()

	var acts []liftEligibleActivity
	for rows.Next() {
		var a liftEligibleActivity
		if err := rows.Scan(&a.ID, &a.GearID); err != nil {
			return nil, fmt.Errorf("listLiftEligibleActivities scan: %w", err)
		}
		acts = append(acts, a)
	}
	return acts, rows.Err()
}

// deleteNonRejectedCandidates deletes activity_lift_candidates rows that are
// not soft-rejected by the user, for the given activity. Returns the number of
// deleted rows.
//
// enrichStreamLifts also performs this delete inside its transaction, but the
// explicit pre-deletion here makes the backfill idempotent if a previous run
// was interrupted between the delete and the re-enrichment commit.
func deleteNonRejectedCandidates(ctx context.Context, pool *pgxpool.Pool, activityID, userID string) (int, error) {
	tag, err := pool.Exec(ctx, `
		DELETE FROM activity_lift_candidates alc
		USING activities a
		WHERE alc.activity_id = $1
		  AND a.id = alc.activity_id
		  AND a.user_id = $2
		  AND NOT alc.rejected_by_user
	`, activityID, userID)
	if err != nil {
		return 0, fmt.Errorf("deleteNonRejectedCandidates: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// runAckBackfill sets activities.lifts_acknowledged_at = updated_at for every
// activity whose heuristic candidates are all already resolved (confirmed or
// rejected). Activities with pending heuristic candidates keep lifts_acknowledged_at
// = NULL so the review modal continues to appear.
//
// This prevents the review modal from popping up on all historic activities the
// first time a user visits them after the lift-detection migration
// (Adversarial-Review-Finding #22).
func runAckBackfill(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `
		UPDATE activities
		SET lifts_acknowledged_at = updated_at
		WHERE id IN (
		  SELECT activity_id
		  FROM activity_lift_candidates
		  GROUP BY activity_id
		  HAVING COUNT(*) FILTER (
		    WHERE source = 'heuristic'
		      AND confirmed = false
		      AND NOT rejected_by_user
		  ) = 0
		)
		AND lifts_acknowledged_at IS NULL
	`)
	if err != nil {
		return 0, fmt.Errorf("runAckBackfill: %w", err)
	}
	return tag.RowsAffected(), nil
}

