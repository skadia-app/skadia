-- Add extended session metrics to activities table.
ALTER TABLE activities
  ADD COLUMN IF NOT EXISTS total_calories            INTEGER,
  ADD COLUMN IF NOT EXISTS aerobic_training_effect   DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS anaerobic_training_effect DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS training_load_peak        DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS avg_spo2_pct              SMALLINT,
  ADD COLUMN IF NOT EXISTS avg_respiration_rate      DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS avg_stress_level          SMALLINT,
  ADD COLUMN IF NOT EXISTS workout_rpe               SMALLINT,
  ADD COLUMN IF NOT EXISTS workout_feel              SMALLINT,
  ADD COLUMN IF NOT EXISTS session_rmssd_ms          INTEGER,
  ADD COLUMN IF NOT EXISTS avg_core_temperature_c    DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS threshold_power_w         INTEGER,
  ADD COLUMN IF NOT EXISTS variability_index         DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS efficiency_factor         DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS aerobic_decoupling        DOUBLE PRECISION;
