CREATE INDEX IF NOT EXISTS idx_osm_boundaries_admin8
    ON osm_boundaries USING GIST(geom)
    WHERE admin_level = 8 AND boundary_type = 'administrative';
