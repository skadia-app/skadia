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

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/importutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
)

var (
	// libraryBase is the root of the Unified ShadowFS library (data/library).
	// Each user has a sub-directory inside: data/library/{userDir}/activities/
	libraryBase      = flag.String("library", "", "Library-Basisverzeichnis (data/library)")
	dbURL            = flag.String("db-url", os.Getenv("DATABASE_URL"), "PostgreSQL Connection String")
	dryRun           = flag.Bool("dry-run", true, "Nur prüfen, nicht importieren (Standard: true)")
	conflictStrategy = flag.String("conflict", "skip", "Duplikat-Strategie: skip|replace|enrich_missing")
	// userDir may be specified to restrict the scan to one user's directory.
	userDir = flag.String("user-dir", "", "Optionaler User-Verzeichnisname (z.B. 'alice'); leer = alle User scannen")
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

	if *libraryBase == "" {
		fmt.Fprintln(os.Stderr, "Usage: skadia-restore -library <data/library> [-user-dir <name>] [-dry-run=false] [-conflict skip|replace|enrich_missing]")
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
	// Collect all restorable entries first (scan phase).
	// Library layout: {libraryBase}/{userDir}/activities/{...}/{file}.fit
	// Sidecar:        {libraryBase}/{userDir}/activities/{...}/{file}.fit.meta
	type entry struct {
		meta            sidecar.MetaFile
		activityPath    string // absolute path to the activity file
		relPath         string // path relative to libraryBase (storage_path in user_files)
		userDirFromPath string // first component of relPath (used to resolve user_id from DB)
		fileSize        int64
		fileHash        string
		contentType     string
	}

	var entries []entry

	// Scan statistics for the summary line.
	var totalMeta, totalFiles, totalErrors int

	// Determine the scan root: either a specific user dir or all users.
	scanRoot := *libraryBase
	if *userDir != "" {
		scanRoot = filepath.Join(*libraryBase, *userDir, "activities")
	}

	slog.Info("scanning for .meta sidecars", "root", scanRoot, "dry_run", *dryRun)

	scanErr := filepath.Walk(scanRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".meta") {
			return nil
		}

		totalMeta++

		// Read and parse .meta sidecar.
		data, err := os.ReadFile(path)
		if err != nil {
			slog.Error("meta lesen fehlgeschlagen", "path", path, "error", err)
			totalErrors++
			return nil
		}

		var meta sidecar.MetaFile
		if err := json.Unmarshal(data, &meta); err != nil {
			slog.Error("meta parsen fehlgeschlagen", "path", path, "error", err)
			totalErrors++
			return nil
		}

		// Derive activity file path (strip .meta suffix).
		activityPath := strings.TrimSuffix(path, ".meta")

		// Check if the actual activity file exists — if not, note and skip.
		fi, statErr := os.Stat(activityPath)
		if os.IsNotExist(statErr) {
			slog.Warn("activity file fehlt für meta — übersprungen",
				"meta", path,
				"erwartet", activityPath,
			)
			return nil
		}
		if statErr != nil {
			slog.Error("stat activity file fehlgeschlagen", "path", activityPath, "error", statErr)
			totalErrors++
			return nil
		}

		totalFiles++

		// Compute path relative to libraryBase (this becomes storage_path).
		relPath, err := filepath.Rel(*libraryBase, activityPath)
		if err != nil {
			slog.Error("rel path berechnen fehlgeschlagen", "path", activityPath, "error", err)
			totalErrors++
			return nil
		}

		// Detect content type from filename (avoid reading whole file just for MIME).
		ct := shadowfs.DetectContentType(filepath.Base(activityPath), "", nil)

		// Extract the first path component of relPath to identify the user directory.
		// relPath is like "alice/activities/2024/01/2024-01-15.fit" — first component = "alice".
		userDirName := strings.SplitN(relPath, string(filepath.Separator), 2)[0]

		entries = append(entries, entry{
			meta:            meta,
			activityPath:    activityPath,
			relPath:         relPath,
			userDirFromPath: userDirName,
			fileSize:        fi.Size(),
			fileHash:        meta.FileHashSHA256,
			contentType:     ct,
		})
		return nil
	})

	if scanErr != nil {
		return fmt.Errorf("scan failed: %w", scanErr)
	}

	fmt.Printf("Scan complete: %d .meta files found, %d activity files present, %d parse errors\n",
		totalMeta, totalFiles, totalErrors)

	total := len(entries)
	slog.Info("scan abgeschlossen", "total", total)

	if total == 0 {
		fmt.Println("Keine wiederherstellbaren Aktivitäten gefunden.")
		return nil
	}

	if *dryRun {
		// Dry-run: nur auflisten, keine DB-Verbindung nötig.
		for i, e := range entries {
			filename := filepath.Base(e.activityPath)
			fmt.Printf("[%d/%d] %s ... (dry-run) user-dir=%s id=%s sport=%s format=%s size=%d\n",
				i+1, total,
				filename,
				e.userDirFromPath,
				e.meta.SkadiaID,
				e.meta.SportType,
				e.meta.FileFormat,
				e.fileSize,
			)
		}
		fmt.Printf("\nErgebnis (dry-run): %d Dateien gefunden — mit --dry-run=false importieren\n", total)
		return nil
	}

	// Live-run: DB-Verbindung aufbauen.
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

	// Cache userDir → user_id lookups to avoid one DB query per file.
	userIDCache := make(map[string]string)
	resolveUserID := func(dirName string) string {
		if id, ok := userIDCache[dirName]; ok {
			return id
		}
		var id string
		// Match by library_dir_name first, fall back to UUID equality.
		err := db.Pool.QueryRow(ctx,
			`SELECT id FROM users
			 WHERE library_dir_name = $1
			    OR (library_dir_name IS NULL AND id::text = $1)
			 LIMIT 1`,
			dirName,
		).Scan(&id)
		if err != nil {
			slog.Warn("user_id für userDir nicht gefunden", "dir", dirName, "error", err)
			return ""
		}
		userIDCache[dirName] = id
		return id
	}

	var res result

	for i, e := range entries {
		filename := filepath.Base(e.activityPath)
		prefix := fmt.Sprintf("[%d/%d] %s", i+1, total, filename)

		userID := resolveUserID(e.userDirFromPath)
		if userID == "" {
			fmt.Printf("%s ... FEHLER: user_id für dir %q nicht gefunden\n", prefix, e.userDirFromPath)
			res.errors++
			continue
		}

		status, importErr := restoreOne(ctx, db, userID, e.meta, e.activityPath, e.relPath, e.fileSize, e.fileHash, e.contentType, strategy)
		if importErr != nil {
			fmt.Printf("%s ... FEHLER: %v\n", prefix, importErr)
			res.errors++
		} else {
			fmt.Printf("%s ... %s\n", prefix, status)
			switch status {
			case "OK":
				res.ok++
			default:
				// "SKIP (duplicate)", "SKIP (registry)", etc.
				res.skipped++
			}
		}
	}

	fmt.Printf("\nErgebnis: %d OK, %d Skip, %d Fehler\n", res.ok, res.skipped, res.errors)
	return nil
}

