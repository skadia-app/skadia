CREATE TABLE activity_tracks (
    activity_id         UUID PRIMARY KEY REFERENCES activities(id) ON DELETE CASCADE,
    track_simplified    GEOMETRY(LineString, 4326),
    track_full          GEOMETRY(LineString, 4326),
    track_public        GEOMETRY(LineString, 4326),
    bbox                GEOMETRY(Polygon, 4326),
    point_count         INTEGER
);
CREATE INDEX ON activity_tracks USING GIST (track_full);

CREATE TABLE activity_streams (
    time            TIMESTAMPTZ NOT NULL,
    activity_id     UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    lat             DOUBLE PRECISION,
    lng             DOUBLE PRECISION,
    altitude_m      DOUBLE PRECISION,
    speed_ms        DOUBLE PRECISION,
    distance_m      DOUBLE PRECISION,
    heart_rate      SMALLINT,
    power_w         SMALLINT,
    cadence         SMALLINT,
    temperature_c   SMALLINT
);
SELECT create_hypertable('activity_streams', 'time');
CREATE INDEX ON activity_streams (activity_id, time DESC);

CREATE TABLE activity_sensors (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id         UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    device_index        SMALLINT NOT NULL,
    serial_number       BIGINT,
    source_type         TEXT NOT NULL,
    antplus_device_type TEXT,
    all_device_types    TEXT[],
    manufacturer        TEXT,
    manufacturer_id     SMALLINT,
    garmin_product      TEXT,
    garmin_product_id   INTEGER,
    product_name        TEXT,
    software_version    NUMERIC(6,2),
    hardware_version    SMALLINT,
    battery_status      TEXT,
    battery_voltage     NUMERIC(5,3),
    created_at          TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX ON activity_sensors (activity_id);
CREATE INDEX ON activity_sensors (serial_number) WHERE serial_number IS NOT NULL;
