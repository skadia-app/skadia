-- 000062_streams_running_dynamics.down.sql

ALTER TABLE activity_streams
    DROP COLUMN IF EXISTS ground_contact_time_ms,
    DROP COLUMN IF EXISTS vertical_oscillation_mm,
    DROP COLUMN IF EXISTS stance_time_percent,
    DROP COLUMN IF EXISTS left_right_balance;
