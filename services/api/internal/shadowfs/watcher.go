package shadowfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ignoreTTL is the fixed duration a self-written path stays in the ignore list.
const ignoreTTL = 10 * time.Minute

// maxPendingSizes caps the pendingSizes map to prevent unbounded memory growth
// when the watcher encounters a directory with a very large number of new files.
const maxPendingSizes = 10_000

// allowedExtensions is the set of activity file extensions that the watcher will
// register as new files. Other files are ignored to prevent unrelated files
// (logs, temp data, thumbnails) from polluting the user_files registry.
var allowedExtensions = map[string]bool{
	".fit":     true,
	".gpx":     true,
	".tcx":     true,
	".gz":      true,
	".fit.gz":  true,
	".gpx.gz":  true,
	".tcx.gz":  true,
}

// defaultInterval is the base polling interval.
const defaultInterval = 60 * time.Second

// idleInterval is the polling interval when no changes were seen for an hour.
const idleInterval = 5 * time.Minute

// idleThreshold is the duration without changes before switching to idleInterval.
const idleThreshold = 1 * time.Hour

// skipDirs are directory base names to skip during subtree walks.
var skipDirs = map[string]bool{
	"_edits":   true,
	"_archive": true,
	"_cache":   true,
	"_temp":    true,
}

// LibraryWatcher polls the local library directory for externally-added,
// deleted, or moved files and reconciles them with the user_files registry.
type LibraryWatcher struct {
	pool        *pgxpool.Pool
	basePath    string
	interval    time.Duration
	stateFile   string
	dirMtimes   map[string]time.Time // userDir -> last known mtime
	writeIgnore sync.Map             // path -> expiry time.Time
	lastChange  time.Time
	autoImport  bool

	// pendingSizes tracks files that were newly detected but need a second
	// poll cycle to confirm size stability (i.e. the file is fully written).
	// Key: absolute path, Value: size at first detection.
	pendingSizes map[string]int64
}

// NewLibraryWatcher creates a watcher for the local library directory.
func NewLibraryWatcher(pool *pgxpool.Pool, basePath string, autoImport bool) *LibraryWatcher {
	cacheDir := filepath.Join(basePath, "_cache")
	return &LibraryWatcher{
		pool:         pool,
		basePath:     basePath,
		interval:     defaultInterval,
		stateFile:    filepath.Join(cacheDir, "watcher_state.json"),
		dirMtimes:    make(map[string]time.Time),
		lastChange:   time.Now(),
		autoImport:   autoImport,
		pendingSizes: make(map[string]int64),
	}
}

// IgnorePath marks a path as self-written so the watcher will skip it.
// The path should be absolute.
func (w *LibraryWatcher) IgnorePath(path string) {
	w.writeIgnore.Store(path, time.Now().Add(ignoreTTL))
}

// Run starts the polling loop. It blocks until ctx is cancelled.
// NOTE: The watcher is complementary to the Reconciliation job, not a replacement.
// The watcher detects real-time changes; the reconciler handles inconsistencies
// from crashes, manual edits, or missed events. Both are needed for full consistency.
//
// The watcher uses directory mtime to detect changes, which on some
// filesystems does not propagate when files in deep subdirectories change.
// This is acceptable because the Reconciler performs periodic full-scans.
// If sub-second detection of deep subdirectory changes becomes necessary,
// consider adding an occasional forced full-scan (e.g. every 10th cycle).
func (w *LibraryWatcher) Run(ctx context.Context) {
	w.loadState()
	slog.Info("library-watcher: started", "basePath", w.basePath, "interval", w.interval, "autoImport", w.autoImport)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("library-watcher: shutting down")
			return
		case <-ticker.C:
			w.poll(ctx)
			w.adjustInterval(ticker)
		}
	}
}

