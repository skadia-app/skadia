UPDATE osm_pois SET category = 'peak' WHERE category = 'summit';

DROP INDEX IF EXISTS idx_osm_pois_summit;

CREATE INDEX IF NOT EXISTS idx_osm_pois_peak ON osm_pois USING GIST(geom) WHERE category = 'peak';
