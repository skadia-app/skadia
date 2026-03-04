-- 000062_streams_running_dynamics.up.sql
-- Add running dynamics columns to activity_streams (Phase 8: FIT Parser Extensions)

ALTER TABLE activity_streams
    ADD COLUMN IF NOT EXISTS ground_contact_time_ms   SMALLINT,
    ADD COLUMN IF NOT EXISTS vertical_oscillation_mm  SMALLINT,
    ADD COLUMN IF NOT EXISTS stance_time_percent       REAL,
    ADD COLUMN IF NOT EXISTS left_right_balance        SMALLINT;
