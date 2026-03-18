package osm

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsCorruptionError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"regular error", fmt.Errorf("something failed"), false},
		{"PgError non-corruption", &pgconn.PgError{Code: "42P10"}, false},
		{"PgError XX001 data_corrupted", &pgconn.PgError{Code: "XX001"}, true},
		{"PgError XX002 index_corrupted", &pgconn.PgError{Code: "XX002"}, true},
		{"wrapped PgError corruption", fmt.Errorf("clear osm_pois: %w", &pgconn.PgError{Code: "XX001"}), true},
		{"wrapped PgError non-corruption", fmt.Errorf("query failed: %w", &pgconn.PgError{Code: "23505"}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCorruptionError(tt.err); got != tt.want {
				t.Errorf("isCorruptionError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractCorruptedTable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil error", nil, ""},
		{"regular error", fmt.Errorf("something failed"), ""},
		{"PgError with TableName", &pgconn.PgError{Code: "XX001", TableName: "osm_boundaries"}, "osm_boundaries"},
		{"PgError without TableName", &pgconn.PgError{Code: "XX001"}, ""},
		{"clear prefix", fmt.Errorf("clear osm_pois: %s", "some error"), "osm_pois"},
		{"import prefix", fmt.Errorf("import osm_ways: %s", "some error"), "osm_ways"},
		{"wrapped PgError with TableName", fmt.Errorf("clear osm_forests: %w", &pgconn.PgError{Code: "XX001", TableName: "osm_forests"}), "osm_forests"},
		{"no matching prefix", fmt.Errorf("unknown error format"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractCorruptedTable(tt.err); got != tt.want {
				t.Errorf("extractCorruptedTable() = %v, want %v", got, tt.want)
			}
		})
	}
}
