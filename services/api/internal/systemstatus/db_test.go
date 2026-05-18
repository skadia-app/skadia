package systemstatus

import "testing"

func TestDBStatusFromMigrationDirty(t *testing.T) {
	got := DBStatusFromSnapshot(DBSnapshot{PingOK: true, MigrationVersion: 370, MigrationDirty: true, PostGISVersion: "3.5"})
	if got.State != StatusDegraded || got.ReasonCode != "migration_dirty" {
		t.Fatalf("status = %+v, want degraded migration_dirty", got)
	}
}

func TestDBStatusRequiresPostGIS(t *testing.T) {
	got := DBStatusFromSnapshot(DBSnapshot{PingOK: true, MigrationVersion: 370})
	if got.State != StatusDegraded || got.ReasonCode != "postgis_missing" {
		t.Fatalf("status = %+v, want degraded postgis_missing", got)
	}
}

func TestDBStatusReportsMissingExpectedExtension(t *testing.T) {
	got := DBStatusFromSnapshot(DBSnapshot{
		PingOK:           true,
		MigrationVersion: 370,
		Extensions: map[string]ExtensionSnapshot{
			"postgis":     {Installed: true, Version: "3.5.0"},
			"timescaledb": {Installed: false},
		},
		ExpectedExtensions: []string{"postgis", "timescaledb"},
	})
	if got.State != StatusDegraded || got.ReasonCode != "db_extension_missing" {
		t.Fatalf("status = %+v, want degraded db_extension_missing", got)
	}
}

func TestDBStatusDoesNotDegradeOnMissingOptionalExtension(t *testing.T) {
	got := DBStatusFromSnapshot(DBSnapshot{
		PingOK:           true,
		MigrationVersion: 370,
		PostGISVersion:   "3.5.0",
		Extensions: map[string]ExtensionSnapshot{
			"postgis": {Installed: true, Version: "3.5.0"},
		},
		ExpectedExtensions: []string{"postgis"},
		OptionalExtensions: []string{"vectorscale"},
	})
	if got.State != StatusOK || got.Details["optional_extension_vectorscale"] != "missing" {
		t.Fatalf("status = %+v, want ok with optional extension note", got)
	}
}

func TestDBStatusReportsCompressionDrift(t *testing.T) {
	got := DBStatusFromSnapshot(DBSnapshot{
		PingOK:           true,
		MigrationVersion: 370,
		PostGISVersion:   "3.5.0",
		Extensions: map[string]ExtensionSnapshot{
			"postgis":     {Installed: true, Version: "3.5.0"},
			"timescaledb": {Installed: true, Version: "2.17.0"},
		},
		ExpectedExtensions: []string{"postgis", "timescaledb"},
		Compression: CompressionSnapshot{
			SettingEnabled:       true,
			HypertableCompressed: true,
			PolicyPresent:        false,
			SegmentBy:            "activity_id",
			OrderBy:              "time DESC",
		},
	})
	if got.State != StatusDegraded || got.ReasonCode != "timescale_compression_policy_missing" {
		t.Fatalf("status = %+v, want degraded timescale_compression_policy_missing", got)
	}
}

func TestDBStatusReportsDisabledSettingButCompressedTable(t *testing.T) {
	got := DBStatusFromSnapshot(DBSnapshot{
		PingOK:           true,
		MigrationVersion: 370,
		PostGISVersion:   "3.5.0",
		Compression: CompressionSnapshot{
			SettingEnabled:       false,
			HypertableCompressed: true,
			PolicyPresent:        true,
			SegmentBy:            "activity_id",
			OrderBy:              "time DESC",
		},
	})
	if got.State != StatusDegraded || got.ReasonCode != "timescale_compression_setting_drift" {
		t.Fatalf("status = %+v, want degraded timescale_compression_setting_drift", got)
	}
}

func TestCompressionStatusDisabledWhenSettingOffAndDBOff(t *testing.T) {
	got := CompressionStatusFromSnapshot(CompressionSnapshot{SettingEnabled: false})
	if got.State != StatusDisabled || got.ReasonCode != "timescale_compression_disabled" {
		t.Fatalf("status = %+v, want disabled timescale_compression_disabled", got)
	}
}
