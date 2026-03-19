package handler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// StartDeviceOrphanCleanupScheduler launches a background goroutine that runs
// a one-time cleanup of devices that are no longer referenced by any activity
// or activity sensor. The cleanup runs once, 1 hour after startup.
// Devices only become orphaned when activities are deleted, which is rare, so
// a periodic ticker is not needed.
// Call once from main after the handler is initialised.
func (h *Handler) StartDeviceOrphanCleanupScheduler(ctx context.Context) {
	go func() {
		// Wait 1 hour after startup before running so the server is fully
		// warmed up and the initial burst of requests has settled.
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Hour):
		}
		h.runDeviceOrphanCleanup(ctx)
	}()
}

func (h *Handler) runDeviceOrphanCleanup(ctx context.Context) {
	deleted, err := cleanupOrphanDevices(ctx, h.DB)
	if err != nil {
		slog.Error("device_orphan_cleanup: failed", "error", err)
		return
	}
	if deleted > 0 {
		slog.Info("orphan device cleanup", "deleted", deleted)
	}
}

// cleanupOrphanDevices removes hardware devices that are no longer linked to
// any activity (via recording_device_id) or any activity_sensor (via device_id).
// The devices table uses ON DELETE SET NULL on both FKs, so deletion is safe.
func cleanupOrphanDevices(ctx context.Context, db *database.DB) (int64, error) {
	result, err := db.Pool.Exec(ctx, `
		DELETE FROM devices d
		WHERE NOT EXISTS (
			SELECT 1 FROM activities a WHERE a.recording_device_id = d.id
		)
		AND NOT EXISTS (
			SELECT 1 FROM activity_sensors s WHERE s.device_id = d.id
		)`)
	if err != nil {
		return 0, fmt.Errorf("cleanup orphan devices: %w", err)
	}
	return result.RowsAffected(), nil
}
