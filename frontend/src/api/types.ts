/** @deprecated Use sportTaxonomy.ts for sport type lookups. Kept for backwards compatibility. */
export type SportType =
  | 'cycling'
  | 'running'
  | 'hiking'
  | 'swimming'
  | 'skiing'
  | 'paddling'
  | 'walking'
  | 'strength'
  | 'rock_climbing'
  | 'stand_up_paddleboarding'
  | 'generic'
  | 'unknown'

export type Visibility = 'private' | 'bekannte' | 'instanz' | 'federation' | 'public'

export type GearStatus = 'active' | 'stored' | 'retired'

export interface WeatherReading {
  id: string
  reading_type: 'single' | 'start' | 'summit' | 'midpoint'
  label?: string | null
  recorded_at: string
  lat?: number | null
  lng?: number | null
  elevation_m?: number | null
  temp_c?: number | null
  apparent_temp_c?: number | null
  precipitation_mm?: number | null
  wind_speed_ms?: number | null
  wind_gusts_ms?: number | null
  humidity?: number | null
  uv_index?: number | null
  freezing_level_m?: number | null
  condition?: string | null
  tags?: string[]
  model?: string | null
}

export interface WeatherPrefs {
  reading_mode: 'auto' | 'always_one' | 'always_two'
  mountain_threshold_m: number
  long_duration_s: number
  display_fields: string[]
  auto_tags: boolean
}

export const WEATHER_DISPLAY_FIELD_OPTIONS: { key: string; label: string }[] = [
  { key: 'temp_c',           label: 'Temperatur' },
  { key: 'apparent_temp_c',  label: 'Gefühlte Temperatur' },
  { key: 'precipitation_mm', label: 'Niederschlag' },
  { key: 'wind_speed_ms',    label: 'Windgeschwindigkeit' },
  { key: 'wind_gusts_ms',    label: 'Windböen' },
  { key: 'humidity',         label: 'Luftfeuchtigkeit' },
  { key: 'uv_index',         label: 'UV-Index' },
  { key: 'freezing_level_m', label: 'Schneefallgrenze' },
  { key: 'condition',        label: 'Wetterlage (Text)' },
]

export interface Activity {
  id: string
  user_id: string
  title: string | null
  description?: string | null
  sport_type: string
  sub_type: string | null
  is_ebike: boolean
  is_virtual: boolean
  exclude_from_heatmap: boolean
  exclude_from_stats: boolean
  started_at: string
  ended_at?: string
  elapsed_time_s: number
  moving_time_s?: number
  distance_m: number | null
  elevation_gain_m: number | null
  elevation_loss_m: number | null
  max_elevation_m?: number | null
  min_elevation_m?: number | null
  avg_speed_ms?: number | null
  max_speed_ms?: number | null
  avg_heart_rate?: number | null
  max_heart_rate?: number | null
  avg_power_w?: number | null
  max_power_w?: number | null
  normalized_power_w?: number | null
  avg_cadence?: number | null
  avg_temperature_c?: number | null
  min_temperature_c?: number | null
  max_temperature_c?: number | null
  tss?: number | null
  trimp?: number | null
  is_indoor: boolean
  tags: string[]
  visibility: Visibility
  source?: string | null
  source_device?: string | null
  enrichment_status: string
  needs_classification: boolean
  has_gps: boolean
  title_auto?: string | null
  start_city?: string | null
  end_city?: string | null
  start_state?: string | null
  start_country?: string | null
  start_country_code?: string | null
  weather_readings?: WeatherReading[]
  is_round_trip?: boolean
  intensity?: string | null
  time_of_day?: string[] | null
  sunrise_at?: string | null
  sunset_at?: string | null
  gear_id?: string | null
  min_heart_rate?: number | null
  total_calories?: number | null
  aerobic_training_effect?: number | null
  anaerobic_training_effect?: number | null
  training_load_peak?: number | null
  avg_spo2_pct?: number | null
  workout_rpe?: number | null
  workout_feel?: number | null
  threshold_power_w?: number | null
  variability_index?: number | null
  efficiency_factor?: number | null
  aerobic_decoupling?: number | null
  original_filename?: string | null
  created_at: string
  updated_at: string
  // Manually assigned location from a location tag (independent of GPS start_lat/start_lng).
  location_lat?: number | null
  location_lng?: number | null
  has_edits?: boolean
  edits_applied_at?: string | null
  lift_count?: number | null
  lift_distance_m?: number | null
  lift_ascent_m?: number | null
  lift_descent_m?: number | null
  lift_duration_s?: number | null
  exclude_lift_subtraction?: boolean | null
  elevation_source?: string | null
  distance_source?: string | null
  // Multisport parent/child
  parent_id?: string | null
  child_count?: number
  // Fusion result: maps stream fields to source activity IDs.
  fusion_sources?: Record<string, string> | null
  // Platform IDs for deep-linking back to the source platform.
  garmin_activity_id?: number | null
  strava_activity_id?: number | null
  komoot_tour_id?: number | null
  // Sport conflict resolution
  sport_conflict_pending?: boolean
  sport_conflict?: {
    file_sport: string
    file_source: string
    platform_sport: string
    platform_source: string
  } | null
  // Recording device (linked to hardware registry)
  recording_device_id?: string | null
  recording_device_name?: string | null

