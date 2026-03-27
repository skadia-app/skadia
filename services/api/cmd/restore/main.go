package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/importutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
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

	backendMode = flag.String("backend", "local", "Storage-Backend: local|s3")

	// S3-specific flags (only used when -backend s3).
	s3Endpoint       = flag.String("s3-endpoint", os.Getenv("S3_ENDPOINT"), "S3 endpoint URL")
	s3AccessKey      = flag.String("s3-access-key", os.Getenv("S3_ACCESS_KEY"), "S3 access key")
	s3SecretKey      = flag.String("s3-secret-key", os.Getenv("S3_SECRET_KEY"), "S3 secret key")
	s3Bucket         = flag.String("s3-bucket", os.Getenv("S3_BUCKET"), "S3 bucket name")
	s3Region         = flag.String("s3-region", os.Getenv("S3_REGION"), "S3 region")
	s3ForcePathStyle = flag.Bool("s3-force-path-style", os.Getenv("S3_FORCE_PATH_STYLE") == "true", "Use path-style S3 URLs")
)

type result struct {
	ok      int
	skipped int
	errors  int
}

// entry holds all information needed to restore one activity file.
type entry struct {
	meta            sidecar.MetaFile
	activityKey     string // S3 key or absolute local path to the activity file
	relPath         string // path relative to libraryBase / S3 key (storage_path in user_files)
	userDirFromPath string // first component of relPath (used to resolve user_id from DB)
	fileSize        int64
	fileHash        string
	contentType     string
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	flag.Parse()

	switch *backendMode {
	case "local":
		if *libraryBase == "" {
			fmt.Fprintln(os.Stderr, "Usage: skadia-restore -library <data/library> [-user-dir <name>] [-dry-run=false] [-conflict skip|replace|enrich_missing]")
			os.Exit(1)
		}
	case "s3":
		if *s3Bucket == "" {
			fmt.Fprintln(os.Stderr, "Fehler: -s3-bucket (oder $S3_BUCKET) muss gesetzt sein wenn --backend s3")
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Fehler: unbekanntes -backend %q (local|s3)\n", *backendMode)
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
	var entries []entry
	var scanErr error
	var s3Client *s3.Client

	switch *backendMode {
	case "local":
		entries, scanErr = scanLocal(ctx)
	case "s3":
		s3Client = s3.New(s3.Options{
			BaseEndpoint: aws.String(*s3Endpoint),
			Region:       *s3Region,
			Credentials:  credentials.NewStaticCredentialsProvider(*s3AccessKey, *s3SecretKey, ""),
			UsePathStyle: *s3ForcePathStyle,
		})
		entries, scanErr = scanS3(ctx, s3Client)
	default:
		return fmt.Errorf("unknown backend: %s", *backendMode)
	}

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
		// Dry-run: nur auflisten, keine DB-Verbindung nötig.
		for i, e := range entries {
			filename := filepath.Base(e.activityKey)
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
	resolveUserID := func(dirOrID string, metaUserID string) string {
		// For S3 mode the UserID is in the MetaFile directly — prefer it.
		if metaUserID != "" {
			return metaUserID
		}
		if id, ok := userIDCache[dirOrID]; ok {
			return id
		}
		var id string
		var resolvedDirName *string
		// M28: Cross-validate: match by library_dir_name first, fall back to UUID equality.
		// Also fetch library_dir_name to verify the directory actually belongs to the matched user.
		err := db.Pool.QueryRow(ctx,
			`SELECT id, library_dir_name FROM users
			 WHERE library_dir_name = $1
			    OR (library_dir_name IS NULL AND id::text = $1)
			 LIMIT 1`,
			dirOrID,
		).Scan(&id, &resolvedDirName)
		if err != nil {
			slog.Warn("user_id für userDir nicht gefunden", "dir", dirOrID, "error", err)
			return ""
		}
		// Cross-validate: the directory name must match either the user's library_dir_name or their UUID.
		expectedDir := id
		if resolvedDirName != nil && *resolvedDirName != "" {
			expectedDir = *resolvedDirName
		}
		if dirOrID != expectedDir {
			slog.Warn("user directory mismatch — possible UUID spoofing",
				"dir", dirOrID, "user_id", id, "expected_dir", expectedDir)
			return ""
		}
		userIDCache[dirOrID] = id
		return id
	}

	var res result

	for i, e := range entries {
		filename := filepath.Base(e.activityKey)
		prefix := fmt.Sprintf("[%d/%d] %s", i+1, total, filename)

		userID := resolveUserID(e.userDirFromPath, e.meta.UserID)
		if userID == "" {
			fmt.Printf("%s ... FEHLER: user_id für dir %q nicht gefunden\n", prefix, e.userDirFromPath)
			res.errors++
			continue
		}

		status, importErr := restoreOne(ctx, db, userID, e.meta, e.activityKey, e.relPath, e.fileSize, e.fileHash, e.contentType, strategy, s3Client)
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

// scanLocal scans the local library directory for .meta sidecars.
func scanLocal(_ context.Context) ([]entry, error) {
	// Scan statistics for the summary line.
	var totalMeta, totalFiles, totalErrors int

	// Determine the scan root: either a specific user dir or all users.
	scanRoot := *libraryBase
	if *userDir != "" {
		scanRoot = filepath.Join(*libraryBase, *userDir, "activities")
	}

	slog.Info("scanning for .meta sidecars", "root", scanRoot, "dry_run", *dryRun)

	var entries []entry

	scanErr := filepath.Walk(scanRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// L36: Validate symlinks don't escape the scan root.
		realPath, evalErr := filepath.EvalSymlinks(path)
		if evalErr != nil {
			slog.Warn("symlink eval failed, skipping", "path", path, "error", evalErr)
			return nil
		}
		if !strings.HasPrefix(realPath, filepath.Clean(scanRoot)) {
			slog.Warn("symlink escapes scan root, skipping", "path", path, "real", realPath)
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

		// v1/v2 compatibility warning.
		if meta.SchemaVersion < 3 {
			slog.Warn("restore: legacy meta version — user-editable fields may be incomplete",
				"path", path, "version", meta.SchemaVersion)
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
			activityKey:     activityPath,
			relPath:         relPath,
			userDirFromPath: userDirName,
			fileSize:        fi.Size(),
			fileHash:        meta.FileHashSHA256,
			contentType:     ct,
		})
		return nil
	})

	if scanErr != nil {
		return nil, scanErr
	}

	fmt.Printf("Scan complete: %d .meta files found, %d activity files present, %d parse errors\n",
		totalMeta, totalFiles, totalErrors)

	return entries, nil
}

// scanS3 scans an S3 bucket for .meta sidecars. Activity files are NOT pre-loaded;
// their existence is verified via HeadObject, and bytes are fetched on-demand during import.
func scanS3(ctx context.Context, client *s3.Client) ([]entry, error) {
	bucket := *s3Bucket

	// Determine S3 prefix: optionally restrict to one user.
	prefix := ""
	if *userDir != "" {
		prefix = *userDir + "/"
	}

	slog.Info("scanning S3 for .meta sidecars", "bucket", bucket, "prefix", prefix, "dry_run", *dryRun)

	var totalMeta, totalFiles, totalErrors int
	var entries []entry

	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}

		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			if !strings.HasSuffix(key, ".meta") {
				continue
			}

			totalMeta++

			// Fetch and parse the .meta sidecar.
			metaOut, err := client.GetObject(ctx, &s3.GetObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(key),
			})
			if err != nil {
				slog.Error("meta lesen fehlgeschlagen (S3)", "key", key, "error", err)
				totalErrors++
				continue
			}
			metaData, err := io.ReadAll(metaOut.Body)
			metaOut.Body.Close()
			if err != nil {
				slog.Error("meta lesen fehlgeschlagen (S3 body)", "key", key, "error", err)
				totalErrors++
				continue
			}

			var meta sidecar.MetaFile
			if err := json.Unmarshal(metaData, &meta); err != nil {
				slog.Error("meta parsen fehlgeschlagen (S3)", "key", key, "error", err)
				totalErrors++
				continue
			}

			// v1/v2 compatibility warning.
			if meta.SchemaVersion < 3 {
				slog.Warn("restore: legacy meta version — user-editable fields may be incomplete",
					"path", key, "version", meta.SchemaVersion)
			}

			// Derive activity S3 key (strip .meta suffix).
			activityKey := strings.TrimSuffix(key, ".meta")

			// Check whether the activity file exists and fetch its size via HeadObject
			// (avoids loading the entire file into memory during scan).
			headOut, err := client.HeadObject(ctx, &s3.HeadObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(activityKey),
			})
			if err != nil {
				slog.Warn("activity file fehlt für meta (S3) — übersprungen",
					"meta", key, "erwartet", activityKey, "error", err)
				continue
			}

			totalFiles++

			var fileSize int64
			if headOut.ContentLength != nil {
				fileSize = *headOut.ContentLength
			}

			// Detect content type from the key basename.
			ct := shadowfs.DetectContentType(filepath.Base(activityKey), "", nil)

			// Extract the first path component (user directory / UUID).
			// Key format: {user_id}/activities/... — first component = user dir.
			userDirName := strings.SplitN(activityKey, "/", 2)[0]

			entries = append(entries, entry{
				meta:            meta,
				activityKey:     activityKey,
				relPath:         activityKey, // S3 key IS the storage_path
				userDirFromPath: userDirName,
				fileSize:        fileSize,
				fileHash:        meta.FileHashSHA256,
				contentType:     ct,
			})
		}
	}

	fmt.Printf("Scan complete (S3): %d .meta files found, %d activity files present, %d parse errors\n",
		totalMeta, totalFiles, totalErrors)

	return entries, nil
}

