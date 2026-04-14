ALTER TABLE user_enrichment_settings
    DROP COLUMN IF EXISTS geocode_fallback_timeout_s;
ALTER TABLE user_enrichment_settings
    DROP COLUMN IF EXISTS geocode_fallback_url;
ALTER TABLE user_enrichment_settings
    DROP COLUMN IF EXISTS geocode_fallback_provider;

DROP TABLE IF EXISTS reverse_geocode_cache;
