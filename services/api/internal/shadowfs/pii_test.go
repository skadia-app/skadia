// services/api/internal/shadowfs/pii_test.go
package shadowfs

import "testing"

func TestContainsPIIVariable(t *testing.T) {
	tests := []struct {
		name   string
		schema string
		want   string
	}{
		{"clean schema", "{year}/{month}/{title}", ""},
		{"notes rejected", "{year}/{notes}", "notes"},
		{"description rejected", "{description}/{title}", "description"},
		{"hr_avg rejected", "{year}-{hr_avg}", "hr_avg"},
		{"power_avg rejected", "{power_avg}", "power_avg"},
		{"case-sensitive Notes ok", "{year}/{Notes}", ""},
		{"notes_extended ok (no exact match)", "{year}/{notes_extended}", ""},
		{"multiple pii returns first", "{notes}-{description}", "notes"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ContainsPIIVariable(tc.schema); got != tc.want {
				t.Errorf("ContainsPIIVariable(%q) = %q; want %q", tc.schema, got, tc.want)
			}
		})
	}
}