// restoreOne restores a single activity:
//  1. Parses the activity file and checks for duplicates.
//  2. Inserts or updates the activity in the DB.
//  3. Creates a user_files registry entry for the file (ON CONFLICT skip).
//
// Returns a status string ("OK", "SKIP (duplicate)", …) or an error.
func restoreOne(
	ctx context.Context,
	db *database.DB,
	userID string,
	meta sidecar.MetaFile,
	activityPath string,
	relPath string,
	fileSize int64,
	fileHash string,
	contentType string,
	strategy importer.ConflictStrategy,
) (string, error) {
	fileBytes, err := os.ReadFile(activityPath)
	if err != nil {
		return "", fmt.Errorf("activity file lesen: %w", err)
	}

	format := importutil.DetectFormat(fileBytes, activityPath)
	if format == "" {
		return "", fmt.Errorf("unbekanntes Dateiformat: %s", activityPath)
	}

	// Datei parsen.
	var actData *model.ActivityData
	switch format {
	case "fit":
		actData, err = fitimport.ParseFIT(bytes.NewReader(fileBytes), filepath.Base(activityPath))
	case "gpx":
		actData, err = gpximport.ParseGPX(bytes.NewReader(fileBytes), filepath.Base(activityPath))
	case "tcx":
		actData, err = tcximport.ParseTCX(bytes.NewReader(fileBytes), filepath.Base(activityPath))
	}
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", format, err)
	}

	// Duplikat-Check (Tier-1).
	match, err := importer.CheckTier1(ctx, db, userID, fileBytes, actData, 0, 0, 0)
	if err != nil {
		slog.Warn("duplikat-check fehlgeschlagen, importiere trotzdem", "error", err)
	}

	var activityID string

	if match != nil {
		switch strategy {
		case importer.StrategySkip:
			return fmt.Sprintf("SKIP (duplicate, id=%s, confidence=%s)", match.ExistingActivityID, match.Confidence), nil

		case importer.StrategyReplaceAll:
			if replaceErr := importer.ReplaceActivity(ctx, db, match.ExistingActivityID, actData); replaceErr != nil {
				return "", fmt.Errorf("replace activity %s: %w", match.ExistingActivityID, replaceErr)
			}
			activityID = match.ExistingActivityID

		case importer.StrategyEnrichMissing:
			existing, getErr := db.GetActivity(ctx, match.ExistingActivityID)
			if getErr != nil {
				slog.Warn("existing activity laden fehlgeschlagen", "id", match.ExistingActivityID, "error", getErr)
			}
			n, enrichErr := importer.EnrichActivity(ctx, db, match.ExistingActivityID, existing, actData)
			if enrichErr != nil {
				return "", fmt.Errorf("enrich activity %s: %w", match.ExistingActivityID, enrichErr)
			}
			activityID = match.ExistingActivityID
			_ = n
		}
	}

	// subErr tracks whether any non-fatal sub-insert failed.
	var subErr error

	if activityID == "" {
		// Neue Aktivität + alle Sub-Daten in einer Transaktion einfügen.
		var activity *model.Activity
		txErr := pgx.BeginFunc(ctx, db.Pool, func(tx pgx.Tx) error {
			var insertErr error
			activity, insertErr = db.InsertActivity(ctx, userID, actData)
			if insertErr != nil {
				return fmt.Errorf("insert activity: %w", insertErr)
			}
			activityID = activity.ID

			// Streams einfügen.
			if len(actData.Streams) > 0 {
				if err := db.InsertStreams(ctx, activityID, actData.Streams); err != nil {
					slog.Warn("insert streams fehlgeschlagen", "id", activityID, "error", err)
					subErr = err
				}
			}

			// Track einfügen.
			if len(actData.TrackPoints) > 0 {
				if err := db.InsertTrack(ctx, activityID, actData.TrackPoints); err != nil {
					slog.Warn("insert track fehlgeschlagen", "id", activityID, "error", err)
					subErr = err
				}
			}

			// R-R-Intervalle (FIT only).
			if len(actData.RRIntervals) > 0 {
				if err := db.InsertRRIntervals(ctx, activityID, actData.RRIntervals); err != nil {
					slog.Warn("insert rr intervals fehlgeschlagen", "id", activityID, "error", err)
					subErr = err
				}
			}

			// Laps, Splits, Lengths, ClimbPro.
			if len(actData.Laps) > 0 {
				if err := db.InsertLaps(ctx, activityID, actData.Laps); err != nil {
					slog.Warn("insert laps fehlgeschlagen", "id", activityID, "error", err)
					subErr = err
				}
			}
			if len(actData.Splits) > 0 {
				if err := db.InsertSplits(ctx, activityID, actData.Splits); err != nil {
					slog.Warn("insert splits fehlgeschlagen", "id", activityID, "error", err)
					subErr = err
				}
			}
			if len(actData.SplitSummaries) > 0 {
				if err := db.InsertSplitSummaries(ctx, activityID, actData.SplitSummaries); err != nil {
					slog.Warn("insert split summaries fehlgeschlagen", "id", activityID, "error", err)
					subErr = err
				}
			}
			if len(actData.Lengths) > 0 {
				if err := db.InsertLengths(ctx, activityID, actData.Lengths); err != nil {
					slog.Warn("insert lengths fehlgeschlagen", "id", activityID, "error", err)
					subErr = err
				}
			}
			if len(actData.ClimbProEvents) > 0 {
				if err := db.InsertClimbProEvents(ctx, activityID, actData.ClimbProEvents); err != nil {
					slog.Warn("insert climb pro fehlgeschlagen", "id", activityID, "error", err)
					subErr = err
				}
			}

			// Sensoren.
			if len(actData.Sensors) > 0 {
				if err := db.InsertSensors(ctx, activityID, actData.Sensors); err != nil {
					slog.Warn("insert sensors fehlgeschlagen", "id", activityID, "error", err)
					subErr = err
				}
			}

			// Time-in-Zones.
			if len(actData.TimeInZoneData) > 0 {
				if err := db.SaveTimeInZones(ctx, activityID, nil, nil, actData.TimeInZoneData); err != nil {
					slog.Warn("save time in zones fehlgeschlagen", "id", activityID, "error", err)
					subErr = err
				}
			}

			// Heatmap (skip for heatmap-excluded activities).
			if len(actData.TrackPoints) >= 2 && !activity.ExcludeFromHeatmap {
				var subTypePtr *string
				if actData.SubType != "" {
					subTypePtr = &actData.SubType
				}
				interval, _ := db.GetUserHeatmapInterval(ctx, userID)
				if err := db.InsertHeatmapData(ctx, userID, activityID, actData.SportType, subTypePtr, actData.StartedAt, interval); err != nil {
					slog.Warn("insert heatmap data fehlgeschlagen", "id", activityID, "error", err)
					subErr = err
				}
			}

			// Register file in user_files and link back to activity.file_id.
			// ON CONFLICT: if the row already exists (idempotent re-run), still attempt
			// to update activities.file_id so previously-missed links are repaired.
			var fileID string
			regErr := tx.QueryRow(ctx,
				`INSERT INTO user_files
					(user_id, file_type, storage_backend, storage_path, content_type,
					 file_name, size_bytes, file_hash, parent_type, parent_id, has_sidecar, status)
				VALUES ($1, 'activity', 'local', $2, $3, $4, $5, $6, 'activity', $7, true, 'active')
				ON CONFLICT (storage_backend, storage_path) DO UPDATE
					SET updated_at = user_files.updated_at
				RETURNING id`,
				userID,
				relPath,
				contentType,
				filepath.Base(activityPath),
				fileSize,
				fileHash,
				activityID,
			).Scan(&fileID)
			if regErr != nil {
				return fmt.Errorf("user_files registrieren: %w", regErr)
			}
			if fileID != "" {
				if _, err := tx.Exec(ctx,
					`UPDATE activities SET file_id = $1 WHERE id = $2 AND file_id IS NULL`,
					fileID, activityID,
				); err != nil {
					return fmt.Errorf("activities.file_id verknüpfen: %w", err)
				}
			}
			return nil
		})
		if txErr != nil {
			return "", txErr
		}
	} else {
		// Für replace/enrich: user_files + file_id-Link außerhalb einer neuen Transaktion registrieren.
		// (Die eigentliche Aktivitäts-Operation lief schon, hier nur noch Registry-Eintrag.)
		var fileID string
		regErr := db.Pool.QueryRow(ctx,
			`INSERT INTO user_files
				(user_id, file_type, storage_backend, storage_path, content_type,
				 file_name, size_bytes, file_hash, parent_type, parent_id, has_sidecar, status)
			VALUES ($1, 'activity', 'local', $2, $3, $4, $5, $6, 'activity', $7, true, 'active')
			ON CONFLICT (storage_backend, storage_path) DO UPDATE
				SET updated_at = user_files.updated_at
			RETURNING id`,
			userID,
			relPath,
			contentType,
			filepath.Base(activityPath),
			fileSize,
			fileHash,
			activityID,
		).Scan(&fileID)
		if regErr != nil {
			slog.Warn("user_files Eintrag fehlgeschlagen", "path", relPath, "error", regErr)
		} else if fileID != "" {
			if _, err := db.Pool.Exec(ctx,
				`UPDATE activities SET file_id = $1 WHERE id = $2 AND file_id IS NULL`,
				fileID, activityID,
			); err != nil {
				slog.Warn("activities.file_id verknüpfen fehlgeschlagen",
					"activity_id", activityID, "file_id", fileID, "error", err)
			}
		}
	}

	if subErr != nil {
		return "PARTIAL", nil
	}
	return "OK", nil
}
