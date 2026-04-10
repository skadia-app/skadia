package auth

import (
	"archive/zip"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// querier abstracts pgx.Tx and pgxpool.Pool so that exportTable can work
// with either a transaction or a bare pool connection.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// formatUUID converts the raw [16]byte that pgx returns for `uuid` columns
// into the standard 8-4-4-4-12 dashed string form. Inlined here (rather than
// importing google/uuid) so the export package keeps a small dependency
// footprint and the format is not coupled to a third-party UUID lib.
func formatUUID(b [16]byte) string {
	const hexDigits = "0123456789abcdef"
	var buf [36]byte
	idx := 0
	for i := 0; i < 16; i++ {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			buf[idx] = '-'
			idx++
		}
		buf[idx] = hexDigits[b[i]>>4]
		buf[idx+1] = hexDigits[b[i]&0x0f]
		idx += 2
	}
	return string(buf[:])
}

// columnInfo holds the name and underlying PostgreSQL type name of a column.
type columnInfo struct {
	Name    string
	UDTName string // e.g. "int4", "text", "jsonb", "geometry", "bytea"
}

// exportTableError records a failure to export a single table. The export
// worker collects these and either marks the job as failed (with the list
// surfaced in background_jobs.metadata.export_errors) or — when empty —
// marks the job as completed.
type exportTableError struct {
	Table string `json:"table"`
	Error string `json:"error"`
}

// introspectColumns returns the ordered list of columns for the given table
// from information_schema.columns, restricted to the public schema.
func introspectColumns(ctx context.Context, q querier, table string) ([]columnInfo, error) {
	rows, err := q.Query(ctx,
		`SELECT column_name, udt_name
		 FROM information_schema.columns
		 WHERE table_schema = 'public'
		   AND table_name = $1
		 ORDER BY ordinal_position`,
		table,
	)
	if err != nil {
		return nil, fmt.Errorf("introspectColumns %q: %w", table, err)
	}
	defer rows.Close()

	var cols []columnInfo
	for rows.Next() {
		var col columnInfo
		if err := rows.Scan(&col.Name, &col.UDTName); err != nil {
			return nil, fmt.Errorf("introspectColumns %q scan: %w", table, err)
		}
		cols = append(cols, col)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspectColumns %q iteration: %w", table, err)
	}
	return cols, nil
}

// isDenied reports whether the given column of the given table must be excluded
// from a GDPR export. The deny-list is defined in gdpr_columns.go.
func isDenied(table, column string) bool {
	denied, ok := ExportDenyColumns[table]
	if !ok {
		return false
	}
	for _, d := range denied {
		if d == column {
			return true
		}
	}
	return false
}

// buildExportQuery constructs a SELECT statement for the given table that:
//   - skips denied columns (ExportDenyColumns)
//   - rewrites GEOMETRY columns as ST_AsGeoJSON(col)::jsonb AS col
//
// The returned []columnInfo reflects only the active (non-denied) columns,
// with GEOMETRY columns having their UDTName overridden to "jsonb".
//
// SECURITY: table and column names come from GDPRRegistry (hardcoded Go map)
// and information_schema (DB-internal). No user input flows into these values.
// pgx.Identifier.Sanitize() is used as defense-in-depth against future misuse.
func buildExportQuery(cols []columnInfo, table, where string) (string, []columnInfo) {
	var (
		selects    []string
		activeCols []columnInfo
	)

	for _, col := range cols {
		if isDenied(table, col.Name) {
			continue
		}
		if col.UDTName == "geometry" {
			// Convert PostGIS geometry to GeoJSON so it is JSON-serialisable.
			sanitized := pgx.Identifier{col.Name}.Sanitize()
			selects = append(selects, fmt.Sprintf("ST_AsGeoJSON(%s)::jsonb AS %s", sanitized, sanitized))
			activeCols = append(activeCols, columnInfo{Name: col.Name, UDTName: "jsonb"})
		} else {
			selects = append(selects, pgx.Identifier{col.Name}.Sanitize())
			activeCols = append(activeCols, col)
		}
	}

	query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(selects, ", "), pgx.Identifier{table}.Sanitize())
	if where != "" {
		query += " WHERE " + where
	}
	return query, activeCols
}

