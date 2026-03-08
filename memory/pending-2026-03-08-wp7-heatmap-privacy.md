## docs/07_DATABASE_SCHEMA.md — Heatmap MVT Functions
**Suche nach:** heatmap_lines_mvt
**Delta:** Migration 000104 adds user_id filter parameter to both heatmap_lines_mvt and heatmap_points_mvt functions. Critical privacy fix.

## CHANGELOG.md
- fix(privacy): heatmap MVT functions now filter by user_id — prevents cross-user GPS track visibility (Migration 000104)
- fix(frontend): pass user_id to all heatmap tile requests (MapPage, SegmentEditorModal)
