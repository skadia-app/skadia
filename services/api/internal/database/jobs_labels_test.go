package database

import (
	"strings"
	"testing"
)

// knownJobTypes is the authoritative list of job_type values written to background_jobs.
// Add a new entry here whenever a new job type is introduced — the test below enforces
// that the label map covers it, so a missing label becomes a CI failure rather than a
// silent raw-string in the UI.
var knownJobTypes = []string{
	"osm_import",
	"osm_enrichment",
	"osm_update",
	"osm_update_all",
	"garmin_import",
	"strava_import",
	"komoot_import",
	"data_import",
	"data_import_enrichment",
	"gdpr_export",
	"gdpr_purge",
	"segment_recompute",
	"segment_publish",
	"segment_matching",
	"activity_recompute",
	"activity_recompute_all",
	"activity_re_enrich",
	"backfill_activity_enrichments",
	"recompute_multisport",
	"photo_preview_recompute",
	"photo_thumb_recompute",
	"photo_clip_embed",
	"photo_embed_backfill",
	"phash_backfill",
	"thumbnail_backfill",
	"meta_backfill",
	"elevation_reset",
	"elevation_correction",
	"distance_correction",
	"distance_reset",
	"dem_import",
	"dem_download",
	"apply_rules",
	"rule_apply",
	"routing_graph_build",
	"tile_build",
	"brouter_segment_build",
	"brouter_segment_download",
	"coverage_build",
	"weather_backfill",
	"geocoding_backfill",
	"heatmap_rebuild",
	"pmc_recalc",
	"recalc_best_efforts",
	"recalc_best_efforts_all",
	"best_efforts_recalc",
	"recompute_all_tss",
	"fusion_queue_compute",
	"shadowfs_reconcile",
	"shadowfs_rename",
	"compression_toggle",
	"device_orphan_cleanup",
}

func TestJobTypeLabelsCoversAllKnownTypes(t *testing.T) {
	var missing []string
	for _, jt := range knownJobTypes {
		if _, ok := jobTypeLabels[jt]; !ok {
			missing = append(missing, jt)
		}
	}
	if len(missing) > 0 {
		t.Errorf("job types missing from jobTypeLabels (add labels to jobs.go AND frontend/src/lib/jobLabels.ts):\n  %s",
			strings.Join(missing, "\n  "))
	}
}
