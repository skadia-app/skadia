-- System-wide cache for external reverse-geocode lookups (Nominatim / Photon).
-- Generic coord→name mapping; NOT user-specific, NOT part of GDPR export.
-- Key resolution: lat_key = round(lat * 1000) = ~100 m grid (privacy + de-dup).
CREATE TABLE IF NOT EXISTS reverse_geocode_cache (
    lat_key      INT NOT NULL,
    lng_key      INT NOT NULL,
    provider     TEXT NOT NULL,  -- 'nominatim' | 'photon'
    city         TEXT,
    state        TEXT,
    country      TEXT,
    country_code TEXT,
    fetched_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (lat_key, lng_key, provider)
);

CREATE INDEX IF NOT EXISTS idx_reverse_geocode_cache_age
    ON reverse_geocode_cache (fetched_at);

COMMENT ON TABLE reverse_geocode_cache IS
    'System-wide cache for external reverse-geocode lookups (Nominatim/Photon). '
    'Keys are lat*1000, lng*1000 (~100 m grid). TTL enforced by app layer (30 days). '
    'GDPR: system-wide, not user-specific, not exported, not deleted on user-delete.';

-- Per-user opt-in settings for external reverse-geocode fallback.
-- Primary geocoding always uses osm_boundaries (PostGIS); this fallback kicks in
-- ONLY when the local OSM data has no match (user hasn't imported OSM for that region).
ALTER TABLE user_enrichment_settings
    ADD COLUMN IF NOT EXISTS geocode_fallback_provider TEXT NOT NULL DEFAULT 'off'
        CHECK (geocode_fallback_provider IN ('off', 'nominatim', 'photon'));

ALTER TABLE user_enrichment_settings
    ADD COLUMN IF NOT EXISTS geocode_fallback_url TEXT;

ALTER TABLE user_enrichment_settings
    ADD COLUMN IF NOT EXISTS geocode_fallback_timeout_s INT NOT NULL DEFAULT 5
        CHECK (geocode_fallback_timeout_s BETWEEN 1 AND 30);
