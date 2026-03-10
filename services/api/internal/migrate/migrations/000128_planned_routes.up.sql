-- 000128_planned_routes.up.sql
CREATE TABLE planned_routes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id),
    name            TEXT NOT NULL,
    description     TEXT,
    sport_type      TEXT NOT NULL DEFAULT 'hiking',
    difficulty      TEXT,
    profile         TEXT NOT NULL DEFAULT 'hiking-sac2',
    geometry        GEOMETRY(LineString, 4326) NOT NULL,
    waypoints       JSONB NOT NULL,
    segments        JSONB NOT NULL,
    total_distance_m DOUBLE PRECISION,
    total_elevation_gain DOUBLE PRECISION,
    total_elevation_loss DOUBLE PRECISION,
    estimated_time_min  INTEGER,
    time_override   BOOLEAN DEFAULT FALSE,
    auto_tag        BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_planned_routes_user ON planned_routes(user_id);
CREATE INDEX idx_planned_routes_geom ON planned_routes USING GIST(geometry);

CREATE TABLE route_photos (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id        UUID NOT NULL REFERENCES planned_routes(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id),
    file_hash       TEXT NOT NULL,
    s3_key          TEXT NOT NULL,
    s3_key_thumb    TEXT NOT NULL,
    s3_key_preview  TEXT,
    original_name   TEXT,
    width           INTEGER,
    height          INTEGER,
    sort_order      INTEGER DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_route_photos_route ON route_photos(route_id);

CREATE TABLE route_tags (
    route_id    UUID NOT NULL REFERENCES planned_routes(id) ON DELETE CASCADE,
    tag_id      UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (route_id, tag_id)
);