// poll performs a single polling cycle.
func (w *LibraryWatcher) poll(ctx context.Context) {
	// List user directories in basePath.
	entries, err := os.ReadDir(w.basePath)
	if err != nil {
		slog.Error("library-watcher: read base dir", "error", err, "path", w.basePath)
		return
	}

	// Build map of active users: library_dir_name/UUID -> user_id.
	userDirs, err := w.listUserDirs(ctx)
	if err != nil {
		slog.Error("library-watcher: list user dirs", "error", err)
		return
	}

	// Tier 1: check directory mtimes.
	changedDirs := make(map[string]string) // dirName -> userID
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		userID, ok := userDirs[e.Name()]
		if !ok {
			continue // Not a known user directory.
		}

		info, err := e.Info()
		if err != nil {
			continue
		}
		mtime := info.ModTime()
		prev, known := w.dirMtimes[e.Name()]
		if !known || mtime.After(prev) {
			changedDirs[e.Name()] = userID
		}
		// Update mtime (also for unchanged — handles clock skew on restart).
		w.dirMtimes[e.Name()] = mtime
	}

	if len(changedDirs) == 0 {
		// Also check pending sizes even when no dir changed.
		w.checkPendingSizes(ctx, userDirs)
		w.saveState()
		return
	}

	w.lastChange = time.Now()

	// Tier 2: subtree walk for changed directories.
	for dirName, userID := range changedDirs {
		if ctx.Err() != nil {
			return
		}
		w.scanUserDir(ctx, dirName, userID)
	}

	// Check pending sizes (files from previous cycles that need stability check).
	// Reuse userDirs from poll() to avoid redundant DB query.
	w.checkPendingSizes(ctx, userDirs)

	w.saveState()
}

