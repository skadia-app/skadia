-- Add ~100 new FIT SDK session fields to activities table.
-- All columns nullable. No DEFAULT — instant ALTER TABLE.
-- Section 2.3 columns (already existing) are intentionally excluded.
-- workout_id excluded — added in 000218 after workouts table exists.

ALTER TABLE activities

  -- Movement / Cycles
  ADD COLUMN total_cycles                          INTEGER,
  ADD COLUMN total_work_j                          INTEGER,
  ADD COLUMN total_fat_calories                    INTEGER,
  ADD COLUMN metabolic_calories                    INTEGER,
  ADD COLUMN total_moving_time_s                   REAL,

  -- Swimming
  ADD COLUMN swim_stroke                           TEXT,
  ADD COLUMN pool_length_m                         REAL,
  ADD COLUMN pool_length_unit                      TEXT,
  ADD COLUMN num_lengths                           INTEGER,
  ADD COLUMN num_active_lengths                    INTEGER,
  ADD COLUMN avg_stroke_count                      REAL,
  ADD COLUMN avg_stroke_distance_m                 REAL,
  ADD COLUMN stroke_count                          INTEGER[],
  ADD COLUMN zone_count                            INTEGER[],

  -- Cadence Extended
  ADD COLUMN avg_fractional_cadence                REAL,
  ADD COLUMN max_fractional_cadence                REAL,
  ADD COLUMN total_fractional_cycles               REAL,

  -- Running Dynamics
  ADD COLUMN avg_vertical_oscillation_mm           REAL,
  ADD COLUMN avg_stance_time_ms                    REAL,
  ADD COLUMN avg_stance_time_pct                   REAL,
  ADD COLUMN avg_stance_time_balance_pct           REAL,
  ADD COLUMN avg_step_length_mm                    REAL,
  ADD COLUMN avg_vertical_ratio_pct                REAL,

  -- Grade
  ADD COLUMN avg_grade_pct                         REAL,
  ADD COLUMN avg_pos_grade_pct                     REAL,
  ADD COLUMN avg_neg_grade_pct                     REAL,
  ADD COLUMN max_pos_grade_pct                     REAL,
  ADD COLUMN max_neg_grade_pct                     REAL,

  -- Vertical Speed
  ADD COLUMN avg_pos_vertical_speed_ms             REAL,
  ADD COLUMN avg_neg_vertical_speed_ms             REAL,
  ADD COLUMN max_pos_vertical_speed_ms             REAL,
  ADD COLUMN max_neg_vertical_speed_ms             REAL,
  ADD COLUMN avg_vam_ms                            REAL,

  -- Altitude Extended
  ADD COLUMN avg_altitude_m                        REAL,
  ADD COLUMN total_fractional_ascent_m             REAL,
  ADD COLUMN total_fractional_descent_m            REAL,

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

  -- Power/Cadence by Position (arrays)
  ADD COLUMN avg_power_position_w                  INTEGER[],
  ADD COLUMN max_power_position_w                  INTEGER[],
  ADD COLUMN avg_cadence_position                  SMALLINT[],
  ADD COLUMN max_cadence_position                  SMALLINT[],

  -- Standing/Sitting
  ADD COLUMN time_standing_s                       REAL,
  ADD COLUMN stand_count                           INTEGER,

  -- E-Bike
  ADD COLUMN avg_lev_motor_power_w                 INTEGER,
  ADD COLUMN max_lev_motor_power_w                 INTEGER,
  ADD COLUMN lev_battery_consumption_pct           REAL,

  -- MTB Grit/Flow
  ADD COLUMN total_grit                            REAL,
  ADD COLUMN total_flow                            REAL,
  ADD COLUMN avg_grit                              REAL,
  ADD COLUMN avg_flow                              REAL,

  -- Jumps
  ADD COLUMN jump_count                            INTEGER,

  -- Ball Sports
  ADD COLUMN max_ball_speed_ms                     REAL,
  ADD COLUMN avg_ball_speed_ms                     REAL,
  ADD COLUMN player_score                          INTEGER,
  ADD COLUMN opponent_score                        INTEGER,
  ADD COLUMN opponent_name                         TEXT,

  -- Hemoglobin (REAL[] arrays for multi-sensor)
  ADD COLUMN avg_total_hemoglobin_conc             REAL[],
  ADD COLUMN min_total_hemoglobin_conc             REAL[],
  ADD COLUMN max_total_hemoglobin_conc             REAL[],
  ADD COLUMN avg_saturated_hemoglobin_pct          REAL[],
  ADD COLUMN min_saturated_hemoglobin_pct          REAL[],
  ADD COLUMN max_saturated_hemoglobin_pct          REAL[],

  -- Diving
  ADD COLUMN avg_depth_m                           REAL,
  ADD COLUMN max_depth_m                           REAL,
  ADD COLUMN surface_interval_s                    INTEGER,
  ADD COLUMN dive_number                           INTEGER,
  ADD COLUMN start_cns_pct                         SMALLINT,
  ADD COLUMN end_cns_pct                           SMALLINT,
  ADD COLUMN start_n2_pct                          INTEGER,
  ADD COLUMN end_n2_pct                            INTEGER,
  ADD COLUMN o2_toxicity_otu                       INTEGER,

  -- Respiration / HRV Extended
  ADD COLUMN max_respiration_rate                  REAL,
  ADD COLUMN min_respiration_rate                  REAL,
  ADD COLUMN sdrr_hrv_ms                           SMALLINT,

  -- Temperature Extended
  ADD COLUMN min_core_temperature_c                REAL,
  ADD COLUMN max_core_temperature_c                REAL,

  -- Misc
  ADD COLUMN avg_lap_time_s                        REAL,
  ADD COLUMN best_lap_index                        SMALLINT,
  ADD COLUMN num_laps                              INTEGER,
  ADD COLUMN first_lap_index                       INTEGER,
  ADD COLUMN intensity_factor                      REAL,
  ADD COLUMN left_right_balance                    REAL,
  ADD COLUMN sport_profile_name                    TEXT,
  ADD COLUMN sport_index                           SMALLINT,
  ADD COLUMN nec_lat                               DOUBLE PRECISION,
  ADD COLUMN nec_lng                               DOUBLE PRECISION,
  ADD COLUMN swc_lat                               DOUBLE PRECISION,
  ADD COLUMN swc_lng                               DOUBLE PRECISION;
