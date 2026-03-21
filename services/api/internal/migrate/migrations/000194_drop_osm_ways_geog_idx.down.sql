CREATE INDEX IF NOT EXISTS osm_ways_geog_idx ON osm_ways USING gist ((geom::geography));
