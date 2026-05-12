package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	codeActivityPhotoLimitReached = "activity_photo_limit_reached"
	codeRoutePhotoLimitReached    = "route_photo_limit_reached"
	codeStorageQuotaExceeded      = "storage_quota_exceeded"
)

type routePhotoLimitError struct {
	RouteID string
	Limit   int
	Current int
}

func (e *routePhotoLimitError) Error() string {
	return fmt.Sprintf("route photo limit exceeded for route %s: %d/%d", e.RouteID, e.Current, e.Limit)
}

func mediaLimitValue(store *settings.SettingsStore, key string) int {
	def := 0
	if kd, ok := settings.KeysByKey[key]; ok {
		if parsed, err := strconv.Atoi(kd.Default); err == nil && parsed >= 0 {
			def = parsed
		}
	}
	if store == nil {
		return def
	}
	got, err := store.GetInt(key)
	if err != nil || got < 0 {
		return def
	}
	return got
}

func NewActivityPhotoCreateGuard(db *database.DB, store *settings.SettingsStore) importer.ActivityPhotoCreateGuard {
	return func(ctx context.Context, userID, activityID string) (func(), error) {
		limit := mediaLimitValue(store, "limits.max_activity_photos_per_activity")
		if limit == 0 {
			return nil, nil
		}
		release, err := acquireMediaParentLock(ctx, db.Pool, "activity_photo_limit:"+userID+":"+activityID)
		if err != nil {
			return nil, err
		}
		releaseOnError := true
		defer func() {
			if releaseOnError {
				release()
			}
		}()

		current, err := countActivityMediaRows(ctx, db.Pool, userID, activityID)
		if err != nil {
			return nil, err
		}
		if current >= limit {
			return nil, &importer.ActivityPhotoLimitError{
				ActivityID: activityID,
				Limit:      limit,
				Current:    current,
			}
		}
		releaseOnError = false
		return release, nil
	}
}

func (h *Handler) enforceRoutePhotoLimit(ctx context.Context, userID, routeID string) (func(), error) {
	limit := mediaLimitValue(h.Settings, "limits.max_route_photos_per_route")
	if limit == 0 {
		return nil, nil
	}
	release, err := acquireMediaParentLock(ctx, h.DB.Pool, "route_photo_limit:"+userID+":"+routeID)
	if err != nil {
		return nil, err
	}
	releaseOnError := true
	defer func() {
		if releaseOnError {
			release()
		}
	}()

	current, err := countRoutePhotoRows(ctx, h.DB.Pool, userID, routeID)
	if err != nil {
		return nil, err
	}
	if current >= limit {
		return nil, &routePhotoLimitError{RouteID: routeID, Limit: limit, Current: current}
	}
	releaseOnError = false
	return release, nil
}

func acquireMediaParentLock(ctx context.Context, pool *pgxpool.Pool, key string) (func(), error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("media parent lock acquire: %w", err)
	}
	locked := false
	defer func() {
		if !locked {
			conn.Release()
		}
	}()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, key); err != nil {
		return nil, fmt.Errorf("media parent advisory lock: %w", err)
	}
	locked = true
	return func() {
		if _, err := conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, key); err != nil {
			slog.Warn("media parent advisory unlock failed", "key", key, "error", err)
		}
		conn.Release()
	}, nil
}

func countActivityMediaRows(ctx context.Context, pool *pgxpool.Pool, userID, activityID string) (int, error) {
	var current int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_photos WHERE user_id = $1 AND activity_id = $2`,
		userID, activityID,
	).Scan(&current); err != nil {
		return 0, fmt.Errorf("count activity media rows: %w", err)
	}
	return current, nil
}

func countRoutePhotoRows(ctx context.Context, pool *pgxpool.Pool, userID, routeID string) (int, error) {
	var current int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM route_photos WHERE user_id = $1 AND route_id = $2`,
		userID, routeID,
	).Scan(&current); err != nil {
		return 0, fmt.Errorf("count route photo rows: %w", err)
	}
	return current, nil
}

func writeMediaUploadProblem(w http.ResponseWriter, r *http.Request, err error) bool {
	if errors.Is(err, importer.ErrActivityPhotoLimitExceeded) {
		httputil.WriteProblem(w, http.StatusConflict, "Conflict", "activity photo limit reached", codeActivityPhotoLimitReached, r.URL.Path)
		return true
	}
	var routeLimit *routePhotoLimitError
	if errors.As(err, &routeLimit) {
		httputil.WriteProblem(w, http.StatusConflict, "Conflict", "route photo limit reached", codeRoutePhotoLimitReached, r.URL.Path)
		return true
	}
	if errors.Is(err, shadowfs.ErrQuotaExceeded) {
		httputil.WriteProblem(w, http.StatusInsufficientStorage, "Insufficient Storage", "storage quota exceeded", codeStorageQuotaExceeded, r.URL.Path)
		return true
	}
	return false
}
