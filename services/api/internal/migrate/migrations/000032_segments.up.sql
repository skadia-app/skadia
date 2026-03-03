-- Segments: User-defined or auto-created route segments
CREATE TABLE segments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_by          UUID NOT NULL REFERENCES users(id),
    name                TEXT NOT NULL,
    description         TEXT,
    sport_group         TEXT NOT NULL DEFAULT 'generic',
    allowed_sport_types TEXT[],
    allowed_sub_types   TEXT[],
    segment_type        TEXT NOT NULL DEFAULT 'standard',
    segment_type_meta   JSONB DEFAULT '{}',
    source_activity_id  UUID REFERENCES activities(id) ON DELETE SET NULL,
    osm_lift_id         BIGINT,
    distance_m          DOUBLE PRECISION,
    elevation_gain_m    DOUBLE PRECISION,
    effort_count        INTEGER DEFAULT 0,
    athlete_count       INTEGER DEFAULT 0,
    visibility          TEXT DEFAULT 'public',
    route_hash          TEXT,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX ON segments (segment_type);
CREATE INDEX ON segments (sport_group);
CREATE INDEX ON segments (created_by);

-- Segment Versions: Versioned geometry for route changes over time
CREATE TABLE segment_versions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    segment_id      UUID NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
    version_number  INTEGER NOT NULL,
    track           GEOMETRY(LineString, 4326) NOT NULL,
    start_point     GEOMETRY(Point, 4326),
    end_point       GEOMETRY(Point, 4326),
    valid_from      DATE NOT NULL,
    change_note     TEXT,
    created_by      UUID REFERENCES users(id),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(segment_id, version_number)
);
CREATE INDEX ON segment_versions USING GIST (track);
CREATE INDEX ON segment_versions (segment_id, valid_from DESC);

-- Segment Efforts: TimescaleDB hypertable for leaderboard data
CREATE TABLE segment_efforts (
    id                  UUID NOT NULL DEFAULT gen_random_uuid(),
    segment_id          UUID NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
    segment_version_id  UUID NOT NULL REFERENCES segment_versions(id),
    activity_id         UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    user_id             UUID NOT NULL REFERENCES users(id),
    started_at          TIMESTAMPTZ NOT NULL,
    elapsed_time_s      INTEGER NOT NULL,
    is_reverse          BOOLEAN NOT NULL DEFAULT false,
    sport_type          TEXT NOT NULL DEFAULT 'generic',
    sub_type            TEXT NOT NULL DEFAULT 'generic',
    gear_id             UUID REFERENCES gear(id) ON DELETE SET NULL,
    avg_power_w         DOUBLE PRECISION,
    avg_heart_rate      DOUBLE PRECISION,
    avg_speed_ms        DOUBLE PRECISION,
    rank_overall        INTEGER,
    is_personal_best    BOOLEAN DEFAULT false,
    is_segment_record   BOOLEAN DEFAULT false,
    in_leaderboard      BOOLEAN DEFAULT true,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, started_at)
);
SELECT create_hypertable('segment_efforts', 'started_at');
CREATE INDEX ON segment_efforts (segment_id, sport_type, is_reverse, elapsed_time_s ASC) WHERE in_leaderboard = true;
CREATE INDEX ON segment_efforts (segment_id, sport_type, sub_type, is_reverse, elapsed_time_s ASC) WHERE in_leaderboard = true;
CREATE INDEX ON segment_efforts (segment_id, gear_id, elapsed_time_s ASC) WHERE in_leaderboard = true AND gear_id IS NOT NULL;
