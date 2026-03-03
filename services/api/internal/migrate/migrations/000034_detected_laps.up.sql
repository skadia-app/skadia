-- Automatisch erkannte Runden (nicht FIT-Laps!)
CREATE TABLE activity_detected_laps (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id     UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    lap_number      INTEGER NOT NULL,
    start_time      TIMESTAMPTZ NOT NULL,
    end_time        TIMESTAMPTZ NOT NULL,
    elapsed_time_s  INTEGER NOT NULL,
    distance_m      DOUBLE PRECISION,
    avg_speed_ms    DOUBLE PRECISION,
    avg_heart_rate  DOUBLE PRECISION,
    avg_power_w     DOUBLE PRECISION,
    avg_cadence     DOUBLE PRECISION,
    total_ascent_m  DOUBLE PRECISION,
    total_descent_m DOUBLE PRECISION,
    start_lat       DOUBLE PRECISION,
    start_lng       DOUBLE PRECISION,
    created_at      TIMESTAMPTZ DEFAULT now(),
    UNIQUE(activity_id, lap_number)
);
CREATE INDEX ON activity_detected_laps (activity_id);

-- Enrichment-Setting für Rundenerkennung
ALTER TABLE user_enrichment_settings
    ADD COLUMN enable_lap_detection BOOLEAN NOT NULL DEFAULT true;
