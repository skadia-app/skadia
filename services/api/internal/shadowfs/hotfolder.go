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
	dir       string
	callback  FileCallback
	watcher   *fsnotify.Watcher
	debounce  time.Duration
	stopCh    chan struct{}
	wg        sync.WaitGroup
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
	slog.Info("hotfolder watcher stopped")
}

func (w *Watcher) loop() {
	defer w.wg.Done()

	// pending tracks files that have been written to but not yet processed.
	// The timer is reset on each write event to debounce rapid writes.
	pending := make(map[string]*time.Timer)
	var mu sync.Mutex

	for {
		select {
		case <-w.stopCh:
			mu.Lock()
			for _, t := range pending {
				t.Stop()
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
			if strings.Contains(evt.Name, "/quarantine/") {
				continue
			}

			// Check it's a supported file type
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".fit" && ext != ".gpx" && ext != ".tcx" {
				continue
			}

			mu.Lock()
			if t, exists := pending[evt.Name]; exists {
				t.Reset(w.debounce)
			} else {
				filePath := evt.Name
				pending[filePath] = time.AfterFunc(w.debounce, func() {
					mu.Lock()
					delete(pending, filePath)
					mu.Unlock()

					w.processFile(filePath)
				})
			}
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
