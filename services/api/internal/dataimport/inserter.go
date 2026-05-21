package dataimport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Result types
// ---------------------------------------------------------------------------

// ImportResult summarises a completed import run.
type ImportResult struct {
	UserID           string
	PhasesCompleted  int
	EntitiesImported int
	EntitiesSkipped  int
	EntitiesFailed   int
	FilesImported    int
	BytesImported    int64
	Errors           []ImportError
	Warnings         []string
	Duration         time.Duration
	importedFileIDs  []string
}

// ImportError captures a single row-level import failure.
type ImportError struct {
	Phase    int
	Table    string
	RowIndex int
	OldID    string
	Error    string
}

// ---------------------------------------------------------------------------
// insertResult enum
// ---------------------------------------------------------------------------

type insertResult int

const (
	inserted insertResult = iota
	skipped
	skippedWithWarning
	failed
)

// ---------------------------------------------------------------------------
// importTable — main entry point for one table
// ---------------------------------------------------------------------------

// importTable imports all rows for spec from source into tx.
func (e *Engine) importTable(
	ctx context.Context,
	tx pgx.Tx,
	spec tableSpec,
	source Source,
	idMap *IDMap,
	userID string,
	targetCols []targetColumn,
	exportMigration int,
	result *ImportResult,
	onProgress func(ProgressEvent),
	deferredFKs *[]deferredFK,
) error {
	// 1. Determine data path.
	dataPath := fmt.Sprintf("data/%s.json", spec.Name)
	if isHealthTable(spec.Name) {
		dataPath = fmt.Sprintf("data/health/%s.json", spec.Name)
	}

	// 2. Open + read JSON array.
	rc, err := source.OpenDataStream(dataPath)
	if err != nil {
		// Missing file is not fatal — table may simply have no rows.
		return nil
	}
	defer rc.Close()

	// 2b. Set up streaming JSON decoder.
	dec := json.NewDecoder(rc)

	// Read opening token: must be '[' or 'null'.
	openTok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("importTable %s: read opening token: %w", spec.Name, err)
	}
	switch v := openTok.(type) {
	case json.Delim:
		if v != '[' {
			return fmt.Errorf("importTable %s: expected JSON array, got %q", spec.Name, v)
		}
	default:
		// null or any other scalar — treat as empty array (Finding 25).
		return nil
	}

	// 3. Determine export columns (we need the first row if columns.json is absent).
	exportCols := exportColumnsForTable(source, spec.Name)

	// 4. Insertable columns and geometry set — computed after we know exportCols.
	//    When exportCols is empty we defer until the first row is decoded.
	var insertCols []string
	var geometryCols map[string]bool
	colsReady := len(exportCols) > 0
	if colsReady {
		insertCols, geometryCols = computeInsertColumns(exportCols, targetCols)
		if len(insertCols) == 0 {
			return nil
		}
	}

	// 5. For cmReplace: delete existing rows before iterating.
	if spec.Conflict == cmReplace {
		if err := deleteForReplace(ctx, tx, spec, userID); err != nil {
			return fmt.Errorf("importTable %s: deleteForReplace: %w", spec.Name, err)
		}
	}

	// 6. Per-row streaming iteration (Finding 24).
	rowIndex := 0
	totalRows := 0
	for dec.More() {
		var rawRow map[string]any
		if err := dec.Decode(&rawRow); err != nil {
			return fmt.Errorf("importTable %s: decode row %d: %w", spec.Name, rowIndex, err)
		}
		totalRows++

		// Lazily compute columns from first row when columns.json was absent.
		if !colsReady {
			exportCols = extractKeysFromRow(rawRow)
			insertCols, geometryCols = computeInsertColumns(exportCols, targetCols)
			colsReady = true
			if len(insertCols) == 0 {
				return nil
			}
		}

		// Apply schema-migration transformers.
		rawRow = applyTransformers(exportMigration, spec.Name, rawRow)

		// Capture old PK before prepareRow replaces it.
		oldID := extractOldID(rawRow, spec)

		var normalizeErr error
		rawRow, normalizeErr = normalizeSportTaxonomyImportRow(spec.Name, rawRow, targetCols)
		if normalizeErr != nil {
			result.EntitiesFailed++
			result.Errors = append(result.Errors, ImportError{
				Phase:    spec.Phase,
				Table:    spec.Name,
				RowIndex: rowIndex,
				OldID:    oldID,
				Error:    normalizeErr.Error(),
			})
			rowIndex++
			continue
		}
		insertCols, geometryCols = augmentInsertColumnsForRow(insertCols, geometryCols, targetCols, rawRow)

		rowIDMap := idMap.Clone()
		var localDeferred []deferredFK
		prepared := e.prepareRow(spec, rawRow, rowIDMap, userID, insertCols, &localDeferred)

		if _, err := tx.Exec(ctx, "SAVEPOINT import_row"); err != nil {
			return fmt.Errorf("importTable %s row %d: savepoint: %w", spec.Name, rowIndex, err)
		}
		res, insertErr := e.insertRow(ctx, tx, spec, prepared, rowIDMap, oldID, userID, insertCols, geometryCols, targetCols)

		if res == failed {
			if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT import_row"); err != nil {
				return fmt.Errorf("importTable %s row %d: rollback savepoint after %v: %w", spec.Name, rowIndex, insertErr, err)
			}
			if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT import_row"); err != nil {
				return fmt.Errorf("importTable %s row %d: release failed savepoint: %w", spec.Name, rowIndex, err)
			}
		} else if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT import_row"); err != nil {
			return fmt.Errorf("importTable %s row %d: release savepoint: %w", spec.Name, rowIndex, err)
		}

		if shouldCommitRowState(spec, res) {
			idMap.MergeFrom(rowIDMap)
			if len(localDeferred) > 0 {
				*deferredFKs = append(*deferredFKs, localDeferred...)
			}
		}

		switch res {
		case inserted:
			result.EntitiesImported++
		case skipped:
			result.EntitiesSkipped++
		case skippedWithWarning:
			result.EntitiesSkipped++
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("table %s row %d: skipped (FK target missing)", spec.Name, rowIndex))
		case failed:
			result.EntitiesFailed++
			errMsg := ""
			if insertErr != nil {
				errMsg = insertErr.Error()
			}
			result.Errors = append(result.Errors, ImportError{
				Phase:    spec.Phase,
				Table:    spec.Name,
				RowIndex: rowIndex,
				OldID:    oldID,
				Error:    errMsg,
			})
		}
		rowIndex++
	}

	if totalRows == 0 {
		return nil
	}

	// 7. Progress callback.
	if onProgress != nil {
		onProgress(ProgressEvent{
			Phase:            spec.Phase,
			Table:            spec.Name,
			EntitiesTotal:    totalRows,
			EntitiesImported: result.EntitiesImported,
			EntitiesSkipped:  result.EntitiesSkipped,
			EntitiesFailed:   result.EntitiesFailed,
		})
	}

	return nil
}

