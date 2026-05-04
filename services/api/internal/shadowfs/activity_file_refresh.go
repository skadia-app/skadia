package shadowfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ActivityFileRefreshRequest struct {
	Pool               *pgxpool.Pool
	Settings           *settings.SettingsStore
	Renderer           *SchemaRenderer
	BasePath           string
	UserID             string
	ActivityID         string
	FileID             string
	SchemaOverride     string
	LibraryDirOverride string
}

type ActivityFilePathRender struct {
	FileID         string
	OldPath        string
	NewPath        string
	StorageBackend string
}

type activityFileRefreshRow struct {
	activityID, fileID, storageBackend, storagePath, fileName, userDir string
	hasSidecar                                                         bool
	format, source, sportType, subType, title, city, country           string
	startedAt                                                          time.Time
	distanceM, elevationGainM                                          float64
	elapsedTimeS                                                       int
	isElectric, isIndoor, isVirtual, isCommute                         bool
}

func RefreshActivityFilePath(ctx context.Context, req ActivityFileRefreshRequest) (bool, error) {
	rendered, err := RenderActivityFilePath(ctx, req)
	if err != nil {
		return false, err
	}
	if rendered.FileID == "" || rendered.StorageBackend != "local" || rendered.NewPath == rendered.OldPath {
		return false, nil
	}
	if req.BasePath == "" {
		return false, fmt.Errorf("shadowfs: activity refresh requires base path for local backend")
	}
	if err := ValidateRelPath(req.BasePath, rendered.NewPath); err != nil {
		return false, fmt.Errorf("shadowfs: activity refresh target validation: %w", err)
	}
	if err := ValidateRelPath(req.BasePath, rendered.OldPath); err != nil {
		return false, fmt.Errorf("shadowfs: activity refresh source validation: %w", err)
	}

	conn, err := req.Pool.Acquire(ctx)
	if err != nil {
		return false, fmt.Errorf("shadowfs: activity refresh acquire connection: %w", err)
	}
	defer conn.Release()

	lockName := "shadowfs:activity-refresh:" + rendered.FileID
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtext($1)::bigint)`, lockName); err != nil {
		return false, fmt.Errorf("shadowfs: activity refresh advisory lock: %w", err)
	}
	defer func() {
		if _, err := conn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext($1)::bigint)`, lockName); err != nil {
			slog.Warn("shadowfs: activity refresh advisory unlock failed", "file_id", rendered.FileID, "error", err)
		}
	}()

	cmd, err := conn.Exec(ctx,
		`UPDATE user_files SET is_renaming = true, updated_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND storage_backend = 'local' AND storage_path = $3`,
		rendered.FileID, req.UserID, rendered.OldPath)
	if err != nil {
		return false, fmt.Errorf("shadowfs: activity refresh mark renaming: %w", err)
	}
	if cmd.RowsAffected() != 1 {
		return false, nil
	}
	clearRenaming := true
	defer func() {
		if clearRenaming {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := req.Pool.Exec(cleanupCtx,
				`UPDATE user_files SET is_renaming = false, updated_at = NOW()
				 WHERE id = $1 AND user_id = $2`,
				rendered.FileID, req.UserID); err != nil {
				slog.Warn("shadowfs: activity refresh clear is_renaming failed", "file_id", rendered.FileID, "error", err)
			}
		}
	}()

	oldFull := filepath.Join(req.BasePath, rendered.OldPath)
	newRel, err := copyActivityFileWithCollisionRetry(req.BasePath, oldFull, rendered.NewPath)
	if err != nil {
		return false, err
	}
	newFull := filepath.Join(req.BasePath, newRel)
	cmd, err = conn.Exec(ctx,
		`UPDATE user_files SET storage_path = $1, is_renaming = false, updated_at = NOW()
		 WHERE id = $2 AND user_id = $3 AND storage_backend = 'local' AND storage_path = $4`,
		newRel, rendered.FileID, req.UserID, rendered.OldPath)
	if err != nil {
		_ = os.Remove(newFull)
		return false, fmt.Errorf("shadowfs: activity refresh update storage_path: %w", err)
	}
	if cmd.RowsAffected() != 1 {
		_ = os.Remove(newFull)
		return false, fmt.Errorf("shadowfs: activity refresh update storage_path affected %d rows", cmd.RowsAffected())
	}
	clearRenaming = false
	if err := os.Remove(oldFull); err != nil && !os.IsNotExist(err) {
		slog.Warn("shadowfs: activity refresh remove old file failed", "file_id", rendered.FileID, "path", oldFull, "error", err)
	}
	if renderedHasSidecar(ctx, req.Pool, rendered.FileID, req.UserID) {
		moveMetaSidecar(oldFull, newFull)
	}
	cleanEmptyDirs(filepath.Dir(oldFull), req.BasePath)
	return true, nil
}

