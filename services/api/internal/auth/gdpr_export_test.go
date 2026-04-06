package auth

import (
	"strings"
	"testing"
)

func TestBuildExportQuery_Geometry(t *testing.T) {
	cols := []columnInfo{
		{Name: "id", UDTName: "uuid"},
		{Name: "center", UDTName: "geometry"},
		{Name: "name", UDTName: "text"},
	}

	query, activeCols := buildExportQuery(cols, "some_table", "user_id = $1")

	if !strings.Contains(query, `ST_AsGeoJSON("center")::jsonb AS "center"`) {
		t.Errorf("expected ST_AsGeoJSON rewrite for center, got: %s", query)
	}
	if len(activeCols) != 3 {
		t.Errorf("expected 3 active cols, got %d", len(activeCols))
	}
	if activeCols[1].UDTName != "jsonb" {
		t.Errorf("expected activeCols[1].UDTName == jsonb, got %q", activeCols[1].UDTName)
	}
}

func TestBuildExportQuery_DeniedColumns(t *testing.T) {
	cols := []columnInfo{
		{Name: "id", UDTName: "uuid"},
		{Name: "password_hash", UDTName: "text"},
		{Name: "username", UDTName: "text"},
	}

	query, activeCols := buildExportQuery(cols, "users", "id = $1")

	if strings.Contains(query, "password_hash") {
		t.Errorf("expected password_hash to be excluded, got: %s", query)
	}
	if len(activeCols) != 2 {
		t.Errorf("expected 2 active cols (id, username), got %d", len(activeCols))
	}
}

func TestBuildExportQuery_MultipleGeometry(t *testing.T) {
	cols := []columnInfo{
		{Name: "id", UDTName: "uuid"},
		{Name: "center", UDTName: "geometry"},
		{Name: "polygon", UDTName: "geometry"},
		{Name: "radius_m", UDTName: "float8"},
	}

	query, activeCols := buildExportQuery(cols, "user_privacy_zones", "user_id = $1")

	if !strings.Contains(query, `ST_AsGeoJSON("center")::jsonb AS "center"`) {
		t.Errorf("expected ST_AsGeoJSON rewrite for center, got: %s", query)
	}
	if !strings.Contains(query, `ST_AsGeoJSON("polygon")::jsonb AS "polygon"`) {
		t.Errorf("expected ST_AsGeoJSON rewrite for polygon, got: %s", query)
	}
	if len(activeCols) != 4 {
		t.Errorf("expected 4 active cols, got %d", len(activeCols))
	}
	// Both geometry cols should be rewritten to jsonb
	for _, col := range activeCols {
		if col.UDTName == "geometry" {
			t.Errorf("expected no raw geometry cols in activeCols, but found one: %q", col.Name)
		}
	}
}

func TestBuildExportQuery_NoSpecialCols(t *testing.T) {
	cols := []columnInfo{
		{Name: "id", UDTName: "uuid"},
		{Name: "name", UDTName: "text"},
		{Name: "color", UDTName: "text"},
	}

	query, activeCols := buildExportQuery(cols, "gear", "user_id = $1")

	if strings.Contains(query, "ST_AsGeoJSON") {
		t.Errorf("expected plain SELECT without ST_AsGeoJSON, got: %s", query)
	}
	if !strings.HasPrefix(query, `SELECT "id", "name", "color" FROM "gear"`) {
		t.Errorf("expected plain SELECT with all cols, got: %s", query)
	}
	if len(activeCols) != 3 {
		t.Errorf("expected 3 active cols, got %d", len(activeCols))
	}
}

func TestFormatUUID(t *testing.T) {
	// Standard UUID round-trip: known bytes → standard 8-4-4-4-12 string.
	tests := []struct {
		name string
		in   [16]byte
		want string
	}{
		{
			name: "all zeros",
			in:   [16]byte{},
			want: "00000000-0000-0000-0000-000000000000",
		},
		{
			name: "user dev (id=1)",
			in:   [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			want: "00000000-0000-0000-0000-000000000001",
		},
		{
			name: "all 0xff",
			in:   [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			want: "ffffffff-ffff-ffff-ffff-ffffffffffff",
		},
		{
			name: "mixed",
			in:   [16]byte{0xde, 0xad, 0xbe, 0xef, 0xca, 0xfe, 0xba, 0xbe, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0},
			want: "deadbeef-cafe-babe-1234-56789abcdef0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatUUID(tc.in)
			if got != tc.want {
				t.Errorf("formatUUID(%v) = %q, want %q", tc.in, got, tc.want)
			}
			if len(got) != 36 {
				t.Errorf("formatUUID length = %d, want 36", len(got))
			}
		})
	}
}

func TestIsDenied(t *testing.T) {
	// Sync with ExportDenyColumns in gdpr_columns.go.
	// totp_secret + recovery_codes are NOT in the deny-list because TOTP/2FA
	// is not yet implemented in Skadia — when those columns are added to
	// `users`, both the deny-list and this test must be updated.
	tests := []struct {
		table  string
		column string
		want   bool
	}{
		{"users", "password_hash", true},
		{"users", "totp_secret", false},   // not in schema yet, not in deny-list
		{"users", "recovery_codes", false}, // not in schema yet, not in deny-list
		{"users", "username", false},
		{"oauth_connections", "access_token", true},
		{"oauth_connections", "refresh_token", true},
		{"oauth_connections", "provider", false},
		{"gear", "name", false},
		{"gear", "password_hash", false}, // same column name, different (unlisted) table
	}

	for _, tc := range tests {
		got := isDenied(tc.table, tc.column)
		if got != tc.want {
			t.Errorf("isDenied(%q, %q) = %v, want %v", tc.table, tc.column, got, tc.want)
		}
	}
}
