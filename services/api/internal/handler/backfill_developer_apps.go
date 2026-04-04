package handler

import (
	"context"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// backfillDeveloperAppsRow is the result of a single backfill query row.
type backfillDeveloperAppsRow struct {
	ActivityID string
	AppIDHex   string
	UserID     string
}

// BackfillDeveloperApps backfills activity_developer_apps for all activities
// that have developer field definitions with known application IDs but no
// corresponding entry in activity_developer_apps yet.
//
// POST /api/v1/admin/backfill-developer-apps
func (h *Handler) BackfillDeveloperApps(w http.ResponseWriter, r *http.Request) {
	_, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	ctx := r.Context()

	// Load all activity IDs that have developer_fields in streams
	// and have matching developer_field_definitions with application_id,
	// but lack an activity_developer_apps entry.
	type activityUser struct {
		ActivityID string
		UserID     string
		StartedAt  time.Time
	}

	// Discovery query: find activities that have developer fields with known
	// application_ids but no activity_developer_apps entry yet.
	//
	// Fix #3: The inner subquery uses DISTINCT ON (activity_id) so
	// jsonb_object_keys is evaluated on only ONE stream row per activity,
	// instead of the full 10k+ rows that TimescaleDB-compressed chunks can have.
	rows, err := h.DB.Pool.Query(ctx, `
		SELECT DISTINCT a.id::text, a.user_id::text, a.started_at
		FROM activities a
		WHERE EXISTS (
			SELECT 1
			FROM (
			    SELECT DISTINCT ON (activity_id) activity_id, developer_fields
			    FROM activity_streams
			    WHERE activity_id = a.id
			      AND developer_fields IS NOT NULL
			    ORDER BY activity_id
			) s
			JOIN developer_field_definitions dfd
			    ON dfd.field_name IN (SELECT jsonb_object_keys(s.developer_fields))
			WHERE dfd.application_id IS NOT NULL
		)
		AND NOT EXISTS (
			SELECT 1 FROM activity_developer_apps ada WHERE ada.activity_id = a.id
		)
		ORDER BY a.started_at DESC
	`)
	if err != nil {
		slog.Error("backfill developer apps: failed to list activities", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	var activities []activityUser
	for rows.Next() {
		var au activityUser
		if err := rows.Scan(&au.ActivityID, &au.UserID, &au.StartedAt); err != nil {
			rows.Close()
			slog.Error("backfill developer apps: scan activity row failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to scan activities", r.URL.Path)
			return
		}
		activities = append(activities, au)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		slog.Error("backfill developer apps: rows error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to iterate activities", r.URL.Path)
		return
	}

	if len(activities) == 0 {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"processed": 0, "inserted": 0})
		return
	}

	// Return immediately; run backfill in background.
	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"queued": len(activities),
	})

	go func() {
		bgCtx := context.Background()
		processed := 0
		inserted := 0

		const batchSize = 50
		for batchStart := range (len(activities) + batchSize - 1) / batchSize {
			end := min((batchStart+1)*batchSize, len(activities))
			batch := activities[batchStart*batchSize : end]

			// Collect activity IDs for this batch.
			activityIDs := make([]string, len(batch))
			for i, au := range batch {
				activityIDs[i] = au.ActivityID
			}

			// Build user/startedAt lookup for this batch.
			userByActivity := make(map[string]activityUser, len(batch))
			for _, au := range batch {
				userByActivity[au.ActivityID] = au
			}

			// Query developer app matches for this batch.
			//
			// Fix #3: Use DISTINCT ON (activity_id) to read only ONE row per activity
			// instead of evaluating jsonb_object_keys on every stream row (up to 10k+
			// rows per activity). This is critical for TimescaleDB-compressed chunks.
			//
			// Fix #15: ORDER BY activity_id for deterministic results.
			//
			// Note (#2 limitation): developer_field_definitions has no activity_id
			// column, so we still join by field_name. Field-name collisions across
			// apps (e.g. two apps both named "Power") are a V1 limitation — they will
			// produce extra rows that may match additional apps beyond the actual one.
			// Mitigated in practice because LookupDeveloperApp filters to known apps.
			batchRows, err := h.DB.Pool.Query(bgCtx, `
				SELECT DISTINCT
				    s.activity_id::text,
				    encode(dfd.application_id, 'hex') AS app_id_hex,
				    a.user_id::text
				FROM (
				    SELECT DISTINCT ON (activity_id) activity_id, developer_fields
				    FROM activity_streams
				    WHERE activity_id = ANY($1::uuid[])
				      AND developer_fields IS NOT NULL
				    ORDER BY activity_id
				) s
				JOIN developer_field_definitions dfd
				    ON dfd.field_name IN (SELECT jsonb_object_keys(s.developer_fields))
				JOIN activities a ON a.id = s.activity_id
				WHERE dfd.application_id IS NOT NULL
				ORDER BY s.activity_id
			`, activityIDs)
			if err != nil {
				slog.Error("backfill developer apps: batch query failed",
					"batch_start", batchStart, "error", err)
				processed += len(batch)
				continue
			}

			var batchResults []backfillDeveloperAppsRow
			for batchRows.Next() {
				var row backfillDeveloperAppsRow
				if err := batchRows.Scan(&row.ActivityID, &row.AppIDHex, &row.UserID); err != nil {
					slog.Error("backfill developer apps: scan batch row failed", "error", err)
					continue
				}
				batchResults = append(batchResults, row)
			}
			batchRows.Close()
			if err := batchRows.Err(); err != nil {
				slog.Error("backfill developer apps: batch rows error",
					"batch_start", batchStart, "error", err)
				processed += len(batch)
				continue
			}

			// Process each (activity, app) pair.
			for _, row := range batchResults {
				appName, appCategory, known := fitimport.LookupDeveloperApp(row.AppIDHex)
				if !known {
					// Unknown app — skip device resolution but still record what we found.
					slog.Debug("backfill developer apps: unknown app, skipping",
						"activity_id", row.ActivityID, "app_id_hex", row.AppIDHex)
					continue
				}

				// Decode application_id bytes from hex.
				appIDBytes, decErr := hex.DecodeString(row.AppIDHex)
				if decErr != nil {
					slog.Error("backfill developer apps: hex decode failed",
						"app_id_hex", row.AppIDHex, "error", decErr)
					continue
				}

				// Determine seenAt from the activity.
				var seenAt time.Time
				if au, ok := userByActivity[row.ActivityID]; ok {
					seenAt = au.StartedAt
				} else {
					seenAt = time.Now()
				}

				// Insert into activity_developer_apps.
				apps := []model.DeveloperAppRecord{
					{
						ApplicationID: appIDBytes,
						AppName:       appName,
						AppCategory:   appCategory,
					},
				}
				if err := h.DB.InsertDeveloperApps(bgCtx, row.ActivityID, apps); err != nil {
					slog.Error("backfill developer apps: insert failed",
						"activity_id", row.ActivityID, "app_id_hex", row.AppIDHex, "error", err)
					continue
				}
				inserted++

				// Upsert hardware device and link.
				deviceID, err := h.DB.UpsertHWDevice(bgCtx, model.DeviceUpsert{
					UserID:      row.UserID,
					Type:        "sensor",
					ProductName: appName,
					SeenAt:      seenAt,
				})
				if err != nil {
					slog.Error("backfill developer apps: upsert hw device failed",
						"activity_id", row.ActivityID, "app_name", appName, "error", err)
					continue
				}
				if deviceID != "" {
					if err := h.DB.SetDeveloperAppDeviceID(bgCtx, row.ActivityID, appIDBytes, deviceID); err != nil {
						slog.Error("backfill developer apps: set device id failed",
							"activity_id", row.ActivityID, "error", err)
					}
				}
			}

			processed += len(batch)
		}

		slog.Info("backfill developer apps: completed",
			"processed_activities", processed,
			"inserted", inserted,
		)
	}()
}
