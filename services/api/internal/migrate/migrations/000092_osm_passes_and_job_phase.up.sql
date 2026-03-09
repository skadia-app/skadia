-- Neue Tabelle für echte Passstraßen (mountain_pass=yes in OSM)
CREATE TABLE IF NOT EXISTS osm_passes (
    id      BIGSERIAL PRIMARY KEY,
    osm_id  BIGINT,
    name    TEXT,
    ele     REAL,
    geom    GEOMETRY(Point, 4326)
);

CREATE INDEX IF NOT EXISTS osm_passes_geog_idx ON osm_passes USING gist ((geom::geography));

-- Phase-Feld für Background-Jobs (download / import / index)
ALTER TABLE background_jobs ADD COLUMN IF NOT EXISTS phase TEXT;
