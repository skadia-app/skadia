package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	var dir string
	var dryRun bool
	flag.StringVar(&dir, "dir", "", "directory to scan for .meta files")
	flag.BoolVar(&dryRun, "dry-run", true, "only print what would be restored (default: true)")
	flag.Parse()

	if dir == "" {
		fmt.Fprintln(os.Stderr, "Usage: skadia restore --dir=/path/to/shadow")
		os.Exit(1)
	}

	slog.Info("scanning for .meta files", "dir", dir, "dry_run", dryRun)

	var found int
	var errors int

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".meta") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			slog.Error("failed to read meta file", "path", path, "error", err)
			errors++
			return nil
		}

		var meta shadowfs.ShadowMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			slog.Error("failed to parse meta file", "path", path, "error", err)
			errors++
			return nil
		}

		// The original file is the same path without .meta suffix
		originalPath := strings.TrimSuffix(path, ".meta")
		if _, err := os.Stat(originalPath); os.IsNotExist(err) {
			slog.Warn("original file missing for meta",
				"meta", path,
				"expected", originalPath,
			)
			errors++
			return nil
		}

		found++
		slog.Info("found restorable activity",
			"skadia_id", meta.SkadiaID,
			"file_hash", meta.FileHashSHA256,
			"format", meta.FileFormat,
			"sport", meta.SportType,
			"original", meta.OriginalFilename,
			"path", originalPath,
		)

		if !dryRun {
			// In a real restore scenario, this would:
			// 1. Connect to the database
			// 2. Check if the activity already exists (by hash)
			// 3. Re-parse the original file
			// 4. Re-insert into the database
			slog.Info("restore not yet implemented - use API reprocess endpoint after re-upload")
		}

		return nil
	})

	if err != nil {
		slog.Error("scan failed", "error", err)
		os.Exit(1)
	}

	slog.Info("scan complete",
		"found", found,
		"errors", errors,
	)
}