func shouldCommitRowState(spec tableSpec, res insertResult) bool {
	if res == inserted {
		return true
	}
	return res == skipped && spec.Conflict == cmMergeID
}

// exportColumnsForTable reads columns.json from source and returns the column
// list for the given table. Returns nil if columns.json is absent or does not
// contain an entry for table.
func exportColumnsForTable(source Source, table string) []string {
	colMap, err := source.Columns()
	if err != nil || len(colMap) == 0 {
		return nil
	}
	return colMap[table]
}

// ---------------------------------------------------------------------------
// prepareRow
// ---------------------------------------------------------------------------

// prepareRow transforms a raw export row into a values map ready for INSERT:
//   - replaces user column with target userID
//   - remaps PK through IDMap (generating a new UUID)
//   - remaps FK columns; deferred FKs are set to nil and recorded
//   - applies table-specific field overrides
//   - remaps embedded UUIDs in JSONB columns
func (e *Engine) prepareRow(
	spec tableSpec,
	row map[string]any,
	idMap *IDMap,
	userID string,
	insertCols []string,
	deferredFKsOut *[]deferredFK,
) map[string]any {
	out := make(map[string]any, len(row))
	for k, v := range row {
		out[k] = v
	}

	// 1. User column.
	if userCol := getUserCol(spec); userCol != "" {
		out[userCol] = userID
	}

	// 2. PK remap.
	if spec.PKField != "" {
		if oldID, ok := stringVal(out[spec.PKField]); ok && oldID != "" {
			out[spec.PKField] = idMap.Get(oldID)
		}
	}

	// 3. FK fields remap (skip deferred ones).
	for _, fk := range spec.FKFields {
		if isDeferred(spec, fk) {
			continue
		}
		val := out[fk]
		if val == nil {
			continue
		}
		if oldRef, ok := stringVal(val); ok && oldRef != "" {
			// File-reference columns: use GetIfExists to avoid phantom UUIDs
			// for files that weren't in the export.
			if isFileRefColumn(spec.Name, fk) {
				if newRef, exists := idMap.GetIfExists(oldRef); exists {
					out[fk] = newRef
				} else {
					out[fk] = nil // File missing from export
				}
			} else {
				out[fk] = idMap.Get(oldRef)
			}
		}
	}

	// 4. Deferred FKs: set to nil and record for Phase 5b resolution.
	if spec.PKField != "" {
		for _, fk := range spec.Deferred {
			val := out[fk]
			if val == nil {
				continue
			}
			oldRef, ok := stringVal(val)
			if !ok || oldRef == "" {
				out[fk] = nil
				continue
			}
			// The row's new PK has already been set in step 2.
			newPK, _ := stringVal(out[spec.PKField])
			*deferredFKsOut = append(*deferredFKsOut, deferredFK{
				table:    spec.Name,
				rowID:    newPK,
				column:   fk,
				oldValue: oldRef,
			})
			out[fk] = nil
		}
	}

	// 5. file_attachments: remap entity_id.
	if spec.Name == "file_attachments" {
		if oldEntityID, ok := stringVal(out["entity_id"]); ok && oldEntityID != "" {
			if newEntityID, exists := idMap.GetIfExists(oldEntityID); exists {
				out["entity_id"] = newEntityID
			} else {
				out["entity_id"] = nil
			}
		}
	}

	// 6. deleted_at → nil.
	if _, has := out["deleted_at"]; has {
		out["deleted_at"] = nil
	}

	// 7. oauth_connections: sanitise sensitive fields.
	if spec.Name == "oauth_connections" {
		out["access_token"] = nil
		out["refresh_token"] = nil
		out["token_expiry"] = nil
		out["status"] = "reconnection_required"
	}
	switch spec.Name {
	case "segments":
		out["visibility"] = "private"
		out["access_scope_id"] = nil
		out["source_activity_publish_confirmed_at"] = nil
		out["source_activity_publish_confirmed_by"] = nil
	case "target_places", "target_regions", "target_collections":
		out["lifecycle_owner_kind"] = "user"
		out["lifecycle_owner_ref"] = nil
		out["created_by"] = userID
		out["visibility"] = "private"
		out["access_scope_id"] = nil
	case "target_collection_members":
		out["added_by"] = userID
	}

	return out
}

