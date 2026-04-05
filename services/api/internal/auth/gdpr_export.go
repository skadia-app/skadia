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

// columnInfo holds the name and underlying PostgreSQL type name of a column.
type columnInfo struct {
	Name    string
	UDTName string // e.g. "int4", "text", "jsonb", "geometry", "bytea"
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

// exportTable introspects the table's columns, builds a safe SELECT query,
// streams all matching rows as a JSON array into a ZIP entry at zipPath,
// and returns the number of rows written.
//
// The where clause and args follow the same security contract as buildExportQuery:
// they originate from internal, hardcoded call sites — never from user input.
func exportTable(ctx context.Context, zw *zip.Writer, q querier, zipPath, table, where string, args ...any) int {
	cols, err := introspectColumns(ctx, q, table)
	if err != nil {
		slog.Error("gdpr export: introspect columns failed", "table", table, "error", err)
		return 0
	}
	if len(cols) == 0 {
		slog.Warn("gdpr export: no columns found, skipping", "table", table)
		return 0
	}

	query, activeCols := buildExportQuery(cols, table, where)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		slog.Error("gdpr export: query failed", "table", table, "error", err)
		return 0
	}
	defer rows.Close()

	w, err := createZipEntry(zw, zipPath)
	if err != nil {
		slog.Error("gdpr export: create zip entry failed", "path", zipPath, "error", err)
		return 0
	}

	w.Write([]byte("["))
	count := 0
	row := make(map[string]any, len(activeCols))

	for rows.Next() {
		rawVals, err := rows.Values()
		if err != nil {
			slog.Error("gdpr export: row values failed", "table", table, "error", err)
			continue
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
				}
			}

			// Default: pass through; encoding/json handles standard Go types.
			row[col.Name] = val
		}

		if count > 0 {
			w.Write([]byte(","))
		}
		data, encErr := json.Marshal(row)
		if encErr != nil {
			slog.Error("gdpr export: encode row failed", "table", table, "error", encErr)
		} else {
			w.Write(data)
		}
		count++
	}

	w.Write([]byte("]"))

	if err := rows.Err(); err != nil {
		slog.Error("gdpr export: rows iteration error", "table", table, "error", err)
	}

	return count
}

// exportColumns returns the list of exportable column names for table —
// denied columns are excluded; geometry columns are included by their original
// name (e.g. "center").
//
// NOTE: Geometry columns are listed by their original name here, but the actual
// export data contains GeoJSON (produced via ST_AsGeoJSON in buildExportQuery).
// The importer (Phase 3) must detect geometry columns via information_schema
// when importing and handle the GeoJSON→WKB conversion accordingly.
func exportColumns(ctx context.Context, q querier, table string) []string {
	cols, err := introspectColumns(ctx, q, table)
	if err != nil {
		slog.Error("gdpr export: exportColumns introspect failed", "table", table, "error", err)
		return nil
	}

	names := make([]string, 0, len(cols))
	for _, col := range cols {
		if isDenied(table, col.Name) {
			continue
		}
		names = append(names, col.Name)
	}
	return names
}
