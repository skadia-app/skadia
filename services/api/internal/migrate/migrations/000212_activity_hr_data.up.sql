CREATE TABLE activity_hr_data (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id          UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    timestamp            TIMESTAMPTZ NOT NULL,
    fractional_timestamp DOUBLE PRECISION,
    filtered_bpm         SMALLINT[],
    event_timestamp      DOUBLE PRECISION[]
);
CREATE INDEX idx_activity_hr_data_activity ON activity_hr_data(activity_id, timestamp);
