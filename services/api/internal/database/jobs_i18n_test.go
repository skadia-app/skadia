package database

import "testing"

func TestApplyBackgroundJobI18nMetadataExposesOnlyStructuredFields(t *testing.T) {
	job := BackgroundJob{
		Metadata: []byte(`{
			"message_code":"admin.osm.import.failed",
			"status_code":"failed",
			"entity_key":"osm.import",
			"params":{"region_id":"europe"},
			"artifact":{"private":"not exported as metadata"}
		}`),
	}

	applyBackgroundJobI18nMetadata(&job)

	if job.MessageCode != "admin.osm.import.failed" {
		t.Fatalf("MessageCode = %q", job.MessageCode)
	}
	if job.StatusCode != "failed" {
		t.Fatalf("StatusCode = %q", job.StatusCode)
	}
	if job.EntityKey != "osm.import" {
		t.Fatalf("EntityKey = %q", job.EntityKey)
	}
	if job.Params["region_id"] != "europe" {
		t.Fatalf("Params = %#v", job.Params)
	}
}