// exportTable streams all rows of `table` matching `where` as a JSON array
// into a ZIP entry at zipPath. Column metadata MUST be provided in `cols` —
// the function does NOT introspect the schema itself. This decoupling is
// deliberate: introspection happens once via the connection pool BEFORE the
// export transaction begins, so a failed data query inside the TX cannot
// poison subsequent information_schema reads (SQLSTATE 25P02 cascade).
//
// Returns the number of rows written and an error. A non-nil error means the
// table's data is incomplete or missing entirely; the worker collects these
// errors and surfaces them via background_jobs.metadata.export_errors.
//
// The where clause and args follow the same security contract as buildExportQuery:
// they originate from internal, hardcoded call sites — never from user input.
func exportTable(ctx context.Context, zw *zip.Writer, q querier, zipPath, table, where string, cols []columnInfo, args ...any) (int, error) {
	if len(cols) == 0 {
		// Empty cols means introspection failed pre-TX. The error is already
		// in the worker's collected list; we skip the entry without creating
		// a misleading empty `[]` file.
		slog.Warn("gdpr export: no columns cached, skipping", "table", table)
		return 0, nil
	}

	query, activeCols := buildExportQuery(cols, table, where)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		slog.Error("gdpr export: query failed", "table", table, "error", err)
		return 0, fmt.Errorf("export %s: query: %w", table, err)
	}
	defer rows.Close()

	w, err := createZipEntry(zw, zipPath)
	if err != nil {
		slog.Error("gdpr export: create zip entry failed", "path", zipPath, "error", err)
		return 0, fmt.Errorf("export %s: create zip entry: %w", table, err)
	}

	// wrErr aggregates the first write error from the zip writer. All subsequent
	// writes are skipped once wrErr is set to avoid further corruption. Checked
	// at the end of the function; disk-full or internal-buffer failures in the
	// zip writer would otherwise produce syntactically invalid JSON silently.
	var wrErr error
	zipWrite := func(b []byte) {
		if wrErr != nil {
			return
		}
		if _, werr := w.Write(b); werr != nil {
			wrErr = werr
		}
	}

	zipWrite([]byte("["))
	count := 0
	row := make(map[string]any, len(activeCols))

	for rows.Next() {
		rawVals, err := rows.Values()
		if err != nil {
			slog.Error("gdpr export: row values failed", "table", table, "error", err)
			zipWrite([]byte("]"))
			return count, fmt.Errorf("export %s: row values: %w", table, err)
		}

		fds := rows.FieldDescriptions()
		clear(row)

		for i, col := range activeCols {
			if i >= len(rawVals) {
				break
			}
			val := rawVals[i]
			if val == nil {
				row[col.Name] = nil
				continue
			}

			// OID-based type dispatch using the wire OID from the field description.
			if i < len(fds) {
				switch fds[i].DataTypeOID {
				case pgtype.JSONBOID, pgtype.JSONOID:
					// Preserve JSON as raw message to avoid double-encoding.
					switch v := val.(type) {
					case []byte:
						row[col.Name] = json.RawMessage(v)
					case string:
						row[col.Name] = json.RawMessage(v)
					default:
						// pgx may already have decoded it to map/slice — re-encode.
						if b, encErr := json.Marshal(v); encErr == nil {
							row[col.Name] = json.RawMessage(b)
						} else {
							row[col.Name] = v
						}
					}
					continue
				case pgtype.ByteaOID:
					// Encode binary as lowercase hex string.
					switch v := val.(type) {
					case []byte:
						row[col.Name] = hex.EncodeToString(v)
					default:
						row[col.Name] = val
					}
					continue
				case pgtype.UUIDOID:
					// pgx returns uuid columns as [16]byte. Without conversion the
					// JSON encoder serialises them as a 16-element integer array,
					// which is unreadable and unimportable.
					// Defensive future-proofing: a pgx TypeMap update may cause pgx
					// to decode UUIDs as pgtype.UUID (struct) instead of [16]byte.
					// Handle both to avoid {"Bytes":[…],"Valid":true} in the export.
					switch v := val.(type) {
					case [16]byte:
						row[col.Name] = formatUUID(v)
					case pgtype.UUID:
						if v.Valid {
							row[col.Name] = formatUUID(v.Bytes)
						} else {
							row[col.Name] = nil
						}
					case string:
						row[col.Name] = v
					default:
						row[col.Name] = val
					}
					continue
				}
			}

			// Default: pass through; encoding/json handles standard Go types.
			row[col.Name] = val
		}

		if count > 0 {
			zipWrite([]byte(","))
		}
		data, encErr := json.Marshal(row)
		if encErr != nil {
			slog.Error("gdpr export: encode row failed", "table", table, "error", encErr)
			zipWrite([]byte("]"))
			return count, fmt.Errorf("export %s: encode row: %w", table, encErr)
		}
		zipWrite(data)
		count++
	}

	zipWrite([]byte("]"))

	if err := rows.Err(); err != nil {
		slog.Error("gdpr export: rows iteration error", "table", table, "error", err)
		return count, fmt.Errorf("export %s: rows iteration: %w", table, err)
	}

	if wrErr != nil {
		return count, fmt.Errorf("export %s: write to zip: %w", table, wrErr)
	}

	return count, nil
}

// introspectAllTables introspects every table in `tables` via the connection
// pool (NOT a transaction). Failures are collected per-table; successful
// introspections are returned in the cache map. This must run BEFORE the
// export TX is opened — see exportTable doc for the rationale.
func introspectAllTables(ctx context.Context, pool querier, tables []string) (map[string][]columnInfo, []exportTableError) {
	cache := make(map[string][]columnInfo, len(tables))
	var errs []exportTableError
	for _, table := range tables {
		cols, err := introspectColumns(ctx, pool, table)
		if err != nil {
			slog.Error("gdpr export: introspect columns failed", "table", table, "error", err)
			errs = append(errs, exportTableError{Table: table, Error: err.Error()})
			continue
		}
		if len(cols) == 0 {
			slog.Warn("gdpr export: introspect returned no columns", "table", table)
			errs = append(errs, exportTableError{Table: table, Error: "no columns found in information_schema"})
			continue
		}
		cache[table] = cols
	}
	return cache, errs
}

// columnsMapToExported converts the introspected column cache into the
// {table: [col, ...]} shape used by columns.json. Denied columns are filtered
// out via isDenied; geometry columns retain their original name (the export
// data contains GeoJSON via ST_AsGeoJSON in buildExportQuery).
//
// The importer (Phase 3) must consult information_schema on the target DB
// to detect geometry columns and apply the GeoJSON→WKB conversion.
func columnsMapToExported(cache map[string][]columnInfo) map[string][]string {
	out := make(map[string][]string, len(cache))
	for table, cols := range cache {
		names := make([]string, 0, len(cols))
		for _, col := range cols {
			if isDenied(table, col.Name) {
				continue
			}
			names = append(names, col.Name)
		}
		if len(names) > 0 {
			out[table] = names
		}
	}
	return out
}
