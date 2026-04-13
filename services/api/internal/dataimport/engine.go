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
	const totalPhases = 11 // phases 0–10

	// Phase 0: Disk-space check + import all files (except edit-sidecars).
	if e.Config.LibraryBasePath != "" && e.Config.StorageBackend == "local" {
		est := input.Source.EstimatedFileBytes()
		if est > 0 {
			var stat syscall.Statfs_t
			if err := syscall.Statfs(e.Config.LibraryBasePath, &stat); err == nil {
				available := int64(stat.Bavail) * int64(stat.Bsize)
				if est > available {
					return result, fmt.Errorf("RunImport: insufficient disk space — need ~%d bytes, have %d available", est, available)
				}
			}
		}
	}
	if input.OnProgress != nil {
		input.OnProgress(ProgressEvent{Phase: 0, PhaseName: "files", PhasesTotal: totalPhases, Message: "Importing files..."})
	}
	fileIndex, err := e.importAllFiles(ctx, input.Source, idMap, input.TargetUserID, result, input.OnProgress)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Phase 0 partial failure: %v — orphaned files may exist, delete the account to clean up", err))
		return result, fmt.Errorf("RunImport phase 0 (files): %w", err)
	}
	result.PhasesCompleted = 1

	// Phase 1: profile
	if input.OnProgress != nil {
		input.OnProgress(ProgressEvent{Phase: 1, PhaseName: "profile", PhasesTotal: totalPhases, Message: "Updating profile..."})
	}
	if err := e.runPhase1(ctx, input, manifest); err != nil {
		return result, fmt.Errorf("RunImport phase 1: %w", err)
	}
	result.PhasesCompleted = 2

	// Phases 2, 3, 4 — single TX each
	for _, phase := range []int{2, 3, 4} {
		specs := byPhase[phase]
		if len(specs) == 0 {
			result.PhasesCompleted++
			continue
		}
		if input.OnProgress != nil {
			input.OnProgress(ProgressEvent{Phase: phase, PhasesTotal: totalPhases, Message: fmt.Sprintf("Importing phase %d...", phase)})
		}
		if err := e.runSingleTXPhase(ctx, phase, specs, input, idMap, schemaMap, manifest.SchemaMigration, result); err != nil {
			return result, fmt.Errorf("RunImport phase %d: %w", phase, err)
		}
		result.PhasesCompleted++
	}

	// Phase 5: Activities — per-activity TX (WAL safety, parent_id self-FK).
	if input.OnProgress != nil {
		input.OnProgress(ProgressEvent{Phase: 5, PhasesTotal: totalPhases, Message: "Importing activities..."})
	}
	activityCount, err := e.runPhase5Activities(ctx, input, idMap, schemaMap, manifest.SchemaMigration, result)
	if err != nil {
		return result, fmt.Errorf("RunImport phase 5: %w", err)
	}
	result.PhasesCompleted++

	// Phase 6: Activity-dependent entities + edit-sidecars.
	phase6Specs := byPhase[6]
	if input.OnProgress != nil {
		input.OnProgress(ProgressEvent{Phase: 6, PhasesTotal: totalPhases, Message: "Importing activity-dependent data..."})
	}
	// Import edit-sidecars first (they need activity IDs in IDMap).
	e.importEditSidecars(ctx, input.Source, idMap, input.TargetUserID, fileIndex, result)
	// Then import Phase 6 tables.
	if len(phase6Specs) > 0 {
		if err := e.runSingleTXPhase(ctx, 6, phase6Specs, input, idMap, schemaMap, manifest.SchemaMigration, result); err != nil {
			return result, fmt.Errorf("RunImport phase 6: %w", err)
		}
	}
	result.PhasesCompleted++

	// Phase 7: Health + External + file_attachments.
	phase7Specs := byPhase[7]
	if len(phase7Specs) > 0 {
		if input.OnProgress != nil {
			input.OnProgress(ProgressEvent{Phase: 7, PhasesTotal: totalPhases, Message: "Importing health data..."})
		}
		if err := e.runSingleTXPhase(ctx, 7, phase7Specs, input, idMap, schemaMap, manifest.SchemaMigration, result); err != nil {
			return result, fmt.Errorf("RunImport phase 7: %w", err)
		}
	}
	result.PhasesCompleted++

	// Phase 8: Deferred FK resolution.
	if input.OnProgress != nil {
		input.OnProgress(ProgressEvent{Phase: 8, PhasesTotal: totalPhases, Message: "Resolving deferred references..."})
	}
	if err := e.runPhase5b(ctx, idMap); err != nil {
		return result, fmt.Errorf("RunImport phase 8 (deferred FKs): %w", err)
	}
	result.PhasesCompleted++

	// Phase 9: JSONB remapping (after all entity IDs are finalized).
	if input.OnProgress != nil {
		input.OnProgress(ProgressEvent{Phase: 9, PhasesTotal: totalPhases, Message: "Remapping JSONB references..."})
	}
	if err := e.runJSONBRemap(ctx, idMap, input.TargetUserID); err != nil {
		return result, fmt.Errorf("RunImport phase 9 (JSONB remap): %w", err)
	}
	result.PhasesCompleted++

	// Phase 10: Enrichment.
	if input.OnProgress != nil {
		input.OnProgress(ProgressEvent{Phase: 10, PhasesTotal: totalPhases, Message: "Triggering enrichment..."})
	}
	if err := triggerEnrichment(ctx, e.DB, input.TargetUserID, activityCount); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("triggerEnrichment: %v", err))
	}
	result.PhasesCompleted++

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

