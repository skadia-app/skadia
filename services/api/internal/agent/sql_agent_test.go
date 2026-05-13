package agent

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestValidateSQLRejectsUnsafeQueries(t *testing.T) {
	tests := []string{
		"SELECT * FROM pg_catalog.pg_tables",
		"SELECT * FROM users",
		"SELECT * FROM activities a, users u",
		"SELECT * FROM activity_streams",
		"SELECT count(*) FROM analytics_cache",
		"SELECT * FROM segments",
		"SELECT * FROM segment_versions",
		"SELECT se.id FROM segment_efforts se JOIN segments s ON s.id = se.segment_id",
		"SELECT 1; SELECT 2",
		"SELECT * FROM activities WHERE user_id = '00000000-0000-0000-0000-000000000000'",
		"SELECT current_user",
		"SELECT current_setting('app.current_user_id')",
		"SELECT current_setting('skadia.skip_system_modifier_tag_sync', true)",
		"SELECT set_config('app.current_user_id', '00000000-0000-0000-0000-000000000000', true)",
		"SELECT pg_advisory_lock(42)",
		"SELECT pg_try_advisory_xact_lock(42)",
		"SELECT nextval('activities_id_seq')",
		"SELECT currval('activities_id_seq')",
		"SELECT lo_create(1234)",
		"SELECT lo_unlink(1234)",
		"SET LOCAL app.current_user_id = '00000000-0000-0000-0000-000000000000'",
		"RESET app.current_user_id",
		"SHOW app.current_user_id",
		"SELECT * FROM activities WHERE id = $1",
		"INSERT INTO activities (id) VALUES ('00000000-0000-0000-0000-000000000000')",
		"UPDATE activities SET title = 'x'",
		"WITH changed AS (UPDATE activities SET title = 'x' RETURNING id) SELECT id FROM changed",
		"WITH deleted AS (DELETE FROM activities RETURNING id) SELECT id FROM deleted",
	}
	for _, sql := range tests {
		t.Run(sql, func(t *testing.T) {
			if err := ValidateSQL(sql); err == nil {
				t.Fatal("ValidateSQL returned nil, want error")
			}
		})
	}
}

func TestTableWhitelistExcludesKnownUnscopedTables(t *testing.T) {
	for _, table := range []string{"segments", "segment_versions", "activity_streams", "analytics_cache", "activity_lift_candidates", "users"} {
		if TableWhitelist[table] {
			t.Fatalf("TableWhitelist[%q] = true, want false", table)
		}
	}
}

func TestTableWhitelistExcludesUngrantedOSMTables(t *testing.T) {
	for _, table := range []string{"osm_lifts", "osm_waterways", "osm_waters", "osm_forests", "osm_boundaries"} {
		if TableWhitelist[table] {
			t.Fatalf("TableWhitelist[%q] = true, want false", table)
		}
	}
}

func TestValidateSQLAllowsReadOnlyRLSTables(t *testing.T) {
	tests := []string{
		"SELECT id FROM activities",
		"SELECT activity_id, start_time FROM activity_laps",
		"SELECT id, name FROM gear",
		"SELECT id, name FROM tags",
		"SELECT at.activity_id, t.name FROM activity_tags at JOIN tags t ON t.id = at.tag_id",
	}
	for _, sql := range tests {
		t.Run(sql, func(t *testing.T) {
			if err := ValidateSQL(sql); err != nil {
				t.Fatalf("ValidateSQL: %v", err)
			}
		})
	}
}

func TestValidateSQLAllowsWhitelistedCTEAndUnion(t *testing.T) {
	sql := `WITH recent AS (SELECT id FROM activities) SELECT id FROM recent UNION SELECT id FROM gear`
	if err := ValidateSQL(sql); err != nil {
		t.Fatalf("ValidateSQL: %v", err)
	}
}

