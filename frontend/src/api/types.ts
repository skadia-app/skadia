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

export interface Activity {
  id: string
  user_id: string
  title: string | null
  description?: string | null
  sport_type: SportType
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
  gear_id?: string | null
  created_at: string
  updated_at: string
}

export interface ActivitiesResponse {
  activities: Activity[]
  next_cursor: string | null
  total: number  // Gesamtzahl aller Aktivitäten des Nutzers
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

// Privacy / Visibility – Felder die vom Backend noch nicht geliefert werden (Type-Stubs)
// TODO: Backend muss hide_map und data_visibility in Activity-Response aufnehmen
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
  climb_min_gradient_pct: number
  climb_min_elevation_m: number
  round_trip_threshold_m: number
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
  total_time_s: number
  total_activities: number
  purchased_at?: string | null
  retired_at?: string | null
  is_retired: boolean
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
  installed_at?: string | null
  removed_at?: string | null
  is_retired: boolean
  notes?: string | null
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

export interface AutomationRule {
  id: string
  user_id: string
  name: string
  is_active: boolean
  priority: number
  conditions: RuleCondition[]
  actions: RuleAction[]
  times_applied: number
  created_at: string
}

export interface RuleCondition {
  field: string
  op: string
  value: any
}

export interface RuleAction {
  action: string
  params: Record<string, any>
}