// ---------------------------------------------------------------------------
// JSONB remap helpers
// ---------------------------------------------------------------------------

// walkAndRemap recursively traverses a JSONB value (decoded as any) and
// replaces any string value whose key is listed in jsonbRemapKeys with
// its IDMap-mapped counterpart.
func walkAndRemap(val any, idMap *IDMap) any {
	switch v := val.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, child := range v {
			// Check if this key is a remap target across any table.
			if isJSONBRemapKey(k) {
				if s, ok := stringVal(child); ok && s != "" {
					if mapped, exists := idMap.GetIfExists(s); exists {
						out[k] = mapped
					} else {
						out[k] = child // preserve original — entity not in export
					}
					continue
				}
			}
			out[k] = walkAndRemap(child, idMap)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = walkAndRemap(item, idMap)
		}
		return out
	default:
		return val
	}
}

// isJSONBRemapKey reports whether key is a UUID-bearing key that must be
// remapped during JSONB traversal. Keys are defined in jsonbRemapKeys.
func isJSONBRemapKey(key string) bool {
	return jsonbRemapKeys[key]
}

// ---------------------------------------------------------------------------
// insertRow
// ---------------------------------------------------------------------------

// insertRow dispatches to the appropriate conflict-mode handler and executes
// the INSERT (or skip) for a single prepared row.
//
// oldID is the source-side PK value before remapping; it is used to update
// the IDMap when an existing row is found via natural-key lookup (cmMergeID).
func (e *Engine) insertRow(
	ctx context.Context,
	tx pgx.Tx,
	spec tableSpec,
	row map[string]any,
	idMap *IDMap,
	oldID string,
	userID string,
	insertCols []string,
	geometryCols map[string]bool,
	targetCols []targetColumn,
) (insertResult, error) {
	if spec.Name == "file_attachments" && (row["file_id"] == nil || row["entity_id"] == nil) {
		return skippedWithWarning, nil
	}
	if spec.Name == "automation_rules" {
		return e.insertAutomationRule(ctx, tx, spec, row, idMap, oldID, userID, insertCols, geometryCols)
	}

	switch spec.Conflict {
	case cmMergeID:
		existingID, found, err := findByNaturalKey(ctx, tx, spec, userID, row)
		if err != nil {
			return failed, err
		}
		if found {
			// Re-map oldID → existingID so all downstream FK references resolve to
			// the pre-existing row rather than a fresh UUID that was never inserted.
			if oldID != "" {
				idMap.Set(oldID, existingID)
			}
			return skipped, nil
		}
		return e.doInsert(ctx, tx, spec, row, insertCols, geometryCols, targetCols)

	case cmSkipWarn:
		exists, err := fkTargetExists(ctx, tx, spec, row)
		if err != nil {
			return failed, err
		}
		if !exists {
			return skippedWithWarning, nil
		}
		return e.doInsert(ctx, tx, spec, row, insertCols, geometryCols, targetCols)

	default: // cmInsert, cmReplace (delete already done), cmUpsert
		return e.doInsert(ctx, tx, spec, row, insertCols, geometryCols, targetCols)
	}
}

