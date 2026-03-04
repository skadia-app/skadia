/** @deprecated Use sportTaxonomy.ts for sport type lookups. Kept for backwards compatibility. */
export type SportType =
  | 'cycling'
  | 'running'
  | 'hiking'
  | 'swimming'
  | 'e_biking'
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

export interface Activity {
  id: string
  user_id: string
  title: string | null
  description?: string | null
  sport_type: string
  sub_type: string | null
  is_ebike: boolean
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
  weather_temp_c?: number | null
  weather_condition?: string | null
  weather_humidity?: number | null
  weather_wind_speed_ms?: number | null
  weather_tags?: string[]
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
}

export interface ActivitiesResponse {
  activities: Activity[]
  next_cursor: string | null
  total: number
}

export interface StreamPoint {
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
  manufacturer?: string
  product_name?: string
  serial_number?: number
  ant_device_number?: number
  software_version?: string
  battery_status?: string
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
  created_at: string
}

export interface EnrichmentSettings {
  user_id: string
  enable_summit_detection: boolean
  enable_weather_enrichment: boolean
  enable_stop_detection: boolean
  enable_climb_detection: boolean
  enable_commute_detection: boolean
  enable_auto_naming: boolean
  enable_lap_detection: boolean
  climb_min_gradient_pct: number
  climb_min_elevation_m: number
  round_trip_threshold_m: number
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
}

export interface ActivityStop {
  id: string
  started_at: string
  ended_at: string
  duration_s: number
  lat?: number
  lng?: number
  name?: string
}

export interface ActivitySurface {
  id: string
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
  s3_bucket?: string
  s3_key?: string
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
