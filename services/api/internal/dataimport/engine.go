package dataimport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

// EngineConfig holds configuration for the import engine.
type EngineConfig struct {
	StorageBackend  string
	LibraryBasePath string
	MaxStorageBytes int64
}

// Engine orchestrates the GDPR data import.
type Engine struct {
	DB          *pgxpool.Pool
	ShadowFS    shadowfs.ShadowFS
	Config      EngineConfig
	deferredFKs []deferredFK
	mu          sync.Mutex
}

type deferredFK struct {
	table    string
	rowID    string
	column   string
	oldValue string
}

// ImportInput describes a single import run.
type ImportInput struct {
	Source       Source
	TargetUserID string
	OnProgress   func(ProgressEvent)
}

// ValidationResult is the outcome of ValidateAccount.
type ValidationResult struct {
	CanImport bool
	Warnings  []string
}

// ---------------------------------------------------------------------------
// ValidateAccount
// ---------------------------------------------------------------------------

// ValidateAccount checks whether TargetUserID already owns activities or files
// that would conflict with a fresh import.
// Auto-generated defaults (tags, gear, zones, settings) are not checked here.
// Returns CanImport=false when activities or user_files are present.
func (e *Engine) ValidateAccount(ctx context.Context, userID string) (*ValidationResult, error) {
	var actCount, fileCount int
	if err := e.DB.QueryRow(ctx,
		"SELECT COUNT(*) FROM activities WHERE user_id = $1", userID,
	).Scan(&actCount); err != nil {
		return nil, fmt.Errorf("ValidateAccount: count activities: %w", err)
	}
	if err := e.DB.QueryRow(ctx,
		"SELECT COUNT(*) FROM user_files WHERE user_id = $1", userID,
	).Scan(&fileCount); err != nil {
		return nil, fmt.Errorf("ValidateAccount: count user_files: %w", err)
	}

	res := &ValidationResult{CanImport: true}
	if actCount > 0 || fileCount > 0 {
		res.CanImport = false
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"account has %d activities and %d files; import requires an account without activities or uploaded files — create a new account or delete existing data first",
			actCount, fileCount))
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// RunImport
// ---------------------------------------------------------------------------

