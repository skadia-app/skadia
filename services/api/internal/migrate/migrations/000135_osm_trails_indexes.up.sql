-- 000135_osm_trails_indexes.up.sql

-- I10: osm_trails osm_id index for lookups
CREATE INDEX IF NOT EXISTS idx_osm_trails_osm_id ON osm_trails(osm_id);

-- I12: Document tobler_speed parameter semantics
-- slope_ratio: Steigung als Dezimalbruch (0.1 = 10%), NICHT als Prozent
COMMENT ON FUNCTION tobler_speed(double precision) IS 'slope_ratio as decimal fraction (0.1 = 10%), NOT percentage. Returns km/h.';

-- M7: distance_source CHECK constraint
ALTER TABLE activities ADD CONSTRAINT chk_distance_source CHECK (distance_source IN ('device', 'computed')) NOT VALID;
ALTER TABLE activities VALIDATE CONSTRAINT chk_distance_source;
