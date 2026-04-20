package enrichment

// surfaceIDToGroup maps OSM-surface-enum-IDs (1..37, see routing/graph/tags.go
// surfaceEnum) onto the German parent-group labels used by
// frontend/src/lib/surfaceConstants.ts SURFACE_GROUPS. Backend persists only
// these parent-group strings into segment_type_meta.surface — never raw OSM IDs,
// never user-only sub-types ("Naturweg (fest)"/"Wechselnd"/...).
//
// Unknown or out-of-range IDs return "" — the handler then emits Reason="no_data".
//
// NOTE: For segment-enrichment use the segment-specific variant
// surfaceIDToGroupForSegment, which collapses rarely-relevant OSM buckets
// (concrete → Asphalt, metal/wood → "") so the editor dropdown stays lean.
// Non-segment callers (activity-enrichment, route-surface aggregation) keep
// using this canonical mapper for data fidelity.
func surfaceIDToGroup(id uint8) string {
	switch id {
	case 1:
		return "Asphalt"
	case 2, 34, 35:
		return "Beton"
	case 3, 4, 5, 24, 32:
		return "Pflaster"
	case 6, 36:
		return "Metall"
	case 7, 19:
		return "Holz"
	case 8:
		return "Schotter (fein)"
	case 9, 10, 11:
		return "Schotter (grob)"
	case 12, 13, 16, 17, 25, 37:
		return "Naturweg"
	case 14, 15:
		return "Gras"
	case 18:
		return "Sand"
	case 20, 21:
		return "Schnee"
	case 22, 23:
		return "Fels"
	case 26, 27, 28, 29, 30, 31:
		return "Sportbelag"
	case 33:
		return "Asphalt"
	default:
		return ""
	}
}

// surfaceIDToGroupForSegment is the segment-editor variant of surfaceIDToGroup.
// It collapses OSM buckets that user-authored segments don't practically need:
//   - concrete / concrete_plates / concrete_lanes (2, 34, 35) → "Asphalt"
//     (user-mental-model: segment-level doesn't distinguish Beton from Asphalt)
//   - metal / metal_grid (6, 36) → "" (omitted — metal bridges/grids are rarely
//     the defining surface of an authored segment)
//   - wood / woodchips (7, 19) → "" (omitted — same reasoning; wood boardwalks
//     are incidental, not segment-characterizing)
//
// All other IDs delegate to surfaceIDToGroup unchanged. Empty-string returns
// mean the edge contributes no surface to the aggregation — the dominant
// surface will be derived from the other (non-excluded) edges along the track.
// If all edges fall into excluded buckets, the handler emits Reason="no_data".
//
// This function is ONLY used by SuggestSegmentEnrichment. Activity-enrichment
// and route-surface aggregation continue to use surfaceIDToGroup for full
// OSM fidelity.
func surfaceIDToGroupForSegment(id uint8) string {
	switch id {
	case 2, 34, 35:
		// concrete* → Asphalt (segment-editor merges these two in the dropdown)
		return "Asphalt"
	case 6, 7, 19, 36:
		// metal / wood / woodchips / metal_grid — not offered in segment dropdown
		return ""
	default:
		return surfaceIDToGroup(id)
	}
}
