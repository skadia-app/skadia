package sidecar

import (
	"context"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

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
