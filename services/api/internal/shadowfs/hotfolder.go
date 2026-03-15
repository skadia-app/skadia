package shadowfs

import (
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
// or an error if the file should be quarantined.
type FileCallback func(filePath string) error

// Watcher monitors a hotfolder directory for new files.
type Watcher struct {
	dir      string
	callback FileCallback
	watcher  *fsnotify.Watcher
	debounce time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
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

	// Ensure quarantine subdirectory exists
	if err := EnsureDir(filepath.Join(w.dir, "quarantine")); err != nil {
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

	w.wg.Add(1)
	go w.loop()

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
	slog.Info("hotfolder watcher stopped", "dir", w.dir)
}

// pendingEntry tracks an in-flight debounce timer with pointer identity so we
// can detect when a newer event has superseded an older one before it fires.
type pendingEntry struct {
	timer *time.Timer
}

func (w *Watcher) loop() {
	defer w.wg.Done()

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

			// Skip directories, dotfiles, .meta files, and quarantine/
			name := filepath.Base(evt.Name)
			if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".meta") {
				continue
			}
			if strings.Contains(evt.Name, string(filepath.Separator)+"quarantine"+string(filepath.Separator)) ||
				strings.HasSuffix(evt.Name, string(filepath.Separator)+"quarantine") {
				continue
			}

			// Check it's a supported file type
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".fit" && ext != ".gpx" && ext != ".tcx" {
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
				mu.Unlock()
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
		slog.Error("hotfolder: import failed, quarantining",
			"path", filePath,
			"error", err,
		)
		w.quarantine(filePath)
		return
	}

	slog.Info("hotfolder: import succeeded", "path", filePath)
}

func (w *Watcher) quarantine(filePath string) {
	name := filepath.Base(filePath)
	dest := filepath.Join(w.dir, "quarantine", name)

	// Avoid overwriting existing quarantine files
	if _, err := os.Stat(dest); err == nil {
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		ext := filepath.Ext(name)
		dest = filepath.Join(w.dir, "quarantine",
			stem+"_"+time.Now().Format("20060102-150405")+ext)
	}

	if err := os.Rename(filePath, dest); err != nil {
		slog.Error("hotfolder: quarantine move failed",
			"src", filePath,
			"dest", dest,
			"error", err,
		)
	}
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
func (wm *WatcherManager) UserPath(userID string) string {
	return filepath.Join(wm.basePath, userID)
}

// EnableUser starts a watcher for the user's hotfolder subdirectory.
// If a watcher is already running for this user, this is a no-op.
func (wm *WatcherManager) EnableUser(userID string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	if _, exists := wm.watchers[userID]; exists {
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
