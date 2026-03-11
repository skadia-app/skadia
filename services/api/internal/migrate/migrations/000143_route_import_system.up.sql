-- Migration 000143: Route Import System
--
-- Adds source tracking to planned_routes for cross-platform duplicate detection
-- and user-configurable route duplicate thresholds.

-- 1. Source tracking on planned_routes
ALTER TABLE planned_routes ADD COLUMN IF NOT EXISTS source_platform TEXT;
ALTER TABLE planned_routes ADD COLUMN IF NOT EXISTS source_id TEXT;
ALTER TABLE planned_routes ADD COLUMN IF NOT EXISTS source_url TEXT;

-- Unique constraint: same route from same platform can't be imported twice
CREATE UNIQUE INDEX IF NOT EXISTS idx_planned_routes_source
    ON planned_routes (user_id, source_platform, source_id)
    WHERE source_platform IS NOT NULL AND source_id IS NOT NULL;

-- 2. Geometry hash for fast duplicate detection (snap to ~1m grid)
ALTER TABLE planned_routes ADD COLUMN IF NOT EXISTS geometry_hash TEXT;

-- Backfill geometry_hash for existing routes
UPDATE planned_routes
SET geometry_hash = encode(digest(ST_AsEWKB(ST_SnapToGrid(geometry, 0.00001)), 'sha256'), 'hex')
WHERE geometry_hash IS NULL;

CREATE INDEX IF NOT EXISTS idx_planned_routes_geometry_hash
    ON planned_routes (user_id, geometry_hash);

-- 3. User settings for route duplicate thresholds
ALTER TABLE user_enrichment_settings
    ADD COLUMN IF NOT EXISTS route_duplicate_hausdorff_m INTEGER NOT NULL DEFAULT 100;
ALTER TABLE user_enrichment_settings
    ADD COLUMN IF NOT EXISTS route_duplicate_length_diff_km DOUBLE PRECISION NOT NULL DEFAULT 2.0;

-- 4. Komoot integration credentials (encrypted with AES-256-GCM, key from KOMOOT_ENCRYPTION_KEY env var)
CREATE TABLE IF NOT EXISTS komoot_credentials (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    encrypted_password TEXT NOT NULL,
    komoot_user_id TEXT,
    last_sync_at TIMESTAMPTZ,
    sync_enabled BOOLEAN NOT NULL DEFAULT false,
    import_planned BOOLEAN NOT NULL DEFAULT true,
    import_recorded BOOLEAN NOT NULL DEFAULT true,
    import_photos BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 5. Komoot Tour-ID on activities for Tier-1 duplicate detection (analog to garmin_activity_id, strava_id)
ALTER TABLE activities ADD COLUMN IF NOT EXISTS komoot_tour_id BIGINT;
CREATE UNIQUE INDEX IF NOT EXISTS uq_activities_komoot_tour_id
    ON activities (user_id, komoot_tour_id) WHERE komoot_tour_id IS NOT NULL;
