-- Rename osm_pois.category 'peak' → 'summit' for consistency with activity_geo_tags.category
-- activity_geo_tags already uses 'summit'; osm_pois was the only outlier.

UPDATE osm_pois SET category = 'summit' WHERE category = 'peak';

-- Drop old partial index (now matches 0 rows)
DROP INDEX IF EXISTS idx_osm_pois_peak;

-- Create new partial index for 'summit'
CREATE INDEX IF NOT EXISTS idx_osm_pois_summit ON osm_pois USING GIST(geom) WHERE category = 'summit';
