package dataimport

// conflictMode controls how an import row is reconciled with existing data.
type conflictMode int

const (
	cmInsert   conflictMode = iota // New UUIDs → no conflict possible.
	cmMergeID                      // Natural-key match → reuse existing ID.
	cmReplace                      // DELETE existing, then INSERT.
	cmUpsert                       // ON CONFLICT DO UPDATE (reserved, v1 unused).
	cmSkipWarn                     // FK target check, skip+warn if missing.
)

// tableSpec describes one importable table.
type tableSpec struct {
	Name       string
	Phase      int
	FKFields   []string
	PKField    string       // "id" for most, "" for composite/BIGSERIAL PK.
	Conflict   conflictMode
	NaturalKey []string     // Columns for conflict detection (besides user_id).
	UserCol    string       // "" = default "user_id", "none" = no user col, other = explicit name.
	Skip       bool         // true = don't import (best_efforts, import_log, users).
	Deferred   []string     // FK columns set NULL and updated in Phase 5b.
}

// getUserCol resolves the effective user-column name for a tableSpec.
// "none" → no user column; "" → default "user_id"; anything else → verbatim.
func getUserCol(spec tableSpec) string {
	switch spec.UserCol {
	case "none":
		return ""
	case "":
		return "user_id"
	default:
		return spec.UserCol
	}
}

