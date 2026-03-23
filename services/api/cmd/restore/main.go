package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/importutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
)

var (
	dir              = flag.String("dir", "", "Shadow File System Verzeichnis (enthält .meta-Dateien)")
	dbURL            = flag.String("db-url", os.Getenv("DATABASE_URL"), "PostgreSQL Connection String")
	dryRun           = flag.Bool("dry-run", true, "Nur prüfen, nicht importieren (Standard: true)")
	conflictStrategy = flag.String("conflict", "skip", "Duplikat-Strategie: skip|replace|enrich_missing")
)

type result struct {
	ok      int
	skipped int
	errors  int
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	flag.Parse()

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "Usage: skadia-restore -dir <shadowfs-path> [-dry-run=false] [-conflict skip|replace|enrich_missing]")
		os.Exit(1)
	}

	if !*dryRun && *dbURL == "" {
		fmt.Fprintln(os.Stderr, "Fehler: -db-url oder $DATABASE_URL muss gesetzt sein wenn --dry-run=false")
		os.Exit(1)
	}

	ctx := context.Background()
	if err := run(ctx); err != nil {
		slog.Error("restore failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// Collect all restorable entries first (scan phase)
	type entry struct {
		meta         shadowfs.ShadowMeta
		originalPath string
		fileBytes    []byte
	}

	var entries []entry

	slog.Info("scanning for .meta files", "dir", *dir, "dry_run", *dryRun)

	scanErr := filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
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
			slog.Error("meta lesen fehlgeschlagen", "path", path, "error", err)
			return nil
		}

		var meta shadowfs.ShadowMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			slog.Error("meta parsen fehlgeschlagen", "path", path, "error", err)
			return nil
		}

		originalPath := strings.TrimSuffix(path, ".meta")
		if _, err := os.Stat(originalPath); os.IsNotExist(err) {
			slog.Warn("original file fehlt für meta",
				"meta", path,
				"erwartet", originalPath,
			)
			return nil
		}

		fileBytes, err := os.ReadFile(originalPath)
		if err != nil {
			slog.Error("original file lesen fehlgeschlagen", "path", originalPath, "error", err)
			return nil
		}

		entries = append(entries, entry{
			meta:         meta,
			originalPath: originalPath,
			fileBytes:    fileBytes,
		})
		return nil
	})

	if scanErr != nil {
		return fmt.Errorf("scan failed: %w", scanErr)
	}

	total := len(entries)
	slog.Info("scan abgeschlossen", "total", total)

	if total == 0 {
		fmt.Println("Keine wiederherstellbaren Aktivitäten gefunden.")
		return nil
	}

	if *dryRun {
		// Dry-run: nur auflisten, keine DB-Verbindung nötig
		for i, e := range entries {
			filename := filepath.Base(e.originalPath)
			fmt.Printf("[%d/%d] %s ... (dry-run) user=%s sport=%s format=%s id=%s\n",
				i+1, total,
				filename,
				e.meta.UserID,
				e.meta.SportType,
				e.meta.FileFormat,
				e.meta.SkadiaID,
			)
		}
		fmt.Printf("\nErgebnis (dry-run): %d Dateien gefunden — mit --dry-run=false importieren\n", total)
		return nil
	}

	// Live-run: DB-Verbindung aufbauen
	db, err := database.New(ctx, *dbURL)
	if err != nil {
		return fmt.Errorf("database verbinden: %w", err)
	}
	defer db.Close()

	strategy := importer.ConflictStrategy(*conflictStrategy)
	switch strategy {
	case importer.StrategySkip, importer.StrategyReplaceAll, importer.StrategyEnrichMissing:
		// gültig
	default:
		slog.Warn("unbekannte conflict-Strategie, nutze 'skip'", "conflict", *conflictStrategy)
		strategy = importer.StrategySkip
	}

	var res result

	for i, e := range entries {
		filename := filepath.Base(e.originalPath)
		prefix := fmt.Sprintf("[%d/%d] %s", i+1, total, filename)

		status, importErr := restoreOne(ctx, db, e.meta, e.fileBytes, e.originalPath, strategy)
		if importErr != nil {
			fmt.Printf("%s ... FEHLER: %v\n", prefix, importErr)
			res.errors++
		} else {
			fmt.Printf("%s ... %s\n", prefix, status)
			switch status {
			case "OK":
				res.ok++
			default:
				// "SKIP (duplicate)" etc.
				res.skipped++
			}
		}
	}

	fmt.Printf("\nErgebnis: %d OK, %d Skip, %d Fehler\n", res.ok, res.skipped, res.errors)
	return nil
}

