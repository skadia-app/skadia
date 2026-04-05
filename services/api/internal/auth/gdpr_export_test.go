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

func TestIsDenied(t *testing.T) {
	tests := []struct {
		table  string
		column string
		want   bool
	}{
		{"users", "password_hash", true},
		{"users", "totp_secret", true},
		{"users", "recovery_codes", true},
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
