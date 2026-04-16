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
//
// v_old_fusion wiring:
//   engine.go calls RunLegacyFusionMembershipsIfNeeded after phase 6 completes.
//   It re-reads data/activities.json, extracts fusion_sources JSONB, and
//   calls DeriveFromLegacyFusionSources to produce memberships that are then
//   inserted via database.InsertFusionMemberships.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

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

// DeriveFromLegacyFusionSources converts a pre-000302 fusion_sources JSONB value
// into activity_fusion_memberships rows for a single merged activity.
//
//	fusionSources:        the JSONB map {field_key -> source_id | "fuse"}
//	mergedID:             the activity owning the fusion_sources field (new/remapped UUID)
//	parentIDChildren:     other new activity IDs in import scope that had parent_id == mergedID
//	                      in the old export (used for "fuse" fan-out expansion;
//	                      pass nil if not derivable — V1 parent_id info is absent)
//	importUserID:         the new user's UUID (membership's user_id field)
//	importedActivityIDs:  all new activity IDs in import scope (source_id must be in this set)
//
// Returns the derived memberships. Warnings are logged for "fuse" literals that
// cannot be expanded (empty parentIDChildren) — lost information from V1 exports.
func DeriveFromLegacyFusionSources(
	fusionSources map[string]string,
	mergedID string,
	parentIDChildren []string,
	importUserID string,
	importedActivityIDs map[string]bool,
) []database.FusionMembership {
	out := make([]database.FusionMembership, 0, len(fusionSources))
	for fieldKey, value := range fusionSources {
		if value == "fuse" {
			if len(parentIDChildren) == 0 {
				slog.Warn("dataimport: fusion_sources fuse-literal could not be expanded (no parent_id children in scope)",
					"merged_id", mergedID, "field_key", fieldKey)
				continue
			}
			for _, childID := range parentIDChildren {
				if !importedActivityIDs[childID] {
					slog.Warn("dataimport: fuse-expansion child outside import scope — skipping",
						"merged_id", mergedID, "child_id", childID)
					continue
				}
				out = append(out, database.FusionMembership{
					MergedID: mergedID, SourceID: childID,
					FieldKey: fieldKey, UserID: importUserID,
				})
			}
			continue
		}
		// UUID value (concrete source_id). This is a new/remapped UUID already.
		if !importedActivityIDs[value] {
			slog.Warn("dataimport: fusion_sources concrete source outside import scope — skipping",
				"merged_id", mergedID, "source_id", value, "field_key", fieldKey)
			continue
		}
		out = append(out, database.FusionMembership{
			MergedID: mergedID, SourceID: value,
			FieldKey: fieldKey, UserID: importUserID,
		})
	}
	return out
}

// legacyActivityRecord holds the fields extracted from a single activity row in
// a pre-302 export that are needed for v_old_fusion derivation.
type legacyActivityRecord struct {
	OldID         string
	OldParentID   string
	FusionSources map[string]string // nil if absent
}