func RenderActivityFilePath(ctx context.Context, req ActivityFileRefreshRequest) (ActivityFilePathRender, error) {
	if req.Pool == nil {
		return ActivityFilePathRender{}, fmt.Errorf("shadowfs: activity refresh requires DB pool")
	}
	if req.UserID == "" {
		return ActivityFilePathRender{}, fmt.Errorf("shadowfs: activity refresh requires user_id")
	}
	if req.ActivityID == "" && req.FileID == "" {
		return ActivityFilePathRender{}, fmt.Errorf("shadowfs: activity refresh requires activity_id or file_id")
	}
	if req.Renderer == nil {
		req.Renderer = NewSchemaRenderer()
	}

	row, err := loadActivityFileRefreshRow(ctx, req.Pool, req.UserID, req.ActivityID, req.FileID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ActivityFilePathRender{}, nil
	}
	if err != nil {
		return ActivityFilePathRender{}, err
	}
	if row.storageBackend != "local" {
		return ActivityFilePathRender{
			FileID:         row.fileID,
			OldPath:        row.storagePath,
			NewPath:        row.storagePath,
			StorageBackend: row.storageBackend,
		}, nil
	}

	schema := req.Renderer.DefaultSchema("activity")
	if req.SchemaOverride != "" {
		schema = req.SchemaOverride
	} else if req.Settings != nil {
		resolved, err := ResolveUserSchema(ctx, req.Pool, req.Settings, req.Renderer, req.UserID, "activity")
		if err != nil {
			return ActivityFilePathRender{}, fmt.Errorf("shadowfs: activity refresh resolve schema: %w", err)
		}
		schema = resolved
	}

	tags := ActivitySchemaTags(ActivitySchemaTagsInput{
		Format:         row.format,
		Source:         row.source,
		StartedAt:      row.startedAt,
		SportType:      row.sportType,
		SubType:        row.subType,
		Title:          row.title,
		City:           row.city,
		Country:        row.country,
		DistanceM:      row.distanceM,
		ElapsedTimeS:   row.elapsedTimeS,
		ElevationGainM: row.elevationGainM,
		IsElectric:     row.isElectric,
		IsIndoor:       row.isIndoor,
		IsVirtual:      row.isVirtual,
		IsCommute:      row.isCommute,
	})
	userDir := row.userDir
	if req.LibraryDirOverride != "" {
		userDir = req.LibraryDirOverride
	}
	ext := filepath.Ext(row.storagePath)
	rendered := req.Renderer.RenderPath("activity", schema, tags, ext)
	targetRel := filepath.Join(userDir, DirForType("activity"), rendered)
	if targetRel != row.storagePath && req.BasePath != "" {
		targetRel = ResolveCollision(req.BasePath, targetRel)
	}
	return ActivityFilePathRender{
		FileID:         row.fileID,
		OldPath:        row.storagePath,
		NewPath:        targetRel,
		StorageBackend: row.storageBackend,
	}, nil
}

