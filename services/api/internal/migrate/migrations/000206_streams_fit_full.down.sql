-- 000206_streams_fit_full.down.sql
-- Remove ~58 FIT SDK columns from activity_streams.

-- 1. Remove compression policy
SELECT remove_compression_policy('activity_streams', if_exists => true);

-- 2. Batch-wise decompress all chunks
DO $$
DECLARE
    chunk REGCLASS;
BEGIN
    FOR chunk IN
        SELECT c FROM show_chunks('activity_streams') c
    LOOP
        BEGIN
            PERFORM decompress_chunk(chunk, true);
        EXCEPTION WHEN OTHERS THEN
            RAISE WARNING 'Failed to decompress %: %', chunk, SQLERRM;
        END;
    END LOOP;
END $$;

-- 3. Drop all added columns

-- Terrain & Movement
ALTER TABLE activity_streams DROP COLUMN IF EXISTS grade_pct;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS vertical_speed_ms;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS gps_accuracy_m;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS calories;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS resistance;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS total_cycles;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS accumulated_power_w;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS absolute_pressure_pa;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS ascent_rate_ms;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS time_from_course_s;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS activity_type;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS device_index;

-- Running Dynamics extended
ALTER TABLE activity_streams DROP COLUMN IF EXISTS step_length_mm;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS vertical_ratio_pct;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS stance_time_balance_pct;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS cycle_length_m;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS fractional_cadence;

-- Pedal Analytics
ALTER TABLE activity_streams DROP COLUMN IF EXISTS left_torque_effectiveness_pct;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS right_torque_effectiveness_pct;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS left_pedal_smoothness_pct;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS right_pedal_smoothness_pct;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS combined_pedal_smoothness_pct;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS left_pco_mm;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS right_pco_mm;

-- Power Phase
ALTER TABLE activity_streams DROP COLUMN IF EXISTS left_power_phase_start_deg;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS left_power_phase_end_deg;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS left_power_phase_peak_start_deg;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS left_power_phase_peak_end_deg;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS right_power_phase_start_deg;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS right_power_phase_end_deg;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS right_power_phase_peak_start_deg;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS right_power_phase_peak_end_deg;

-- Hemoglobin / Muscle Oxygen
ALTER TABLE activity_streams DROP COLUMN IF EXISTS total_hemoglobin_conc;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS total_hemoglobin_conc_min;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS total_hemoglobin_conc_max;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS saturated_hemoglobin_pct;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS saturated_hemoglobin_pct_min;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS saturated_hemoglobin_pct_max;

-- E-Bike
ALTER TABLE activity_streams DROP COLUMN IF EXISTS motor_power_w;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS ebike_travel_range_km;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS ebike_battery_level_pct;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS ebike_assist_mode;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS ebike_assist_level_pct;

-- Health / Stress
ALTER TABLE activity_streams DROP COLUMN IF EXISTS core_temperature_c;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS enhanced_respiration_rate;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS current_stress;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS battery_soc_pct;

-- MTB Metrics
ALTER TABLE activity_streams DROP COLUMN IF EXISTS grit;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS flow;

-- Racket / Ball Sports
ALTER TABLE activity_streams DROP COLUMN IF EXISTS stroke_type;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS zone;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS ball_speed_ms;

-- Diving
ALTER TABLE activity_streams DROP COLUMN IF EXISTS depth_m;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS next_stop_depth_m;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS next_stop_time_s;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS time_to_surface_s;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS ndl_time_s;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS cns_load_pct;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS n2_load_pct;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS air_time_remaining_s;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS pressure_sac;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS volume_sac;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS rmv;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS po2;

-- Developer Fields
ALTER TABLE activity_streams DROP COLUMN IF EXISTS developer_fields;

-- 4. Re-enable compression with same settings
ALTER TABLE activity_streams SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'activity_id',
    timescaledb.compress_orderby = 'time DESC'
);

-- 5. Re-add compression policy
SELECT add_compression_policy('activity_streams', INTERVAL '7 days');

-- 6. Recompress all chunks
SELECT compress_chunk(c, true) FROM show_chunks('activity_streams') c;
