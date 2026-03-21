CREATE INDEX IF NOT EXISTS idx_osm_landuse_coverage_geom
ON osm_landuse USING gist (geom)
WHERE landuse IN ('farmland', 'residential', 'industrial', 'commercial', 'quarry', 'military', 'landfill');
