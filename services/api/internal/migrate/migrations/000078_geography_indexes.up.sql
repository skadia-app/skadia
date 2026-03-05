-- 000078_geography_indexes.up.sql
-- Erstellt funktionale geography-GIST-Indexes auf osm_*-Tabellen
-- die mit ::geography-Cast abgefragt werden (water.py, peaks.py,
-- stops.py, lifts.py, settlements.py, borders.py).
--
-- Die bestehenden geometry-GIST-Indexes bleiben erhalten
-- (forests.py nutzt ST_Intersects ohne Cast, MVT-Funktionen auch).

CREATE INDEX IF NOT EXISTS osm_waterways_geog_idx
    ON osm_waterways USING gist ((geom::geography));

CREATE INDEX IF NOT EXISTS osm_waters_geog_idx
    ON osm_waters USING gist ((geom::geography));

CREATE INDEX IF NOT EXISTS osm_peaks_geog_idx
    ON osm_peaks USING gist ((geom::geography));

CREATE INDEX IF NOT EXISTS osm_amenities_geog_idx
    ON osm_amenities USING gist ((geom::geography));

CREATE INDEX IF NOT EXISTS osm_lifts_geog_idx
    ON osm_lifts USING gist ((geom::geography));

CREATE INDEX IF NOT EXISTS osm_settlements_geog_idx
    ON osm_settlements USING gist ((geom::geography));

CREATE INDEX IF NOT EXISTS osm_boundaries_geog_idx
    ON osm_boundaries USING gist ((geom::geography));

ANALYZE osm_waterways;
ANALYZE osm_waters;
ANALYZE osm_peaks;
ANALYZE osm_amenities;
ANALYZE osm_lifts;
ANALYZE osm_settlements;
ANALYZE osm_boundaries;
