CREATE TABLE activity_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id     UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    timestamp       TIMESTAMPTZ NOT NULL,
    event           SMALLINT NOT NULL,
    event_type      SMALLINT NOT NULL,
    event_group     SMALLINT,
    data            BIGINT,
    front_gear_num  SMALLINT,
    front_gear      SMALLINT,
    rear_gear_num   SMALLINT,
    rear_gear       SMALLINT,
    radar_threat_level_max              SMALLINT,
    radar_threat_count                  SMALLINT,
    radar_threat_avg_approach_speed_ms  REAL,
    radar_threat_max_approach_speed_ms  REAL,
    score           INTEGER,
    opponent_score  INTEGER,
    device_index    SMALLINT,
    activity_type   SMALLINT,
    start_timestamp TIMESTAMPTZ
);
CREATE INDEX idx_activity_events_activity ON activity_events(activity_id, timestamp);
CREATE INDEX idx_activity_events_type ON activity_events(activity_id, event);