// RunImport executes a full GDPR import from input.Source into TargetUserID.
func (e *Engine) RunImport(ctx context.Context, input ImportInput) (*ImportResult, error) {
	start := time.Now()
	result := &ImportResult{UserID: input.TargetUserID}

	// Reset deferred FKs so repeated calls to RunImport don't accumulate state
	// from a previous run.
	e.mu.Lock()
	e.deferredFKs = nil
	e.mu.Unlock()

	// ── 1. Read + validate manifest ─────────────────────────────────────────
	manifest, err := input.Source.Manifest()
	if err != nil {
		return nil, fmt.Errorf("RunImport: read manifest: %w", err)
	}
	if manifest.FormatVersion != 1 {
		return nil, fmt.Errorf("RunImport: unsupported format_version %d (expected 1)", manifest.FormatVersion)
	}

	// ── 2. Schema-migration check ────────────────────────────────────────────
	var currentMigration int
	if err := e.DB.QueryRow(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`,
	).Scan(&currentMigration); err != nil {
		return nil, fmt.Errorf("RunImport: read schema_migrations: %w", err)
	}
	if manifest.SchemaMigration > currentMigration {
		return nil, fmt.Errorf(
			"RunImport: export requires schema migration %d but current is %d — upgrade the server first",
			manifest.SchemaMigration, currentMigration,
		)
	}

	// ── 3. Warn about incomplete tables ──────────────────────────────────────
	if len(manifest.IncompleteTables) > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("export has incomplete tables: %s", strings.Join(manifest.IncompleteTables, ", ")))
	}

	// ── 4. Mutual exclusion: no concurrent export or import for this user ────
	// Known race window — acceptable for CLI usage; the API handler performs an
	// atomic INSERT-based lock before calling RunImport.
	var conflictCount int
	if err := e.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM background_jobs
		WHERE user_id = $1
		  AND job_type IN ('data_export', 'data_import')
		  AND status IN ('queued', 'running')
	`, input.TargetUserID).Scan(&conflictCount); err != nil {
		return nil, fmt.Errorf("RunImport: conflict check: %w", err)
	}
	if conflictCount > 0 {
		return nil, fmt.Errorf("RunImport: a data export or import is already running for this user")
	}

	// ── 5. ValidateAccount — hard gate: reject non-empty accounts ───────────
	validation, err := e.ValidateAccount(ctx, input.TargetUserID)
	if err != nil {
		return nil, fmt.Errorf("RunImport: ValidateAccount: %w", err)
	}
	if !validation.CanImport {
		return nil, fmt.Errorf("RunImport: %s", validation.Warnings[0])
	}

	// ── 6. IDMap + schema introspection ──────────────────────────────────────
	idMap := NewIDMap(manifest.UserID, input.TargetUserID)

	schemaMap, err := introspectAllTables(ctx, e.DB)
	if err != nil {
		return nil, fmt.Errorf("RunImport: introspectAllTables: %w", err)
	}

	// ── 7. Build ordered table list ───────────────────────────────────────────
	tables := allTables()
	tables = append(tables, healthTables()...)

	// ── 8. Run phases ────────────────────────────────────────────────────────
	byPhase := groupByPhase(tables)

	// Phase 1: profile
	if err := e.runPhase1(ctx, input, manifest); err != nil {
		return nil, fmt.Errorf("RunImport phase 1: %w", err)
	}
	result.PhasesCompleted++

	// Phases 2, 3, 4 — single TX each
	for _, phase := range []int{2, 3, 4} {
		specs := byPhase[phase]
		if len(specs) == 0 {
			result.PhasesCompleted++
			continue
		}
		if err := e.runSingleTXPhase(ctx, phase, specs, input, idMap, schemaMap, manifest.SchemaMigration, result); err != nil {
			return nil, fmt.Errorf("RunImport phase %d: %w", phase, err)
		}
		result.PhasesCompleted++
	}

	// Disk-space check before Phase 5
	if e.Config.LibraryBasePath != "" {
		totalFileBytes := manifest.TotalFileSizeBytes()
		if totalFileBytes > 0 {
			var stat syscall.Statfs_t
			if err := syscall.Statfs(e.Config.LibraryBasePath, &stat); err == nil {
				available := int64(stat.Bavail) * int64(stat.Bsize)
				if totalFileBytes > available {
					return nil, fmt.Errorf(
						"RunImport: insufficient disk space — need %d bytes, have %d bytes available in %s",
						totalFileBytes, available, e.Config.LibraryBasePath,
					)
				}
			}
		}
	}

	// Phase 5: per-activity TX + remaining files
	activitySpec := findSpec("activities", tables)
	fileAttachSpec := findSpec("file_attachments", tables)
	activityCount, err := e.runPhase5(ctx, input, idMap, schemaMap, activitySpec, fileAttachSpec, manifest.SchemaMigration, result)
	if err != nil {
		return nil, fmt.Errorf("RunImport phase 5: %w", err)
	}
	result.PhasesCompleted++

	// Phase 5b: deferred FK resolution
	if err := e.runPhase5b(ctx, idMap); err != nil {
		return nil, fmt.Errorf("RunImport phase 5b: %w", err)
	}

	// Phases 6, 7 — single TX each
	for _, phase := range []int{6, 7} {
		specs := byPhase[phase]
		if len(specs) == 0 {
			result.PhasesCompleted++
			continue
		}
		if err := e.runSingleTXPhase(ctx, phase, specs, input, idMap, schemaMap, manifest.SchemaMigration, result); err != nil {
			return nil, fmt.Errorf("RunImport phase %d: %w", phase, err)
		}
		result.PhasesCompleted++
	}

	// Phase 8: enrichment jobs + warnings
	if err := triggerEnrichment(ctx, e.DB, input.TargetUserID, activityCount); err != nil {
		// Non-fatal: log as warning but don't abort.
		result.Warnings = append(result.Warnings, fmt.Sprintf("triggerEnrichment: %v", err))
	}

	// ── 9. Write import_log entry ────────────────────────────────────────────
	if err := e.writeImportLog(ctx, input.TargetUserID, result, manifest, start); err != nil {
		// Non-fatal.
		result.Warnings = append(result.Warnings, fmt.Sprintf("import_log write: %v", err))
	}

	// ── 10. Result ────────────────────────────────────────────────────────────
	result.Duration = time.Since(start)
	return result, nil
}

// ---------------------------------------------------------------------------
// Phase helpers
// ---------------------------------------------------------------------------

