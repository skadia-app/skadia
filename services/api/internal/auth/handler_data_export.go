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
			"segments", "planned_routes",
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
	userDir := filepath.Join(basePath, userID)
	if _, err := os.Stat(userDir); os.IsNotExist(err) {
		return
	}

	err := filepath.Walk(userDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
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