// restoreOne versucht eine einzelne Aktivität wiederherzustellen.
// Gibt ein Status-String zurück ("OK", "SKIP (duplicate)", …) oder einen Fehler.
func restoreOne(
	ctx context.Context,
	db *database.DB,
	meta shadowfs.ShadowMeta,
	fileBytes []byte,
	originalPath string,
	strategy importer.ConflictStrategy,
) (string, error) {
	format := detectFormat(fileBytes, originalPath)
	if format == "" {
		return "", fmt.Errorf("unbekanntes Dateiformat: %s", originalPath)
	}

	// Datei parsen
	var (
		data *model.ActivityData
		err  error
	)
	switch format {
	case "fit":
		data, err = fitimport.ParseFIT(bytes.NewReader(fileBytes), filepath.Base(originalPath))
	case "gpx":
		data, err = gpximport.ParseGPX(bytes.NewReader(fileBytes), filepath.Base(originalPath))
	case "tcx":
		data, err = tcximport.ParseTCX(bytes.NewReader(fileBytes), filepath.Base(originalPath))
	}
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", format, err)
	}

	// Duplikat-Check (Tier-1)
	match, err := importer.CheckTier1(ctx, db, meta.UserID, fileBytes, data, 0, 0, 0)
	if err != nil {
		slog.Warn("duplikat-check fehlgeschlagen, importiere trotzdem", "error", err)
	}

	if match != nil {
		switch strategy {
		case importer.StrategySkip:
			return fmt.Sprintf("SKIP (duplicate, id=%s, confidence=%s)", match.ExistingActivityID, match.Confidence), nil

		case importer.StrategyReplaceAll:
			if replaceErr := importer.ReplaceActivity(ctx, db, match.ExistingActivityID, data); replaceErr != nil {
				return "", fmt.Errorf("replace activity %s: %w", match.ExistingActivityID, replaceErr)
			}
			return fmt.Sprintf("OK (replaced id=%s)", match.ExistingActivityID), nil

		case importer.StrategyEnrichMissing:
			existing, getErr := db.GetActivity(ctx, match.ExistingActivityID)
			if getErr != nil {
				slog.Warn("existing activity laden fehlgeschlagen", "id", match.ExistingActivityID, "error", getErr)
			}
			n, enrichErr := importer.EnrichActivity(ctx, db, match.ExistingActivityID, existing, data)
			if enrichErr != nil {
				return "", fmt.Errorf("enrich activity %s: %w", match.ExistingActivityID, enrichErr)
			}
			return fmt.Sprintf("OK (enriched id=%s, fields=%d)", match.ExistingActivityID, n), nil
		}
	}

	// Neue Aktivität einfügen
	// TODO: h.Storage (Garage S3) steht hier nicht zur Verfügung — Original-Datei wird
	// nicht in den Objekt-Speicher hochgeladen. Wer den Restore-Pfad vollständig mit
	// S3-Upload braucht, muss storage.New() aus dem storage-Package hier einbinden und
	// den s3Key (userID/fileHash.format) analog zu upload.go befüllen.
	activity, err := db.InsertActivity(ctx, meta.UserID, data)
	if err != nil {
		return "", fmt.Errorf("insert activity: %w", err)
	}

	// Streams einfügen
	if len(data.Streams) > 0 {
		if err := db.InsertStreams(ctx, activity.ID, data.Streams); err != nil {
			slog.Warn("insert streams fehlgeschlagen", "id", activity.ID, "error", err)
		}
	}

	// Track einfügen
	if len(data.TrackPoints) > 0 {
		if err := db.InsertTrack(ctx, activity.ID, data.TrackPoints); err != nil {
			slog.Warn("insert track fehlgeschlagen", "id", activity.ID, "error", err)
		}
	}

	// R-R-Intervalle (FIT only)
	if len(data.RRIntervals) > 0 {
		if err := db.InsertRRIntervals(ctx, activity.ID, data.RRIntervals); err != nil {
			slog.Warn("insert rr intervals fehlgeschlagen", "id", activity.ID, "error", err)
		}
	}

	// Laps, Splits, Lengths, ClimbPro
	if len(data.Laps) > 0 {
		if err := db.InsertLaps(ctx, activity.ID, data.Laps); err != nil {
			slog.Warn("insert laps fehlgeschlagen", "id", activity.ID, "error", err)
		}
	}
	if len(data.Splits) > 0 {
		if err := db.InsertSplits(ctx, activity.ID, data.Splits); err != nil {
			slog.Warn("insert splits fehlgeschlagen", "id", activity.ID, "error", err)
		}
	}
	if len(data.SplitSummaries) > 0 {
		if err := db.InsertSplitSummaries(ctx, activity.ID, data.SplitSummaries); err != nil {
			slog.Warn("insert split summaries fehlgeschlagen", "id", activity.ID, "error", err)
		}
	}
	if len(data.Lengths) > 0 {
		if err := db.InsertLengths(ctx, activity.ID, data.Lengths); err != nil {
			slog.Warn("insert lengths fehlgeschlagen", "id", activity.ID, "error", err)
		}
	}
	if len(data.ClimbProEvents) > 0 {
		if err := db.InsertClimbProEvents(ctx, activity.ID, data.ClimbProEvents); err != nil {
			slog.Warn("insert climb pro fehlgeschlagen", "id", activity.ID, "error", err)
		}
	}

	// Sensoren
	if len(data.Sensors) > 0 {
		if err := db.InsertSensors(ctx, activity.ID, data.Sensors); err != nil {
			slog.Warn("insert sensors fehlgeschlagen", "id", activity.ID, "error", err)
		}
	}

	// Time-in-Zones
	if len(data.TimeInZoneData) > 0 {
		if err := db.SaveTimeInZones(ctx, activity.ID, nil, nil, data.TimeInZoneData); err != nil {
			slog.Warn("save time in zones fehlgeschlagen", "id", activity.ID, "error", err)
		}
	}

	// Heatmap (skip for heatmap-excluded activities)
	if len(data.TrackPoints) >= 2 && !activity.ExcludeFromHeatmap {
		var subTypePtr *string
		if data.SubType != "" {
			subTypePtr = &data.SubType
		}
		interval, _ := db.GetUserHeatmapInterval(ctx, meta.UserID)
		if err := db.InsertHeatmapData(ctx, meta.UserID, activity.ID, data.SportType, subTypePtr, data.StartedAt, interval); err != nil {
			slog.Warn("insert heatmap data fehlgeschlagen", "id", activity.ID, "error", err)
		}
	}

	return "OK", nil
}

func detectFormat(data []byte, filename string) string {
	return importutil.DetectFormat(data, filename)
}