// runPhase1 reads data/profile.json and applies allow-listed fields to users.
func (e *Engine) runPhase1(ctx context.Context, input ImportInput, manifest *Manifest) error {
	rc, err := input.Source.OpenData("data/profile.json")
	if err != nil {
		// Profile file is optional.
		return nil
	}
	defer rc.Close()

	raw, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("read profile.json: %w", err)
	}

	var profile map[string]any
	if err := json.Unmarshal(raw, &profile); err != nil {
		return fmt.Errorf("decode profile.json: %w", err)
	}

	// Build SET clause from allow-listed fields that are present in the export.
	setClauses := make([]string, 0, len(profileAllowList))
	args := make([]any, 0, len(profileAllowList)+2)
	args = append(args, input.TargetUserID) // $1

	n := 2
	for _, col := range profileAllowList {
		val, ok := profile[col]
		if !ok {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", pgx.Identifier{col}.Sanitize(), n))
		args = append(args, val)
		n++
	}

	if len(setClauses) == 0 {
		return nil
	}

	// Also reset deleted_at so soft-deleted accounts are restored.
	setClauses = append(setClauses, "deleted_at = NULL")

	query := fmt.Sprintf(
		"UPDATE users SET %s WHERE id = $1",
		strings.Join(setClauses, ", "),
	)

	if _, err := e.DB.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("update users profile: %w", err)
	}

	return nil
}

