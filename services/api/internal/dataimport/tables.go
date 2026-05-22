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
	Name            string
	Phase           int
	FKFields        []string
	PKField         string // "id" for most, "" for composite/BIGSERIAL PK.
	Conflict        conflictMode
	NaturalKey      []string // Columns for conflict detection (besides user_id).
	UserCol         string   // "" = default "user_id", "none" = no user col, other = explicit name.
	Skip            bool     // true = don't import (best_efforts, import_log, users).
	Deferred        []string // FK columns set NULL and updated in Phase 5b.
	IgnoreConflicts bool     // true = append ON CONFLICT DO NOTHING (composite-PK / idempotent tables).
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
			Name:       "user_map_layers",
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
			Name:     "chat_user_preferences",
			Phase:    2,
			PKField:  "",
			Conflict: cmReplace,
		},
		{
			Name:     "user_readiness_preferences",
			Phase:    2,
			PKField:  "",
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
			Name:       "automation_rules",
			Phase:      2,
			PKField:    "id",
			Conflict:   cmMergeID,
			NaturalKey: []string{"name"},
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
			Name:     "target_places",
			Phase:    3,
			PKField:  "id",
			UserCol:  "owner_user_id",
			FKFields: []string{"access_scope_id"},
		},
		{
			Name:     "target_regions",
			Phase:    3,
			PKField:  "id",
			UserCol:  "owner_user_id",
			FKFields: []string{"access_scope_id"},
		},
		{
			Name:     "target_collections",
			Phase:    3,
			PKField:  "id",
			UserCol:  "owner_user_id",
			FKFields: []string{"access_scope_id"},
		},
		{
			Name:     "segments",
			Phase:    3,
			PKField:  "id",
			UserCol:  "created_by",
			FKFields: []string{"access_scope_id", "source_activity_publish_confirmed_by"},
			Deferred: []string{"source_activity_id", "source_activity_publish_confirmed_segment_version_id"},
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
			// gear_schema_definitions has only a user_id FK; no Phase-2/3 table FKs.
			// Phase 3 so it lands before Phase-4 tables that may reference gear.
			Name:    "gear_schema_definitions",
			Phase:   3,
			PKField: "id",
		},
		{
			// gear_component_swap_events FK to gear (Phase 3) and optionally to
			// gear_components (Phase 4). retired_component_id / new_component_id are
			// nullable. They are deferred so Phase 5b resolves them after all UUID
			// remappings are done; this avoids phantom UUID generation when the
			// referenced component wasn't included in the export.
			Name:     "gear_component_swap_events",
			Phase:    4,
			PKField:  "id",
			FKFields: []string{"gear_id"},
			Deferred: []string{"retired_component_id", "new_component_id"},
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
		{
			Name:     "target_collection_members",
			Phase:    4,
			PKField:  "id",
			UserCol:  "added_by",
			FKFields: []string{"collection_id", "member_id"},
			Conflict: cmSkipWarn,
		},
		{
			// Geometry lives here (track/start_point/end_point) — without this
			// an imported segment has no routeable shape. Phase 4 so the parent
			// segment already exists with remapped id.
			Name:     "segment_versions",
			Phase:    4,
			PKField:  "id",
			UserCol:  "created_by",
			FKFields: []string{"segment_id"},
		},
		{
			Name:     "course_version_controls",
			Phase:    4,
			PKField:  "id",
			UserCol:  "none",
			FKFields: []string{"segment_id", "segment_version_id"},
		},

		// ── Phase 5: Activities + files ────────────────────────────────────────

		{
			// gear_id was dropped from activities (Wave-5/Cluster I); legacy
			// exports still carry the field on the row but Phase 5 captures
			// it directly via rawRow["gear_id"] and Phase 6c synthesizes the
			// activity_gear_assignments row. Listing it here would have no
			// effect (it is no longer a target column) but is misleading.
			Name:     "activities",
			Phase:    5,
			PKField:  "id",
			FKFields: []string{"recording_group_id", "file_id", "sidecar_file_id", "parent_id", "recording_device_id", "workout_id"},
			Deferred: []string{"parent_id"},
		},

		// ── Phase 6: Activity-dependent ────────────────────────────────────────

		{
			// activity_gear_assignments FK to gear (Phase 3) and activities (Phase 5).
			// Phase 6 so that activities are already imported before assignments are inserted.
			Name:     "activity_gear_assignments",
			Phase:    6,
			PKField:  "id",
			FKFields: []string{"gear_id", "activity_id"},
		},
		{
			Name:     "activity_lift_candidates",
			Phase:    6,
			PKField:  "id",
			FKFields: []string{"activity_id"},
		},
		{
			Name:            "activity_fusion_memberships",
			Phase:           6,
			PKField:         "",
			FKFields:        []string{"merged_id", "source_id"},
			Conflict:        cmInsert,
			IgnoreConflicts: true, // composite PK; idempotent with ON CONFLICT DO NOTHING
		},
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
			FKFields: []string{"activity_id", "file_id", "source_activity_id"},
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
		{
			Name:     "target_activity_observations",
			Phase:    7,
			PKField:  "id",
			UserCol:  "user_id",
			FKFields: []string{"subject_id", "target_version_id", "activity_id", "access_scope_id"},
			Conflict: cmSkipWarn,
		},
		{
			Name:     "target_reports",
			Phase:    7,
			PKField:  "id",
			UserCol:  "author_user_id",
			FKFields: []string{"subject_id", "target_version_id", "source_activity_id", "access_scope_id"},
			Conflict: cmSkipWarn,
		},
		{
			Name:     "target_user_content",
			Phase:    7,
			PKField:  "id",
			UserCol:  "user_id",
			FKFields: []string{"subject_id", "target_version_id", "activity_id", "access_scope_id"},
			Conflict: cmSkipWarn,
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
	"automation_rules": {"actions", "conditions", "condition_groups", "branches"},
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
	"file_attachments":   {"file_id"},
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

// ImportCoverageTableNamesForGDPRGuard returns table names that the GDPR import
// path either imports through table specs or handles explicitly outside the
// generic table loop.
func ImportCoverageTableNamesForGDPRGuard() map[string]bool {
	covered := make(map[string]bool)
	for _, spec := range allTables() {
		if spec.Skip {
			continue
		}
		covered[spec.Name] = true
	}
	for _, spec := range healthTables() {
		covered[spec.Name] = true
	}
	for _, table := range []string{
		"users",      // data/profile.json is handled by runPhase1.
		"user_files", // metadata plus blobs are handled by importAllFiles.
	} {
		covered[table] = true
	}
	return covered
}

// ImportSkippedTableRationalesForGDPRGuard documents exported tables that are
// intentionally not imported by the generic GDPR table path.
func ImportSkippedTableRationalesForGDPRGuard() map[string]string {
	return map[string]string{
		"best_efforts":                           "Derived activity achievement rows are recomputed from imported activities.",
		"governance_actor_roles":                 "Governance roles require live actors and explicit local trust; import intentionally does not recreate edit/admin permissions.",
		"governance_actor_target_projection":     "Governance actor-target rows are derived authorization projections and are rebuilt from local governance state.",
		"governance_scope_target_bindings":       "Governance bindings require live scopes and targets; import intentionally does not recreate edit/admin permissions.",
		"governance_scopes":                      "Governance scopes are access-control state; imported objects start without delegated edit/admin authority.",
		"import_log":                             "Historical import audit rows are not restored; imported activities/files carry the user data.",
		"object_access_grants":                   "Sharing grants need live principals and scopes; export keeps the metadata, import intentionally does not recreate sharing state.",
		"object_access_scopes":                   "Sharing scopes are access-control state; export keeps the metadata, import intentionally recreates private objects without old shares.",
		"planned_route_snapshots":                "Route publication snapshots are immutable publication history; planned_routes remain importable and can be republished after import.",
		"route_course_publications":              "Published-course rows depend on immutable segment/version ids and access scopes; planned_routes and segments remain importable separately.",
		"route_course_relationships":             "Published-course variant edges are publication graph metadata; import intentionally avoids recreating public graph state.",
		"route_profile_snapshots":                "Route profile snapshots are derived from route geometry/enrichment and can be regenerated.",
		"route_publication_events":               "Publication events are audit metadata; import intentionally does not replay historical publication operations.",
		"route_publication_privacy_reviews":      "Privacy reviews are publication-governance metadata; import requires a fresh review before publishing again.",
		"route_relationships":                    "Route forks and variants can be recreated explicitly; import avoids restoring cross-route graph metadata before ownership is revalidated.",
		"target_change_events":                   "Target object change events are audit metadata; import intentionally starts imported objects with fresh local audit history.",
		"target_report_status_events":            "Target report status events are moderation history; import intentionally does not replay historical status changes.",
		"target_collection_member_reverse_index": "Collection reverse-index rows are derived from target_collection_members and rebuilt locally.",
		"target_collection_summary_projection":   "Collection summary rows are derived from target_collections and target_collection_members and rebuilt locally.",
		"target_object_search_projection":        "Target object search rows are derived from target_places and target_regions and rebuilt locally.",
		"target_revision_snapshots":              "Target object revision snapshots can contain private historical state; import intentionally does not replay historical snapshots.",
	}
}

// findSpecByName returns the tableSpec with the given name from allTables and
// healthTables. Returns a zero-value tableSpec if not found.
func findSpecByName(name string) tableSpec {
	return findSpec(name, append(allTables(), healthTables()...))
}
