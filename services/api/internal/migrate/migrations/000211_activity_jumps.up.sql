CREATE TABLE activity_jumps (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id   UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    timestamp     TIMESTAMPTZ NOT NULL,
    distance_m    REAL,
    height_m      REAL,
    hang_time_s   REAL,
    score         REAL,
    rotations     SMALLINT,
    speed_ms      REAL,
    lat           DOUBLE PRECISION,
    lng           DOUBLE PRECISION
);
CREATE INDEX idx_activity_jumps_activity ON activity_jumps(activity_id);
