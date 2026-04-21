-- 000323_segments_meta_filter_indexes.up.sql
-- Funktionale Indizes für Filter-Queries (§5.3.3 Spec)
-- OHNE CONCURRENTLY, weil Migrations-Framework Tx-wrappt
CREATE INDEX IF NOT EXISTS segments_sac_scale_idx
  ON segments ((segment_type_meta->>'sac_scale'))
  WHERE segment_type_meta ? 'sac_scale';

CREATE INDEX IF NOT EXISTS segments_mtb_scale_idx
  ON segments ((segment_type_meta->>'mtb_scale'))
  WHERE segment_type_meta ? 'mtb_scale';

CREATE INDEX IF NOT EXISTS segments_surface_idx
  ON segments ((segment_type_meta->>'surface'))
  WHERE segment_type_meta ? 'surface';