func (e *Engine) insertAutomationRule(
	ctx context.Context,
	tx pgx.Tx,
	spec tableSpec,
	row map[string]any,
	idMap *IDMap,
	oldID string,
	userID string,
	insertCols []string,
	geometryCols map[string]bool,
) (insertResult, error) {
	if err := setImportUserRLS(ctx, tx, userID); err != nil {
		return failed, err
	}
	isSystem := boolValue(row["is_system"])
	existingID, found, err := findAutomationRuleForImport(ctx, tx, userID, row, isSystem)
	if err != nil {
		return failed, err
	}
	if !found {
		return e.doInsert(ctx, tx, spec, row, insertCols, geometryCols, nil)
	}
	if oldID != "" {
		idMap.Set(oldID, existingID)
	}
	if isSystem {
		if err := updateExistingSystemAutomationRule(ctx, tx, existingID, userID, row, insertCols); err != nil {
			return failed, err
		}
	}
	return skipped, nil
}

func findAutomationRuleForImport(ctx context.Context, tx pgx.Tx, userID string, row map[string]any, isSystem bool) (string, bool, error) {
	var query string
	var args []any
	if isSystem {
		systemType, ok := stringVal(row["system_type"])
		if !ok || strings.TrimSpace(systemType) == "" {
			return "", false, nil
		}
		query = `
			SELECT id::text
			FROM automation_rules
			WHERE user_id = $1 AND is_system = true AND system_type = $2
			LIMIT 1`
		args = []any{userID, systemType}
	} else {
		name, ok := stringVal(row["name"])
		if !ok || strings.TrimSpace(name) == "" {
			return "", false, nil
		}
		query = `
			SELECT id::text
			FROM automation_rules
			WHERE user_id = $1 AND is_system = false AND name = $2
			LIMIT 1`
		args = []any{userID, name}
	}

	var existingID string
	err := tx.QueryRow(ctx, query, args...).Scan(&existingID)
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find automation_rules import match: %w", err)
	}
	return existingID, true, nil
}

