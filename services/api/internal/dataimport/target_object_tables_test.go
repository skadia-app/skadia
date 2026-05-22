package dataimport_test

import (
	"slices"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/auth"
	"github.com/skadia-app/skadia/services/api/internal/dataimport"
)

func TestDataImportRegistersTargetObjectTables(t *testing.T) {
	specsByName := make(map[string]struct {
		Phase   int
		UserCol string
	})
	for _, spec := range dataimport.ExportedTableSpecs() {
		specsByName[spec.Name] = struct {
			Phase   int
			UserCol string
		}{Phase: spec.Phase, UserCol: spec.UserCol}
	}

	for _, tc := range []struct {
		table   string
		phase   int
		userCol string
	}{
		{table: "target_places", phase: 3, userCol: "owner_user_id"},
		{table: "target_regions", phase: 3, userCol: "owner_user_id"},
		{table: "target_collections", phase: 3, userCol: "owner_user_id"},
		{table: "target_collection_members", phase: 4, userCol: "added_by"},
		{table: "target_activity_observations", phase: 7, userCol: "user_id"},
		{table: "target_reports", phase: 7, userCol: "author_user_id"},
		{table: "target_user_content", phase: 7, userCol: "user_id"},
	} {
		got, ok := specsByName[tc.table]
		if !ok {
			t.Fatalf("%s missing from dataimport table specs", tc.table)
		}
		if got.Phase != tc.phase || got.UserCol != tc.userCol {
			t.Fatalf("%s spec phase/userCol = %d/%q, want %d/%q", tc.table, got.Phase, got.UserCol, tc.phase, tc.userCol)
		}
		if !slices.Contains(auth.ExportedTables, tc.table) {
			t.Fatalf("%s missing from GDPR export registry", tc.table)
		}
	}

	skipped := dataimport.ImportSkippedTableRationalesForGDPRGuard()
	for _, table := range []string{
		"governance_scopes",
		"governance_actor_roles",
		"governance_scope_target_bindings",
		"target_change_events",
		"target_report_status_events",
		"target_revision_snapshots",
		"governance_actor_target_projection",
		"target_object_search_projection",
		"target_collection_summary_projection",
		"target_collection_member_reverse_index",
	} {
		if skipped[table] == "" {
			t.Fatalf("%s missing skipped-table rationale", table)
		}
	}
}