func TestAutofixSQL(t *testing.T) {
	got := AutofixSQL(`SELECT ROUND(SUM(duration_s)/3600.0, 1), agt.elevation FROM activity_geo_tags agt`)
	if !strings.Contains(got, "elapsed_time_s") {
		t.Fatalf("autofix did not rewrite duration_s: %s", got)
	}
	if !strings.Contains(got, "agt.ele") {
		t.Fatalf("autofix did not rewrite agt.elevation: %s", got)
	}
	if !strings.Contains(strings.ToLower(got), "::numeric") {
		t.Fatalf("autofix did not cast ROUND arg to numeric: %s", got)
	}
}

func TestSQLAgentExecuteUsesReadonlyPoolAndSlicesRows(t *testing.T) {
	ctx := context.Background()
	db := openSQLAgentTestDB(t)
	agent := NewSQLAgent(db)

	rows, err := agent.ExecuteSQL(ctx, `SELECT generate_series(1, 200) AS n`, "00000000-0000-0000-0000-000000000123", 100)
	if err != nil {
		t.Fatalf("ExecuteSQL: %v", err)
	}
	if len(rows) != 100 {
		t.Fatalf("row count = %d, want 100", len(rows))
	}
	if rows[0]["n"] == nil {
		t.Fatalf("first row missing n: %#v", rows[0])
	}
}

func TestCollectRowsStopsAtRequestedLimit(t *testing.T) {
	rows := &countingRows{total: 1000}

	got, err := collectRows(rows, 101)
	if err != nil {
		t.Fatalf("collectRows: %v", err)
	}
	if len(got) != 101 {
		t.Fatalf("row count = %d, want 101", len(got))
	}
	if rows.valuesCalls != 101 {
		t.Fatalf("materialized rows = %d, want 101", rows.valuesCalls)
	}
	if rows.nextCalls != 101 {
		t.Fatalf("Next calls = %d, want 101", rows.nextCalls)
	}
	if !rows.closed {
		t.Fatal("rows were not closed after reaching the collection limit")
	}
}

func TestSQLAgentExecuteRejectsValidatorFailures(t *testing.T) {
	ctx := context.Background()
	db := openSQLAgentTestDB(t)
	agent := NewSQLAgent(db)

	if _, err := agent.ExecuteSQL(ctx, `SELECT * FROM users`, "00000000-0000-0000-0000-000000000123", 100); err == nil {
		t.Fatal("ExecuteSQL returned nil error, want validator error")
	}
}

type countingRows struct {
	total       int
	nextCalls   int
	valuesCalls int
	closed      bool
}

func (r *countingRows) Close() {
	r.closed = true
}

func (r *countingRows) Err() error {
	return nil
}

func (r *countingRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (r *countingRows) FieldDescriptions() []pgconn.FieldDescription {
	return []pgconn.FieldDescription{{Name: "n"}}
}

func (r *countingRows) Next() bool {
	if r.closed || r.nextCalls >= r.total {
		r.closed = true
		return false
	}
	r.nextCalls++
	return true
}

func (r *countingRows) Scan(dest ...any) error {
	return nil
}

func (r *countingRows) Values() ([]any, error) {
	r.valuesCalls++
	return []any{r.valuesCalls}, nil
}

func (r *countingRows) RawValues() [][]byte {
	return nil
}

func (r *countingRows) Conn() *pgx.Conn {
	return nil
}

func openSQLAgentTestDB(t *testing.T) *database.DB {
	t.Helper()
	ctx := context.Background()
	dsn := testdb.RequireSafeDSN(t)
	db, err := database.New(ctx, dsn)
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(db.Close)
	if err := db.OpenReadonly(ctx, readonlyDSN(t, dsn)); err != nil {
		t.Fatalf("OpenReadonly: %v", err)
	}
	return db
}

func readonlyDSN(t *testing.T, dsn string) string {
	t.Helper()
	if _, err := pgx.ParseConfig(dsn); err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn url: %v", err)
	}
	u.User = url.UserPassword("skadia_readonly", "skadia_readonly_password")
	return u.String()
}