func updateExistingSystemAutomationRule(ctx context.Context, tx pgx.Tx, existingID, userID string, row map[string]any, insertCols []string) error {
	mutable := map[string]bool{
		"name":                true,
		"is_active":           true,
		"priority":            true,
		"conditions":          true,
		"conditions_logic":    true,
		"condition_groups":    true,
		"actions":             true,
		"trigger_event":       true,
		"stop_on_match":       true,
		"apply_retroactively": true,
		"branches":            true,
	}
	var sets []string
	var args []any
	for _, col := range insertCols {
		if !mutable[col] {
			continue
		}
		val, ok := row[col]
		if !ok {
			continue
		}
		converted, err := convertValue(val, col)
		if err != nil {
			return fmt.Errorf("update automation_rules.%s: %w", col, err)
		}
		args = append(args, converted)
		sets = append(sets, fmt.Sprintf("%s = $%d", pgx.Identifier{col}.Sanitize(), len(args)))
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, existingID, userID)
	query := fmt.Sprintf(
		`UPDATE automation_rules SET %s WHERE id = $%d AND user_id = $%d AND is_system = true`,
		strings.Join(sets, ", "),
		len(args)-1,
		len(args),
	)
	if _, err := tx.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("update existing system automation rule: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// doInsert
// ---------------------------------------------------------------------------

// doInsert executes the parameterised INSERT for one row.
// Geometry columns are wrapped with ST_GeomFromGeoJSON($n).
// JSONB/array columns are marshalled to []byte so pgx sends them as JSON.
func (e *Engine) doInsert(
	ctx context.Context,
	tx pgx.Tx,
	spec tableSpec,
	row map[string]any,
	insertCols []string,
	geometryCols map[string]bool,
	targetCols []targetColumn,
) (insertResult, error) {
	colParts := make([]string, 0, len(insertCols))
	valParts := make([]string, 0, len(insertCols))
	args := make([]any, 0, len(insertCols))

	n := 1
	for _, col := range insertCols {
		val, ok := row[col]
		if !ok {
			continue
		}
		colParts = append(colParts, pgx.Identifier{col}.Sanitize())
		if geometryCols[col] {
			if val == nil {
				valParts = append(valParts, fmt.Sprintf("$%d", n))
				args = append(args, nil)
				n++
				continue
			}
			// Value is a GeoJSON string/object from export.
			b, err := marshalGeoJSON(val)
			if err != nil {
				return failed, fmt.Errorf("doInsert %s.%s: marshal geojson: %w", spec.Name, col, err)
			}
			valParts = append(valParts, fmt.Sprintf("ST_GeomFromGeoJSON($%d)", n))
			args = append(args, string(b))
		} else {
			target, _ := targetColumnByName(targetCols, col)
			converted, cvErr := convertValueForTargetColumn(val, col, target)
			if cvErr != nil {
				return failed, fmt.Errorf("doInsert %s.%s: %w", spec.Name, col, cvErr)
			}
			valParts = append(valParts, fmt.Sprintf("$%d", n))
			args = append(args, converted)
		}
		n++
	}

	if len(colParts) == 0 {
		return skipped, nil
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		pgx.Identifier{spec.Name}.Sanitize(),
		strings.Join(colParts, ", "),
		strings.Join(valParts, ", "),
	)

	ignoreConflicts := isHealthTable(spec.Name) || spec.IgnoreConflicts
	if ignoreConflicts {
		query += " ON CONFLICT DO NOTHING"
	}

	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return failed, fmt.Errorf("doInsert %s: %w", spec.Name, err)
	}
	if ignoreConflicts && tag.RowsAffected() == 0 {
		return skipped, nil
	}
	return inserted, nil
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// convertValue coerces val into a type that pgx can bind directly.
// Maps and slices are JSON-marshalled to []byte so pgx sends them as JSONB.
// col is included in error messages for diagnostics.
func convertValue(val any, col string) (any, error) {
	if val == nil {
		return nil, nil
	}
	switch val.(type) {
	case map[string]any, []any:
		b, err := json.Marshal(val)
		if err != nil {
			return nil, fmt.Errorf("convertValue: marshal column %q: %w", col, err)
		}
		return b, nil
	default:
		return val, nil
	}
}

func convertValueForTargetColumn(val any, col string, target targetColumn) (any, error) {
	if isPostgresArrayUDT(target.UDTName) {
		return convertArrayValue(val, col, target.UDTName)
	}
	return convertValue(val, col)
}

func isPostgresArrayUDT(udtName string) bool {
	return strings.HasPrefix(udtName, "_")
}

func convertArrayValue(val any, col, udtName string) (any, error) {
	if val == nil {
		return nil, nil
	}
	items, ok := valueSlice(val)
	if !ok {
		return val, nil
	}
	switch udtName {
	case "_text":
		out := make([]string, len(items))
		for i, item := range items {
			if item == nil {
				return nil, fmt.Errorf("convertArrayValue: column %q contains null element", col)
			}
			out[i] = fmt.Sprint(item)
		}
		return out, nil
	case "_bool":
		out := make([]bool, len(items))
		for i, item := range items {
			v, ok := item.(bool)
			if !ok {
				return nil, fmt.Errorf("convertArrayValue: column %q has non-boolean value %v", col, item)
			}
			out[i] = v
		}
		return out, nil
	case "_int2":
		out := make([]int16, len(items))
		for i, item := range items {
			n, err := int64ArrayValue(item, col)
			if err != nil {
				return nil, err
			}
			if n < -32768 || n > 32767 {
				return nil, fmt.Errorf("convertArrayValue: column %q value %d overflows int2", col, n)
			}
			out[i] = int16(n)
		}
		return out, nil
	case "_int4":
		out := make([]int32, len(items))
		for i, item := range items {
			n, err := int64ArrayValue(item, col)
			if err != nil {
				return nil, err
			}
			if n < -2147483648 || n > 2147483647 {
				return nil, fmt.Errorf("convertArrayValue: column %q value %d overflows int4", col, n)
			}
			out[i] = int32(n)
		}
		return out, nil
	case "_int8":
		out := make([]int64, len(items))
		for i, item := range items {
			n, err := int64ArrayValue(item, col)
			if err != nil {
				return nil, err
			}
			out[i] = n
		}
		return out, nil
	case "_float4":
		out := make([]float32, len(items))
		for i, item := range items {
			n, err := float64ArrayValue(item, col)
			if err != nil {
				return nil, err
			}
			out[i] = float32(n)
		}
		return out, nil
	case "_float8":
		out := make([]float64, len(items))
		for i, item := range items {
			n, err := float64ArrayValue(item, col)
			if err != nil {
				return nil, err
			}
			out[i] = n
		}
		return out, nil
	default:
		return nil, fmt.Errorf("convertArrayValue: unsupported SQL array type %q for column %q", udtName, col)
	}
}

func valueSlice(val any) ([]any, bool) {
	switch v := val.(type) {
	case []any:
		return v, true
	case []string:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = item
		}
		return out, true
	case []int:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = item
		}
		return out, true
	case []int16:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = item
		}
		return out, true
	case []int32:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = item
		}
		return out, true
	case []int64:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = item
		}
		return out, true
	case []float32:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = item
		}
		return out, true
	case []float64:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = item
		}
		return out, true
	default:
		return nil, false
	}
}

