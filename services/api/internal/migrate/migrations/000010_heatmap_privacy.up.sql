-- Phase 3: Heatmap + Privacy Zones

-- Privacy-Zonen pro User (z.B. Zuhause, Arbeit)
CREATE TABLE user_privacy_zones (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    center      GEOMETRY(Point, 4326) NOT NULL,
    radius_m    INTEGER NOT NULL DEFAULT 200,
    is_active   BOOLEAN DEFAULT true,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX ON user_privacy_zones (user_id);

-- Heatmap-Lines: eine Zeile pro Aktivität mit Track-Geometrie
CREATE TABLE heatmap_lines (
    id            BIGSERIAL PRIMARY KEY,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_id   UUID REFERENCES activities(id) ON DELETE CASCADE,
    sport_type    TEXT,
    geom          GEOMETRY(LineString, 4326) NOT NULL,
    activity_date DATE
);

CREATE INDEX ON heatmap_lines USING GIST (geom);
CREATE INDEX ON heatmap_lines (user_id, sport_type);
CREATE INDEX ON heatmap_lines (activity_id);

-- Betroffene Track-Abschnitte pro Privacy-Zone
CREATE TABLE activity_privacy_sections (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id     UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    privacy_zone_id UUID NOT NULL REFERENCES user_privacy_zones(id) ON DELETE CASCADE,
    section_geom    GEOMETRY(LineString, 4326)
);

CREATE INDEX ON activity_privacy_sections (activity_id);
CREATE INDEX ON activity_privacy_sections (privacy_zone_id);

-- Sichtbarkeitsfelder für Aktivitäten
ALTER TABLE activities ADD COLUMN IF NOT EXISTS hide_map BOOLEAN DEFAULT false;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS data_visibility JSONB DEFAULT '{}';