// allTables returns the full ordered import configuration for all user tables.
func allTables() []tableSpec {
	return []tableSpec{
		// ── Phase 2: Root entities ─────────────────────────────────────────────

		{
			Name:       "tags",
			Phase:      2,
			PKField:    "id",
			FKFields:   []string{"cover_file_id"},
			Conflict:   cmMergeID,
			NaturalKey: []string{"name"},
		},
		{
			Name:     "training_zones",
			Phase:    2,
			PKField:  "id",
			Conflict: cmReplace,
		},
		{
			Name:    "devices",
			Phase:   2,
			PKField: "id",
		},
		{
			Name:     "recording_groups",
			Phase:    2,
			PKField:  "id",
			Deferred: []string{"primary_activity_id"},
		},
		{
			Name:       "workouts",
			Phase:      2,
			PKField:    "id",
			Conflict:   cmMergeID,
			NaturalKey: []string{"name"},
		},
		{
			Name:       "custom_maps",
			Phase:      2,
			PKField:    "id",
			Conflict:   cmMergeID,
			NaturalKey: []string{"name"},
		},
		{
			Name:       "routing_profiles",
			Phase:      2,
			PKField:    "id",
			Conflict:   cmMergeID,
			NaturalKey: []string{"name"},
		},
		{
			Name:     "user_enrichment_settings",
			Phase:    2,
			PKField:  "id",
			Conflict: cmReplace,
		},
		{
			Name:     "ml_user_settings",
			Phase:    2,
			PKField:  "id",
			Conflict: cmReplace,
		},
		{
			Name:     "llm_user_config",
			Phase:    2,
			PKField:  "id",
			Conflict: cmReplace,
		},
		{
			Name:    "user_file_schemas",
			Phase:   2,
			PKField: "id",
		},
		{
			Name:    "user_privacy_zones",
			Phase:   2,
			PKField: "id",
			// Natural key: [name, ST_AsText(location)] — location requires
			// geometry serialisation; handled separately in the merge logic.
			Conflict:   cmMergeID,
			NaturalKey: []string{"name"},
		},
		{
			Name:    "automation_rules",
			Phase:   2,
			PKField: "id",
		},
		{
			Name:    "oauth_connections",
			Phase:   2,
			PKField: "id",
		},

		// ── Phase 3: FK to Phase 2 ─────────────────────────────────────────────

		{
			Name:     "tag_zones",
			Phase:    3,
			PKField:  "id",
			FKFields: []string{"tag_id"},
		},
		{
			Name:     "custom_map_markers",
			Phase:    3,
			PKField:  "id",
			FKFields: []string{"map_id", "image_file_id"},
		},
		{
			Name:     "workout_steps",
			Phase:    3,
			PKField:  "id",
			FKFields: []string{"workout_id"},
		},
		{
			Name:     "chain_rotation_groups",
			Phase:    3,
			PKField:  "id",
			FKFields: []string{"gear_id"},
		},
		{
			Name:     "chain_rotation_members",
			Phase:    4,
			PKField:  "id",
			FKFields: []string{"group_id", "component_id"},
		},
		{
			Name:       "gear",
			Phase:      3,
			PKField:    "id",
			Conflict:   cmMergeID,
			NaturalKey: []string{"name", "type"},
			Deferred:   []string{"source_document_id"},
		},
		{
			Name:    "planned_routes",
			Phase:   3,
			PKField: "id",
		},
		{
			Name:     "segments",
			Phase:    3,
			PKField:  "id",
			UserCol:  "created_by",
			Deferred: []string{"source_activity_id"},
		},

		// ── Phase 4: FK to Phase 3 ─────────────────────────────────────────────

		{
			Name:     "gear_components",
			Phase:    4,
			PKField:  "id",
			FKFields: []string{"gear_id", "parent_component_id"},
			Deferred: []string{"source_document_id"},
		},
		{
			Name:     "component_assignments",
			Phase:    4,
			PKField:  "id",
			FKFields: []string{"component_id", "gear_id"},
		},
		{
			// Must come before gear_service_events (events.interval_id → intervals.id).
			Name:     "gear_service_intervals",
			Phase:    4,
			PKField:  "id",
			FKFields: []string{"component_id"},
		},
		{
			Name:     "gear_service_events",
			Phase:    4,
			PKField:  "id",
			FKFields: []string{"component_id", "interval_id"},
		},
		{
			Name:     "gear_documents",
			Phase:    4,
			PKField:  "id",
			FKFields: []string{"gear_id", "component_id", "service_event_id", "file_id"},
		},
		{
			Name:     "bike_setups",
			Phase:    4,
			PKField:  "id",
			FKFields: []string{"gear_id"},
		},
		{
			Name:     "route_photos",
			Phase:    4,
			PKField:  "id",
			FKFields: []string{"route_id", "file_id"},
		},
		{
			Name:     "route_tags",
			Phase:    4,
			PKField:  "id",
			FKFields: []string{"route_id", "tag_id"},
		},
		{
			// Composite PK; skip+warn if referenced segment doesn't exist on target.
			Name:     "segment_favorites",
			Phase:    4,
			PKField:  "",
			FKFields: []string{"segment_id"},
			Conflict: cmSkipWarn,
		},

		// ── Phase 5: Activities + files ────────────────────────────────────────

		{
			Name:     "activities",
			Phase:    5,
			PKField:  "id",
			FKFields: []string{"gear_id", "recording_group_id", "file_id", "sidecar_file_id", "parent_id", "recording_device_id", "workout_id"},
			Deferred: []string{"parent_id"},
		},

		// ── Phase 6: Activity-dependent ────────────────────────────────────────

		{
			Name:     "activity_tags",
			Phase:    6,
			PKField:  "id",
			FKFields: []string{"activity_id", "tag_id"},
		},
		{
			Name:     "activity_edits",
			Phase:    6,
			PKField:  "id",
			FKFields: []string{"activity_id"},
		},
		{
			Name:     "activity_photos",
			Phase:    6,
			PKField:  "id",
			FKFields: []string{"activity_id", "file_id"},
		},
		{
			// Composite PK (activity_id, application_id where application_id is BYTEA).
			// No FK to an external UUID table — application_id is device-assigned BYTEA,
			// not a UUID referencing another table. cmInsert is safe; no collision possible.
			Name:     "activity_developer_apps",
			Phase:    6,
			PKField:  "",
			FKFields: []string{"activity_id", "device_id"},
			Conflict: cmInsert,
		},
		{
			// Composite PK.
			Name:     "best_effort_exclusions",
			Phase:    6,
			PKField:  "",
			FKFields: []string{"activity_id"},
		},
		{
			Name:     "recording_group_members",
			Phase:    6,
			PKField:  "id",
			FKFields: []string{"group_id", "activity_id"},
		},
		{
			Name:    "chat_sessions",
			Phase:   6,
			PKField: "id",
		},
		{
			Name:     "chat_messages",
			Phase:    6,
			PKField:  "id",
			FKFields: []string{"session_id"},
			UserCol:  "none",
		},

		// ── Phase 7: External ──────────────────────────────────────────────────

		{
			// entity_id is resolved dynamically (polymorphic FK).
			// Phase 7 because entity_id may reference Phase 6 entities
			// (chat_sessions, activity_photos, etc.).
			Name:     "file_attachments",
			Phase:    7,
			PKField:  "id",
			FKFields: []string{"file_id"},
		},
		{
			Name:     "activity_weather",
			Phase:    7,
			PKField:  "id",
			FKFields: []string{"activity_id"},
		},

		// ── Skipped ────────────────────────────────────────────────────────────

		{Name: "best_efforts", Skip: true},
		{Name: "import_log", Skip: true},
		{Name: "users", Skip: true},
	}
}