// runSingleTXPhase imports all tables for phase within a single DB transaction.
func (e *Engine) runSingleTXPhase(
	ctx context.Context,
	phase int,
	specs []tableSpec,
	input ImportInput,
	idMap *IDMap,
	schemaMap map[string][]targetColumn,
	exportMigration int,
	result *ImportResult,
) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("phase %d begin tx: %w", phase, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, spec := range specs {
		if spec.Skip {
			continue
		}
		if input.OnProgress != nil {
			input.OnProgress(ProgressEvent{
				Phase:            phase,
				Table:            spec.Name,
				Message:          fmt.Sprintf("Importing %s...", spec.Name),
				EntitiesImported: result.EntitiesImported,
				EntitiesSkipped:  result.EntitiesSkipped,
				EntitiesFailed:   result.EntitiesFailed,
				FilesImported:    result.FilesImported,
				BytesImported:    result.BytesImported,
			})
		}
		targetCols := schemaMap[spec.Name]
		if err := e.importTable(ctx, tx, spec, input.Source, idMap, input.TargetUserID, targetCols, exportMigration, result, input.OnProgress); err != nil {
			return fmt.Errorf("phase %d table %s: %w", phase, spec.Name, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("phase %d commit: %w", phase, err)
	}
	return nil
}

// runPhase5 handles the per-activity-TX import of activities and their files.
// Returns the count of activity rows imported.
func (e *Engine) runPhase5(
	ctx context.Context,
	input ImportInput,
	idMap *IDMap,
	schemaMap map[string][]targetColumn,
	activitySpec tableSpec,
	fileAttachSpec tableSpec,
	exportMigration int,
	result *ImportResult,
) (int, error) {
	// Load file index once.
	fileIndex, err := loadUserFilesIndex(input.Source)
	if err != nil {
		return 0, fmt.Errorf("loadUserFilesIndex: %w", err)
	}

	// Read activities JSON.
	rc, err := input.Source.OpenData("data/activities.json")
	if err != nil {
		// No activities — import remaining files then return.
		if err2 := e.importRemainingFiles(ctx, input, idMap, fileIndex, result); err2 != nil {
			return 0, fmt.Errorf("importRemainingFiles (no activities): %w", err2)
		}
		return 0, nil
	}
	defer rc.Close()

	rawActivities, err := io.ReadAll(rc)
	if err != nil {
		return 0, fmt.Errorf("read activities.json: %w", err)
	}

	var activityRows []map[string]any
	if err := json.Unmarshal(rawActivities, &activityRows); err != nil {
		return 0, fmt.Errorf("decode activities.json: %w", err)
	}

	// Determine export columns for activities.
	exportCols := exportColumnsForTable(input.Source, "activities")
	if len(exportCols) == 0 {
		exportCols = extractKeysFromFirstRow(activityRows)
	}
	targetCols := schemaMap[activitySpec.Name]
	insertCols, geometryCols := computeInsertColumns(exportCols, targetCols)

	activityCount := 0
	activitiesWithoutFile := 0

	for i, rawRow := range activityRows {
		// Apply transformers before anything else.
		rawRow = applyTransformers(exportMigration, "activities", rawRow)

		// Import file_id and sidecar_file_id BEFORE the activity row.
		// Track whether this activity has a primary original file.
		fileID, hasFileID := stringVal(rawRow["file_id"])
		hasOriginalFile := hasFileID && fileID != ""

		for _, fkCol := range []string{"file_id", "sidecar_file_id"} {
			if oldID, ok := stringVal(rawRow[fkCol]); ok && oldID != "" {
				if _, alreadyMapped := idMap.GetIfExists(oldID); !alreadyMapped {
					if err := e.importFileByID(ctx, input.Source, idMap, input.TargetUserID, oldID, fileIndex, result); err != nil {
						return activityCount, fmt.Errorf("activity row %d: importFileByID %s=%s: %w", i, fkCol, oldID, err)
					}
				}
			}
		}

		if !hasOriginalFile {
			activitiesWithoutFile++
		}

		// Capture old PK before prepareRow remaps it.
		oldID := extractOldID(rawRow, activitySpec)

		// ── Activity dedup ────────────────────────────────────────────────────
		// For activities with a file: deduplicate via file_hash.
		if hasFileID && fileID != "" {
			if rec, ok := fileIndex[fileID]; ok && rec.FileHash != nil && *rec.FileHash != "" {
				var existingFileID, existingActivityID string
				dedupErr := e.DB.QueryRow(ctx,
					`SELECT uf.id, COALESCE(a.id, '') FROM user_files uf
					 LEFT JOIN activities a ON a.file_id = uf.id
					 WHERE uf.user_id = $1 AND uf.file_hash = $2
					 LIMIT 1`,
					input.TargetUserID, *rec.FileHash,
				).Scan(&existingFileID, &existingActivityID)
				if dedupErr == nil {
					// File already exists — skip this activity.
					idMap.Set(fileID, existingFileID)
					if existingActivityID != "" && oldID != "" {
						idMap.Set(oldID, existingActivityID)
					}
					result.EntitiesSkipped++
					continue
				}
				// pgx.ErrNoRows → file doesn't exist yet, proceed with import.
			}
		} else {
			// Activity without a file: dedup via started_at + sport_type.
			startedAt := rawRow["started_at"]
			sportType := rawRow["sport_type"]
			if startedAt != nil && sportType != nil {
				var existingID string
				dedupErr := e.DB.QueryRow(ctx,
					`SELECT id FROM activities
					 WHERE user_id = $1 AND started_at = $2 AND sport_type = $3
					 LIMIT 1`,
					input.TargetUserID, startedAt, sportType,
				).Scan(&existingID)
				if dedupErr == nil {
					if oldID != "" {
						idMap.Set(oldID, existingID)
					}
					result.EntitiesSkipped++
					continue
				}
			}
		}

		// Per-activity transaction.
		tx, err := e.DB.Begin(ctx)
		if err != nil {
			return activityCount, fmt.Errorf("activity row %d: begin tx: %w", i, err)
		}

		var localDeferred []deferredFK
		prepared := e.prepareRow(activitySpec, rawRow, idMap, input.TargetUserID, insertCols, &localDeferred)

		res, insertErr := e.insertRow(ctx, tx, activitySpec, prepared, idMap, oldID, input.TargetUserID, insertCols, geometryCols)

		if res == failed {
			tx.Rollback(ctx) //nolint:errcheck
			errMsg := ""
			if insertErr != nil {
				errMsg = insertErr.Error()
			}
			result.EntitiesFailed++
			result.Errors = append(result.Errors, ImportError{
				Phase:    5,
				Table:    "activities",
				RowIndex: i,
				OldID:    oldID,
				Error:    errMsg,
			})
			continue
		}

		// Merge deferred FKs only for successfully inserted rows.
		if len(localDeferred) > 0 {
			e.mu.Lock()
			e.deferredFKs = append(e.deferredFKs, localDeferred...)
			e.mu.Unlock()
		}

		if err := tx.Commit(ctx); err != nil {
			tx.Rollback(ctx) //nolint:errcheck
			return activityCount, fmt.Errorf("activity row %d: commit: %w", i, err)
		}

		switch res {
		case inserted:
			result.EntitiesImported++
			activityCount++
		case skipped:
			result.EntitiesSkipped++
		case skippedWithWarning:
			result.EntitiesSkipped++
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("table activities row %d: skipped (FK target missing)", i))
		}

		// Progress every 50 activities.
		if input.OnProgress != nil && (i+1)%50 == 0 {
			input.OnProgress(ProgressEvent{
				Phase:            5,
				PhaseName:        "activities",
				Table:            "activities",
				EntitiesTotal:    len(activityRows),
				EntitiesImported: result.EntitiesImported,
				EntitiesSkipped:  result.EntitiesSkipped,
				EntitiesFailed:   result.EntitiesFailed,
				FilesImported:    result.FilesImported,
				BytesImported:    result.BytesImported,
			})
		}
	}

	// Warn if any activities lack an original file (no sensor-data reconstruction possible).
	if activitiesWithoutFile > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("%d activities without original file — sensor data (streams) cannot be reconstructed", activitiesWithoutFile))
	}

	// Import file_attachments in a single TX.
	if !fileAttachSpec.Skip && fileAttachSpec.Name != "" {
		faTargetCols := schemaMap[fileAttachSpec.Name]
		if err := func() error {
			tx, err := e.DB.Begin(ctx)
			if err != nil {
				return fmt.Errorf("file_attachments begin tx: %w", err)
			}
			defer tx.Rollback(ctx) //nolint:errcheck
			if err := e.importTable(ctx, tx, fileAttachSpec, input.Source, idMap, input.TargetUserID, faTargetCols, exportMigration, result, input.OnProgress); err != nil {
				return fmt.Errorf("file_attachments importTable: %w", err)
			}
			return tx.Commit(ctx)
		}(); err != nil {
			return activityCount, err
		}
	}

	// Import remaining files (media, documents not linked to activities).
	if err := e.importRemainingFiles(ctx, input, idMap, fileIndex, result); err != nil {
		return activityCount, fmt.Errorf("importRemainingFiles: %w", err)
	}

	return activityCount, nil
}

