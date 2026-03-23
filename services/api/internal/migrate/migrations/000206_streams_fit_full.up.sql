-- 000206_streams_fit_full.up.sql
-- Add ~58 new FIT SDK columns to activity_streams (TimescaleDB hypertable with compression).

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

-- 3. Add all new columns (all nullable, instant metadata-only ops)

-- Terrain & Movement (12 columns)
ALTER TABLE activity_streams ADD COLUMN grade_pct REAL;
ALTER TABLE activity_streams ADD COLUMN vertical_speed_ms REAL;
ALTER TABLE activity_streams ADD COLUMN gps_accuracy_m SMALLINT;
ALTER TABLE activity_streams ADD COLUMN calories INTEGER;
ALTER TABLE activity_streams ADD COLUMN resistance SMALLINT;
ALTER TABLE activity_streams ADD COLUMN total_cycles INTEGER;
ALTER TABLE activity_streams ADD COLUMN accumulated_power_w INTEGER;
ALTER TABLE activity_streams ADD COLUMN absolute_pressure_pa INTEGER;
ALTER TABLE activity_streams ADD COLUMN ascent_rate_ms REAL;
ALTER TABLE activity_streams ADD COLUMN time_from_course_s REAL;
ALTER TABLE activity_streams ADD COLUMN activity_type SMALLINT;
ALTER TABLE activity_streams ADD COLUMN device_index SMALLINT;

-- Running Dynamics extended (5 columns)
ALTER TABLE activity_streams ADD COLUMN step_length_mm REAL;
ALTER TABLE activity_streams ADD COLUMN vertical_ratio_pct REAL;
ALTER TABLE activity_streams ADD COLUMN stance_time_balance_pct REAL;
ALTER TABLE activity_streams ADD COLUMN cycle_length_m REAL;
ALTER TABLE activity_streams ADD COLUMN fractional_cadence REAL;

-- Pedal Analytics (7 columns)
ALTER TABLE activity_streams ADD COLUMN left_torque_effectiveness_pct REAL;
ALTER TABLE activity_streams ADD COLUMN right_torque_effectiveness_pct REAL;
ALTER TABLE activity_streams ADD COLUMN left_pedal_smoothness_pct REAL;
ALTER TABLE activity_streams ADD COLUMN right_pedal_smoothness_pct REAL;
ALTER TABLE activity_streams ADD COLUMN combined_pedal_smoothness_pct REAL;
ALTER TABLE activity_streams ADD COLUMN left_pco_mm SMALLINT;
ALTER TABLE activity_streams ADD COLUMN right_pco_mm SMALLINT;

-- Power Phase (8 columns)
ALTER TABLE activity_streams ADD COLUMN left_power_phase_start_deg REAL;
ALTER TABLE activity_streams ADD COLUMN left_power_phase_end_deg REAL;
ALTER TABLE activity_streams ADD COLUMN left_power_phase_peak_start_deg REAL;
ALTER TABLE activity_streams ADD COLUMN left_power_phase_peak_end_deg REAL;
ALTER TABLE activity_streams ADD COLUMN right_power_phase_start_deg REAL;
ALTER TABLE activity_streams ADD COLUMN right_power_phase_end_deg REAL;
ALTER TABLE activity_streams ADD COLUMN right_power_phase_peak_start_deg REAL;
ALTER TABLE activity_streams ADD COLUMN right_power_phase_peak_end_deg REAL;

-- Hemoglobin / Muscle Oxygen (6 columns)
ALTER TABLE activity_streams ADD COLUMN total_hemoglobin_conc REAL;
ALTER TABLE activity_streams ADD COLUMN total_hemoglobin_conc_min REAL;
ALTER TABLE activity_streams ADD COLUMN total_hemoglobin_conc_max REAL;
ALTER TABLE activity_streams ADD COLUMN saturated_hemoglobin_pct REAL;
ALTER TABLE activity_streams ADD COLUMN saturated_hemoglobin_pct_min REAL;
ALTER TABLE activity_streams ADD COLUMN saturated_hemoglobin_pct_max REAL;

-- E-Bike (5 columns)
ALTER TABLE activity_streams ADD COLUMN motor_power_w INTEGER;
ALTER TABLE activity_streams ADD COLUMN ebike_travel_range_km INTEGER;
ALTER TABLE activity_streams ADD COLUMN ebike_battery_level_pct SMALLINT;
ALTER TABLE activity_streams ADD COLUMN ebike_assist_mode SMALLINT;
ALTER TABLE activity_streams ADD COLUMN ebike_assist_level_pct SMALLINT;

-- Health / Stress (4 columns)
ALTER TABLE activity_streams ADD COLUMN core_temperature_c REAL;
ALTER TABLE activity_streams ADD COLUMN enhanced_respiration_rate REAL;
ALTER TABLE activity_streams ADD COLUMN current_stress REAL;
ALTER TABLE activity_streams ADD COLUMN battery_soc_pct REAL;

-- MTB Metrics (2 columns)
ALTER TABLE activity_streams ADD COLUMN grit REAL;
ALTER TABLE activity_streams ADD COLUMN flow REAL;

-- Racket / Ball Sports (3 columns)
ALTER TABLE activity_streams ADD COLUMN stroke_type SMALLINT;
ALTER TABLE activity_streams ADD COLUMN zone SMALLINT;
ALTER TABLE activity_streams ADD COLUMN ball_speed_ms REAL;

-- Diving (12 columns)
ALTER TABLE activity_streams ADD COLUMN depth_m REAL;
ALTER TABLE activity_streams ADD COLUMN next_stop_depth_m REAL;
ALTER TABLE activity_streams ADD COLUMN next_stop_time_s INTEGER;
ALTER TABLE activity_streams ADD COLUMN time_to_surface_s INTEGER;
ALTER TABLE activity_streams ADD COLUMN ndl_time_s INTEGER;
ALTER TABLE activity_streams ADD COLUMN cns_load_pct SMALLINT;
ALTER TABLE activity_streams ADD COLUMN n2_load_pct INTEGER;
ALTER TABLE activity_streams ADD COLUMN air_time_remaining_s INTEGER;
ALTER TABLE activity_streams ADD COLUMN pressure_sac REAL;
ALTER TABLE activity_streams ADD COLUMN volume_sac REAL;
ALTER TABLE activity_streams ADD COLUMN rmv REAL;
ALTER TABLE activity_streams ADD COLUMN po2 REAL;

-- Developer Fields (1 column)
ALTER TABLE activity_streams ADD COLUMN developer_fields JSONB;

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