// healthTables returns the 12 health tables for import.
// All health tables use cmInsert; doInsert appends ON CONFLICT DO NOTHING so
// duplicate rows (same natural key) are silently skipped rather than failing.
func healthTables() []tableSpec {
	names := []string{
		"health_daily_summary",
		"health_sleep",
		"health_snapshots",
		"health_hrv_night_values",
		"health_monitoring_samples",
		"health_weight_entries",
		"health_blood_pressure",
		"health_fitness_age",
		"health_stress_snapshots",
		"health_spo2_snapshots",
		"health_training_load",
		"health_readiness_scores",
	}

	specs := make([]tableSpec, len(names))
	for i, name := range names {
		specs[i] = tableSpec{
			Name:     name,
			Phase:    7,
			PKField:  "",
			Conflict: cmInsert,
		}
	}
	return specs
}

// profileAllowList is the set of profile columns that may be imported.
var profileAllowList = []string{
	"timezone", "display_name", "bio", "measurement_units",
	"date_format", "week_start", "default_sport_type",
	"height_cm", "weight_kg", "birth_year", "gender",
	"max_hr", "resting_hr", "ftp", "run_ftp",
}

// jsonbRemapFields lists the JSONB *column names* (top-level row columns) that
// must be walked and have embedded UUID references remapped through the IDMap.
var jsonbRemapFields = map[string][]string{
	"automation_rules": {"actions", "conditions"},
}

// jsonbRemapKeys is the set of JSON object keys whose string values are UUID
// references that must be remapped through the IDMap during walkAndRemap.
//
// These correspond to the actual fields in actionParams (rules/engine.go):
//   - gear_id  → set_gear action (references gear table)
//   - tag_id   → add_tag / remove_tag actions (references tags table)
//
// Condition JSONB also contains segment_id inside has_segment_effort /
// segment_effort_rank / segment_effort_time_s condition values:
//   - segment_id → references segments table
var jsonbRemapKeys = map[string]bool{
	"gear_id":    true,
	"tag_id":     true,
	"segment_id": true,
}

// fileRefColumns lists FK columns that reference user_files.
// During prepareRow, these use idMap.GetIfExists (not Get) to avoid
// generating phantom UUIDs for files that weren't in the export.
var fileRefColumns = map[string][]string{
	"tags":               {"cover_file_id"},
	"gear_documents":     {"file_id"},
	"route_photos":       {"file_id"},
	"custom_map_markers": {"image_file_id"},
	"activities":         {"file_id", "sidecar_file_id"},
	"activity_photos":    {"file_id"},
}

// isFileRefColumn reports whether col on table is a file-reference column.
func isFileRefColumn(table, col string) bool {
	for _, c := range fileRefColumns[table] {
		if c == col {
			return true
		}
	}
	return false
}

// ExportedTableSpecs returns all table specs (including skipped entries) for
// use in CI tests that verify registry completeness.
func ExportedTableSpecs() []tableSpec {
	return allTables()
}

// findSpecByName returns the tableSpec with the given name from allTables and
// healthTables. Returns a zero-value tableSpec if not found.
func findSpecByName(name string) tableSpec {
	return findSpec(name, append(allTables(), healthTables()...))
}