// restoreOne restores a single activity:
//  1. Parses the activity file and checks for duplicates.
//  2. Inserts or updates the activity in the DB.
//  3. Creates a user_files registry entry for the file (ON CONFLICT skip).
//
// Returns a status string ("OK", "SKIP (duplicate)", …) or an error.
// s3Client is non-nil in S3 mode; the activity file bytes are fetched on-demand.
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
	s3Client *s3.Client,
) (string, error) {
	var fileBytes []byte
	if s3Client != nil {
		// S3 mode: fetch on-demand (streaming, no pre-load during scan).
		actOut, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(*s3Bucket),
			Key:    aws.String(activityPath),
		})
		if err != nil {
			return "", fmt.Errorf("s3 GetObject %s: %w", activityPath, err)
		}
		fileBytes, err = io.ReadAll(actOut.Body)
		actOut.Body.Close()
		if err != nil {
			return "", fmt.Errorf("s3 body lesen %s: %w", activityPath, err)
		}
	} else {
		var err error
		fileBytes, err = os.ReadFile(activityPath)
		if err != nil {
			return "", fmt.Errorf("activity file lesen: %w", err)
		}
	}

	format := importutil.DetectFormat(fileBytes, activityPath)
	if format == "" {
		return "", fmt.Errorf("unbekanntes Dateiformat: %s", activityPath)
	}

	// Datei parsen.
	var actData *model.ActivityData
	var err error
	switch format {
	case "fit":
		actData, err = fitimport.ParseFIT(bytes.NewReader(fileBytes), filepath.Base(activityPath))
	case "gpx":
		actData, err = gpximport.ParseGPX(bytes.NewReader(fileBytes), filepath.Base(activityPath))
	case "tcx":
		actData, err = tcximport.ParseTCX(bytes.NewReader(fileBytes), filepath.Base(activityPath))
	default:
		// L53: Explicit default case prevents nil actData panic for unsupported formats.
		return "", fmt.Errorf("unsupported format: %s", format)
	}
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", format, err)
	}

	// Duplikat-Check (Tier-1).
	match, err := importer.CheckTier1(ctx, db, userID, fileBytes, actData, 0, 0, 0)
	if err != nil {
		slog.Warn("duplikat-check fehlgeschlagen, importiere trotzdem", "error", err)
		// L52: Treat match as nil when CheckTier1 returns error — don't act on partial results.
		match = nil
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

	// Determine storage_backend value from current mode.
	storageBackend := *backendMode // "local" or "s3"

	// subErr tracks whether any non-fatal sub-insert failed.
	var subErr error

	if activityID == "" {
		// Neue Aktivität: Kern-Einfügung (activity + user_files + file_id) in einer Transaktion.
		// Sub-Daten (streams, track, etc.) werden danach best-effort eingefügt —
		// identisch zum normalen Importer-Pattern (service.go PostProcess).
		var activity *model.Activity
		txErr := pgx.BeginFunc(ctx, db.Pool, func(tx pgx.Tx) error {
			// InsertActivity direkt über TX ausführen um die Aktivitäts-Einfügung
			// atomar mit der user_files-Registrierung zu machen.
			var insertErr error
			activity, insertErr = db.InsertActivityTx(ctx, tx, userID, actData)
			if insertErr != nil {
				return fmt.Errorf("insert activity: %w", insertErr)
			}
			activityID = activity.ID

			// Register file in user_files and link back to activity.file_id.
			// ON CONFLICT: if the row already exists (idempotent re-run), still attempt
			// to update activities.file_id so previously-missed links are repaired.
			var fileID string
			regErr := tx.QueryRow(ctx,
				`INSERT INTO user_files
					(user_id, file_type, storage_backend, storage_path, content_type,
					 file_name, size_bytes, file_hash, parent_type, parent_id, has_sidecar, status)
				VALUES ($1, 'activity', $2, $3, $4, $5, $6, $7, 'activity', $8, true, 'active')
				ON CONFLICT (storage_backend, storage_path) DO UPDATE
					SET updated_at = user_files.updated_at
				RETURNING id`,
				userID,
				storageBackend,
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

		// Sub-Daten best-effort außerhalb TX (identisch zum normalen Importer-Pattern).

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
	} else {
		// Für replace/enrich: user_files + file_id-Link in einer kleinen Transaktion registrieren.
		txErr := pgx.BeginFunc(ctx, db.Pool, func(tx pgx.Tx) error {
			var fileID string
			regErr := tx.QueryRow(ctx,
				`INSERT INTO user_files
					(user_id, file_type, storage_backend, storage_path, content_type,
					 file_name, size_bytes, file_hash, parent_type, parent_id, has_sidecar, status)
				VALUES ($1, 'activity', $2, $3, $4, $5, $6, $7, 'activity', $8, true, 'active')
				ON CONFLICT (storage_backend, storage_path) DO UPDATE
					SET updated_at = user_files.updated_at
				RETURNING id`,
				userID,
				storageBackend,
				relPath,
				contentType,
				filepath.Base(activityPath),
				fileSize,
				fileHash,
				activityID,
			).Scan(&fileID)
			if regErr != nil {
				slog.Warn("user_files Eintrag fehlgeschlagen", "path", relPath, "error", regErr)
				return nil // nicht fatal
			}
			if fileID != "" {
				if _, err := tx.Exec(ctx,
					`UPDATE activities SET file_id = $1 WHERE id = $2 AND file_id IS NULL`,
					fileID, activityID,
				); err != nil {
					slog.Warn("activities.file_id verknüpfen fehlgeschlagen",
						"activity_id", activityID, "file_id", fileID, "error", err)
				}
			}
			return nil
		})
		if txErr != nil {
			slog.Warn("user_files Transaktion fehlgeschlagen", "path", relPath, "error", txErr)
		}
	}

	if subErr != nil {
		return "PARTIAL", nil
	}
	return "OK", nil
}
