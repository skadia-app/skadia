package enrichment

// surfaceIDToGroup maps OSM-surface-enum-IDs (1..37, see routing/graph/tags.go
// surfaceEnum) onto the German parent-group labels used by
// frontend/src/lib/surfaceConstants.ts SURFACE_GROUPS. Backend persists only
// these parent-group strings into segment_type_meta.surface — never raw OSM IDs,
// never user-only sub-types ("Naturweg (fest)"/"Wechselnd"/...).
//
// Unknown or out-of-range IDs return "" — the handler then emits Reason="no_data".
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