// runPhase5Activities imports activities with per-activity transactions.
// Files are already imported in Phase 0; no file handling here.
// Uses streaming JSON decoding to avoid loading all activities into memory.
func (e *Engine) runPhase5Activities(
	ctx context.Context,
	input ImportInput,
	idMap *IDMap,
	schemaMap map[string][]targetColumn,
	exportMigration int,
	result *ImportResult,
) (int, error) {
	allTbls := allTables()
	allTbls = append(allTbls, healthTables()...)
	activitySpec := findSpec("activities", allTbls)

	rc, err := input.Source.OpenData("data/activities.json")
	if err != nil {
		// No activities file — skip.
		return 0, nil
	}
	defer rc.Close()

	dec := json.NewDecoder(rc)
	openTok, err := dec.Token()
	if err != nil {
		return 0, fmt.Errorf("runPhase5Activities: read opening token: %w", err)
	}
	switch v := openTok.(type) {
	case json.Delim:
		if v != '[' {
			return 0, fmt.Errorf("runPhase5Activities: expected JSON array, got %q", v)
		}
	default:
		return 0, nil // null or scalar → no activities
	}

	// Determine export columns — lazy from first row if metadata unavailable.
	exportCols := exportColumnsForTable(input.Source, "activities")
	targetCols := schemaMap[activitySpec.Name]
	var insertCols []string
	var geometryCols map[string]bool
	colsReady := len(exportCols) > 0
	if colsReady {
		insertCols, geometryCols = computeInsertColumns(exportCols, targetCols)
	}

	activityCount := 0
	activitiesWithMissingFile := 0
	i := 0

	for dec.More() {
		var rawRow map[string]any
		if err := dec.Decode(&rawRow); err != nil {
			return activityCount, fmt.Errorf("runPhase5Activities: decode row %d: %w", i, err)
		}

		if !colsReady {
			exportCols = extractKeysFromRow(rawRow)
			insertCols, geometryCols = computeInsertColumns(exportCols, targetCols)
			colsReady = true
		}

		rawRow = applyTransformers(exportMigration, "activities", rawRow)

		// Track files referenced but not in export (Finding #14).
		fileID, hasFileID := stringVal(rawRow["file_id"])
		if hasFileID && fileID != "" {
			if _, exists := idMap.GetIfExists(fileID); !exists {
				activitiesWithMissingFile++
			}
		}

		oldID := extractOldID(rawRow, activitySpec)

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
			i++
			continue
		}

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
				EntitiesImported: result.EntitiesImported,
				EntitiesSkipped:  result.EntitiesSkipped,
				EntitiesFailed:   result.EntitiesFailed,
				FilesImported:    result.FilesImported,
				BytesImported:    result.BytesImported,
			})
		}
		i++
	}

	if activitiesWithMissingFile > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("%d activities reference files not found in the export — sensor data (streams) cannot be reconstructed", activitiesWithMissingFile))
	}

	return activityCount, nil
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