func int64ArrayValue(val any, col string) (int64, error) {
	switch v := val.(type) {
	case int:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		if v != float64(int64(v)) {
			return 0, fmt.Errorf("convertArrayValue: column %q has non-integer value %v", col, v)
		}
		return int64(v), nil
	default:
		return 0, fmt.Errorf("convertArrayValue: column %q has non-integer value %v", col, val)
	}
}

func float64ArrayValue(val any, col string) (float64, error) {
	switch v := val.(type) {
	case int:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	default:
		return 0, fmt.Errorf("convertArrayValue: column %q has non-numeric value %v", col, val)
	}
}

// marshalGeoJSON marshals a GeoJSON value (string or map) to []byte suitable
// for ST_GeomFromGeoJSON.
func marshalGeoJSON(val any) ([]byte, error) {
	switch v := val.(type) {
	case string:
		return []byte(v), nil
	default:
		return json.Marshal(v)
	}
}

// isDeferred reports whether col appears in spec.Deferred.
func isDeferred(spec tableSpec, col string) bool {
	for _, d := range spec.Deferred {
		if d == col {
			return true
		}
	}
	return false
}

// isHealthTable reports whether name belongs to the health_ table family.
func isHealthTable(name string) bool {
	return strings.HasPrefix(name, "health_")
}

// extractOldID returns the original (pre-remap) PK value from a raw export row.
// After prepareRow the PK is already replaced; callers should pass the raw row.
func extractOldID(row map[string]any, spec tableSpec) string {
	if spec.PKField == "" {
		return ""
	}
	id, _ := stringVal(row[spec.PKField])
	return id
}

// extractKeysFromFirstRow returns the map keys of the first row, used as a
// fallback when columns.json is absent. row slice variant for callers that
// already hold all rows in memory (e.g. engine.go activity path).
func extractKeysFromFirstRow(rows []map[string]any) []string {
	if len(rows) == 0 {
		return nil
	}
	return extractKeysFromRow(rows[0])
}