  // ── Phase 1: Running Dynamics ──
  avg_vertical_oscillation_mm?: number | null
  avg_stance_time_ms?: number | null
  avg_stance_time_pct?: number | null
  avg_stance_time_balance_pct?: number | null
  avg_step_length_mm?: number | null
  avg_vertical_ratio_pct?: number | null

  // ── Phase 1: Vertical Speed ──
  avg_pos_vertical_speed_ms?: number | null
  avg_neg_vertical_speed_ms?: number | null
  max_pos_vertical_speed_ms?: number | null
  max_neg_vertical_speed_ms?: number | null
  avg_vam_ms?: number | null

  // ── Phase 1: Grade ──
  avg_grade_pct?: number | null
  max_pos_grade_pct?: number | null
  max_neg_grade_pct?: number | null

  // ── Phase 1: Pedal Analytics ──
  avg_left_torque_effectiveness_pct?: number | null
  avg_right_torque_effectiveness_pct?: number | null
  avg_left_pedal_smoothness_pct?: number | null
  avg_right_pedal_smoothness_pct?: number | null
  avg_combined_pedal_smoothness_pct?: number | null
  left_right_balance?: number | null
  time_standing_s?: number | null
  stand_count?: number | null

  // ── Phase 1: Swimming ──
  swim_stroke?: string | null
  pool_length_m?: number | null
  pool_length_unit?: string | null
  num_lengths?: number | null
  num_active_lengths?: number | null
  avg_stroke_count?: number | null
  avg_stroke_distance_m?: number | null

  // ── Phase 1: Diving ──
  avg_depth_m?: number | null
  max_depth_m?: number | null
  dive_number?: number | null
  surface_interval_s?: number | null
  start_cns_pct?: number | null
  end_cns_pct?: number | null
  start_n2_pct?: number | null
  end_n2_pct?: number | null
  o2_toxicity_otu?: number | null

  // ── Phase 1: Ball Sports ──
  player_score?: number | null
  opponent_score?: number | null
  opponent_name?: string | null
  max_ball_speed_ms?: number | null
  avg_ball_speed_ms?: number | null

  // ── Phase 1: E-Bike ──
  avg_lev_motor_power_w?: number | null
  max_lev_motor_power_w?: number | null
  lev_battery_consumption_pct?: number | null

  // ── Phase 1: Jumps ──
  jump_count?: number | null

  // ── Phase 1: Cycles / Work ──
  total_cycles?: number | null
  total_work_j?: number | null
  total_fat_calories?: number | null
  metabolic_calories?: number | null
  total_moving_time_s?: number | null
  workout_id?: string | null
  intensity_factor?: number | null

  // ── Phase 1: Misc ──
  sport_profile_name?: string | null
  sdrr_hrv_ms?: number | null
  avg_respiration_rate?: number | null
  avg_core_temperature_c?: number | null
}

export interface ActivitiesResponse {
  activities: Activity[]
  next_cursor: string | null
  total: number
}

export interface StreamPoint {
  // ── Bestehend (10 Felder) ──
  time: string
  lat?: number
  lng?: number
  altitude_m?: number
  speed_ms?: number
  distance_m?: number
  heart_rate?: number
  power_w?: number
  cadence?: number
  temperature_c?: number

