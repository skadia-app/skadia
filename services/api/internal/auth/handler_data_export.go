package auth

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

var (
	// exportInflight tracks per-user singleflight to prevent duplicate exports.
	exportInflight sync.Map
	// exportSem limits server-wide concurrent exports.
	exportSem = make(chan struct{}, 3)
)

// DataExport handles GET /auth/data-export — GDPR data export as streaming ZIP.
// Concurrency: max 1 per user, max 3 server-wide.
func (h *Handler) DataExport(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	// Per-user singleflight
	if _, loaded := exportInflight.LoadOrStore(userID, true); loaded {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "export_already_running"})
		return
	}
	defer exportInflight.Delete(userID)

	// Server-wide concurrency limit
	select {
	case exportSem <- struct{}{}:
		defer func() { <-exportSem }()
	default:
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "server_busy"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	user, err := h.DB.GetUserByID(ctx, userID)
	if err != nil || !user.IsActive || user.DeletedAt != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "account_inactive"})
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="skadia-export-%s.zip"`,
			time.Now().Format("2006-01-02")))

	zw := zip.NewWriter(w)
	defer zw.Close()

	// Account info (scrub sensitive fields)
	type accountExport struct {
		ID            string     `json:"id"`
		Username      string     `json:"username"`
		DisplayName   *string    `json:"display_name,omitempty"`
		Email         *string    `json:"email,omitempty"`
		IsAdmin       bool       `json:"is_admin"`
		EmailVerified bool       `json:"email_verified"`
		CreatedAt     time.Time  `json:"created_at"`
		UpdatedAt     time.Time  `json:"updated_at"`
		LastLoginAt   *time.Time `json:"last_login_at,omitempty"`
	}
	acct := accountExport{
		ID:            user.ID,
		Username:      user.Username,
		DisplayName:   user.DisplayName,
		Email:         user.Email,
		IsAdmin:       user.IsAdmin,
		EmailVerified: user.EmailVerified,
		CreatedAt:     user.CreatedAt,
		UpdatedAt:     user.UpdatedAt,
		LastLoginAt:   user.LastLoginAt,
	}
	writeZipJSON(zw, "account.json", acct)

	// User profile (physiological data)
	profileRows, err := h.DB.Pool.Query(ctx,
		`SELECT birth_date, gender, height_cm, weight_kg, ftp_w, lthr_bpm,
		        max_hr_bpm, max_hr_detected_bpm, resting_hr_bpm
		 FROM users WHERE id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "profile.json", profileRows)
	}

	// Activities metadata (without streams — too large)
	actRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, title, sport_type, sub_type, started_at, elapsed_time_s, moving_time_s,
		        distance_m, elevation_gain_m, elevation_loss_m, avg_heart_rate, max_heart_rate,
		        avg_power_w, total_calories, description, is_virtual, is_indoor, source,
		        gear_id, start_city, start_country, created_at
		 FROM activities WHERE user_id = $1 ORDER BY started_at DESC`, userID)
	if err == nil {
		exportRowsToZip(zw, "activities.json", actRows)
	} else {
		slog.Error("data export: activities query failed", "error", err)
	}

	// Gear
	gearRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, name, type, sub_type, brand, model, total_distance_m, total_time_s,
		        total_activities, is_retired, notes, created_at
		 FROM gear WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "gear.json", gearRows)
	} else {
		slog.Error("data export: gear query failed", "error", err)
	}

	// Tags
	tagRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, name, color, description, is_auto FROM tags WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "tags.json", tagRows)
	} else {
		slog.Error("data export: tags query failed", "error", err)
	}

	// Segments
	segRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, name, sport_group, segment_type, distance_m, elevation_gain_m,
		        effort_count, created_at
		 FROM segments WHERE created_by = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "segments.json", segRows)
	} else {
		slog.Error("data export: segments query failed", "error", err)
	}

	// Planned routes
	prRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, name, sport_type, total_distance_m, total_elevation_gain,
		        estimated_time_min, created_at
		 FROM planned_routes WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "planned_routes.json", prRows)
	} else {
		slog.Error("data export: planned_routes query failed", "error", err)
	}

	// Activity weather
	awRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, activity_id, query_point, model_used, temperature_c, feels_like_c,
		        humidity_pct, wind_speed_ms, wind_gust_ms, wind_dir_deg, pressure_hpa,
		        cloud_cover_pct, precipitation_mm, snow_mm, visibility_m,
		        weather_code, weather_description, is_daylight, uv_index, created_at
		 FROM activity_weather aw
		 WHERE aw.activity_id IN (SELECT id FROM activities WHERE user_id = $1)`, userID)
	if err == nil {
		exportRowsToZip(zw, "activity_weather.json", awRows)
	} else {
		slog.Error("data export: activity_weather query failed", "error", err)
	}

	// Workouts
	woRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, user_id, name, sport_type, description, created_at
		 FROM workouts WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "workouts.json", woRows)
	} else {
		slog.Error("data export: workouts query failed", "error", err)
	}

	// Workout steps
	wsRows, err := h.DB.Pool.Query(ctx,
		`SELECT ws.id, ws.workout_id, ws.step_order, ws.step_type, ws.duration_type,
		        ws.duration_value, ws.target_type, ws.target_value_low, ws.target_value_high,
		        ws.notes
		 FROM workout_steps ws
		 JOIN workouts w ON w.id = ws.workout_id
		 WHERE w.user_id = $1
		 ORDER BY ws.workout_id, ws.step_order`, userID)
	if err == nil {
		exportRowsToZip(zw, "workout_steps.json", wsRows)
	} else {
		slog.Error("data export: workout_steps query failed", "error", err)
	}

	// Devices
	devRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, user_id, manufacturer, product_name, serial_number, software_version,
		        device_type, battery_status, first_seen_at, last_seen_at, created_at
		 FROM devices WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "devices.json", devRows)
	} else {
		slog.Error("data export: devices query failed", "error", err)
	}

	// Segment efforts
	seRows, err := h.DB.Pool.Query(ctx,
		`SELECT se.id, se.segment_id, se.activity_id, se.started_at, se.elapsed_time_s,
		        se.moving_time_s, se.distance_m, se.avg_heart_rate, se.max_heart_rate,
		        se.avg_power_w, se.avg_cadence, se.avg_speed_ms
		 FROM segment_efforts se
		 WHERE se.activity_id IN (SELECT id FROM activities WHERE user_id = $1)`, userID)
	if err == nil {
		exportRowsToZip(zw, "segment_efforts.json", seRows)
	} else {
		slog.Error("data export: segment_efforts query failed", "error", err)
	}

	// Chat sessions
	ctRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, user_id, title, created_at, updated_at
		 FROM chat_sessions WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "chat_sessions.json", ctRows)
	} else {
		slog.Error("data export: chat_sessions query failed", "error", err)
	}

	// Chat messages
	cmRows, err := h.DB.Pool.Query(ctx,
		`SELECT cm.id, cm.session_id, cm.role, cm.content, cm.tool_calls,
		        cm.created_at
		 FROM chat_messages cm
		 JOIN chat_sessions cs ON cs.id = cm.session_id
		 WHERE cs.user_id = $1
		 ORDER BY cm.created_at`, userID)
	if err == nil {
		exportRowsToZip(zw, "chat_messages.json", cmRows)
	} else {
		slog.Error("data export: chat_messages query failed", "error", err)
	}

	// Route photos
	rpRows, err := h.DB.Pool.Query(ctx,
		`SELECT rp.id, rp.route_id, rp.original_name, rp.width, rp.height,
		        rp.sort_order, rp.created_at
		 FROM route_photos rp
		 WHERE rp.user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "route_photos.json", rpRows)
	} else {
		slog.Error("data export: route_photos query failed", "error", err)
	}

	// Gear documents
	gdRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, gear_id, user_id, document_type, original_name, notes, created_at
		 FROM gear_documents WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "gear_documents.json", gdRows)
	} else {
		slog.Error("data export: gear_documents query failed", "error", err)
	}

	// Activity laps
	lapRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, activity_id, message_index, start_time, end_time, lap_trigger,
		        sport, sub_sport, total_elapsed_time_s, total_timer_time_s,
		        total_moving_time_s, total_distance_m, total_ascent_m, total_descent_m,
		        total_calories, avg_speed_ms, max_speed_ms, avg_heart_rate, max_heart_rate,
		        min_heart_rate, avg_cadence, max_cadence, avg_power_w, max_power_w,
		        normalized_power_w, avg_temperature_c, start_lat, start_lng, end_lat, end_lng
		 FROM activity_laps
		 WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)
		 ORDER BY activity_id, message_index`, userID)
	if err == nil {
		exportRowsToZip(zw, "activity_laps.json", lapRows)
	} else {
		slog.Error("data export: activity_laps query failed", "error", err)
	}

	// Activity splits
	splitRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, activity_id, message_index, split_type, start_time, end_time,
		        total_elapsed_time_s, total_timer_time_s, total_moving_time_s,
		        total_distance_m, total_ascent_m, total_descent_m, total_calories,
		        avg_speed_ms, max_speed_ms, avg_vert_speed_ms,
		        start_lat, start_lng, end_lat, end_lng, start_elevation_m
		 FROM activity_splits
		 WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)
		 ORDER BY activity_id, message_index`, userID)
	if err == nil {
		exportRowsToZip(zw, "activity_splits.json", splitRows)
	} else {
		slog.Error("data export: activity_splits query failed", "error", err)
	}

	// Activity split summaries
	splitSumRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, activity_id, message_index, split_type, num_splits,
		        total_timer_time_s, total_moving_time_s, total_distance_m,
		        total_ascent_m, total_descent_m, total_calories,
		        avg_speed_ms, max_speed_ms, avg_vert_speed_ms, avg_heart_rate, max_heart_rate
		 FROM activity_split_summaries
		 WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)
		 ORDER BY activity_id, message_index`, userID)
	if err == nil {
		exportRowsToZip(zw, "activity_split_summaries.json", splitSumRows)
	} else {
		slog.Error("data export: activity_split_summaries query failed", "error", err)
	}

	// Activity lengths (swim pool)
	lenRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, activity_id, message_index, start_time, end_time,
		        total_elapsed_time_s, total_timer_time_s, total_strokes,
		        total_calories, avg_speed_ms, avg_swimming_cadence,
		        swim_stroke, length_type
		 FROM activity_lengths
		 WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)
		 ORDER BY activity_id, message_index`, userID)
	if err == nil {
		exportRowsToZip(zw, "activity_lengths.json", lenRows)
	} else {
		slog.Error("data export: activity_lengths query failed", "error", err)
	}

	// Activity ClimbPro events
	climbRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, activity_id, timestamp, event_type, climb_number,
		        climb_category, current_dist_m, lat, lng
		 FROM activity_climb_pro_events
		 WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)
		 ORDER BY activity_id, timestamp`, userID)
	if err == nil {
		exportRowsToZip(zw, "activity_climb_pro_events.json", climbRows)
	} else {
		slog.Error("data export: activity_climb_pro_events query failed", "error", err)
	}

	// Activity events
	evtRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, activity_id, timestamp, event, event_type, event_group, data,
		        front_gear_num, front_gear, rear_gear_num, rear_gear,
		        radar_threat_level_max, radar_threat_count,
		        radar_threat_avg_approach_speed_ms, radar_threat_max_approach_speed_ms,
		        score, opponent_score, device_index, activity_type, start_timestamp
		 FROM activity_events
		 WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)
		 ORDER BY activity_id, timestamp`, userID)
	if err == nil {
		exportRowsToZip(zw, "activity_events.json", evtRows)
	} else {
		slog.Error("data export: activity_events query failed", "error", err)
	}

	// Activity sets (strength training)
	setRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, activity_id, message_index, timestamp, start_time,
		        duration_s, set_type, repetitions, weight_kg,
		        weight_display_unit, exercise_categories, exercise_subtypes, wkt_step_index
		 FROM activity_sets
		 WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)
		 ORDER BY activity_id, message_index`, userID)
	if err == nil {
		exportRowsToZip(zw, "activity_sets.json", setRows)
	} else {
		slog.Error("data export: activity_sets query failed", "error", err)
	}

	// Activity detected laps
	dlRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, activity_id, lap_number, start_time, end_time, elapsed_time_s,
		        distance_m, avg_speed_ms, avg_heart_rate, avg_power_w, avg_cadence,
		        total_ascent_m, total_descent_m, start_lat, start_lng, created_at
		 FROM activity_detected_laps
		 WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)
		 ORDER BY activity_id, lap_number`, userID)
	if err == nil {
		exportRowsToZip(zw, "activity_detected_laps.json", dlRows)
	} else {
		slog.Error("data export: activity_detected_laps query failed", "error", err)
	}

	// Activity tags (junction table)
	atRows, err := h.DB.Pool.Query(ctx,
		`SELECT at.activity_id, at.tag_id
		 FROM activity_tags at
		 JOIN activities a ON a.id = at.activity_id
		 WHERE a.user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "activity_tags.json", atRows)
	} else {
		slog.Error("data export: activity_tags query failed", "error", err)
	}

	// Activity photos (metadata only — binary files exported via ShadowFS)
	apRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, activity_id, original_filename, content_type, width, height,
		        taken_at, lat, lng, location_source, created_at
		 FROM activity_photos WHERE user_id = $1
		 ORDER BY activity_id, created_at`, userID)
	if err == nil {
		exportRowsToZip(zw, "activity_photos.json", apRows)
	} else {
		slog.Error("data export: activity_photos query failed", "error", err)
	}

	// User privacy zones
	pzRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, name, ST_Y(center) AS lat, ST_X(center) AS lng,
		        radius_m, is_active, created_at
		 FROM user_privacy_zones WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "user_privacy_zones.json", pzRows)
	} else {
		slog.Error("data export: user_privacy_zones query failed", "error", err)
	}

	// Commute clusters
	ccRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, cluster_label, ST_Y(center) AS lat, ST_X(center) AS lng,
		        radius_m, point_count, created_at
		 FROM commute_clusters WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "commute_clusters.json", ccRows)
	} else {
		slog.Error("data export: commute_clusters query failed", "error", err)
	}

	// Heatmap lines (metadata — geometry exported as GeoJSON text)
	hlRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, activity_id, sport_type, activity_date,
		        ST_AsGeoJSON(geom) AS geom_geojson
		 FROM heatmap_lines WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "heatmap_lines.json", hlRows)
	} else {
		slog.Error("data export: heatmap_lines query failed", "error", err)
	}

	// PMC daily (Performance Management Chart)
	pmcRows, err := h.DB.Pool.Query(ctx,
		`SELECT date, tss, ctl, atl, tsb
		 FROM pmc_daily WHERE user_id = $1 ORDER BY date`, userID)
	if err == nil {
		exportRowsToZip(zw, "pmc_daily.json", pmcRows)
	} else {
		slog.Error("data export: pmc_daily query failed", "error", err)
	}

	// Custom maps
	cmapRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, name, description, color, is_visible, sort_order, created_at, updated_at
		 FROM custom_maps WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "custom_maps.json", cmapRows)
	} else {
		slog.Error("data export: custom_maps query failed", "error", err)
	}

	// Custom map markers
	cmmRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, map_id, name, description, lat, lng, color, created_at, updated_at
		 FROM custom_map_markers WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "custom_map_markers.json", cmmRows)
	} else {
		slog.Error("data export: custom_map_markers query failed", "error", err)
	}

	// Automation rules
	arRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, name, is_active, priority, conditions, actions,
		        times_applied, created_at
		 FROM automation_rules WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "automation_rules.json", arRows)
	} else {
		slog.Error("data export: automation_rules query failed", "error", err)
	}

	// Notifications
	notifRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, category, title, body, data, read, created_at
		 FROM notifications WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err == nil {
		exportRowsToZip(zw, "notifications.json", notifRows)
	} else {
		slog.Error("data export: notifications query failed", "error", err)
	}

	// Import log
	ilRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, source_platform, filename, file_hash, file_size_bytes,
		        status, activity_id, error_msg, duration_ms, metadata,
		        created_at, updated_at
		 FROM import_log WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err == nil {
		exportRowsToZip(zw, "import_log.json", ilRows)
	} else {
		slog.Error("data export: import_log query failed", "error", err)
	}

	// User enrichment settings
	uesRows, err := h.DB.Pool.Query(ctx,
		`SELECT * FROM user_enrichment_settings WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "user_enrichment_settings.json", uesRows)
	} else {
		slog.Error("data export: user_enrichment_settings query failed", "error", err)
	}

	// ML user settings
	mlRows, err := h.DB.Pool.Query(ctx,
		`SELECT setting_key, value_json, updated_at
		 FROM ml_user_settings WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "ml_user_settings.json", mlRows)
	} else {
		slog.Error("data export: ml_user_settings query failed", "error", err)
	}

	// User file schemas
	ufsRows, err := h.DB.Pool.Query(ctx,
		`SELECT id, file_type, schema_template, created_at, updated_at
		 FROM user_file_schemas WHERE user_id = $1`, userID)
	if err == nil {
		exportRowsToZip(zw, "user_file_schemas.json", ufsRows)
	} else {
		slog.Error("data export: user_file_schemas query failed", "error", err)
	}

	// Health data (TimescaleDB hypertables — SELECT * is intentional for full export)
	var exportedTables []string
	for _, table := range database.AllHealthTables() {
		hRows, err := h.DB.Pool.Query(ctx,
			fmt.Sprintf(`SELECT * FROM %s WHERE user_id = $1`, table), userID)
		if err == nil {
			exportRowsToZip(zw, fmt.Sprintf("health/%s.json", table), hRows)
			exportedTables = append(exportedTables, table)
		}
	}

	// Files: original activity files, media, documents — via ShadowFS.
	// Falls back to legacy ShadowFS walk when ShadowFS is not yet wired.
	if h.ShadowFS != nil {
		// New path: ShadowFS includes ALL files (local + S3).
		if err := h.ShadowFS.StreamToZip(ctx, userID, zw); err != nil {
			slog.Error("data export: file store stream failed", "error", err)
		}
	} else if basePath := h.Config.ShadowFSBasePath; basePath != "" {
		// Legacy path: ShadowFS only (pre-migration).
		streamShadowFSToZip(basePath, userID, zw)
	}

	// Export manifest with metadata
	manifest := map[string]any{
		"exported_at": time.Now().Format(time.RFC3339),
		"tables": append([]string{
			"account", "profile", "activities", "gear", "tags",
			"segments", "planned_routes", "activity_weather",
			"workouts", "workout_steps", "devices", "segment_efforts",
			"chat_sessions", "chat_messages", "route_photos", "gear_documents",
			"activity_laps", "activity_splits", "activity_split_summaries",
			"activity_lengths", "activity_climb_pro_events",
			"activity_events", "activity_sets", "activity_detected_laps",
			"activity_tags", "activity_photos",
			"user_privacy_zones", "commute_clusters", "heatmap_lines",
			"pmc_daily", "custom_maps", "custom_map_markers",
			"automation_rules", "notifications", "import_log",
			"user_enrichment_settings", "ml_user_settings", "user_file_schemas",
		}, exportedTables...),
	}
	writeZipJSON(zw, "manifest.json", manifest)

	go h.DB.LogAuthEvent(h.shutdownCtx, userID, "data_export_requested",
		middleware.ClientIP(r), r.UserAgent(), nil)
}

// createZipEntry creates a zip entry with the current timestamp (avoids 1979 default).
func createZipEntry(zw *zip.Writer, name string) (io.Writer, error) {
	return zw.CreateHeader(&zip.FileHeader{
		Name:     name,
		Method:   zip.Deflate,
		Modified: time.Now(),
	})
}

// writeZipJSON writes a single JSON value to the zip archive under the given name.
func writeZipJSON(zw *zip.Writer, name string, data any) {
	w, err := createZipEntry(zw, name)
	if err != nil {
		slog.Error("data export: create zip entry failed", "name", name, "error", err)
		return
	}
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("data export: encode json failed", "name", name, "error", err)
	}
}

// exportRowsToZip streams pgx Rows as a JSON array into the zip archive without buffering all rows.
func exportRowsToZip(zw *zip.Writer, name string, rows pgx.Rows) {
	defer rows.Close()

	cols := rows.FieldDescriptions()
	w, err := createZipEntry(zw, name)
	if err != nil {
		slog.Error("data export: create zip entry failed", "name", name, "error", err)
		return
	}

	w.Write([]byte("["))
	first := true
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			slog.Error("data export: row values failed", "name", name, "error", err)
			continue
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[string(col.Name)] = vals[i]
		}
		if !first {
			w.Write([]byte(","))
		}
		first = false
		json.NewEncoder(w).Encode(row)
	}
	w.Write([]byte("]"))

	if err := rows.Err(); err != nil {
		slog.Error("data export: rows iteration error", "name", name, "error", err)
	}
}

// streamShadowFSToZip walks the user's ShadowFS directory and copies each file into the zip.
func streamShadowFSToZip(basePath, userID string, zw *zip.Writer) {
	const maxLegacyZipBytes int64 = 10 << 30 // 10 GB — matches S3ShadowFS.StreamToZip limit

	userDir := filepath.Join(basePath, userID)
	if _, err := os.Stat(userDir); os.IsNotExist(err) {
		return
	}

	var totalBytes int64
	err := filepath.Walk(userDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Enforce 10 GB limit (same as the ShadowFS/S3 path above).
		totalBytes += info.Size()
		if totalBytes > maxLegacyZipBytes {
			slog.Warn("data export: legacy ShadowFS zip exceeds 10 GB limit, stopping",
				"user_id", userID, "total_bytes", totalBytes)
			return fmt.Errorf("zip export exceeds 10 GB limit")
		}

		rel, relErr := filepath.Rel(userDir, path)
		if relErr != nil {
			return nil
		}
		w, createErr := createZipEntry(zw, "originals/"+rel)
		if createErr != nil {
			slog.Error("data export: create zip entry for shadowfs failed", "path", rel, "error", createErr)
			return nil
		}
		f, openErr := os.Open(path)
		if openErr != nil {
			slog.Error("data export: open shadowfs file failed", "path", path, "error", openErr)
			return nil
		}
		defer f.Close()
		if _, cpErr := io.Copy(w, f); cpErr != nil {
			slog.Error("data export: copy shadowfs file failed", "path", rel, "error", cpErr)
		}
		return nil
	})
	if err != nil {
		slog.Error("data export: walk shadowfs failed", "user_id", userID, "error", err)
	}
}
