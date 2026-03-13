-- pg_trgm bereits in 000151, IF NOT EXISTS ist idempotent
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Trigram-Index auf osm_ways.name für Straßensuche (NEU)
CREATE INDEX IF NOT EXISTS idx_osm_ways_name_trgm
    ON osm_ways USING gin (name gin_trgm_ops)
    WHERE name IS NOT NULL;

-- osm_pois Trigram-Index existiert bereits seit 000151 — IF NOT EXISTS ist no-op
CREATE INDEX IF NOT EXISTS idx_osm_pois_name_trgm
    ON osm_pois USING gin (name gin_trgm_ops)
    WHERE name IS NOT NULL;

-- Composite-Index für Highway-Typ + Name (Straßentyp-Filter) (NEU)
CREATE INDEX IF NOT EXISTS idx_osm_ways_highway_name
    ON osm_ways (highway) WHERE name IS NOT NULL;
