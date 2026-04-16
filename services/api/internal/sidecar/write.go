package sidecar

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

// MetaPath returns the logical ShadowFS key for an activity's .meta sidecar.
// Regular activities (have a source file): {user_id}/{file_id}.meta
// Fusion / manual-entry (no source file):  {user_id}/activity-{activity_id}.meta
//
// The "activity-" prefix avoids the (extremely unlikely) collision where a
// generated file_id happens to equal an activity_id UUID string.
//
// Note: this is a logical addressing key, not a raw storage path. ShadowFS
// methods that accept a file_id use their own DB-resolved storage paths.
// Task 35 uses this key for GetCompanion calls on fusion activities.
func MetaPath(userID, activityID string, fileID *string) string {
	if fileID != nil && *fileID != "" {
		return fmt.Sprintf("%s/%s.meta", userID, *fileID)
	}
	return fmt.Sprintf("%s/activity-%s.meta", userID, activityID)
}

// WriteMetaCompanion generates a v3 MetaFile and writes it as .meta companion
// alongside the activity file in ShadowFS. Safe to call with nil arguments —
// returns silently without error. Best-effort: must never block imports.
func WriteMetaCompanion(
	ctx context.Context,
	fs shadowfs.ShadowFS,
	db *database.DB,
	userID, activityID, fileID string,
	data *model.ActivityData,
	activity *model.Activity,
) {
	if fs == nil || fileID == "" {
		return
	}
	meta := GenerateMetaFull(activityID, data, activity)
	writeMetaToCompanion(ctx, fs, db, userID, activityID, fileID, meta)
}

// WriteMetaCompanionFor is the preferred entry point when the caller already
// has a fully-populated MetaFile (e.g. fusion activities built via
// GenerateMetaFusion). It resolves the write target based on fileID:
//
//   - fileID != nil && non-empty → writes via PutCompanion (file_id-based path)
//   - fileID == nil or empty    → fusion/manual-entry without a source file;
//     the sidecar write is skipped here and will be wired by Task 35 once
//     ShadowFS gains direct-path write support for file-less activities.
//
// Best-effort: never returns an error, never blocks the caller.
func WriteMetaCompanionFor(
	ctx context.Context,
	fs shadowfs.ShadowFS,
	db *database.DB,
	userID, activityID string,
	fileID *string,
	meta *MetaFile,
) {
	if fs == nil || meta == nil {
		return
	}
	if fileID == nil || *fileID == "" {
		// No source-file sidecar path available yet. Task 35 will add
		// direct-path write support for fusion activities.
		slog.Debug("sidecar: WriteMetaCompanionFor skipped — no file_id (fusion/manual-entry)",
			"activity_id", activityID,
			"logical_key", MetaPath(userID, activityID, nil))
		return
	}
	writeMetaToCompanion(ctx, fs, db, userID, activityID, *fileID, meta)
}

// writeMetaToCompanion is the shared write path for both WriteMetaCompanion and
// WriteMetaCompanionFor. It enriches the GearName snapshot and calls PutCompanion.
func writeMetaToCompanion(
	ctx context.Context,
	fs shadowfs.ShadowFS,
	db *database.DB,
	userID, activityID, fileID string,
	meta *MetaFile,
) {
	// GearName snapshot — informativ für manuelle Inspektion, Restore nutzt GearID.
	if meta.GearID != "" && db != nil {
		var gearName string
		if err := db.Pool.QueryRow(ctx,
			`SELECT name FROM gear WHERE id = $1`, meta.GearID,
		).Scan(&gearName); err == nil {
			meta.GearName = gearName
		}
	}

	metaBytes, err := meta.Marshal()
	if err != nil {
		slog.Warn("sidecar: marshal .meta failed", "activity_id", activityID, "error", err)
		return
	}
	if metaBytes == nil {
		return
	}
	if err := fs.PutCompanion(ctx, userID, fileID, ".meta", metaBytes); err != nil {
		slog.Warn("sidecar: write .meta companion failed",
			"activity_id", activityID, "file_id", fileID, "error", err)
	}
}
