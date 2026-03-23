ALTER TABLE activity_laps
  -- Cycles/Strokes
  DROP COLUMN IF EXISTS total_cycles,
  DROP COLUMN IF EXISTS total_fat_calories,
  DROP COLUMN IF EXISTS avg_stroke_distance_m,
  DROP COLUMN IF EXISTS first_length_index,
  DROP COLUMN IF EXISTS avg_fractional_cadence,
  DROP COLUMN IF EXISTS max_fractional_cadence,
  DROP COLUMN IF EXISTS total_fractional_cycles,
  DROP COLUMN IF EXISTS stroke_count,
  DROP COLUMN IF EXISTS zone_count,

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

  -- Temperature
  DROP COLUMN IF EXISTS max_temperature_c,
  DROP COLUMN IF EXISTS min_temperature_c,
  DROP COLUMN IF EXISTS avg_core_temperature_c,
  DROP COLUMN IF EXISTS min_core_temperature_c,
  DROP COLUMN IF EXISTS max_core_temperature_c,

  -- Pedal Analytics
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

  -- L/R Balance
  DROP COLUMN IF EXISTS left_right_balance,

  -- E-Bike
  DROP COLUMN IF EXISTS avg_lev_motor_power_w,
  DROP COLUMN IF EXISTS max_lev_motor_power_w,
  DROP COLUMN IF EXISTS lev_battery_consumption_pct,

  -- Grit/Flow
  DROP COLUMN IF EXISTS total_grit,
  DROP COLUMN IF EXISTS total_flow,
  DROP COLUMN IF EXISTS avg_grit,
  DROP COLUMN IF EXISTS avg_flow,

  -- Jumps
  DROP COLUMN IF EXISTS jump_count,

  -- Scores
  DROP COLUMN IF EXISTS player_score,
  DROP COLUMN IF EXISTS opponent_score,

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

  -- Respiration
  DROP COLUMN IF EXISTS avg_respiration_rate,
  DROP COLUMN IF EXISTS max_respiration_rate,

  -- Altitude
  DROP COLUMN IF EXISTS total_fractional_ascent_m,
  DROP COLUMN IF EXISTS total_fractional_descent_m,

  -- Misc
  DROP COLUMN IF EXISTS repetition_num,
  DROP COLUMN IF EXISTS wkt_step_index,
  DROP COLUMN IF EXISTS event_group,
  DROP COLUMN IF EXISTS gps_accuracy_m;
