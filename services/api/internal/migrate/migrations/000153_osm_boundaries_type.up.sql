-- ============================================================
-- 000153: Add boundary_type and protect_class to osm_boundaries
-- Supports national parks and protected areas alongside admin boundaries
-- ============================================================

ALTER TABLE osm_boundaries ADD COLUMN IF NOT EXISTS boundary_type TEXT;
ALTER TABLE osm_boundaries ADD COLUMN IF NOT EXISTS protect_class TEXT;

-- Backfill existing rows (all are administrative)
UPDATE osm_boundaries SET boundary_type = 'administrative' WHERE boundary_type IS NULL AND admin_level IS NOT NULL;

-- Index for boundary type filtering
CREATE INDEX IF NOT EXISTS idx_osm_boundaries_type ON osm_boundaries(boundary_type);