  // ── Terrain & Movement ──
  grade_pct?: number
  vertical_speed_ms?: number
  gps_accuracy_m?: number
  calories?: number
  resistance?: number
  total_cycles?: number
  accumulated_power_w?: number
  absolute_pressure_pa?: number
  ascent_rate_ms?: number
  time_from_course_s?: number
  activity_type?: number
  device_index?: number

  // ── Running Dynamics ──
  ground_contact_time_ms?: number
  vertical_oscillation_mm?: number
  stance_time_percent?: number
  step_length_mm?: number
  vertical_ratio_pct?: number
  stance_time_balance_pct?: number
  cycle_length_m?: number
  fractional_cadence?: number

  // ── Pedal Analytics ──
  left_torque_effectiveness_pct?: number
  right_torque_effectiveness_pct?: number
  left_pedal_smoothness_pct?: number
  right_pedal_smoothness_pct?: number
  combined_pedal_smoothness_pct?: number
  left_pco_mm?: number
  right_pco_mm?: number
  left_right_balance?: number

  // ── Power Phase ──
  left_power_phase_start_deg?: number
  left_power_phase_end_deg?: number
  left_power_phase_peak_start_deg?: number
  left_power_phase_peak_end_deg?: number
  right_power_phase_start_deg?: number
  right_power_phase_end_deg?: number
  right_power_phase_peak_start_deg?: number
  right_power_phase_peak_end_deg?: number

  // ── Hemoglobin / Muscle Oxygen ──
  total_hemoglobin_conc?: number
  total_hemoglobin_conc_min?: number
  total_hemoglobin_conc_max?: number
  saturated_hemoglobin_pct?: number
  saturated_hemoglobin_pct_min?: number
  saturated_hemoglobin_pct_max?: number

  // ── E-Bike ──
  motor_power_w?: number
  ebike_travel_range_km?: number
  ebike_battery_level_pct?: number
  ebike_assist_mode?: number
  ebike_assist_level_pct?: number

  // ── Health / Stress ──
  core_temperature_c?: number
  enhanced_respiration_rate?: number
  current_stress?: number
  battery_soc_pct?: number

  // ── MTB ──
  grit?: number
  flow?: number

  // ── Racket / Ball Sports ──
  stroke_type?: number
  zone?: number
  ball_speed_ms?: number

  // ── Diving ──
  depth_m?: number
  next_stop_depth_m?: number
  next_stop_time_s?: number
  time_to_surface_s?: number
  ndl_time_s?: number
  cns_load_pct?: number
  n2_load_pct?: number
  air_time_remaining_s?: number
  pressure_sac?: number
  volume_sac?: number
  rmv?: number
  po2?: number

  // ── Developer Fields ──
  developer_fields?: Record<string, unknown>
}

export interface DataVisibility {
  speed: boolean
  heart_rate: boolean
  power: boolean
  cadence: boolean
  temperature: boolean
  altitude: boolean
}

export interface Sensor {
  id: string
  device_index: number
  source_type: string
  primary_type: string
  primary_type_label: string
  all_types: string[]
  all_type_labels?: string[]
  manufacturer?: string
  product_name?: string
  serial_number?: number
  ant_device_number?: number
  software_version?: string
  battery_status?: string
  device_id?: string | null
}

export interface GeoTag {
  id: string
  activity_id: string
  category: string
  name: string
  osm_id?: number
  lat?: number
  lng?: number
  ele?: number
  dist_m?: number
  duration_min?: number
  meta?: Record<string, unknown>
  map_type?: string
  map_point_lat?: number
  map_point_lon?: number
  map_bbox_west?: number
  map_bbox_south?: number
  map_bbox_east?: number
  map_bbox_north?: number
  map_section?: string
  length_m?: number
  ele_m?: number
  created_at: string
  source?: string | null      // poi, boundary
  admin_level?: number | null // 8 for municipalities
}

