package agent

import (
	"reflect"
	"testing"
)

func TestAnalyzeSQLTokenizer(t *testing.T) {
	tests := []struct {
		name           string
		sql            string
		wantTables     []string
		wantCTEs       []string
		wantMultiStmt  bool
		wantTokenError bool
	}{
		{name: "simple select", sql: `SELECT * FROM activities`, wantTables: []string{"activities"}},
		{name: "join", sql: `SELECT * FROM activities a JOIN gear g ON g.id = a.gear_id`, wantTables: []string{"activities", "gear"}},
		{name: "comma separated from", sql: `SELECT * FROM activities a, users u WHERE u.id = a.user_id`, wantTables: []string{"activities", "users"}},
		{name: "comma after join expression", sql: `SELECT * FROM activities a JOIN gear g ON true, users u`, wantTables: []string{"activities", "gear", "users"}},
		{name: "multi statement", sql: `SELECT 1; SELECT 2`, wantMultiStmt: true},
		{name: "semicolon in string", sql: `SELECT ';' AS semi FROM activities`, wantTables: []string{"activities"}},
		{name: "dollar quoted block", sql: `SELECT $$;$$ AS semi FROM activities`, wantTables: []string{"activities"}},
		{name: "line comment stripped", sql: "-- FROM gear\nSELECT * FROM activities", wantTables: []string{"activities"}},
		{name: "block comment stripped", sql: "/* FROM gear */ SELECT * FROM activities", wantTables: []string{"activities"}},
		{name: "cte exempted", sql: `WITH recent AS (SELECT * FROM activities) SELECT * FROM recent`, wantTables: []string{"activities"}, wantCTEs: []string{"recent"}},
		{name: "subquery skipped", sql: `SELECT * FROM (SELECT * FROM activities) AS t`, wantTables: []string{"activities"}},
		{name: "lateral join parsed", sql: `SELECT * FROM activities a JOIN LATERAL (SELECT * FROM gear g WHERE g.id = a.gear_id) x ON true`, wantTables: []string{"activities", "gear"}},
		{name: "union", sql: `SELECT * FROM activities UNION SELECT * FROM gear`, wantTables: []string{"activities", "gear"}},
		{name: "case insensitive", sql: " select * \n FrOm activities \t JoIn gear on true ", wantTables: []string{"activities", "gear"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AnalyzeSQL(tt.sql)
			if tt.wantTokenError {
				if err == nil {
					t.Fatal("AnalyzeSQL returned nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("AnalyzeSQL: %v", err)
			}
			if got.MultiStatement != tt.wantMultiStmt {
				t.Fatalf("MultiStatement = %v, want %v", got.MultiStatement, tt.wantMultiStmt)
			}
			if !reflect.DeepEqual(got.Tables, tt.wantTables) {
				t.Fatalf("Tables = %#v, want %#v", got.Tables, tt.wantTables)
			}
			if !reflect.DeepEqual(got.CTENames, tt.wantCTEs) {
				t.Fatalf("CTENames = %#v, want %#v", got.CTENames, tt.wantCTEs)
			}
		})
	}
}
