package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5"
)

var sqlBlocklist = regexp.MustCompile(`(?i)\b(pg_catalog|pg_stat|pg_read_file|lo_import|information_schema|COPY|DO\s+\$|pg_sleep|set\s+role|set\s+session)\b`)
var sqlPlaceholder = regexp.MustCompile(`\$\d+`)
var sqlUserIDFilter = regexp.MustCompile(`(?i)\buser_id\s*=`)
var sqlRoleKeywords = regexp.MustCompile(`(?i)\b(current_user|session_user|current_role)\b`)
var sqlWriteKeywords = regexp.MustCompile(`(?i)^\s*(insert|update|delete|drop|alter|create)\b`)
var sqlRoundCall = regexp.MustCompile(`(?i)ROUND\(([^()]+(?:\([^()]*\))?[^()]*),\s*(\d+)\)`)
var sqlGeoElevation = regexp.MustCompile(`\b(agt?|gt)\.elevation\b`)

var TableWhitelist = map[string]bool{
	"activities": true, "activity_laps": true, "activity_splits": true,
	"activity_geo_tags": true, "activity_stops": true, "activity_rr_intervals": true, "activity_photos": true,
	"activity_gear_assignments": true,
	"health_snapshots":          true, "health_sleep": true, "health_weight_entries": true,
	"gear": true, "gear_components": true, "segment_efforts": true,
	"pmc_daily": true, "user_daily_features": true, "tags": true, "activity_tags": true,
	"osm_pois": true, "osm_ways": true, "osm_pistes": true, "osm_glaciers": true, "osm_wetlands": true, "osm_routes": true,
	"osm_import_regions": true,
}

type SQLAgent struct {
	db *database.DB
}

func NewSQLAgent(db *database.DB) *SQLAgent {
	return &SQLAgent{db: db}
}

func ValidateSQL(sql string) error {
	if sqlBlocklist.MatchString(sql) {
		return fmt.Errorf("system table or function access is not allowed")
	}
	if sqlPlaceholder.MatchString(sql) {
		return fmt.Errorf("parameter placeholders are not allowed")
	}
	if sqlUserIDFilter.MatchString(sql) {
		return fmt.Errorf("user_id filters are not allowed; RLS handles user isolation")
	}
	if sqlRoleKeywords.MatchString(sql) {
		return fmt.Errorf("current_user/session_user/current_role are not allowed")
	}
	if sqlWriteKeywords.MatchString(sql) {
		return fmt.Errorf("only SELECT queries are allowed")
	}

	analysis, err := AnalyzeSQL(sql)
	if err != nil {
		return err
	}
	if analysis.MultiStatement {
		return fmt.Errorf("only a single statement is allowed")
	}
	if !isKeyword(analysis.FirstToken, "select") && !isKeyword(analysis.FirstToken, "with") {
		return fmt.Errorf("only SELECT queries are allowed")
	}
	if len(analysis.UnsafeKeywords) > 0 {
		return fmt.Errorf("only read-only SELECT queries are allowed")
	}
	if len(analysis.UnsafeFunctions) > 0 {
		return fmt.Errorf("session or configuration function access is not allowed")
	}
	for _, table := range analysis.Tables {
		if !TableWhitelist[strings.ToLower(table)] {
			return fmt.Errorf("table is not allowed: %s", table)
		}
	}
	return nil
}

func AutofixSQL(sql string) string {
	fixed := sql
	prev := ""
	for fixed != prev {
		prev = fixed
		fixed = sqlRoundCall.ReplaceAllStringFunc(fixed, func(match string) string {
			parts := sqlRoundCall.FindStringSubmatch(match)
			if len(parts) != 3 || strings.Contains(strings.ToLower(parts[1]), "::numeric") {
				return match
			}
			return fmt.Sprintf("ROUND((%s)::numeric, %s)", strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2]))
		})
	}
	fixed = regexp.MustCompile(`\bduration_s\b`).ReplaceAllString(fixed, "elapsed_time_s")
	fixed = sqlGeoElevation.ReplaceAllString(fixed, "$1.ele")
	return fixed
}

func (a *SQLAgent) ExecuteSQL(ctx context.Context, sql, userID string, maxRows int) ([]map[string]any, error) {
	if a == nil || a.db == nil || a.db.ReadonlyPool == nil {
		return nil, fmt.Errorf("readonly database pool is not configured")
	}
	if maxRows <= 0 {
		maxRows = 100
	}

	sql = AutofixSQL(sql)
	if err := ValidateSQL(sql); err != nil {
		return nil, fmt.Errorf("SQL validation failed: %w", err)
	}

	tx, err := a.db.ReadonlyPool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := verifyReadonlyRole(ctx, tx); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_user_id', $1, true)", userID); err != nil {
		return nil, fmt.Errorf("set local rls context: %w", err)
	}

	rows, err := tx.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result, err := collectRows(rows, maxRows+1)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if len(result) > maxRows {
		result = result[:maxRows]
	}
	return result, nil
}

func verifyReadonlyRole(ctx context.Context, tx pgx.Tx) error {
	var currentUser string
	if err := tx.QueryRow(ctx, "SELECT current_user").Scan(&currentUser); err != nil {
		return fmt.Errorf("verify readonly role: %w", err)
	}
	if currentUser != "skadia_readonly" {
		return fmt.Errorf("readonly database pool uses role %q, want skadia_readonly", currentUser)
	}
	return nil
}

func collectRows(rows pgx.Rows, maxRows int) ([]map[string]any, error) {
	fields := rows.FieldDescriptions()
	var result []map[string]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, len(fields))
		for i, field := range fields {
			row[field.Name] = values[i]
		}
		result = append(result, row)
		if maxRows > 0 && len(result) >= maxRows {
			rows.Close()
			return result, rows.Err()
		}
	}
	return result, rows.Err()
}
