package dataimport

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// triggerEnrichment inserts background jobs that process imported activity data.
// Five jobs are queued: data_import_enrichment (activityCount total), then
// heatmap_rebuild, pmc_recalc, best_efforts_recalc, and segment_matching
// (each with total=1).
func triggerEnrichment(ctx context.Context, pool *pgxpool.Pool, userID string, activityCount int) error {
	type jobSpec struct {
		jobType string
		total   int64
	}
	jobs := []jobSpec{
		{"data_import_enrichment", int64(activityCount)},
		{"heatmap_rebuild", 1},
		{"pmc_recalc", 1},
		{"best_efforts_recalc", 1},
		{"segment_matching", 1},
	}

	for _, j := range jobs {
		id := uuid.New().String()
		_, err := pool.Exec(ctx, `
			INSERT INTO background_jobs (id, user_id, job_type, status, progress_total)
			VALUES ($1, $2, $3, 'queued', $4)
		`, id, userID, j.jobType, j.total)
		if err != nil {
			return fmt.Errorf("triggerEnrichment: insert %s: %w", j.jobType, err)
		}
	}
	return nil
}
