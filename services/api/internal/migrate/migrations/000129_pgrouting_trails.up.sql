-- 000129_pgrouting_trails.up.sql

-- pgRouting Extension (bereits in timescaledb-ha:pg18 enthalten, nur aktivieren)
CREATE EXTENSION IF NOT EXISTS pgrouting;

CREATE TABLE osm_trails (
    id          BIGSERIAL PRIMARY KEY,
    osm_id      BIGINT,
    highway     TEXT NOT NULL,
    surface     TEXT,
    tracktype   TEXT,
    sac_scale   TEXT,
    mtb_scale   TEXT,
    name        TEXT,
    oneway      BOOLEAN DEFAULT FALSE,
    bridge      BOOLEAN DEFAULT FALSE,
    tunnel      BOOLEAN DEFAULT FALSE,
    lit         BOOLEAN DEFAULT FALSE,
    length_m    DOUBLE PRECISION,
    avg_slope   DOUBLE PRECISION,
    source      BIGINT,
    target      BIGINT,
    cost        DOUBLE PRECISION,
    reverse_cost DOUBLE PRECISION,
    is_lift     BOOLEAN DEFAULT FALSE,
    geom        GEOMETRY(LineString, 4326) NOT NULL
);
CREATE INDEX idx_osm_trails_geom ON osm_trails USING GIST(geom);
CREATE INDEX idx_osm_trails_source ON osm_trails(source);
CREATE INDEX idx_osm_trails_target ON osm_trails(target);
CREATE INDEX idx_osm_trails_highway ON osm_trails(highway);

CREATE TABLE heatmap_trail_matches (
    user_id     UUID NOT NULL REFERENCES users(id),
    trail_id    BIGINT NOT NULL REFERENCES osm_trails(id) ON DELETE CASCADE,
    visit_count INTEGER NOT NULL DEFAULT 1,
    last_visit  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, trail_id)
);
