-- Add ~83 new FIT SDK lap fields to activity_laps table.
-- All columns nullable. No DEFAULT — instant ALTER TABLE.
-- Already existing columns intentionally excluded:
--   lap_trigger, sport, sub_sport, intensity, swim_stroke,
--   num_lengths, num_active_lengths, total_elapsed_time_s,
--   total_timer_time_s, total_moving_time_s, total_distance_m,
--   total_ascent_m, total_descent_m, total_calories, avg_speed_ms,
--   max_speed_ms, avg_heart_rate, max_heart_rate, min_heart_rate,
--   avg_cadence, max_cadence, avg_power_w, max_power_w,
--   normalized_power_w, avg_temperature_c, start_lat, start_lng,
--   end_lat, end_lng, total_work_j, avg_altitude_m, max_altitude_m,
--   min_altitude_m

ALTER TABLE activity_laps

  -- Cycles/Strokes (9)
  ADD COLUMN total_cycles                          INTEGER,
  ADD COLUMN total_fat_calories                    INTEGER,
  ADD COLUMN avg_stroke_distance_m                 REAL,
  ADD COLUMN first_length_index                    INTEGER,
  ADD COLUMN avg_fractional_cadence                REAL,
  ADD COLUMN max_fractional_cadence                REAL,
  ADD COLUMN total_fractional_cycles               REAL,
  ADD COLUMN stroke_count                          INTEGER[],
  ADD COLUMN zone_count                            INTEGER[],

  -- Running Dynamics (6)
  ADD COLUMN avg_vertical_oscillation_mm           REAL,
  ADD COLUMN avg_stance_time_ms                    REAL,
  ADD COLUMN avg_stance_time_pct                   REAL,
  ADD COLUMN avg_stance_time_balance_pct           REAL,
  ADD COLUMN avg_step_length_mm                    REAL,
  ADD COLUMN avg_vertical_ratio_pct                REAL,

  -- Grade (5)
  ADD COLUMN avg_grade_pct                         REAL,
  ADD COLUMN avg_pos_grade_pct                     REAL,
  ADD COLUMN avg_neg_grade_pct                     REAL,
  ADD COLUMN max_pos_grade_pct                     REAL,
  ADD COLUMN max_neg_grade_pct                     REAL,

  -- Vertical Speed (5)
  ADD COLUMN avg_pos_vertical_speed_ms             REAL,
  ADD COLUMN avg_neg_vertical_speed_ms             REAL,
  ADD COLUMN max_pos_vertical_speed_ms             REAL,
  ADD COLUMN max_neg_vertical_speed_ms             REAL,
  ADD COLUMN avg_vam_ms                            REAL,

  -- Temperature (5) — avg_temperature_c already exists
  ADD COLUMN max_temperature_c                     SMALLINT,
  ADD COLUMN min_temperature_c                     SMALLINT,
  ADD COLUMN avg_core_temperature_c                REAL,
  ADD COLUMN min_core_temperature_c                REAL,
  ADD COLUMN max_core_temperature_c                REAL,

  -- Pedal Analytics (15)
  ADD COLUMN avg_left_torque_effectiveness_pct     REAL,
  ADD COLUMN avg_right_torque_effectiveness_pct    REAL,
  ADD COLUMN avg_left_pedal_smoothness_pct         REAL,
  ADD COLUMN avg_right_pedal_smoothness_pct        REAL,
  ADD COLUMN avg_combined_pedal_smoothness_pct     REAL,
  ADD COLUMN avg_left_pco_mm                       SMALLINT,
  ADD COLUMN avg_right_pco_mm                      SMALLINT,
  ADD COLUMN avg_left_power_phase_start_deg        REAL,
  ADD COLUMN avg_left_power_phase_end_deg          REAL,
  ADD COLUMN avg_left_power_phase_peak_start_deg   REAL,
  ADD COLUMN avg_left_power_phase_peak_end_deg     REAL,
  ADD COLUMN avg_right_power_phase_start_deg       REAL,
  ADD COLUMN avg_right_power_phase_end_deg         REAL,
  ADD COLUMN avg_right_power_phase_peak_start_deg  REAL,
  ADD COLUMN avg_right_power_phase_peak_end_deg    REAL,

  -- Power/Cadence by Position (4 arrays)
  ADD COLUMN avg_power_position_w                  INTEGER[],
  ADD COLUMN max_power_position_w                  INTEGER[],
  ADD COLUMN avg_cadence_position                  SMALLINT[],
  ADD COLUMN max_cadence_position                  SMALLINT[],

  -- Standing/Sitting (2)
  ADD COLUMN time_standing_s                       REAL,
  ADD COLUMN stand_count                           INTEGER,

  -- L/R Balance (1)
  ADD COLUMN left_right_balance                    REAL,

  -- E-Bike (3)
  ADD COLUMN avg_lev_motor_power_w                 INTEGER,
  ADD COLUMN max_lev_motor_power_w                 INTEGER,
  ADD COLUMN lev_battery_consumption_pct           REAL,

  -- Grit/Flow (4)
  ADD COLUMN total_grit                            REAL,
  ADD COLUMN total_flow                            REAL,
  ADD COLUMN avg_grit                              REAL,
  ADD COLUMN avg_flow                              REAL,

  -- Jumps (1)
  ADD COLUMN jump_count                            INTEGER,

  -- Scores (2)
  ADD COLUMN player_score                          INTEGER,
  ADD COLUMN opponent_score                        INTEGER,

  -- Hemoglobin (6 REAL[] arrays)
  ADD COLUMN avg_total_hemoglobin_conc             REAL[],
  ADD COLUMN min_total_hemoglobin_conc             REAL[],
  ADD COLUMN max_total_hemoglobin_conc             REAL[],
  ADD COLUMN avg_saturated_hemoglobin_pct          REAL[],
  ADD COLUMN min_saturated_hemoglobin_pct          REAL[],
  ADD COLUMN max_saturated_hemoglobin_pct          REAL[],

  -- Diving (2)
  ADD COLUMN avg_depth_m                           REAL,
  ADD COLUMN max_depth_m                           REAL,

  -- Respiration (2)
  ADD COLUMN avg_respiration_rate                  REAL,
  ADD COLUMN max_respiration_rate                  REAL,

  -- Altitude (2)
  ADD COLUMN total_fractional_ascent_m             REAL,
  ADD COLUMN total_fractional_descent_m            REAL,

  -- Misc (4)
  ADD COLUMN repetition_num                        INTEGER,
  ADD COLUMN wkt_step_index                        SMALLINT,
  ADD COLUMN event_group                           SMALLINT,
  ADD COLUMN gps_accuracy_m                        SMALLINT;