export interface EnrichmentSettings {
  user_id: string
  // Detection modules
  enable_summit_detection: boolean
  enable_stop_detection: boolean
  enable_commute_detection: boolean
  enable_ascent_detection: boolean
  enable_lap_detection: boolean
  enable_forest_detection: boolean
  enable_water_detection: boolean
  enable_settlement_detection: boolean
  enable_border_detection: boolean
  enable_lift_detection: boolean
  enable_surface_detection: boolean
  enable_interval_detection: boolean
  enable_embeddings: boolean
  // Non-detection features
  enable_weather_enrichment: boolean
  enable_auto_naming: boolean
  // Thresholds
  climb_min_gradient_pct: number
  climb_min_elevation_m: number
  round_trip_threshold_m: number
  // OCR
  ocr_invoice_prompt?: string | null
  ocr_gear_prompt?: string | null
  // Other settings
  photo_preview_size?: string
  // Terra Incognita Sichtweite in Metern (100–10000, Default 1000). Migration 000121.
  terra_incognita_radius_m?: number
  route_duplicate_hausdorff_m?: number
  route_duplicate_length_diff_km?: number
  video_clip_frames?: number
  // pHash Hamming-distance threshold for photo deduplication (1–10, Default 3). Migration 000149.
  phash_threshold_bits?: number
}

export interface Climb {
  id: string
  climb_number: number
  start_distance_m: number
  end_distance_m: number
  length_m: number
  elevation_gain_m: number
  avg_gradient_pct: number
  max_gradient_pct: number
  category: string
  sport_class?: string | null  // S1-S5 (hiking), leicht/mittel/schwer (generic), null (cycling)
}

export interface ActivityInterval {
  id: string
  activity_id: string
  segment_num: number
  start_time: string
  end_time: string
  duration_s: number
  avg_power_w?: number | null
  avg_heart_rate?: number | null
  avg_speed_ms?: number | null
  avg_cadence?: number | null
  intensity: 'hard' | 'moderate' | 'easy' | 'recovery'
}

export interface ActivityStop {
  id: string
  started_at: string
  ended_at: string
  duration_s: number
  lat?: number
  lng?: number
  name?: string
  poi_type?: string
  poi_name?: string
  poi_osm_id?: number
  poi_category?: string | null  // gastronomy, summit, viewpoint, landmark, etc.
  confidence?: string | null    // poi_matched, speed_fallback
}

export interface ActivitySurface {
  id: string
  category?: 'surface' | 'highway'
  surface_type: string
  path_type: string
  length_m: number
  percentage: number
}

export interface DetectedLap {
  id: string
  activity_id: string
  lap_number: number
  start_time: string
  end_time: string
  elapsed_time_s: number
  distance_m?: number
  avg_speed_ms?: number
  avg_heart_rate?: number
  avg_power_w?: number
  avg_cadence?: number
  total_ascent_m?: number
  total_descent_m?: number
  start_lat?: number
  start_lng?: number
}

export interface Gear {
  id: string
  user_id: string
  name: string
  type: string
  sub_type?: string | null
  brand?: string | null
  model?: string | null
  total_distance_m: number
  total_elevation_gain_m: number
  total_time_s: number
  total_activities: number
  purchased_at?: string | null
  retired_at?: string | null
  is_retired: boolean
  status: GearStatus
  default_for_sports: string[]
  notes?: string | null
  weight_g?: number | null
  components?: GearComponent[]
  created_at: string
}

export interface GearComponent {
  id: string
  gear_id: string
  parent_component_id?: string | null
  name: string
  type: string
  brand?: string | null
  model?: string | null
  total_distance_m: number
  total_elevation_gain_m: number
  total_time_s: number
  purchase_price?: number | null
  purchase_date?: string | null
  weight_g?: number | null
  serial_number?: string | null
  installed_at?: string | null
  removed_at?: string | null
  is_retired: boolean
  status: GearStatus
  notes?: string | null
}

export interface ComponentAssignment {
  id: string
  component_id: string
  gear_id: string
  gear_name: string
  installed_at: string
  removed_at?: string | null
}

export interface ChainRotationGroup {
  id: string
  gear_id: string
  name: string
  swap_interval_km: number
  members: ChainRotationMember[]
  created_at: string
}

export interface ChainRotationMember {
  id: string
  group_id: string
  component_id: string
  component_name: string
  position: number
  is_active: boolean
  distance_since_swap_m: number
}

