-- activity_enrichment_meta persists which enrichment path was taken for each
-- activity's track analysis. This replaces the heuristic in the track-summary
-- handler that derived the path from the presence of surface_ranges — a
-- heuristic that produced false negatives for single-surface-type routes.
--
-- path: 'graph' = graph (map-snap) path; 'sql' = SQL PostGIS fallback path.
-- resample_spacing_m: the uniform point spacing (meters) used during track
--   resampling. Indices in activity_enrichments.ranges refer to this resampled
--   track, NOT to the raw activity_tracks.track_full points.
-- computed_at: timestamp when the enrichment was written.
--
-- GDPR: Derived category — regenerated from primary activity data on re-import.
-- Cascade via activity_id FK covers deletion; no explicit gdpr.go entry needed.
CREATE TABLE IF NOT EXISTS activity_enrichment_meta (
    activity_id         UUID        NOT NULL PRIMARY KEY REFERENCES activities(id) ON DELETE CASCADE,
    path                TEXT        NOT NULL CHECK (path IN ('graph', 'sql')),
    resample_spacing_m  REAL        NOT NULL,
    computed_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_activity_enrichment_meta_activity
    ON activity_enrichment_meta(activity_id);

COMMENT ON TABLE activity_enrichment_meta IS
    'Tracks which enrichment path (graph or sql) was taken for each activity. '
    'Used by the track-summary handler to populate graph_snap_used without a heuristic.';

COMMENT ON COLUMN activity_enrichment_meta.path IS
    'graph = viterbi map-snap path (ranges are coordinate-index aligned to resampled track); '
    'sql = PostGIS lateral-join fallback path (ranges absent, surface percentages only).';

COMMENT ON COLUMN activity_enrichment_meta.resample_spacing_m IS
    'Uniform point spacing in meters used during track resampling. '
    'Range indices in activity_enrichments.ranges refer to the resampled track at this spacing, '
    'not to the raw track_full points in activity_tracks.';
