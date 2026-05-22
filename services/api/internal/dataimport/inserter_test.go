package dataimport

import (
	"bytes"
	"testing"
)

func TestConvertValueForTargetColumnBindsSQLArraysAsTypedSlices(t *testing.T) {
	tests := []struct {
		name    string
		udtName string
		value   any
		assert  func(t *testing.T, got any)
	}{
		{
			name:    "empty integer array",
			udtName: "_int4",
			value:   []any{},
			assert: func(t *testing.T, got any) {
				v, ok := got.([]int32)
				if !ok {
					t.Fatalf("type = %T, want []int32", got)
				}
				if len(v) != 0 {
					t.Fatalf("len = %d, want 0", len(v))
				}
			},
		},
		{
			name:    "text array",
			udtName: "_text",
			value:   []any{"mtb", "road"},
			assert: func(t *testing.T, got any) {
				v, ok := got.([]string)
				if !ok {
					t.Fatalf("type = %T, want []string", got)
				}
				if len(v) != 2 || v[0] != "mtb" || v[1] != "road" {
					t.Fatalf("value = %#v", v)
				}
			},
		},
		{
			name:    "real array",
			udtName: "_float4",
			value:   []any{1.5, 2.25},
			assert: func(t *testing.T, got any) {
				v, ok := got.([]float32)
				if !ok {
					t.Fatalf("type = %T, want []float32", got)
				}
				if len(v) != 2 || v[0] != 1.5 || v[1] != 2.25 {
					t.Fatalf("value = %#v", v)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertValueForTargetColumn(tt.value, "test_col", targetColumn{UDTName: tt.udtName})
			if err != nil {
				t.Fatalf("convertValueForTargetColumn: %v", err)
			}
			tt.assert(t, got)
		})
	}
}

func TestConvertValueForTargetColumnKeepsJSONArraysAsJSON(t *testing.T) {
	got, err := convertValueForTargetColumn([]any{"a", "b"}, "conditions", targetColumn{UDTName: "jsonb"})
	if err != nil {
		t.Fatalf("convertValueForTargetColumn: %v", err)
	}
	raw, ok := got.([]byte)
	if !ok {
		t.Fatalf("type = %T, want []byte", got)
	}
	if !bytes.Equal(raw, []byte(`["a","b"]`)) {
		t.Fatalf("json = %s", raw)
	}
}