// RunLegacyFusionMembershipsIfNeeded implements the v_old_fusion backwards-compat
// path described in Spec §7.2.
//
// It is a no-op when the export already contains data/activity_fusion_memberships.json
// (v_new path — memberships are imported by the generic phase 6 table handler).
//
// When the memberships file is absent, it re-reads data/activities.json,
// extracts fusion_sources JSONB from each row, maps old UUIDs to new ones via
// idMap, and inserts the derived memberships via database.InsertFusionMemberships.
//
// exportMigration is the SchemaMigration value from the manifest; for exports
// older than 302 the fusion_sources column is expected to be present when
// fusion was in use.
func RunLegacyFusionMembershipsIfNeeded(
	ctx context.Context,
	db *pgxpool.Pool,
	src Source,
	idMap *IDMap,
	importUserID string,
	exportMigration int,
) error {
	// v_new check: if the memberships file exists, the generic importer already
	// handled it — nothing to do.
	rc, err := src.OpenData("data/activity_fusion_memberships.json")
	if err == nil {
		rc.Close()
		return nil // v_new — handled by phase 6 generic importTable
	}
	// Only treat "file absent" as the legitimate v_old path.
	// IO errors, permission issues, or corruption must propagate — silently
	// falling through would skip derivation without telling the caller.
	if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("dataimport: check memberships.json: %w", err)
	}
	// fs.ErrNotExist → legitimate v_old path, continue below.

	// Re-read activities.json to collect fusion_sources + parent_id.
	actRC, err := src.OpenData("data/activities.json")
	if err != nil {
		return nil // No activities file — nothing to derive.
	}
	defer actRC.Close()

	dec := json.NewDecoder(actRC)
	// Consume opening '['.
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("RunLegacyFusionMemberships: read opening token: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		// Finding #23: warn instead of silently returning nothing.
		slog.Warn("dataimport: activities.json top-level not an array — fusion derivation skipped",
			"token", fmt.Sprintf("%v", tok))
		return nil
	}

	// Streaming pre-filter (Finding #12): only accumulate records that have
	// fusion_sources populated; all other rows are decoded and discarded inline.
	// Peak heap usage therefore scales with the number of fused activities, not
	// the total activity count.
	//
	// However, the "fuse" fan-out expansion needs to know which activities had
	// parent_id = <merged_id> — including children that themselves have no
	// fusion_sources.  We therefore also build parentToChildren inline during
	// the single streaming pass, keyed by the old parent UUID.
	var records []legacyActivityRecord
	// parentOldID → []oldChildID (for "fuse" expansion)
	parentToChildrenOld := make(map[string][]string)

	for dec.More() {
		var row map[string]any
		if err := dec.Decode(&row); err != nil {
			slog.Warn("dataimport: RunLegacyFusionMemberships: decode row failed — skipping", "error", err)
			continue
		}
		oldID, _ := stringVal(row["id"])
		if oldID == "" {
			continue
		}

		// Collect parent_id for every row so the parentToChildrenOld map is complete.
		oldParentID, _ := stringVal(row["parent_id"])
		if oldParentID != "" {
			parentToChildrenOld[oldParentID] = append(parentToChildrenOld[oldParentID], oldID)
		}

		// Extract fusion_sources JSONB — drop the row when absent.
		var fusionSources map[string]string
		if fsRaw := row["fusion_sources"]; fsRaw != nil {
			// fusion_sources arrives as map[string]any from JSON decode.
			if fsMap, ok := fsRaw.(map[string]any); ok && len(fsMap) > 0 {
				sources := make(map[string]string, len(fsMap))
				for k, v := range fsMap {
					if sv, ok := v.(string); ok {
						sources[k] = sv
					}
				}
				if len(sources) > 0 {
					fusionSources = sources
				}
			}
		}
		if fusionSources == nil {
			continue // Not a fused activity — no need to keep the record.
		}

		records = append(records, legacyActivityRecord{
			OldID:         oldID,
			OldParentID:   oldParentID,
			FusionSources: fusionSources,
		})
	}

	// Pre-filter means records only contains fused activities — any empty slice = v_regular.
	if len(records) == 0 {
		return nil
	}

	// importedActivityIDs must cover all imported activities (not just the fused ones)
	// so that concrete source UUIDs in fusion_sources can be validated.
	importedActivityIDs := idMap.NewIDSet()

	// parentToChildren maps old merged ID → []new child IDs, built from the
	// parentToChildrenOld map (old parent → []old child) by remapping via idMap.
	parentToChildren := make(map[string][]string, len(parentToChildrenOld))
	for oldParent, oldChildren := range parentToChildrenOld {
		for _, oldChild := range oldChildren {
			newChildID, ok := idMap.GetIfExists(oldChild)
			if !ok {
				continue
			}
			parentToChildren[oldParent] = append(parentToChildren[oldParent], newChildID)
		}
	}

	// Derive memberships.
	var allMemberships []database.FusionMembership
	for _, r := range records {
		newMergedID, ok := idMap.GetIfExists(r.OldID)
		if !ok {
			slog.Warn("dataimport: RunLegacyFusionMemberships: merged activity not in IDMap — skipping",
				"old_id", r.OldID)
			continue
		}

		// Remap fusion_sources values: concrete UUIDs are old IDs that need remapping.
		remapped := make(map[string]string, len(r.FusionSources))
		for fieldKey, value := range r.FusionSources {
			if value == "fuse" {
				remapped[fieldKey] = "fuse"
				continue
			}
			newSourceID, ok := idMap.GetIfExists(value)
			if !ok {
				slog.Warn("dataimport: RunLegacyFusionMemberships: source UUID not in IDMap — skipping field",
					"old_merged_id", r.OldID, "old_source_id", value, "field_key", fieldKey)
				continue
			}
			remapped[fieldKey] = newSourceID
		}

		// parent_id children for "fuse" expansion: look up children of the old merged ID.
		var parentChildren []string
		if kids, ok := parentToChildren[r.OldID]; ok {
			parentChildren = kids
		}

		derived := DeriveFromLegacyFusionSources(
			remapped, newMergedID, parentChildren, importUserID, importedActivityIDs,
		)
		allMemberships = append(allMemberships, derived...)
	}

	if len(allMemberships) == 0 {
		return nil
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("RunLegacyFusionMemberships: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := database.InsertFusionMemberships(ctx, tx, allMemberships); err != nil {
		return fmt.Errorf("RunLegacyFusionMemberships: insert memberships: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("RunLegacyFusionMemberships: commit: %w", err)
	}

	slog.Info("dataimport: RunLegacyFusionMemberships: derived and inserted legacy memberships",
		"count", len(allMemberships))
	return nil
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