// scanUserDir walks a user's library subtree looking for new, deleted, or moved files.
func (w *LibraryWatcher) scanUserDir(ctx context.Context, dirName, userID string) {
	userRoot := filepath.Join(w.basePath, dirName)

	// Get all registered files for this user.
	registered, err := w.registeredFiles(ctx, userID)
	if err != nil {
		slog.Error("library-watcher: query registered files", "error", err, "user", userID)
		return
	}

	// Track which registered paths we see on disk.
	seen := make(map[string]bool, len(registered))

	// Walk filesystem.
	err = filepath.WalkDir(userRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // Skip inaccessible entries.
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Skip special directories.
		if d.IsDir() {
			base := filepath.Base(path)
			if skipDirs[base] {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden files.
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		// Skip sidecar files (.meta, .meta.json).
		if strings.HasSuffix(d.Name(), ".meta") || strings.HasSuffix(d.Name(), ".meta.json") {
			return nil
		}

		// Check self-write ignore.
		if w.isIgnored(path) {
			return nil
		}

		// Compute relative path from basePath.
		relPath, err := filepath.Rel(w.basePath, path)
		if err != nil {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		if rec, ok := registered[relPath]; ok {
			// Known file — mark as seen.
			seen[relPath] = true

			// Check if file was modified externally (mtime or size changed).
			if info.Size() != rec.SizeBytes {
				slog.Info("library-watcher: file modified externally",
					"path", relPath, "user", userID,
					"registeredSize", rec.SizeBytes, "diskSize", info.Size())
				// TODO: re-hash and update registry (future enhancement).
			}
		} else {
			// Unknown file — candidate for new-file detection.
			w.handleNewFile(ctx, path, relPath, userID, info)
		}
		return nil
	})
	if err != nil && ctx.Err() == nil {
		slog.Error("library-watcher: walk failed", "error", err, "dir", userRoot)
	}

	// Detect deleted files: registered but not seen on disk.
	for relPath, rec := range registered {
		if !seen[relPath] {
			w.handleDeletedFile(ctx, relPath, rec, userID)
		}
	}

	// Detect moves/renames among newly-missing and newly-found files.
	// This is handled implicitly: if a file disappeared and a new file with
	// the same size+hash appeared, the hash match in handleNewFile (via
	// pendingSizes → import path) will catch it.
}

// handleNewFile processes a file found on disk that is not in the registry.
func (w *LibraryWatcher) handleNewFile(ctx context.Context, absPath, relPath, userID string, info fs.FileInfo) {
	// Size-stability check: record size on first detection, only proceed
	// when the size is unchanged at the next poll cycle.
	prevSize, pending := w.pendingSizes[absPath]
	if !pending {
		// Cap pendingSizes to prevent unbounded memory growth.
		if len(w.pendingSizes) >= maxPendingSizes {
			// Evict the oldest entry (arbitrary key — map iteration order is random,
			// which is acceptable since all entries are equally stale).
			for k := range w.pendingSizes {
				slog.Warn("library-watcher: pendingSizes at capacity, evicting entry",
					"evicted", k, "capacity", maxPendingSizes)
				delete(w.pendingSizes, k)
				break
			}
		}
		// First time seeing this file — record and wait.
		w.pendingSizes[absPath] = info.Size()
		slog.Debug("library-watcher: new file detected, awaiting stability",
			"path", relPath, "size", info.Size())
		return
	}

	if info.Size() != prevSize {
		// Size changed — still being written. Update and wait.
		w.pendingSizes[absPath] = info.Size()
		slog.Debug("library-watcher: file size changed, resetting stability",
			"path", relPath, "prevSize", prevSize, "newSize", info.Size())
		return
	}

	// Size is stable — process the file.
	delete(w.pendingSizes, absPath)

	// Validate the path is not a symlink escaping the basePath.
	if err := validateWatcherPath(absPath, w.basePath); err != nil {
		slog.Warn("library-watcher: path validation failed — skipping",
			"path", absPath, "error", err)
		return
	}

	// Validate symlinks before hashing to prevent path traversal between
	// detection and registry insertion. A minimal TOCTOU gap remains between
	// validateWatcherPath and hashFile (both use the same path), but the
	// registry INSERT uses ON CONFLICT to handle concurrent races safely.
	hash, err := hashFile(absPath)
	if err != nil {
		slog.Warn("library-watcher: hash file failed", "error", err, "path", absPath)
		return
	}

	// Check for existing file with same hash (move/rename detection).
	moved, err := w.detectMove(ctx, userID, relPath, hash, info.Size())
	if err != nil {
		slog.Warn("library-watcher: move detection failed", "error", err, "path", relPath)
	}
	if moved {
		return
	}

	if w.autoImport {
		// Register as new file.
		slog.Info("library-watcher: registering external file",
			"path", relPath, "user", userID, "size", info.Size(), "hash", hash)
		w.registerExternalFile(ctx, relPath, userID, info, hash)
	} else {
		slog.Info("library-watcher: external file detected (auto-import disabled)",
			"path", relPath, "user", userID, "size", info.Size())
	}
}

// handleDeletedFile handles a file that is in the registry but missing from disk.
//
// Deleted-file detection in the watcher is complementary to the Reconciler
// (checkDanglingRecords). The watcher provides near-real-time awareness of
// disappeared files, while the Reconciler performs authoritative cleanup with
// safety guards (>50% check, 1h grace period). The watcher intentionally does
// NOT delete registry entries — it only logs, leaving the Reconciler to handle
// actual cleanup to avoid race conditions with in-flight operations.
func (w *LibraryWatcher) handleDeletedFile(_ context.Context, relPath string, rec registeredFile, userID string) {
	// Check if the absolute path is in the ignore list (app may have moved it).
	absPath := filepath.Join(w.basePath, relPath)
	if w.isIgnored(absPath) {
		return
	}

	slog.Warn("library-watcher: registered file missing from disk",
		"path", relPath, "user", userID, "fileID", rec.ID, "size", rec.SizeBytes)
	// Future: configurable soft-delete. For now, just log.
}

// detectMove checks if the file matches a registered file that is missing
// from disk (same user, same hash). If so, updates the storage_path.
// Scans ALL candidates and only selects the one whose old path is actually missing on disk.
func (w *LibraryWatcher) detectMove(ctx context.Context, userID, newRelPath, hash string, size int64) (bool, error) {
	rows, err := w.pool.Query(ctx,
		`SELECT id, storage_path FROM user_files
		 WHERE user_id = $1 AND file_hash = $2 AND size_bytes = $3
		   AND storage_backend = 'local' AND status = 'active'
		 ORDER BY updated_at DESC`,
		userID, hash, size,
	)
	if err != nil {
		return false, fmt.Errorf("query move candidates: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		id, path string
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.path); err != nil {
			return false, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}

	// Find the first candidate whose old path is actually missing on disk.
	for _, c := range candidates {
		oldAbs := filepath.Join(w.basePath, c.path)
		if _, statErr := os.Stat(oldAbs); statErr == nil {
			continue // Old file still exists — this is a copy, not a move.
		}
		// This candidate's old file is missing — this is the moved file.
		_, err = w.pool.Exec(ctx,
			`UPDATE user_files SET storage_path = $1, updated_at = NOW()
			 WHERE id = $2 AND user_id = $3`,
			newRelPath, c.id, userID,
		)
		if err != nil {
			return false, err
		}
		slog.Info("library-watcher: detected file move/rename",
			"user", userID, "fileID", c.id,
			"oldPath", c.path, "newPath", newRelPath)
		return true, nil
	}

	return false, nil // All candidates still have their files — this is not a move.
}

// registerExternalFile inserts a new file record for an externally-added file.
// Only files with allowed extensions (.fit, .gpx, .tcx, .gz) are registered.
func (w *LibraryWatcher) registerExternalFile(ctx context.Context, relPath, userID string, info fs.FileInfo, hash string) {
	fileName := filepath.Base(relPath)

	// Extension whitelist: only register known activity file types.
	lower := strings.ToLower(fileName)
	allowed := false
	for ext := range allowedExtensions {
		if strings.HasSuffix(lower, ext) {
			allowed = true
			break
		}
	}
	if !allowed {
		slog.Debug("library-watcher: skipping file with disallowed extension",
			"path", relPath, "name", fileName)
		return
	}

	// Read the first 512 bytes for magic-byte content-type detection.
	// Fall back to extension-only if the file cannot be read.
	var first512 []byte
	absPath := filepath.Join(w.basePath, relPath)
	if f, err := os.Open(absPath); err == nil {
		buf := make([]byte, 512)
		n, _ := io.ReadFull(f, buf)
		f.Close()
		if n > 0 {
			first512 = buf[:n]
		}
	}

	contentType := DetectContentType(fileName, "", first512)
	fileType := w.inferFileType(relPath)

	_, err := w.pool.Exec(ctx,
		`INSERT INTO user_files (user_id, file_type, storage_backend, storage_path,
			content_type, file_name, size_bytes, file_hash, status)
		 VALUES ($1, $2, 'local', $3, $4, $5, $6, $7, 'active')
		 ON CONFLICT (storage_backend, storage_path) DO NOTHING`,
		userID, fileType, relPath, contentType, fileName, info.Size(), hash,
	)
	if err != nil {
		slog.Error("library-watcher: register file failed",
			"error", err, "path", relPath, "user", userID)
	}
}

// inferFileType guesses the file type from the directory structure.
// Example: "user-id/activities/2026/synthetic-activity.fit" -> "activity"
func (w *LibraryWatcher) inferFileType(relPath string) string {
	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) < 2 {
		return "document"
	}
	// The second component (after user dir) maps to a file type.
	dirName := parts[1]
	for ft, dn := range fileTypeDirName {
		if dn == dirName {
			return ft
		}
	}
	return "document" // Fallback.
}

// registeredFile holds minimal info for a file from user_files.
type registeredFile struct {
	ID        string
	SizeBytes int64
	FileHash  string
}

// registeredFiles returns all active local files for a user, keyed by storage_path.
func (w *LibraryWatcher) registeredFiles(ctx context.Context, userID string) (map[string]registeredFile, error) {
	rows, err := w.pool.Query(ctx,
		`SELECT id, storage_path, size_bytes, file_hash
		 FROM user_files
		 WHERE user_id = $1 AND storage_backend = 'local' AND status = 'active'`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]registeredFile)
	for rows.Next() {
		var id, path, hash string
		var size int64
		if err := rows.Scan(&id, &path, &size, &hash); err != nil {
			return nil, err
		}
		result[path] = registeredFile{ID: id, SizeBytes: size, FileHash: hash}
	}
	return result, rows.Err()
}

// listUserDirs returns a map of directory name -> user ID for all active users.
func (w *LibraryWatcher) listUserDirs(ctx context.Context) (map[string]string, error) {
	rows, err := w.pool.Query(ctx,
		`SELECT id, library_dir_name FROM users WHERE is_active = true AND deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dirs := make(map[string]string)
	for rows.Next() {
		var id string
		var dirName *string
		if err := rows.Scan(&id, &dirName); err != nil {
			return nil, err
		}
		name := id
		if dirName != nil && *dirName != "" {
			name = *dirName
		}
		dirs[name] = id
	}
	return dirs, rows.Err()
}

// isIgnored checks if a path is in the self-write ignore list and not expired.
func (w *LibraryWatcher) isIgnored(absPath string) bool {
	val, ok := w.writeIgnore.Load(absPath)
	if !ok {
		return false
	}
	expiry := val.(time.Time)
	if time.Now().After(expiry) {
		w.writeIgnore.Delete(absPath)
		return false
	}
	return true
}

// checkPendingSizes re-checks files in pendingSizes that may now be stable.
// Files whose source path no longer exists are removed from tracking.
// userDirs is passed from poll() to avoid a redundant listUserDirs DB call.
func (w *LibraryWatcher) checkPendingSizes(ctx context.Context, userDirs map[string]string) {
	if len(w.pendingSizes) == 0 {
		return
	}

	// Copy keys to a slice before iterating — handleNewFile mutates the
	// map (deletes the entry on stable size), so ranging directly is unsafe.
	keys := make([]string, 0, len(w.pendingSizes))
	for absPath := range w.pendingSizes {
		keys = append(keys, absPath)
	}

	for _, absPath := range keys {
		if _, stillPending := w.pendingSizes[absPath]; !stillPending {
			continue // already handled by handleNewFile in this loop
		}

		info, err := os.Stat(absPath)
		if err != nil {
			// File disappeared — stop tracking.
			delete(w.pendingSizes, absPath)
			continue
		}

		relPath, relErr := filepath.Rel(w.basePath, absPath)
		if relErr != nil {
			delete(w.pendingSizes, absPath)
			continue
		}

		// Determine user from path.
		parts := strings.SplitN(relPath, string(filepath.Separator), 2)
		if len(parts) < 2 {
			delete(w.pendingSizes, absPath)
			continue
		}

		userID, ok := userDirs[parts[0]]
		if !ok {
			delete(w.pendingSizes, absPath)
			continue
		}

		w.handleNewFile(ctx, absPath, relPath, userID, info)
	}
}

// adjustInterval adapts the polling interval based on recent activity.
// NOTE: The interval is not reset eagerly after loadState(). If the persisted
// lastChange is older than idleThreshold, the first adjustInterval call after
// startup will immediately switch to idleInterval. This is intentional: a
// restart after a long idle period should adopt the slower polling cadence.
func (w *LibraryWatcher) adjustInterval(ticker *time.Ticker) {
	var target time.Duration
	if time.Since(w.lastChange) > idleThreshold {
		target = idleInterval
	} else {
		target = defaultInterval
	}
	if target != w.interval {
		slog.Info("library-watcher: adjusting interval",
			"from", w.interval, "to", target)
		w.interval = target
		ticker.Reset(target)
	}
}

// ---------- Persistent state ----------

// watcherState is the JSON schema for the state file.
type watcherState struct {
	DirMtimes  map[string]time.Time `json:"dir_mtimes"`
	LastChange time.Time            `json:"last_change"`
}

// loadState reads the persistent mtime cache from disk.
func (w *LibraryWatcher) loadState() {
	data, err := os.ReadFile(w.stateFile)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("library-watcher: read state file", "error", err)
		}
		return
	}
	var state watcherState
	if err := json.Unmarshal(data, &state); err != nil {
		slog.Warn("library-watcher: parse state file", "error", err)
		return
	}
	if state.DirMtimes != nil {
		w.dirMtimes = state.DirMtimes
	}
	if !state.LastChange.IsZero() {
		w.lastChange = state.LastChange
	}
	slog.Info("library-watcher: loaded state", "userDirs", len(w.dirMtimes))
}

// saveState persists the mtime cache to disk.
// Uses atomic write (.tmp + rename) to prevent corruption from crashes.
func (w *LibraryWatcher) saveState() {
	state := watcherState{
		DirMtimes:  w.dirMtimes,
		LastChange: w.lastChange,
	}
	data, err := json.Marshal(state)
	if err != nil {
		slog.Error("library-watcher: marshal state", "error", err)
		return
	}

	// Ensure _cache directory exists.
	dir := filepath.Dir(w.stateFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Error("library-watcher: create cache dir", "error", err)
		return
	}

	// Atomic write: write to .tmp, then rename.
	tmpFile := w.stateFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		slog.Error("library-watcher: write temp state file", "error", err)
		return
	}
	if err := os.Rename(tmpFile, w.stateFile); err != nil {
		slog.Error("library-watcher: rename state file", "error", err)
		os.Remove(tmpFile) //nolint:errcheck
	}
}

// ---------- Helpers ----------

// maxHashFileSize is the maximum file size we will hash (500 MB).
// Files larger than this are skipped with a warning.
const maxHashFileSize = 500 * 1024 * 1024

// hashFile computes the SHA256 hex digest of a file.
// Files larger than maxHashFileSize are skipped.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() > maxHashFileSize {
		return "", fmt.Errorf("file too large for hashing (%d bytes > %d limit)", info.Size(), maxHashFileSize)
	}

	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, maxHashFileSize)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// validateWatcherPath resolves symlinks and verifies the real path is within basePath.
// This prevents symlink traversal attacks that could escape the library directory.
func validateWatcherPath(absPath, basePath string) error {
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}
	resolvedBase, err := filepath.EvalSymlinks(basePath)
	if err != nil {
		return fmt.Errorf("resolve basePath symlinks: %w", err)
	}
	if !strings.HasPrefix(resolved, resolvedBase+string(filepath.Separator)) && resolved != resolvedBase {
		return fmt.Errorf("resolved path %q escapes basePath %q", resolved, resolvedBase)
	}
	return nil
}
