-- Migration 000116: Add per-module enable flags to user_enrichment_settings.
-- All new columns default to TRUE for backwards compatibility:
-- existing rows will have all modules enabled unless explicitly changed.

ALTER TABLE user_enrichment_settings
    ADD COLUMN IF NOT EXISTS enable_peaks       BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS enable_lifts       BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS enable_settlements BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS enable_commute     BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS enable_borders     BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS enable_water       BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS enable_forests     BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS enable_surfaces    BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS enable_stops       BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS enable_intervals   BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS enable_ascents     BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS enable_embeddings  BOOLEAN NOT NULL DEFAULT true;

-- Migrate existing enable_summit_detection → enable_peaks for rows that already exist.
-- enable_summit_detection remains for Stage-1 (auto-naming / summit tag) backwards-compat.
UPDATE user_enrichment_settings
    SET enable_peaks = enable_summit_detection;

COMMENT ON COLUMN user_enrichment_settings.enable_peaks       IS 'Stage-2: Gipfel- und Pass-Erkennung via OSM';
COMMENT ON COLUMN user_enrichment_settings.enable_lifts       IS 'Stage-2: Liftmatch (Bergbahnen, Gondeln)';
COMMENT ON COLUMN user_enrichment_settings.enable_settlements IS 'Stage-2: Ortschafts-Tags (Städte, Dörfer)';
COMMENT ON COLUMN user_enrichment_settings.enable_commute     IS 'Stage-2: Pendelstrecken-Erkennung (DBSCAN)';
COMMENT ON COLUMN user_enrichment_settings.enable_borders     IS 'Stage-2: Länder-/Regionsgrenzen';
COMMENT ON COLUMN user_enrichment_settings.enable_water       IS 'Stage-2: Gewässer-Tags';
COMMENT ON COLUMN user_enrichment_settings.enable_forests     IS 'Stage-2: Wald-Tags';
COMMENT ON COLUMN user_enrichment_settings.enable_surfaces    IS 'Stage-2: Untergrundanalyse';
COMMENT ON COLUMN user_enrichment_settings.enable_stops       IS 'Stage-2: Stopp-Erkennung (Hütten, Cafés, …)';
COMMENT ON COLUMN user_enrichment_settings.enable_intervals   IS 'Stage-2: Intervall-Erkennung';
COMMENT ON COLUMN user_enrichment_settings.enable_ascents     IS 'Stage-2: Anstiegsanalyse (Savitzky-Golay)';
COMMENT ON COLUMN user_enrichment_settings.enable_embeddings  IS 'Stage-2: Route-Embedding (BGE-M3)';
