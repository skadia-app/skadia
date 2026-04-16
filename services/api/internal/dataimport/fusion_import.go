package dataimport

// fusion_import.go — Task 36 backwards-compat helpers for fusion membership import.
//
// Three export shapes are handled:
//   v_new         — export has data/activity_fusion_memberships.json (format ≥ 302)
//   v_old_fusion  — export has activities with fusion_sources JSONB but no
//                   memberships file (pre-302 exports)
//   v_regular     — no fusion at all (standard activities)
//
// The per-activity ingest loop in engine.go uses the generic importTable path
// for the memberships file (Phase 6, table "activity_fusion_memberships").
// These helpers are additional utilities used by the import engine and tests.

import (
	"encoding/json"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
)

// rawMembership mirrors the JSON export shape of activity_fusion_memberships.
type rawMembership struct {
	MergedID string `json:"merged_id"`
	SourceID string `json:"source_id"`
	FieldKey string `json:"field_key"`
}

// ReadMembershipsJSON parses data/activity_fusion_memberships.json from a
// GDPR export Source.  Memberships that reference activities outside
// importedActivityIDs (cross-user tampering / corrupted export) are dropped.
// UserID on all returned rows is always overwritten to importUserID.
// Returns nil (not an error) when the file is absent.
func ReadMembershipsJSON(src Source, importUserID string, importedActivityIDs map[string]bool) []database.FusionMembership {
	rc, err := src.OpenData("data/activity_fusion_memberships.json")
	if err != nil {
		// File absent — older export.
		return nil
	}
	defer rc.Close()

	var raw []rawMembership
	if err := json.NewDecoder(rc).Decode(&raw); err != nil {
		slog.Warn("dataimport: activity_fusion_memberships.json parse failed", "error", err)
		return nil
	}

	out := make([]database.FusionMembership, 0, len(raw))
	for _, r := range raw {
		if !importedActivityIDs[r.MergedID] || !importedActivityIDs[r.SourceID] {
			slog.Warn("dataimport: membership references activity outside import scope — skipping",
				"merged_id", r.MergedID, "source_id", r.SourceID)
			continue
		}
		out = append(out, database.FusionMembership{
			MergedID: r.MergedID,
			SourceID: r.SourceID,
			FieldKey: r.FieldKey,
			UserID:   importUserID,
		})
	}
	return out
}

// DeriveFromMetaSnapshot reconstructs fusion memberships from a .meta sidecar's
// FusionMembers snapshot. Used as a last-resort when the export has no
// memberships JSON file and no legacy fusion_sources JSONB.
// All returned memberships use mergedID and importUserID.
func DeriveFromMetaSnapshot(members []sidecar.FusionMemberSnapshot, mergedID, importUserID string) []database.FusionMembership {
	if len(members) == 0 {
		return nil
	}
	out := make([]database.FusionMembership, 0, len(members))
	for _, m := range members {
		if m.SourceID == "" || m.FieldKey == "" {
			continue
		}
		out = append(out, database.FusionMembership{
			MergedID: mergedID,
			SourceID: m.SourceID,
			FieldKey: m.FieldKey,
			UserID:   importUserID,
		})
	}
	return out
}

// FilterByMergedID returns only the memberships where MergedID == mergedID.
func FilterByMergedID(memberships []database.FusionMembership, mergedID string) []database.FusionMembership {
	var out []database.FusionMembership
	for _, m := range memberships {
		if m.MergedID == mergedID {
			out = append(out, m)
		}
	}
	return out
}

// MembershipsEqual does an order-independent comparison of two membership slices.
func MembershipsEqual(a, b []database.FusionMembership) bool {
	if len(a) != len(b) {
		return false
	}
	key := func(m database.FusionMembership) string {
		return m.MergedID + "|" + m.SourceID + "|" + m.FieldKey
	}
	seen := make(map[string]int, len(a))
	for _, m := range a {
		seen[key(m)]++
	}
	for _, m := range b {
		seen[key(m)]--
	}
	for _, v := range seen {
		if v != 0 {
			return false
		}
	}
	return true
}
