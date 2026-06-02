package database

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// knownJobTypes is the authoritative list of job_type values written to background_jobs.
// Add a new entry here whenever a new job type is introduced so notification
// payload tests keep covering the structured contract consumed by UI clients.
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
	"segment_recalculation",
	"lift_recompute",
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

func TestJobCompletionNotificationDataPreservesKnownJobTypes(t *testing.T) {
	for _, jt := range knownJobTypes {
		payload, err := jobCompletionNotificationData(jt, nil, false)
		if err != nil {
			t.Fatalf("jobCompletionNotificationData(%q): %v", jt, err)
		}

		var data struct {
			Key    string         `json:"key"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(payload, &data); err != nil {
			t.Fatalf("unmarshal payload for %q: %v", jt, err)
		}

		if data.Key != "notifications.jobs.completed" {
			t.Fatalf("key for %q = %q, want notifications.jobs.completed", jt, data.Key)
		}
		if data.Params["jobType"] != jt {
			t.Fatalf("jobType param for %q = %#v", jt, data.Params["jobType"])
		}
	}
}

func TestJobCompletionNotificationDataUsesStructuredKey(t *testing.T) {
	entityName := "Morning Ride"
	payload, err := jobCompletionNotificationData("garmin_import", &entityName, false)
	if err != nil {
		t.Fatalf("jobCompletionNotificationData: %v", err)
	}

	var data struct {
		Key    string         `json:"key"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if data.Key != "notifications.jobs.completed" {
		t.Fatalf("key = %q, want notifications.jobs.completed", data.Key)
	}
	if data.Params["jobType"] != "garmin_import" {
		t.Fatalf("jobType param = %#v", data.Params["jobType"])
	}
	if data.Params["entityName"] != entityName {
		t.Fatalf("entityName param = %#v", data.Params["entityName"])
	}
}

func TestStructuredNotificationDataCarriesCanonicalAndColumnStyleKeys(t *testing.T) {
	params := map[string]any{"ruleName": "Morning rule"}

	data := structuredNotificationData("notifications.automation.ruleNotification", params)

	if data["key"] != "notifications.automation.ruleNotification" {
		t.Fatalf("key = %#v", data["key"])
	}
	if data["message_key"] != "notifications.automation.ruleNotification" {
		t.Fatalf("message_key = %#v", data["message_key"])
	}
	if !reflect.DeepEqual(data["params"], params) {
		t.Fatalf("params not preserved: %#v", data["params"])
	}
	if !reflect.DeepEqual(data["message_params"], params) {
		t.Fatalf("message_params not preserved: %#v", data["message_params"])
	}
}

func TestJobCompletionTitleUsesGenericFallback(t *testing.T) {
	entityName := "Morning Ride"

	got := jobCompletionTitle("garmin_import", &entityName, false)
	if got != "Hintergrundjob - Morning Ride abgeschlossen" {
		t.Fatalf("completion title = %q", got)
	}

	got = jobCompletionTitle("garmin_import", nil, true)
	if got != "Hintergrundjob fehlgeschlagen" {
		t.Fatalf("failed title = %q", got)
	}
}

func TestJobCompletionTitleDoesNotExposeUnknownJobType(t *testing.T) {
	got := jobCompletionTitle("private_unknown_job", nil, true)
	if strings.Contains(got, "private_unknown_job") {
		t.Fatalf("unknown job type leaked into title: %q", got)
	}
}
