package handler

import (
	"encoding/json"
	"testing"
)

func TestAdminJobConflictResponseKeepsLegacyErrorAndAddsStableCodes(t *testing.T) {
	resp := adminJobConflictResponse("Import läuft bereits", adminJobI18n{
		MessageCode: "admin.osm.import.already_running",
		StatusCode:  "already_running",
		EntityKey:   "osm.import",
		Params: map[string]any{
			"job_type": "osm_import",
		},
	})

	if resp["error"] != "Import läuft bereits" {
		t.Fatalf("legacy error = %q, want German compatibility text", resp["error"])
	}
	if resp["error_code"] != "admin.osm.import.already_running" {
		t.Fatalf("error_code = %q, want stable message code", resp["error_code"])
	}
	if resp["message_code"] != "admin.osm.import.already_running" {
		t.Fatalf("message_code = %q, want stable message code", resp["message_code"])
	}
	if resp["status_code"] != "already_running" {
		t.Fatalf("status_code = %q, want already_running", resp["status_code"])
	}
	if resp["entity_key"] != "osm.import" {
		t.Fatalf("entity_key = %q, want osm.import", resp["entity_key"])
	}
	params, ok := resp["params"].(map[string]any)
	if !ok {
		t.Fatalf("params type = %T, want map[string]any", resp["params"])
	}
	if params["job_type"] != "osm_import" {
		t.Fatalf("params.job_type = %q, want osm_import", params["job_type"])
	}
}

func TestAdminJobAcceptedResponseKeepsLegacyFieldsAndAddsStableCodes(t *testing.T) {
	resp := adminJobAcceptedResponse("job-1", map[string]any{"status": "started"}, adminJobI18n{
		MessageCode: "admin.routing.tiles.started",
		StatusCode:  "started",
		EntityKey:   "routing.tiles",
		Params: map[string]any{
			"region_count": 2,
			"force":        true,
		},
	})

	if resp["job_id"] != "job-1" {
		t.Fatalf("job_id = %q, want job-1", resp["job_id"])
	}
	if resp["status"] != "started" {
		t.Fatalf("legacy status = %q, want started", resp["status"])
	}
	if resp["message_code"] != "admin.routing.tiles.started" {
		t.Fatalf("message_code = %q, want admin.routing.tiles.started", resp["message_code"])
	}
	if resp["entity_key"] != "routing.tiles" {
		t.Fatalf("entity_key = %q, want routing.tiles", resp["entity_key"])
	}
	params := resp["params"].(map[string]any)
	if params["region_count"] != 2 {
		t.Fatalf("params.region_count = %#v, want 2", params["region_count"])
	}
	if params["force"] != true {
		t.Fatalf("params.force = %#v, want true", params["force"])
	}
}

func TestAdminJobI18nMetadataJSONOmitsEmptyParams(t *testing.T) {
	raw, err := json.Marshal(adminJobI18n{
		MessageCode: "admin.dem.download.started",
		StatusCode:  "started",
		EntityKey:   "dem.download",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != `{"message_code":"admin.dem.download.started","status_code":"started","entity_key":"dem.download"}` {
		t.Fatalf("metadata json = %s", raw)
	}
}

func TestAdminJobFailureMetadataCarriesFailedStatus(t *testing.T) {
	raw, err := json.Marshal(adminJobI18n{
		MessageCode: "osm.import.download_failed",
		StatusCode:  "failed",
		Params:      map[string]any{"region_id": "europe"},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["status_code"] != "failed" {
		t.Fatalf("status_code = %q, want failed", got["status_code"])
	}
}
