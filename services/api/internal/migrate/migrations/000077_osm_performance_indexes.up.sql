-- Performance-Indexes für OSM-Tabellen und geo-enrichment
-- Behebt fehlende Indexes die zu Sequential Scans führen

-- osm_lifts: lifts.py fragt WHERE osm_id = $2 aber nur way_id hatte einen Index
CREATE INDEX IF NOT EXISTS osm_lifts_osm_id_idx ON osm_lifts(osm_id);

-- osm_amenities: amenities_mvt filtert AND subtype IN (...) auf 950k+ Zeilen
CREATE INDEX IF NOT EXISTS osm_amenities_subtype_idx ON osm_amenities(subtype);

-- activity_geo_tags: stops.py macht ON CONFLICT (activity_id, osm_id) DO NOTHING
-- Dafür ist ein UNIQUE Index zwingend erforderlich (sonst schlägt jede Gastronomy-Einfügung fehl)
CREATE UNIQUE INDEX IF NOT EXISTS activity_geo_tags_activity_osm_id_key
    ON activity_geo_tags(activity_id, osm_id)
    WHERE osm_id IS NOT NULL;
