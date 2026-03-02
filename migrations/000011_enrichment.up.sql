-- Phase 4a: Enrichment Fields + Supporting Tables

-- Enrichment-Felder auf activities
ALTER TABLE activities ADD COLUMN IF NOT EXISTS start_city TEXT;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS end_city TEXT;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS start_state TEXT;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS start_country TEXT;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS start_country_code TEXT;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS weather_temp_c DOUBLE PRECISION;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS weather_condition TEXT;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS weather_humidity INTEGER;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS weather_wind_speed_ms DOUBLE PRECISION;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS weather_tags TEXT[];
ALTER TABLE activities ADD COLUMN IF NOT EXISTS is_round_trip BOOLEAN DEFAULT false;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS intensity TEXT; -- locker|moderat|intensiv
ALTER TABLE activities ADD COLUMN IF NOT EXISTS time_of_day TEXT; -- vor-sonnenaufgang|goldene-stunde|tagestour|nach-sonnenuntergang|nacht

-- Geo-Tags (systemgeneriert, strukturiert)
CREATE TABLE activity_geo_tags (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id     UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    category        TEXT NOT NULL,
    name            TEXT NOT NULL,
    osm_id          BIGINT,
    lat             DOUBLE PRECISION,
    lng             DOUBLE PRECISION,
    ele             DOUBLE PRECISION,
    dist_m          DOUBLE PRECISION,
    duration_min    INTEGER,
    meta            JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX ON activity_geo_tags (activity_id);

-- Anstiege
CREATE TABLE activity_climbs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id         UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    climb_number        INTEGER NOT NULL,
    start_distance_m    DOUBLE PRECISION,
    end_distance_m      DOUBLE PRECISION,
    elevation_gain_m    DOUBLE PRECISION,
    length_m            DOUBLE PRECISION,
    avg_gradient_pct    DOUBLE PRECISION,
    max_gradient_pct    DOUBLE PRECISION,
    category            TEXT
);
CREATE INDEX ON activity_climbs (activity_id);

-- Stopps
CREATE TABLE activity_stops (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id     UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    started_at      TIMESTAMPTZ NOT NULL,
    ended_at        TIMESTAMPTZ NOT NULL,
    duration_s      INTEGER,
    location        GEOMETRY(Point, 4326),
    name            TEXT
);
CREATE INDEX ON activity_stops (activity_id);

-- User Enrichment Settings
CREATE TABLE user_enrichment_settings (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    enable_summit_detection BOOLEAN DEFAULT true,
    enable_weather_enrichment BOOLEAN DEFAULT true,
    enable_stop_detection BOOLEAN DEFAULT true,
    enable_climb_detection BOOLEAN DEFAULT true,
    enable_commute_detection BOOLEAN DEFAULT true,
    enable_auto_naming BOOLEAN DEFAULT true,
    climb_min_gradient_pct FLOAT DEFAULT 3.0,
    climb_min_elevation_m FLOAT DEFAULT 20.0,
    round_trip_threshold_m FLOAT DEFAULT 100.0
);

-- Pendel-Cluster
CREATE TABLE commute_clusters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cluster_label INTEGER NOT NULL,
    center GEOMETRY(Point, 4326) NOT NULL,
    radius_m DOUBLE PRECISION,
    point_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Artikel-Cache fuer Auto-Naming
CREATE TABLE geo_name_gender_cache (
    name_normalized TEXT PRIMARY KEY,
    gender CHAR(1) NOT NULL,
    source TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
