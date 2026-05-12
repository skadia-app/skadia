package dataimport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/model"
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
	DB       *pgxpool.Pool
	ShadowFS shadowfs.ShadowFS
	Config   EngineConfig
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

	// deferredFKs collects FK references that cannot be resolved immediately
	// (e.g. self-referential parent_id). Resolved in Phase 8 (runPhase5b).
	var deferredFKs []deferredFK

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
	activityFileTags, activityFileTagWarnings, err := loadActivityFileTagIndex(input.Source)
	if err != nil {
		return result, fmt.Errorf("RunImport phase 0 (activity file tags): %w", err)
	}
	result.Warnings = append(result.Warnings, activityFileTagWarnings...)
	fileIndex, err := e.importAllFiles(ctx, input.Source, idMap, input.TargetUserID, result, input.OnProgress, activityFileTags)
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
		if err := e.runSingleTXPhase(ctx, phase, specs, input, idMap, schemaMap, manifest.SchemaMigration, result, &deferredFKs); err != nil {
			return result, fmt.Errorf("RunImport phase %d: %w", phase, err)
		}
		result.PhasesCompleted++
	}

	// Phase 5: Activities — per-activity TX (WAL safety, parent_id self-FK).
	if input.OnProgress != nil {
		input.OnProgress(ProgressEvent{Phase: 5, PhasesTotal: totalPhases, Message: "Importing activities..."})
	}
	activityCount, legacyGearAssignments, err := e.runPhase5Activities(ctx, input, idMap, schemaMap, manifest.SchemaMigration, result, &deferredFKs)
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
	if err := e.importEditSidecars(ctx, input.Source, idMap, input.TargetUserID, fileIndex, result); err != nil {
		return result, fmt.Errorf("RunImport phase 6 (edit sidecars): %w", err)
	}
	// Then import Phase 6 tables.
	if len(phase6Specs) > 0 {
		if err := e.runSingleTXPhase(ctx, 6, phase6Specs, input, idMap, schemaMap, manifest.SchemaMigration, result, &deferredFKs); err != nil {
			return result, fmt.Errorf("RunImport phase 6: %w", err)
		}
	}
	// Phase 6c: legacy-export gear_id backfill. The activities.gear_id column
	// was dropped in 000355's final block; legacy exports may still carry a
	// gear_id field per activity row. We captured (newActID → newGearID)
	// during Phase 5; insert a primary aga row for any (act, gear) pair that
	// doesn't already have a primary assignment from Phase 6's user-authoritative
	// activity_gear_assignments.json. ON CONFLICT DO NOTHING keeps user rows.
	if err := e.backfillLegacyGearAssignments(ctx, input.TargetUserID, legacyGearAssignments); err != nil {
		return result, fmt.Errorf("RunImport phase 6c (legacy gear backfill): %w", err)
	}
	result.PhasesCompleted++

	// Phase 6b: v_old_fusion backwards-compat — derive fusion memberships from
	// legacy fusion_sources JSONB when no activity_fusion_memberships.json was
	// present in the export (Spec §7.2).
	// This is a no-op for v_new (memberships file present) and v_regular exports.
	if err := RunLegacyFusionMembershipsIfNeeded(ctx, e.DB, input.Source, idMap, input.TargetUserID, manifest.SchemaMigration); err != nil {
		// Non-fatal: log + warn, do not abort the whole import.
		slog.Warn("dataimport: RunLegacyFusionMembershipsIfNeeded failed", "error", err)
		result.Warnings = append(result.Warnings, fmt.Sprintf("legacy fusion memberships derivation: %v", err))
	}

	// Phase 7: Health + External + file_attachments.
	phase7Specs := byPhase[7]
	if len(phase7Specs) > 0 {
		if input.OnProgress != nil {
			input.OnProgress(ProgressEvent{Phase: 7, PhasesTotal: totalPhases, Message: "Importing health data..."})
		}
		if err := e.runSingleTXPhase(ctx, 7, phase7Specs, input, idMap, schemaMap, manifest.SchemaMigration, result, &deferredFKs); err != nil {
			return result, fmt.Errorf("RunImport phase 7: %w", err)
		}
	}
	result.PhasesCompleted++

	// Phase 8: Deferred FK resolution.
	if input.OnProgress != nil {
		input.OnProgress(ProgressEvent{Phase: 8, PhasesTotal: totalPhases, Message: "Resolving deferred references..."})
	}
	if err := e.runPhase5b(ctx, idMap, deferredFKs); err != nil {
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
	if err := refreshImportedTagAutomationState(ctx, e.DB, input.TargetUserID); err != nil {
		return result, fmt.Errorf("RunImport phase 9 (tag automation state): %w", err)
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

func refreshImportedTagAutomationState(ctx context.Context, pool *pgxpool.Pool, userID string) error {
	_, err := pool.Exec(ctx, `
		UPDATE tags t SET has_automation = EXISTS (
			SELECT 1 FROM automation_rules ar
			WHERE ar.user_id = t.user_id AND ar.is_active = true
			  AND (ar.actions::text LIKE '%' || t.id::text || '%' OR COALESCE(ar.branches::text, '') LIKE '%' || t.id::text || '%')
		) WHERE t.user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("refresh tag automation state: %w", err)
	}
	return nil
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

	profile, err := decodeProfile(raw)
	if err != nil {
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

func decodeProfile(raw []byte) (map[string]any, error) {
	var profile map[string]any
	objectErr := json.Unmarshal(raw, &profile)
	if objectErr == nil {
		return profile, nil
	}

	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, objectErr
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) > 1 {
		return nil, fmt.Errorf("expected one user row, got %d", len(rows))
	}
	return rows[0], nil
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
	deferredFKs *[]deferredFK,
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
		if err := e.importTable(ctx, tx, spec, input.Source, idMap, input.TargetUserID, targetCols, exportMigration, result, input.OnProgress, deferredFKs); err != nil {
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
//
// The returned map[newActivityID]newGearID captures the legacy activities.gear_id
// field present on rows from older exports. The activities.gear_id column was
// dropped, so the value is no longer written to the activities row; Phase 6c
// uses the map to backfill activity_gear_assignments rows for activities that
// have no aga row from the export.
func (e *Engine) runPhase5Activities(
	ctx context.Context,
	input ImportInput,
	idMap *IDMap,
	schemaMap map[string][]targetColumn,
	exportMigration int,
	result *ImportResult,
	deferredFKs *[]deferredFK,
) (int, map[string]string, error) {
	allTbls := allTables()
	allTbls = append(allTbls, healthTables()...)
	activitySpec := findSpec("activities", allTbls)

	legacyGearAssignments := make(map[string]string)
	rc, err := input.Source.OpenData("data/activities.json")
	if err != nil {
		// No activities file — skip.
		return 0, legacyGearAssignments, nil
	}
	defer rc.Close()

	dec := json.NewDecoder(rc)
	openTok, err := dec.Token()
	if err != nil {
		return 0, legacyGearAssignments, fmt.Errorf("runPhase5Activities: read opening token: %w", err)
	}
	switch v := openTok.(type) {
	case json.Delim:
		if v != '[' {
			return 0, legacyGearAssignments, fmt.Errorf("runPhase5Activities: expected JSON array, got %q", v)
		}
	default:
		return 0, legacyGearAssignments, nil // null or scalar → no activities
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
			return activityCount, legacyGearAssignments, fmt.Errorf("runPhase5Activities: decode row %d: %w", i, err)
		}

		if !colsReady {
			exportCols = extractKeysFromRow(rawRow)
			insertCols, geometryCols = computeInsertColumns(exportCols, targetCols)
			colsReady = true
		}

		rawRow = applyTransformers(exportMigration, "activities", rawRow)

		oldID := extractOldID(rawRow, activitySpec)
		var normalizeErr error
		rawRow, normalizeErr = normalizeSportTaxonomyImportRow("activities", rawRow, targetCols)
		if normalizeErr != nil {
			result.EntitiesFailed++
			result.Errors = append(result.Errors, ImportError{
				Phase:    5,
				Table:    "activities",
				RowIndex: i,
				OldID:    oldID,
				Error:    normalizeErr.Error(),
			})
			i++
			continue
		}
		insertCols, geometryCols = augmentInsertColumnsForRow(insertCols, geometryCols, targetCols, rawRow)

		// Track files referenced but not in export (Finding #14).
		fileID, hasFileID := stringVal(rawRow["file_id"])
		if hasFileID && fileID != "" {
			if _, exists := idMap.GetIfExists(fileID); !exists {
				activitiesWithMissingFile++
			}
		}

		// Capture legacy activities.gear_id (column was dropped in 000355) so
		// Phase 6c can synthesize an activity_gear_assignments row when the
		// export does not carry data/activity_gear_assignments.json.
		legacyOldGearID, _ := stringVal(rawRow["gear_id"])

		// Per-activity transaction.
		tx, err := e.DB.Begin(ctx)
		if err != nil {
			return activityCount, legacyGearAssignments, fmt.Errorf("activity row %d: begin tx: %w", i, err)
		}

		rowIDMap := idMap.Clone()
		var localDeferred []deferredFK
		prepared := e.prepareRow(activitySpec, rawRow, rowIDMap, input.TargetUserID, insertCols, &localDeferred)

		res, insertErr := e.insertRow(ctx, tx, activitySpec, prepared, rowIDMap, oldID, input.TargetUserID, insertCols, geometryCols)

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

		if err := tx.Commit(ctx); err != nil {
			tx.Rollback(ctx) //nolint:errcheck
			return activityCount, legacyGearAssignments, fmt.Errorf("activity row %d: commit: %w", i, err)
		}

		if shouldCommitRowState(activitySpec, res) {
			idMap.MergeFrom(rowIDMap)
			if len(localDeferred) > 0 {
				*deferredFKs = append(*deferredFKs, localDeferred...)
			}
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

		// Record (newActID, newGearID) for Phase 6c backfill on accepted rows
		// only. Skipped/failed rows have no activity in the DB and would be
		// orphan keys.
		if shouldCommitRowState(activitySpec, res) && legacyOldGearID != "" {
			newActID, _ := stringVal(prepared[activitySpec.PKField])
			if newActID != "" {
				newGearID := idMap.Get(legacyOldGearID)
				if newGearID != "" {
					legacyGearAssignments[newActID] = newGearID
				}
			}
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

	return activityCount, legacyGearAssignments, nil
}

// backfillLegacyGearAssignments synthesizes activity_gear_assignments rows
// from the legacy activities.gear_id values captured during Phase 5. The
// activities.gear_id column was dropped (000355 final block) so this map is
// the only path by which legacy exports can populate aga.
//
// Idempotency: ON CONFLICT DO NOTHING on the partial-unique primary index
// keeps user-authoritative aga rows from data/activity_gear_assignments.json
// (Phase 6) untouched. We bind every row to userID so cross-user aga rows
// cannot be created even if the legacy map were polluted.
//
// Source 'legacy' marks these rows as derived from activities.gear_id rather
// than from an explicit aga export row.
func (e *Engine) backfillLegacyGearAssignments(ctx context.Context, userID string, legacy map[string]string) error {
	if len(legacy) == 0 {
		return nil
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("backfill legacy aga: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for activityID, gearID := range legacy {
		// Bind aga.user_id and assert via the activities/gear FK that they
		// actually belong to userID — the user_match trigger does the deep
		// check; this guard pre-rejects orphan keys early with a clean message.
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_gear_assignments
				(id, user_id, activity_id, gear_id, role_key, source, quantity)
			SELECT gen_random_uuid(), $1::uuid, a.id, g.id, 'primary', 'legacy', 1
			FROM activities a
			JOIN gear g ON g.id = $3::uuid AND g.user_id = $1::uuid
			WHERE a.id = $2::uuid AND a.user_id = $1::uuid
			ON CONFLICT (activity_id) WHERE role_key = 'primary' DO NOTHING
		`, userID, activityID, gearID)
		if err != nil {
			return fmt.Errorf("backfill legacy aga (act=%s gear=%s): %w", activityID, gearID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("backfill legacy aga: commit: %w", err)
	}
	return nil
}

// runPhase5b resolves deferred FK columns collected during earlier phases.
// All updates are wrapped in a single transaction for atomicity.
func (e *Engine) runPhase5b(ctx context.Context, idMap *IDMap, deferredFKs []deferredFK) error {
	if len(deferredFKs) == 0 {
		return nil
	}

	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("phase5b begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, dfk := range deferredFKs {
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

	meta, err := json.Marshal(map[string]any{
		"entities_imported": result.EntitiesImported,
		"entities_skipped":  result.EntitiesSkipped,
		"entities_failed":   result.EntitiesFailed,
		"files_imported":    result.FilesImported,
		"bytes_imported":    result.BytesImported,
		"phases_completed":  result.PhasesCompleted,
		"export_migration":  manifest.SchemaMigration,
		"skadia_version":   manifest.SkadiaVersion,
	})
	if err != nil {
		return fmt.Errorf("marshal import log metadata: %w", err)
	}

	status := "completed"
	if result.EntitiesFailed > 0 {
		status = "error"
	}

	id := uuid.New().String()
	_, err = e.DB.Exec(ctx, `
		INSERT INTO import_log
			(id, user_id, source_platform, filename, status, duration_ms, metadata, import_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, id, userID, "gdpr_import", "gdpr_export.zip", status, durationMs, meta, model.ImportTypeArchive)
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