// extractKeysFromRow returns the map keys of a single row, used as a
// fallback when columns.json is absent in the streaming import path.
func extractKeysFromRow(row map[string]any) []string {
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	return keys
}

// stringVal asserts val as string. Returns ("", false) for nil or non-string.
func stringVal(val any) (string, bool) {
	if val == nil {
		return "", false
	}
	s, ok := val.(string)
	return s, ok
}

func setImportUserRLS(ctx context.Context, tx pgx.Tx, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("set import RLS context: userID is required")
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_user_id', $1, true)", userID); err != nil {
		return fmt.Errorf("set import RLS context: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// runJSONBRemap — Phase 9
// ---------------------------------------------------------------------------

// runJSONBRemap updates JSONB columns that contain entity UUIDs.
// Called in Phase 9 after ALL entities are in the IDMap (including cmMergeID overrides).
func (e *Engine) runJSONBRemap(ctx context.Context, idMap *IDMap, userID string) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("jsonb remap begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := setImportUserRLS(ctx, tx, userID); err != nil {
		return err
	}

	for table, jsonbCols := range jsonbRemapFields {
		// Look up correct user column for this table.
		spec := findSpecByName(table)
		userCol := getUserCol(spec)
		if userCol == "" {
			continue // Table has no user column.
		}

		// Build SELECT for id::text + JSONB columns.
		selectCols := []string{"id::text"}
		for _, col := range jsonbCols {
			selectCols = append(selectCols, pgx.Identifier{col}.Sanitize())
		}
		query := fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1",
			strings.Join(selectCols, ", "),
			pgx.Identifier{table}.Sanitize(),
			pgx.Identifier{userCol}.Sanitize(),
		)
		rows, err := tx.Query(ctx, query, userID)
		if err != nil {
			return fmt.Errorf("jsonb remap SELECT %s: %w", table, err)
		}

		type rowUpdate struct {
			id     string
			values []any
		}
		var updates []rowUpdate

		for rows.Next() {
			vals := make([]any, len(jsonbCols)+1)
			ptrs := make([]any, len(vals))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return fmt.Errorf("jsonb remap scan %s: %w", table, err)
			}

			rowID, _ := vals[0].(string)
			changed := false
			remapped := make([]any, len(jsonbCols))
			for i, raw := range vals[1:] {
				if raw == nil {
					remapped[i] = nil
					continue
				}
				// pgx returns JSONB as map[string]any or []any.
				result := walkAndRemap(raw, idMap)
				remapped[i] = result
				// Change detection: marshal and compare bytes.
				rawJSON, err := json.Marshal(raw)
				if err != nil {
					rows.Close()
					return fmt.Errorf("jsonb remap marshal original %s row %s: %w", table, rowID, err)
				}
				resultJSON, err := json.Marshal(result)
				if err != nil {
					rows.Close()
					return fmt.Errorf("jsonb remap marshal remapped %s row %s: %w", table, rowID, err)
				}
				if !bytes.Equal(rawJSON, resultJSON) {
					changed = true
				}
			}
			if changed {
				updates = append(updates, rowUpdate{id: rowID, values: remapped})
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("jsonb remap rows %s: %w", table, err)
		}

		// Apply updates.
		for _, upd := range updates {
			setClauses := make([]string, len(jsonbCols))
			args := make([]any, 0, len(jsonbCols)+1)
			for i, col := range jsonbCols {
				setClauses[i] = fmt.Sprintf("%s = $%d", pgx.Identifier{col}.Sanitize(), i+1)
				if upd.values[i] == nil {
					args = append(args, nil)
					continue
				}
				// Marshal back to JSON bytes for pgx JSONB binding.
				b, err := json.Marshal(upd.values[i])
				if err != nil {
					return fmt.Errorf("jsonb remap marshal update %s col %s: %w", table, col, err)
				}
				args = append(args, b)
			}
			args = append(args, upd.id)
			updateQuery := fmt.Sprintf("UPDATE %s SET %s WHERE id = $%d",
				pgx.Identifier{table}.Sanitize(),
				strings.Join(setClauses, ", "),
				len(jsonbCols)+1,
			)
			if _, err := tx.Exec(ctx, updateQuery, args...); err != nil {
				return fmt.Errorf("jsonb remap UPDATE %s row %s: %w", table, upd.id, err)
			}
		}
	}

	return tx.Commit(ctx)
}
