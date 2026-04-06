package auth

// TableCategory classifies how a user-related table is handled for GDPR export and deletion.
type TableCategory int

const (
	// Export — Primary user data; must be included in data export.
	Export TableCategory = iota
	// External — Externally sourced data (health, weather); should be exported.
	External
	// Derived — Derivable from primary data; regenerated on re-import.
	Derived
	// System — Security/transient data; not exported.
	System
	// Global — No user reference; no GDPR relevance.
	Global
)

// GDPRRegistry maps every user-related (and relevant global) table to its TableCategory.
// A compile-time test verifies this registry against the live DB schema — any table
// with a user_id column that is absent here will fail CI.
var GDPRRegistry = map[string]TableCategory{
	// -------------------------------------------------------------------------
	// Export — primary user data
	// -------------------------------------------------------------------------
	"users":                    Export,
	"activities":               Export,
	"activity_edits":           Export,
	"activity_tags":            Export,
	"activity_photos":          Export,
	"activity_developer_apps":  Export,
	"automation_rules":         Export,
	"best_effort_exclusions":   Export,
	"best_efforts":             Export, // ableitbar aber günstig; teuer zu recomputen
	"bike_setups":              Export,
	"chain_rotation_groups":    Export,
	"chain_rotation_members":   Export,
	"chat_messages":            Export,
	"chat_sessions":            Export,
	"component_assignments":    Export,
	"custom_map_markers":       Export,
	"custom_maps":              Export,
	"devices":                  Export,
	"file_attachments":         Export,
	"gear":                     Export,
	"gear_components":          Export,
	"gear_documents":           Export,
	"gear_service_events":      Export,
	"gear_service_intervals":   Export,
	"llm_user_config":          Export,
	"ml_user_settings":         Export,
	"oauth_connections":        Export, // Metadaten; Tokens in Deny-List
	"planned_routes":           Export,
	"recording_group_members":  Export,
	"recording_groups":         Export,
	"route_photos":             Export,
	"route_tags":               Export,
	"routing_profiles":         Export,
	"segments":                 Export,
	"segment_favorites":        Export, // Kann fremde Segment-IDs referenzieren → Importer muss skip+warn
	"tag_zones":                Export,
	"tags":                     Export,
	"training_zones":           Export,
	"user_enrichment_settings": Export,
	"user_file_schemas":        Export,
	"user_files":               Export, // Metadaten; Binaries via ShadowFS
	"user_privacy_zones":       Export,
	"workouts":                 Export,
	"workout_steps":            Export,

	// -------------------------------------------------------------------------
	// External — externally sourced; should be exported
	// -------------------------------------------------------------------------
	"activity_weather":          External,
	"health_blood_pressure":     External,
	"health_daily_summary":      External,
	"health_fitness_age":        External,
	"health_hrv_night_values":   External,
	"health_monitoring_samples": External,
	"health_readiness_scores":   External,
	"health_sleep":              External,
	"health_snapshots":          External, // ehem. health_hrv_snapshots, Mig 000113
	"health_spo2_snapshots":     External,
	"health_stress_snapshots":   External,
	"health_training_load":      External,
	"health_weight_entries":     External,
	"import_log":                External,

	// -------------------------------------------------------------------------
	// Derived — regenerated on re-import; exported for convenience only
	// -------------------------------------------------------------------------
	"activity_climb_pro_events": Derived,
	"activity_climbs":           Derived,
	"activity_detected_laps":    Derived,
	"activity_dive_gases":       Derived, // FIT-dive data, cascades from activities
	"activity_dive_settings":    Derived, // FIT-dive data, cascades from activities
	"activity_events":           Derived,
	"activity_geo_tags":         Derived,
	"activity_hr_data":          Derived,
	"activity_intervals":        Derived,
	"activity_jumps":            Derived,
	"activity_laps":             Derived,
	"activity_lengths":          Derived,
	"activity_lift_candidates":  Derived,
	"activity_privacy_sections": Derived,
	"activity_rr_intervals":     Derived,
	"activity_segment_laps":     Derived,
	"activity_sensors":          Derived,
	"activity_sets":             Derived,
	"activity_splits":           Derived,
	"activity_split_summaries":  Derived,
	"activity_stops":            Derived,
	"activity_streams":          Derived,
	"activity_surfaces":         Derived,
	"activity_tag_removals":     Derived,
	"activity_time_in_zone":     Derived, // FIT-zone data, cascades from activities
	"activity_tracks":           Derived,
	"analytics_cache":           Derived,
	"enrichment_errors":         Derived,
	"fusion_candidates":         Derived,
	"heatmap_lines":             Derived,
	"heatmap_points":            Derived,
	"heatmap_trail_matches":     Derived,
	"pmc_daily":                 Derived,
	"segment_efforts":           Derived,
	"segment_versions":          Derived,
	"terra_incognita_cache":     Derived,
	"user_daily_features":       Derived,

	// -------------------------------------------------------------------------
	// System — security/transient; not exported
	// -------------------------------------------------------------------------
	"auth_audit_log":          System, // user_id SET NULL by design
	"auth_codes":              System,
	"background_jobs":         System,
	"device_tokens":           System,
	"email_tokens":            System,
	"garmin_import_jobs":      System,
	"garmin_import_mappings":  System,
	// hotfolder_imports removed — table never existed in DB (stale aspirational entry, Mig cleanup)
	"import_queue":            System, // transiente Import-Queue
	"komoot_credentials":      System,
	"notifications":           System,
	"refresh_tokens":          System,
	"strava_import_jobs":      System,

	// -------------------------------------------------------------------------
	// Global — no user_id; no GDPR relevance
	// -------------------------------------------------------------------------
	"developer_field_definitions": Global, // FIT-Schema-Lookup, kein user_id
	// sport_taxonomy removed — table never existed in DB (stale aspirational entry, Mig cleanup)
	"system_flags": Global,
}

// ExportedTables lists all tables exported via exportTable() in handler_data_export.go.
// Used by compile-time tests to verify that every Export/External table is actually
// included in the data export.
// Keep in sync with the tables slice in exportUserDataInTx().
var ExportedTables = []string{
	"users", // exported as account.json + profile.json with field scrubbing
	"activities",
	"gear",
	"tags",
	"segments",
	"planned_routes",
	"activity_weather",
	"workouts",
	"workout_steps",
	"devices",
	"chat_sessions",
	"chat_messages",
	"route_photos",
	"gear_documents",
	"activity_tags",
	"activity_photos",
	"user_privacy_zones",
	"custom_maps",
	"custom_map_markers",
	"automation_rules",
	"import_log",
	"user_enrichment_settings",
	"ml_user_settings",
	"user_file_schemas",
	"gear_components",
	"component_assignments",
	"gear_service_events",
	"bike_setups",
	"segment_favorites",
	"oauth_connections",
	"user_files",
	"file_attachments",
	"training_zones",
	"activity_edits",
	"tag_zones",
	"route_tags",
	"gear_service_intervals",
	"chain_rotation_groups",
	"chain_rotation_members",
	"llm_user_config",
	"recording_groups",
	"recording_group_members",
	"best_efforts",
	"best_effort_exclusions",
	"activity_developer_apps",
	"routing_profiles",
	// Health tables are exported dynamically via database.AllHealthTables():
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

// commute_clusters is intentionally absent — table was dropped in Mig 000288.
// hotfolder_imports and sport_taxonomy were removed — never existed in DB (stale entries after Mig cleanup).
