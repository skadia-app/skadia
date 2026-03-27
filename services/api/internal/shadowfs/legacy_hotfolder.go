// Legacy ShadowFS functions retained for backward compatibility with the hotfolder watcher
// and the legacy rename handlers. Do not add new code to this file.
package shadowfs

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileCallback is called when a new file is detected in the hotfolder.
// The callback receives the full file path. It should return nil on success
// or an error if the file could not be imported.
type FileCallback func(filePath string) error

// Watcher monitors a hotfolder directory for new files.
type Watcher struct {
	dir       string
	callback  FileCallback
	watcher   *fsnotify.Watcher
	debounce  time.Duration
	stopCh    chan struct{}
	wg        sync.WaitGroup
	processWg sync.WaitGroup // tracks in-flight processFile goroutines
}

// NewWatcher creates a new hotfolder watcher.
func NewWatcher(dir string, callback FileCallback) *Watcher {
	return &Watcher{
		dir:      dir,
		callback: callback,
		debounce: 500 * time.Millisecond,
		stopCh:   make(chan struct{}),
	}
}

// Start begins watching the hotfolder directory.
func (w *Watcher) Start() error {
	if err := EnsureDir(w.dir); err != nil {
		return err
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.watcher = fsw

	if err := fsw.Add(w.dir); err != nil {
		fsw.Close()
		return err
	}

	w.wg.Go(func() { w.loop() })

	// Scan for files already present (dropped while server was down).
	w.wg.Go(func() { w.scanExisting() })

	slog.Info("hotfolder watcher started", "dir", w.dir)
	return nil
}

// Stop gracefully stops the watcher.
func (w *Watcher) Stop() {
	close(w.stopCh)
	if w.watcher != nil {
		w.watcher.Close()
	}
	w.wg.Wait()
	w.processWg.Wait() // wait for in-flight imports to complete
	slog.Info("hotfolder watcher stopped", "dir", w.dir)
}

// pendingEntry tracks an in-flight debounce timer with pointer identity so we
// can detect when a newer event has superseded an older one before it fires.
type pendingEntry struct {
	timer *time.Timer
}

func (w *Watcher) loop() {
	pending := make(map[string]*pendingEntry)
	var mu sync.Mutex

	for {
		select {
		case <-w.stopCh:
			mu.Lock()
			for _, e := range pending {
				e.timer.Stop()
			}
			mu.Unlock()
			return

		case evt, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Only watch for Create and Write events
			if evt.Op&(fsnotify.Create|fsnotify.Write) == 0 {
				continue
			}

			// Skip directories, dotfiles, and .meta files
			name := filepath.Base(evt.Name)
			if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".meta") {
				continue
			}

			// Check it's a supported activity file type (including .gz variants)
			if !isValidActivityExt(name) {
				continue
			}

			// Stop any previous timer for this path and create a fresh entry.
			// Using pointer identity (entry vs. pending[filePath]) prevents the
			// race where a just-fired AfterFunc goroutine processes a file that
			// was superseded by a newer write event.
			mu.Lock()
			if old, exists := pending[evt.Name]; exists {
				old.timer.Stop()
				delete(pending, evt.Name)
			}
			filePath := evt.Name
			entry := &pendingEntry{}
			pending[filePath] = entry
			entry.timer = time.AfterFunc(w.debounce, func() {
				mu.Lock()
				if pending[filePath] != entry {
					// Superseded by a newer event — skip.
					mu.Unlock()
					return
				}
				delete(pending, filePath)
				// Add to processWg inside the lock to prevent race with Stop().
				w.processWg.Add(1)
				mu.Unlock()
				defer w.processWg.Done()
				w.processFile(filePath)
			})
			mu.Unlock()

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Error("hotfolder watcher error", "error", err)
		}
	}
}

func (w *Watcher) processFile(filePath string) {
	slog.Info("hotfolder: processing file", "path", filePath)
	if err := w.callback(filePath); err != nil {
		slog.Error("hotfolder: import failed, file left in place",
			"path", filePath,
			"error", err,
		)
		return
	}
	slog.Info("hotfolder: import succeeded", "path", filePath)
}

// scanExisting processes any files already present in the hotfolder directory
// when the watcher starts. This handles files dropped while the server was down
// and error files left from previous runs.
// Files are processed sequentially to avoid overwhelming DB/S3 on startup.
func (w *Watcher) scanExisting() {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		slog.Warn("hotfolder: startup scan failed", "dir", w.dir, "error", err)
		return
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !isValidActivityExt(e.Name()) {
			continue
		}
		filePath := filepath.Join(w.dir, e.Name())
		count++
		w.processWg.Add(1)
		func() {
			defer w.processWg.Done()
			w.processFile(filePath)
		}()
	}
	if count > 0 {
		slog.Info("hotfolder: startup scan complete", "dir", w.dir, "files", count)
	}
}

// isValidActivityExt returns true if name has a valid activity file extension,
// including .gz variants (.fit, .gpx, .tcx, .fit.gz, .gpx.gz, .tcx.gz).
func isValidActivityExt(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range []string{".fit", ".gpx", ".tcx"} {
		if strings.HasSuffix(lower, ext) || strings.HasSuffix(lower, ext+".gz") {
			return true
		}
	}
	return false
}

// ── WatcherManager ────────────────────────────────────────────────────────────

// WatcherManager manages one hotfolder Watcher per user.
// Each user gets an isolated subdirectory: {BasePath}/{userID}/.
// Files dropped into a user's directory are only imported for that user.
type WatcherManager struct {
	basePath string
	callback func(userID, filePath string) error
	mu       sync.RWMutex
	watchers map[string]*Watcher // userID → Watcher
}

// NewWatcherManager creates a manager rooted at basePath.
// callback is invoked with the owning userID and the full file path.
func NewWatcherManager(basePath string, callback func(userID, filePath string) error) *WatcherManager {
	return &WatcherManager{
		basePath: basePath,
		callback: callback,
		watchers: make(map[string]*Watcher),
	}
}

// UserPath returns the hotfolder path for a specific user.
// Returns an error if userID contains path traversal sequences.
func (wm *WatcherManager) UserPath(userID string) (string, error) {
	if strings.Contains(userID, "..") || strings.ContainsAny(userID, "/\\") {
		return "", fmt.Errorf("invalid userID: %q", userID)
	}
	return filepath.Join(wm.basePath, userID), nil
}

// EnableUser starts a watcher for the user's hotfolder subdirectory.
// If a watcher is already running for this user, this is a no-op.
func (wm *WatcherManager) EnableUser(userID string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	if _, exists := wm.watchers[userID]; exists {
		// TOCTOU note: read-then-write is harmless here because EnableUser is
		// idempotent — a concurrent call would also find the watcher present and
		// return nil without creating a duplicate.
		return nil // already watching
	}
	userPath := filepath.Join(wm.basePath, userID)
	w := NewWatcher(userPath, func(filePath string) error {
		return wm.callback(userID, filePath)
	})
	if err := w.Start(); err != nil {
		return err
	}
	wm.watchers[userID] = w
	return nil
}

// DisableUser stops and removes the watcher for the given user.
func (wm *WatcherManager) DisableUser(userID string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	if w, exists := wm.watchers[userID]; exists {
		w.Stop()
		delete(wm.watchers, userID)
	}
}

// StopAll stops all running watchers. Call on server shutdown.
func (wm *WatcherManager) StopAll() {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	for _, w := range wm.watchers {
		w.Stop()
	}
	wm.watchers = make(map[string]*Watcher)
}
