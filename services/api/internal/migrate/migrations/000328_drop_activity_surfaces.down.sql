CREATE TABLE IF NOT EXISTS activity_surfaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    surface_type TEXT NOT NULL,
    path_type TEXT NOT NULL DEFAULT 'unknown',
    length_m DOUBLE PRECISION,
    percentage DOUBLE PRECISION,
    category TEXT NOT NULL DEFAULT 'surface'
);

CREATE INDEX IF NOT EXISTS idx_activity_surfaces_activity ON activity_surfaces (activity_id);

INSERT INTO activity_surfaces (activity_id, surface_type, path_type, length_m, percentage, category)
SELECT
    e.activity_id,
    CASE WHEN e.category = 'surface' THEN e.type ELSE 'unknown' END AS surface_type,
    CASE WHEN e.category = 'highway' THEN e.type ELSE 'unknown' END AS path_type,
    e.length_m,
    e.percentage,
    e.category
FROM activity_enrichments e
WHERE e.category IN ('surface', 'highway')
  AND NOT EXISTS (
      SELECT 1
      FROM activity_surfaces s
      WHERE s.activity_id = e.activity_id
        AND s.category = e.category
        AND (
            (e.category = 'surface' AND s.surface_type = e.type)
            OR (e.category = 'highway' AND s.path_type = e.type)
        )
  );