// importRemainingFiles imports any user_files that have not yet been mapped
// through the IDMap (i.e. not already imported as activity originals).
func (e *Engine) importRemainingFiles(
	ctx context.Context,
	input ImportInput,
	idMap *IDMap,
	fileIndex map[string]userFileRecord,
	result *ImportResult,
) error {
	for oldID := range fileIndex {
		if _, alreadyMapped := idMap.GetIfExists(oldID); alreadyMapped {
			continue
		}
		if err := e.importFileByID(ctx, input.Source, idMap, input.TargetUserID, oldID, fileIndex, result); err != nil {
			return fmt.Errorf("importRemainingFiles: %w", err)
		}
	}
	return nil
}

// runPhase5b resolves deferred FK columns collected during earlier phases.
// All updates are wrapped in a single transaction for atomicity.
func (e *Engine) runPhase5b(ctx context.Context, idMap *IDMap) error {
	e.mu.Lock()
	deferred := make([]deferredFK, len(e.deferredFKs))
	copy(deferred, e.deferredFKs)
	e.mu.Unlock()

	if len(deferred) == 0 {
		return nil
	}

	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("phase5b begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, dfk := range deferred {
		newValue, ok := idMap.GetIfExists(dfk.oldValue)
		if !ok {
			// Target was never imported — leave NULL.
			continue
		}
		query := fmt.Sprintf(
			"UPDATE %s SET %s = $1 WHERE id = $2",
			pgx.Identifier{dfk.table}.Sanitize(),
			pgx.Identifier{dfk.column}.Sanitize(),
		)
		if _, err := tx.Exec(ctx, query, newValue, dfk.rowID); err != nil {
			return fmt.Errorf("phase5b deferred FK %s.%s row %s: %w", dfk.table, dfk.column, dfk.rowID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("phase5b commit: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// import_log
// ---------------------------------------------------------------------------

// writeImportLog inserts a summary entry into import_log.
func (e *Engine) writeImportLog(
	ctx context.Context,
	userID string,
	result *ImportResult,
	manifest *Manifest,
	start time.Time,
) error {
	durationMs := int(time.Since(start).Milliseconds())

	meta, _ := json.Marshal(map[string]any{
		"entities_imported": result.EntitiesImported,
		"entities_skipped":  result.EntitiesSkipped,
		"entities_failed":   result.EntitiesFailed,
		"files_imported":    result.FilesImported,
		"bytes_imported":    result.BytesImported,
		"phases_completed":  result.PhasesCompleted,
		"export_migration":  manifest.SchemaMigration,
		"skadia_version":   manifest.SkadiaVersion,
	})

	status := "completed"
	if result.EntitiesFailed > 0 {
		status = "error"
	}

	id := uuid.New().String()
	_, err := e.DB.Exec(ctx, `
		INSERT INTO import_log
			(id, user_id, source_platform, filename, status, duration_ms, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, userID, "gdpr_import", "gdpr_export.zip", status, durationMs, meta)
	return err
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// groupByPhase groups tableSpecs by their Phase field.
func groupByPhase(tables []tableSpec) map[int][]tableSpec {
	result := make(map[int][]tableSpec)
	for _, spec := range tables {
		if spec.Skip {
			continue
		}
		result[spec.Phase] = append(result[spec.Phase], spec)
	}
	return result
}

// findSpec returns the tableSpec with the given name from the list.
// Returns a zero-value tableSpec if not found.
func findSpec(name string, tables []tableSpec) tableSpec {
	for _, spec := range tables {
		if spec.Name == name {
			return spec
		}
	}
	return tableSpec{}
}
