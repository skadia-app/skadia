import { apiClient } from './client'

// ── Types ─────────────────────────────────────────────────────────────────────

export interface HealthDailySummary {
  user_id: string
  calendar_date: string // YYYY-MM-DD
  total_steps: number | null
  step_goal: number | null
  total_distance_m: number | null
  active_seconds: number | null
  highly_active_seconds: number | null
  resting_heart_rate: number | null
  min_heart_rate: number | null
  max_heart_rate: number | null
  total_calories: number | null
  active_calories: number | null
  bmr_calories: number | null
  stress_avg: number | null
  body_battery_min: number | null
  body_battery_max: number | null
  hydration_ml: number | null
  hydration_goal_ml: number | null
  source_type: string
  created_at: string
}

export interface HealthSleep {
  user_id: string
  calendar_date: string
  sleep_start_utc: string
  sleep_end_utc: string
  deep_seconds: number | null
  light_seconds: number | null
  rem_seconds: number | null
  awake_seconds: number | null
  score_overall: number | null
  score_quality: number | null
  score_duration: number | null
  score_deep: number | null
  score_rem: number | null
  respiration_avg: number | null
  awake_count: number | null
  restless_moments: number | null
  avg_sleep_stress: number | null
  source_type: string
  created_at: string
}

export interface HealthHRVSnapshot {
  user_id: string
  measured_at_utc: string
  rmssd_ms: number | null
  sdrr_ms: number | null
  time_of_day: string | null // MORNING/AFTERNOON/EVENING/NIGHT
  source_type: string
  created_at: string
}

export interface HealthWeightEntry {
  user_id: string
  measured_at_utc: string
  weight_g: number
  bmi: number | null
  body_fat_pct: number | null
  muscle_mass_g: number | null
  bone_mass_g: number | null
  source_type: string
  created_at: string
}

export interface HealthBloodPressureEntry {
  user_id: string
  measured_at_utc: string
  systolic: number | null
  diastolic: number | null
  pulse: number | null
  source_type: string
  created_at: string
}

export interface HealthReadinessScore {
  user_id: string
  calendar_date: string
  score: number | null
  level: string | null // LOW/MODERATE/HIGH
  sleep_score: number | null
  hrv_weekly_avg_ms: number | null
  recovery_time_min: number | null
  acwr_pct: number | null
  hrv_factor_pct: number | null
  sleep_history_factor_pct: number | null
  stress_history_factor_pct: number | null
  recovery_time_factor_pct: number | null
  source_type: string
  created_at: string
}

export interface HealthTrainingLoad {
  user_id: string
  calendar_date: string
  atl_acute: number | null
  ctl_chronic: number | null
  acwr_pct: number | null
  acwr_status: string | null // LOW/BALANCED/HIGH/OVERREACHING
  source_type: string
  created_at: string
}

export interface HealthOverview {
  daily_summary?: HealthDailySummary
  sleep?: HealthSleep
  hrv?: HealthHRVSnapshot
  weight?: HealthWeightEntry
  readiness?: HealthReadinessScore
}

export interface WeightEntryInput {
  weight_g: number
  measured_at_utc?: string
  bmi?: number | null
  body_fat_pct?: number | null
  muscle_mass_g?: number | null
  bone_mass_g?: number | null
}

// ── API Functions ─────────────────────────────────────────────────────────────

export async function fetchHealthOverview(): Promise<HealthOverview> {
  try {
    return await apiClient<HealthOverview>('/health/overview')
  } catch {
    return {}
  }
}

export async function fetchHealthDailySummary(from: string, to: string): Promise<HealthDailySummary[]> {
  try {
    return await apiClient<HealthDailySummary[]>(`/health/daily-summary?from=${from}&to=${to}`)
  } catch {
    return []
  }
}

export async function fetchHealthSleep(from: string, to: string): Promise<HealthSleep[]> {
  try {
    return await apiClient<HealthSleep[]>(`/health/sleep?from=${from}&to=${to}`)
  } catch {
    return []
  }
}

export async function fetchHealthHRV(from: string, to: string): Promise<HealthHRVSnapshot[]> {
  try {
    return await apiClient<HealthHRVSnapshot[]>(`/health/hrv?from=${from}&to=${to}`)
  } catch {
    return []
  }
}

export async function fetchHealthWeight(from: string, to: string): Promise<HealthWeightEntry[]> {
  try {
    return await apiClient<HealthWeightEntry[]>(`/health/weight?from=${from}&to=${to}`)
  } catch {
    return []
  }
}

export async function fetchHealthReadiness(from: string, to: string): Promise<HealthReadinessScore[]> {
  try {
    return await apiClient<HealthReadinessScore[]>(`/health/readiness?from=${from}&to=${to}`)
  } catch {
    return []
  }
}

export async function fetchHealthTrainingLoad(from: string, to: string): Promise<HealthTrainingLoad[]> {
  try {
    return await apiClient<HealthTrainingLoad[]>(`/health/training-load?from=${from}&to=${to}`)
  } catch {
    return []
  }
}

export async function createWeightEntry(data: WeightEntryInput): Promise<HealthWeightEntry> {
  return apiClient<HealthWeightEntry>('/health/weight', { method: 'POST', body: data })
}