func loadActivityFileRefreshRow(ctx context.Context, pool *pgxpool.Pool, userID, activityID, fileID string) (activityFileRefreshRow, error) {
	const baseQuery = `
		SELECT a.id::text, uf.id::text, uf.storage_backend, uf.storage_path,
		       COALESCE(uf.file_name, ''), uf.has_sidecar,
		       COALESCE(u.library_dir_name, a.user_id::text) AS user_dir,
		       COALESCE(a.original_file_format, ''), COALESCE(a.source, ''),
		       a.started_at, a.sport_type, COALESCE(a.sub_type, ''),
		       COALESCE(NULLIF(a.title, ''), NULLIF(a.title_auto, ''), ''),
		       COALESCE(a.start_city, ''), COALESCE(a.start_country, ''),
		       COALESCE(a.distance_m, 0), COALESCE(a.elapsed_time_s, 0),
		       COALESCE(a.elevation_gain_m, 0),
		       COALESCE(a.is_electric, false), COALESCE(a.is_indoor, false),
		       COALESCE(a.is_virtual, false), COALESCE(a.is_commute, false)
		FROM activities a
		JOIN user_files uf ON uf.id = a.file_id
		JOIN users u ON u.id = a.user_id
		WHERE a.user_id = $1 AND uf.user_id = a.user_id AND uf.file_type = 'activity' AND uf.status = 'active' AND `

	query := baseQuery + "a.id = $2"
	arg := activityID
	if activityID == "" {
		query = baseQuery + "uf.id = $2"
		arg = fileID
	}

	var row activityFileRefreshRow
	err := pool.QueryRow(ctx, query, userID, arg).Scan(
		&row.activityID, &row.fileID, &row.storageBackend, &row.storagePath,
		&row.fileName, &row.hasSidecar, &row.userDir,
		&row.format, &row.source, &row.startedAt, &row.sportType, &row.subType,
		&row.title, &row.city, &row.country, &row.distanceM, &row.elapsedTimeS,
		&row.elevationGainM, &row.isElectric, &row.isIndoor, &row.isVirtual, &row.isCommute,
	)
	return row, err
}

func renderedHasSidecar(ctx context.Context, pool *pgxpool.Pool, fileID, userID string) bool {
	var hasSidecar bool
	err := pool.QueryRow(ctx,
		`SELECT has_sidecar FROM user_files WHERE id = $1 AND user_id = $2`,
		fileID, userID).Scan(&hasSidecar)
	return err == nil && hasSidecar
}

func copyActivityFileWithCollisionRetry(basePath, oldFull, targetRel string) (string, error) {
	rel := targetRel
	for attempt := 0; attempt < 10; attempt++ {
		if err := ValidateRelPath(basePath, rel); err != nil {
			return "", fmt.Errorf("shadowfs: activity refresh target validation: %w", err)
		}
		newFull := filepath.Join(basePath, rel)
		if !strings.HasPrefix(filepath.Clean(newFull), filepath.Clean(basePath)+string(filepath.Separator)) {
			return "", fmt.Errorf("shadowfs: activity refresh target escapes base path")
		}
		if err := os.MkdirAll(filepath.Dir(newFull), 0o755); err != nil {
			return "", fmt.Errorf("shadowfs: activity refresh mkdir: %w", err)
		}
		if err := copyFileExclusive(oldFull, newFull); err != nil {
			if errors.Is(err, os.ErrExist) {
				rel = ResolveCollision(basePath, rel)
				continue
			}
			_ = os.Remove(newFull)
			return "", fmt.Errorf("shadowfs: activity refresh copy: %w", err)
		}
		return rel, nil
	}
	return "", fmt.Errorf("shadowfs: activity refresh collision retry exhausted")
}

func copyFileExclusive(src, dst string) (retErr error) {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	defer func() {
		if cerr := out.Close(); retErr == nil {
			retErr = cerr
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return nil
}

func moveMetaSidecar(oldMainPath, newMainPath string) {
	oldSidecar := oldMainPath + ".meta"
	newSidecar := newMainPath + ".meta"
	if _, err := os.Stat(oldSidecar); os.IsNotExist(err) {
		return
	}
	if err := os.MkdirAll(filepath.Dir(newSidecar), 0o755); err != nil {
		slog.Warn("shadowfs: activity refresh sidecar mkdir failed", "error", err)
		return
	}
	if err := os.Rename(oldSidecar, newSidecar); err != nil {
		slog.Warn("shadowfs: activity refresh sidecar rename failed", "from", oldSidecar, "to", newSidecar, "error", err)
	}
}
