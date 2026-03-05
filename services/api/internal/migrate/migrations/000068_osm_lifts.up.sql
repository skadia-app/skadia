-- 000068_osm_lifts.up.sql
CREATE TABLE IF NOT EXISTS osm_lifts (
    id     BIGSERIAL PRIMARY KEY,
    osm_id BIGINT UNIQUE NOT NULL,
    name   TEXT,
    geom   GEOMETRY(LineString, 4326) NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_osm_lifts_geom ON osm_lifts USING GIST (geom);

ALTER TABLE activities ADD COLUMN IF NOT EXISTS lift_descent_m          DOUBLE PRECISION;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS lift_duration_s         INTEGER;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS exclude_lift_subtraction BOOLEAN NOT NULL DEFAULT false;
