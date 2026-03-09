ALTER TABLE user_enrichment_settings
    DROP COLUMN IF EXISTS enable_peaks,
    DROP COLUMN IF EXISTS enable_lifts,
    DROP COLUMN IF EXISTS enable_settlements,
    DROP COLUMN IF EXISTS enable_commute,
    DROP COLUMN IF EXISTS enable_borders,
    DROP COLUMN IF EXISTS enable_water,
    DROP COLUMN IF EXISTS enable_forests,
    DROP COLUMN IF EXISTS enable_surfaces,
    DROP COLUMN IF EXISTS enable_stops,
    DROP COLUMN IF EXISTS enable_intervals,
    DROP COLUMN IF EXISTS enable_ascents,
    DROP COLUMN IF EXISTS enable_embeddings;
