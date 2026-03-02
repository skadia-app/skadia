ALTER TABLE activities
  ADD COLUMN IF NOT EXISTS min_heart_rate            SMALLINT,
  ADD COLUMN IF NOT EXISTS total_calories            INTEGER,
  ADD COLUMN IF NOT EXISTS aerobic_training_effect   NUMERIC(3,1),
  ADD COLUMN IF NOT EXISTS anaerobic_training_effect NUMERIC(3,1),
  ADD COLUMN IF NOT EXISTS training_load_peak        DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS avg_spo2_pct              SMALLINT,
  ADD COLUMN IF NOT EXISTS avg_respiration_rate      NUMERIC(5,2),
  ADD COLUMN IF NOT EXISTS avg_stress_level          SMALLINT,
  ADD COLUMN IF NOT EXISTS workout_rpe               SMALLINT,
  ADD COLUMN IF NOT EXISTS workout_feel              SMALLINT,
  ADD COLUMN IF NOT EXISTS session_rmssd_ms          SMALLINT,
  ADD COLUMN IF NOT EXISTS avg_core_temperature_c    NUMERIC(5,2),
  ADD COLUMN IF NOT EXISTS threshold_power_w         INTEGER,
  ADD COLUMN IF NOT EXISTS variability_index         DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS efficiency_factor         DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS aerobic_decoupling        DOUBLE PRECISION;

CREATE TABLE activity_time_in_zone (
    activity_id UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    zone_type   TEXT NOT NULL CHECK (zone_type IN ('hr', 'power', 'speed', 'cadence')),
    zone_index  SMALLINT NOT NULL,
    zone_high   DOUBLE PRECISION,
    seconds     INTEGER NOT NULL,
    PRIMARY KEY (activity_id, zone_type, zone_index)
);

ALTER TABLE activity_streams
  ADD COLUMN IF NOT EXISTS respiration_rate SMALLINT,
  ADD COLUMN IF NOT EXISTS stress_level    SMALLINT;
