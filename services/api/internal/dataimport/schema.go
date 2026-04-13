package dataimport

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// targetColumn holds column metadata read from information_schema.
type targetColumn struct {
	Name       string
	UDTName    string // geometry, jsonb, bytea, uuid, timestamptz, …
	IsNullable bool
	HasDefault bool
}

// introspectTarget reads column metadata for table from the target DB.
// Columns are returned in ordinal_position order.
func introspectTarget(ctx context.Context, pool *pgxpool.Pool, table string) ([]targetColumn, error) {
	const q = `
		SELECT column_name,
		       udt_name,
		       is_nullable = 'YES',
		       column_default IS NOT NULL
		FROM   information_schema.columns
		WHERE  table_schema = 'public'
		  AND  table_name   = $1
		ORDER  BY ordinal_position`

	rows, err := pool.Query(ctx, q, table)
	if err != nil {
		return nil, fmt.Errorf("introspectTarget %q: %w", table, err)
	}
	defer rows.Close()

	var cols []targetColumn
	for rows.Next() {
		var c targetColumn
		if err := rows.Scan(&c.Name, &c.UDTName, &c.IsNullable, &c.HasDefault); err != nil {
			return nil, fmt.Errorf("introspectTarget %q scan: %w", table, err)
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspectTarget %q rows: %w", table, err)
	}
	return cols, nil
}

// introspectAllTables reads column metadata for every import-relevant table.
// Skip-flagged tables are not queried.
func introspectAllTables(ctx context.Context, pool *pgxpool.Pool) (map[string][]targetColumn, error) {
	result := make(map[string][]targetColumn)

	for _, spec := range allTables() {
		if spec.Skip {
			continue
		}
		cols, err := introspectTarget(ctx, pool, spec.Name)
		if err != nil {
			return nil, err
		}
		result[spec.Name] = cols
	}

	for _, spec := range healthTables(false) {
		if spec.Skip {
			continue
		}
		if _, already := result[spec.Name]; already {
			continue
		}
		cols, err := introspectTarget(ctx, pool, spec.Name)
		if err != nil {
			return nil, err
		}
		result[spec.Name] = cols
	}

	return result, nil
}

// computeInsertColumns returns the intersection of the columns present in the
// export (exportCols) and the columns that actually exist in the target table
// (targetCols), preserving export order. It also returns the set of column
// names whose UDT is "geometry" so callers can wrap their values with
// ST_GeomFromGeoJSON.
func computeInsertColumns(exportCols []string, targetCols []targetColumn) (columns []string, geometryCols map[string]bool) {
	targetSet := make(map[string]targetColumn, len(targetCols))
	for _, tc := range targetCols {
		targetSet[tc.Name] = tc
	}

	geometryCols = make(map[string]bool)
	for _, col := range exportCols {
		tc, ok := targetSet[col]
		if !ok {
			continue
		}
		columns = append(columns, col)
		if tc.UDTName == "geometry" {
			geometryCols[col] = true
		}
	}
	return columns, geometryCols
}

// ---------------------------------------------------------------------------
// Transformer registry
// ---------------------------------------------------------------------------

// transformerKey identifies a transformer by the maximum export schema
// migration it is applicable to and the table it targets.
// A transformer is applied when the export's SchemaMigration <=
// transformerKey.MaxMigration.
type transformerKey struct {
	MaxMigration int
	Table        string
}

// transformers holds all registered row transformers, keyed by
// (MaxMigration, Table). Transformers are applied in an undefined order;
// register at most one transformer per key.
var transformers = map[transformerKey]func(row map[string]any) map[string]any{}

// applyTransformers applies all registered transformers whose Table matches
// table and whose MaxMigration >= exportMigration.
// Each matching transformer receives the row returned by the previous one.
func applyTransformers(exportMigration int, table string, row map[string]any) map[string]any {
	for key, fn := range transformers {
		if key.Table != table {
			continue
		}
		if exportMigration <= key.MaxMigration {
			row = fn(row)
		}
	}
	return row
}