export interface GearServiceEvent {
  id: string
  component_id: string
  interval_id?: string | null
  performed_at: string
  distance_at_m?: number | null
  description?: string | null
  cost?: number | null
  notes?: string | null
  created_at: string
}

export interface ComponentTypePreset {
  id: string
  name: string
  type: string
  category: string
  gear_type?: string | null
}

export interface MaintenanceAlert {
  gear_id: string
  gear_name: string
  component_id: string
  component_name: string
  interval_name: string
  interval_type: string
  is_overdue: boolean
  current_value: number
  threshold_value: number
}

export interface BikeSetup {
  id: string
  gear_id: string
  name: string
  rider_weight_kg?: number | null
  intended_use?: string | null
  notes?: string | null
  suspension_front?: Record<string, any> | null
  suspension_rear?: Record<string, any> | null
  tires?: Record<string, any> | null
  contact_points?: Record<string, any> | null
  brakes?: Record<string, any> | null
  drivetrain?: Record<string, any> | null
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface GearDocument {
  id: string
  gear_id: string | null
  component_id: string | null
  service_event_id: string | null
  doc_type: 'receipt' | 'invoice' | 'warranty' | 'manual' | 'photo' | 'other'
  title: string | null
  original_filename: string
  content_type: string
  file_size_bytes: number
  file_hash?: string
  document_date?: string | null
  document_amount?: number | null
  document_currency?: string
  processing_status?: 'pending' | 'processed' | 'failed'
  created_at: string
}

export interface AutomationRule {
  id: string
  user_id: string
  name: string
  is_active: boolean
  priority: number
  conditions: RuleCondition[]
  conditions_logic?: 'and' | 'or' | null
  // Optional group-based structure — multiple groups are AND-ed; within each group logic applies
  condition_groups?: RuleConditionGroup[] | null
  actions: RuleAction[]
  times_applied: number
  is_system: boolean
  system_type: string | null
  created_at: string
}

export interface RuleConditionGroup {
  logic: 'and' | 'or'
  conditions: RuleCondition[]
}

export interface RuleCondition {
  field: string
  op: string
  value: any
  // Geofencing params (only when field === 'geofence')
  geofence?: {
    lat: number
    lon: number
    radius_m: number
    mode: 'inside' | 'outside'
  }
  // Zone-based geofence (field === 'geofence_zone')
  geofence_zone?: {
    mode: 'inside' | 'outside'
    shape: 'circle' | 'polygon'
    // circle fields
    lat?: number
    lon?: number
    radius_m?: number
    // polygon field (GeoJSON Polygon JSON string)
    polygon?: string
  }
  // Proximity-to-pin geofence (field === 'geofence_proximity')
  geofence_proximity?: {
    lat: number
    lon: number
    radius_m: number
    mode: 'inside' | 'outside'
  }
}

export interface RuleAction {
  action: 'set_gear' | 'add_tag' | 'remove_tag' | 'set_visibility' | 'set_sport_type' | 'set_name' | 'set_intensity' | 'set_indoor' | string
  params: Record<string, any>
}

export interface UserProfile {
  birth_date?: string | null
  gender: '' | 'M' | 'F' | 'O'
  height_cm?: number | null
  weight_kg?: number | null
  ftp_w?: number | null
  lthr_bpm?: number | null
  max_hr_bpm?: number | null
  resting_hr_bpm?: number | null
  max_hr_formula?: number | null
  max_hr_detected?: number | null
  effective_max_hr: number
}

export interface TimeInZone {
  zone_type: 'hr' | 'power' | 'speed' | 'cadence'
  zone_index: number
  zone_high?: number | null
  seconds: number
}

export interface ZoneDistribution {
  hr_zones: TimeInZone[]
  power_zones: TimeInZone[]
  has_fit_zones: boolean
}

export interface PowerCurvePoint {
  duration_s: number
  power_w: number
  power_w_per_kg?: number | null
}

export interface PowerCurveResponse {
  points: PowerCurvePoint[]
  ftp_w?: number | null
  weight_kg?: number | null
}

export interface GearCostSummary {
  gear_id: string
  user_id: string
  name: string
  total_cost: number
  currency: string
  cost_per_km?: number | null
  cost_per_hour?: number | null
}

export interface Lap {
  id: string
  lap_index: number
  start_time: string
  end_time?: string
  elapsed_time_s?: number
  timer_time_s?: number
  distance_m?: number
  moving_time_s?: number
  total_ascent_m?: number
  total_descent_m?: number
  avg_speed_ms?: number
  max_speed_ms?: number
  avg_heart_rate?: number
  max_heart_rate?: number
  avg_power_w?: number
  max_power_w?: number
  normalized_power_w?: number
  avg_cadence?: number
  max_cadence?: number
  total_calories?: number
  start_lat?: number
  start_lng?: number
  end_lat?: number
  end_lng?: number
  avg_vertical_oscillation_mm?: number
  avg_stance_time_ms?: number
  avg_step_length_mm?: number
  num_lengths?: number
  swim_stroke?: string
  avg_stroke_distance_m?: number
  intensity?: string
  lap_trigger?: string
}

export interface Split {
  id: string
  split_index: number
  split_type: string
  start_time: string
  end_time?: string
  elapsed_time_s?: number
  timer_time_s?: number
  distance_m?: number
  moving_time_s?: number
  total_ascent_m?: number
  total_descent_m?: number
  avg_speed_ms?: number
  max_speed_ms?: number
  avg_vert_speed_ms?: number
  total_calories?: number
  start_elevation_m?: number
  start_lat?: number
  start_lng?: number
  end_lat?: number
  end_lng?: number
  avg_hr?: number
  max_hr?: number
  /** Manuell eingetragener Schwierigkeitsgrad (z.B. "6b", "V4", "7A+") */
  climbing_grade?: string
}

export interface SplitSummary {
  id: string
  summary_index: number
  split_type: string
  num_splits: number
  timer_time_s?: number
  distance_m?: number
  moving_time_s?: number
  total_ascent_m?: number
  total_descent_m?: number
  avg_hr?: number
  max_hr?: number
  total_calories?: number
}

export interface SwimLength {
  id: string
  length_index: number
  length_type: string
  start_time: string
  elapsed_time_s?: number
  timer_time_s?: number
  total_strokes?: number
  avg_speed_ms?: number
  swim_stroke?: string
  avg_swimming_cadence?: number
  total_calories?: number
}

export interface ClimbProEvent {
  id: string
  timestamp: string
  climb_pro_event: string
  climb_number?: number
  climb_category?: number
  current_dist_m?: number
  lat?: number
  lng?: number
}

// ── Strength Training Sets ──
export interface ActivitySet {
  message_index: number
  timestamp?: string
  start_time?: string
  duration_s?: number
  set_type?: number
  repetitions?: number
  weight_kg?: number
  weight_display_unit?: number
  exercise_categories?: number[]
  exercise_subtypes?: number[]
  wkt_step_index?: number
}

// ── Jump Events ──
export interface ActivityJump {
  timestamp: string
  distance_m?: number
  height_m?: number
  hang_time_s?: number
  speed_ms?: number
  score?: number
  rotations?: number
  lat?: number
  lng?: number
}

// ── Activity Events ──
export interface ActivityEvent {
  timestamp: string
  event: number
  event_type: number
  event_group?: number
  data?: number
  front_gear_num?: number
  front_gear?: number
  rear_gear_num?: number
  rear_gear?: number
  radar_threat_level_max?: number
  radar_threat_count?: number
  radar_threat_avg_approach_speed_ms?: number
  radar_threat_max_approach_speed_ms?: number
  score?: number
  opponent_score?: number
  device_index?: number
  activity_type?: number
  start_timestamp?: string
}

// ── Workouts ──
export interface Workout {
  id: string
  name?: string
  description?: string
  sport?: number
  sub_sport?: number
  num_valid_steps?: number
  pool_length_m?: number
  pool_length_unit?: string
  steps: WorkoutStep[]
}

export interface WorkoutStep {
  message_index: number
  step_name?: string
  notes?: string
  duration_type?: number
  duration_value?: number
  target_type?: number
  target_value?: number
  custom_target_low?: number
  custom_target_high?: number
  intensity?: number
  exercise_category?: number
  exercise_name?: number
  exercise_weight_kg?: number
  weight_display_unit?: number
  secondary_target_type?: number
  secondary_target_value?: number
  secondary_custom_target_low?: number
  secondary_custom_target_high?: number
}
