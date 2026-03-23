ALTER TABLE activities
  -- Movement / Cycles
  DROP COLUMN IF EXISTS total_cycles,
  DROP COLUMN IF EXISTS total_work_j,
  DROP COLUMN IF EXISTS total_fat_calories,
  DROP COLUMN IF EXISTS metabolic_calories,
  DROP COLUMN IF EXISTS total_moving_time_s,

  -- Swimming
  DROP COLUMN IF EXISTS swim_stroke,
  DROP COLUMN IF EXISTS pool_length_m,
  DROP COLUMN IF EXISTS pool_length_unit,
  DROP COLUMN IF EXISTS num_lengths,
  DROP COLUMN IF EXISTS num_active_lengths,
  DROP COLUMN IF EXISTS avg_stroke_count,
  DROP COLUMN IF EXISTS avg_stroke_distance_m,
  DROP COLUMN IF EXISTS stroke_count,
  DROP COLUMN IF EXISTS zone_count,

  -- Cadence Extended
  DROP COLUMN IF EXISTS avg_fractional_cadence,
  DROP COLUMN IF EXISTS max_fractional_cadence,
  DROP COLUMN IF EXISTS total_fractional_cycles,

  -- Running Dynamics
  DROP COLUMN IF EXISTS avg_vertical_oscillation_mm,
  DROP COLUMN IF EXISTS avg_stance_time_ms,
  DROP COLUMN IF EXISTS avg_stance_time_pct,
  DROP COLUMN IF EXISTS avg_stance_time_balance_pct,
  DROP COLUMN IF EXISTS avg_step_length_mm,
  DROP COLUMN IF EXISTS avg_vertical_ratio_pct,

  -- Grade
  DROP COLUMN IF EXISTS avg_grade_pct,
  DROP COLUMN IF EXISTS avg_pos_grade_pct,
  DROP COLUMN IF EXISTS avg_neg_grade_pct,
  DROP COLUMN IF EXISTS max_pos_grade_pct,
  DROP COLUMN IF EXISTS max_neg_grade_pct,

  -- Vertical Speed
  DROP COLUMN IF EXISTS avg_pos_vertical_speed_ms,
  DROP COLUMN IF EXISTS avg_neg_vertical_speed_ms,
  DROP COLUMN IF EXISTS max_pos_vertical_speed_ms,
  DROP COLUMN IF EXISTS max_neg_vertical_speed_ms,
  DROP COLUMN IF EXISTS avg_vam_ms,

  -- Altitude Extended
  DROP COLUMN IF EXISTS avg_altitude_m,
  DROP COLUMN IF EXISTS total_fractional_ascent_m,
  DROP COLUMN IF EXISTS total_fractional_descent_m,

  -- Pedal Analytics (15)
  DROP COLUMN IF EXISTS avg_left_torque_effectiveness_pct,
  DROP COLUMN IF EXISTS avg_right_torque_effectiveness_pct,
  DROP COLUMN IF EXISTS avg_left_pedal_smoothness_pct,
  DROP COLUMN IF EXISTS avg_right_pedal_smoothness_pct,
  DROP COLUMN IF EXISTS avg_combined_pedal_smoothness_pct,
  DROP COLUMN IF EXISTS avg_left_pco_mm,
  DROP COLUMN IF EXISTS avg_right_pco_mm,
  DROP COLUMN IF EXISTS avg_left_power_phase_start_deg,
  DROP COLUMN IF EXISTS avg_left_power_phase_end_deg,
  DROP COLUMN IF EXISTS avg_left_power_phase_peak_start_deg,
  DROP COLUMN IF EXISTS avg_left_power_phase_peak_end_deg,
  DROP COLUMN IF EXISTS avg_right_power_phase_start_deg,
  DROP COLUMN IF EXISTS avg_right_power_phase_end_deg,
  DROP COLUMN IF EXISTS avg_right_power_phase_peak_start_deg,
  DROP COLUMN IF EXISTS avg_right_power_phase_peak_end_deg,

  -- Power/Cadence by Position
  DROP COLUMN IF EXISTS avg_power_position_w,
  DROP COLUMN IF EXISTS max_power_position_w,
  DROP COLUMN IF EXISTS avg_cadence_position,
  DROP COLUMN IF EXISTS max_cadence_position,

  -- Standing/Sitting
  DROP COLUMN IF EXISTS time_standing_s,
  DROP COLUMN IF EXISTS stand_count,

  -- E-Bike
  DROP COLUMN IF EXISTS avg_lev_motor_power_w,
  DROP COLUMN IF EXISTS max_lev_motor_power_w,
  DROP COLUMN IF EXISTS lev_battery_consumption_pct,

  -- MTB Grit/Flow
  DROP COLUMN IF EXISTS total_grit,
  DROP COLUMN IF EXISTS total_flow,
  DROP COLUMN IF EXISTS avg_grit,
  DROP COLUMN IF EXISTS avg_flow,

  -- Jumps
  DROP COLUMN IF EXISTS jump_count,

  -- Ball Sports
  DROP COLUMN IF EXISTS max_ball_speed_ms,
  DROP COLUMN IF EXISTS avg_ball_speed_ms,
  DROP COLUMN IF EXISTS player_score,
  DROP COLUMN IF EXISTS opponent_score,
  DROP COLUMN IF EXISTS opponent_name,

  -- Hemoglobin
  DROP COLUMN IF EXISTS avg_total_hemoglobin_conc,
  DROP COLUMN IF EXISTS min_total_hemoglobin_conc,
  DROP COLUMN IF EXISTS max_total_hemoglobin_conc,
  DROP COLUMN IF EXISTS avg_saturated_hemoglobin_pct,
  DROP COLUMN IF EXISTS min_saturated_hemoglobin_pct,
  DROP COLUMN IF EXISTS max_saturated_hemoglobin_pct,

  -- Diving
  DROP COLUMN IF EXISTS avg_depth_m,
  DROP COLUMN IF EXISTS max_depth_m,
  DROP COLUMN IF EXISTS surface_interval_s,
  DROP COLUMN IF EXISTS dive_number,
  DROP COLUMN IF EXISTS start_cns_pct,
  DROP COLUMN IF EXISTS end_cns_pct,
  DROP COLUMN IF EXISTS start_n2_pct,
  DROP COLUMN IF EXISTS end_n2_pct,
  DROP COLUMN IF EXISTS o2_toxicity_otu,

  -- Respiration / HRV Extended
  DROP COLUMN IF EXISTS max_respiration_rate,
  DROP COLUMN IF EXISTS min_respiration_rate,
  DROP COLUMN IF EXISTS sdrr_hrv_ms,

  -- Temperature Extended
  DROP COLUMN IF EXISTS min_core_temperature_c,
  DROP COLUMN IF EXISTS max_core_temperature_c,

  -- Misc
  DROP COLUMN IF EXISTS avg_lap_time_s,
  DROP COLUMN IF EXISTS best_lap_index,
  DROP COLUMN IF EXISTS num_laps,
  DROP COLUMN IF EXISTS first_lap_index,
  DROP COLUMN IF EXISTS intensity_factor,
  DROP COLUMN IF EXISTS left_right_balance,
  DROP COLUMN IF EXISTS sport_profile_name,
  DROP COLUMN IF EXISTS sport_index,
  DROP COLUMN IF EXISTS nec_lat,
  DROP COLUMN IF EXISTS nec_lng,
  DROP COLUMN IF EXISTS swc_lat,
  DROP COLUMN IF EXISTS swc_lng;
