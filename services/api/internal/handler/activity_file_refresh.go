package handler

import (
	"context"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

func activityUpdateAffectsFileSchema(update *model.ActivityUpdate) bool {
	if update == nil {
		return false
	}
	if activityUpdateIsEmpty(update) {
		return true
	}
	return update.Title != nil ||
		(update.ClearTitle != nil && *update.ClearTitle) ||
		update.SportType != nil ||
		update.SubType != nil ||
		update.IsElectric != nil ||
		update.IsIndoor != nil ||
		update.IsVirtual != nil ||
		update.Tags != nil
}

func activityUpdateIsEmpty(update *model.ActivityUpdate) bool {
	return update.Title == nil &&
		update.ClearTitle == nil &&
		update.Description == nil &&
		update.ClearDescription == nil &&
		update.SportType == nil &&
		update.SubType == nil &&
		update.IsElectric == nil &&
		update.IsIndoor == nil &&
		update.IsVirtual == nil &&
		update.Tags == nil &&
		update.Visibility == nil &&
		update.HideMap == nil &&
		update.DataVisibility == nil &&
		update.GearID == nil &&
		update.LocationLat == nil &&
		update.LocationLng == nil &&
		update.ExcludeLiftSubtraction == nil &&
		update.WorkoutFeel == nil &&
		update.ClearWorkoutFeel == nil &&
		update.WorkoutRpe == nil &&
		update.ClearWorkoutRpe == nil &&
		update.ExcludeFromHeatmap == nil &&
		update.ExcludeFromStats == nil
}

func (h *Handler) refreshActivityFilePathBestEffort(ctx context.Context, userID, activityID, fileID string) {
	if h == nil || h.DB == nil || h.DB.Pool == nil || h.Config == nil {
		return
	}
	if h.Config.LibraryBasePath == "" {
		return
	}
	refreshed, err := shadowfs.RefreshActivityFilePath(ctx, shadowfs.ActivityFileRefreshRequest{
		Pool:       h.DB.Pool,
		Settings:   h.Settings,
		BasePath:   h.Config.LibraryBasePath,
		UserID:     userID,
		ActivityID: activityID,
		FileID:     fileID,
	})
	if err != nil {
		slog.Warn("activity file path refresh failed", "activity_id", activityID, "file_id", fileID, "error", err)
		return
	}
	if refreshed {
		slog.Info("activity file path refreshed", "activity_id", activityID, "file_id", fileID)
	}
}
