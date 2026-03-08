import { apiClient } from './client'

// Shared filter query string builder
export function buildFilterParams(filters: Record<string, any>): string {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(filters)) {
    if (value === undefined || value === null || value === '') continue
    if (Array.isArray(value) && value.length === 0) continue
    if (Array.isArray(value)) {
      params.set(key, value.join(','))
    } else {
      params.set(key, String(value))
    }
  }
  return params.toString()
}

// === Training Tab Types ===

export interface PMCExtended {
  days: PMCDay[]
  acwr: number
  acwr_status: string
  ramp_rate: number
  monotony: number
  strain: number
  projection: PMCDay[]
}

export interface PMCDay {
  date: string
  tss: number
  ctl: number
  atl: number
  tsb: number
  acwr: number
}

export interface YearProgressionYearData {
  year: number
  monthly: number[]    // 12 values, index 0 = Jan
  cumulative: number[] // 12 cumulative values
  total: number
}

export interface YearProgressionResponse {
  metric: string
  years: YearProgressionYearData[]
  best_year: number  // year number of the best year
}

export interface TrainingVolumePoint {
  date: string
  distance_m: number
  duration_s: number
  elevation_m: number
  tss: number
  count: number
}

export interface ZoneDistribution {
  zone: number
  zone_label: string
  seconds: number
  percentage: number
}

export interface PersonalRecord {
  type: string
  value: number
  unit: string
  activity_id: string
  activity_name: string
  date: string
}

// === Health Tab Types ===

export interface HRVPoint {
  date: string
  rmssd: number | null
}

export interface HRVAnalysis {
  points: HRVPoint[]
  baseline_mean: number
  baseline_std: number
  seven_day_avg: number
  anomalies: string[]
}

export interface HRVCorrelationPoint {
  date: string
  rmssd: number
  daily_tss: number
}

export interface HRVCorrelation {
  points: HRVCorrelationPoint[]
  correlation: number
  interpretation: string
}

export interface HRVRecovery {
  score: number
  level: string
  hrv_factor: number
  sleep_factor: number
  fatigue_factor: number
  recommendation: string
}

export interface SleepPoint {
  date: string
  duration_min: number | null
  score_overall: number | null
  deep_sleep_pct: number | null
  rem_sleep_pct: number | null
}

export interface SleepAnalysis {
  points: SleepPoint[]
  avg_duration_min: number
  avg_score: number
  trend: string
}

export interface SleepEntry {
  date: string
  duration_min: number
  quality: string | null
  deep_min: number | null
  rem_min: number | null
  light_min: number | null
}

export interface WeightPoint {
  date: string
  weight_kg: number
  body_fat_pct: number | null
}

export interface WeightAnalysis {
  points: WeightPoint[]
  current_kg: number
  change_30d: number
}

export interface WeightEntry {
  date: string
  weight_kg: number
  fat_pct: number | null
}

export interface HealthMetric {
  name: string
  current: number
  unit: string
  trend: number
  points: { date: string; value: number }[]
}

// === Training Volume Time-Series (for TrainingVolumeChart) ===

export interface TrainingVolumeSeriesPoint {
  period: string
  sport_type: string
  value: number
}

export async function fetchTrainingVolumeSeries(params: string): Promise<TrainingVolumeSeriesPoint[]> {
  const result = await apiClient<{ periods?: Array<{ period: string; by_sport: Record<string, number> }> } | TrainingVolumeSeriesPoint[]>(`/analytics/training-volume?${params}`)
  if (Array.isArray(result)) return result
  const flat: TrainingVolumeSeriesPoint[] = []
  for (const period of ((result as { periods?: Array<{ period: string; by_sport: Record<string, number> }> })?.periods ?? [])) {
    for (const [sport_type, value] of Object.entries(period.by_sport ?? {})) {
      flat.push({ period: period.period, sport_type, value: value as number })
    }
  }
  return flat
}

// === Zone Distribution Time-Series (for ZoneDistributionChart) ===

export interface ZoneDistributionSeriesPoint {
  period: string
  zone_index: number
  total_seconds: number
}

export async function fetchZoneDistributionSeries(params: string): Promise<ZoneDistributionSeriesPoint[]> {
  const result = await apiClient<{ periods?: Array<{ period: string; zones: Record<string, number> }> } | ZoneDistributionSeriesPoint[]>(`/analytics/zone-distribution?${params}`)
  if (Array.isArray(result)) return result
  const flat: ZoneDistributionSeriesPoint[] = []
  for (const period of ((result as { periods?: Array<{ period: string; zones: Record<string, number> }> })?.periods ?? [])) {
    for (const [zoneKey, seconds] of Object.entries(period.zones ?? {})) {
      const zone_index = parseInt(zoneKey.replace('zone_', ''), 10)
      if (!isNaN(zone_index)) {
        flat.push({ period: period.period, zone_index, total_seconds: seconds as number })
      }
    }
  }
  return flat
}

// === Stats Tab Types ===

export interface LifetimeStats {
  total_activities: number
  total_distance_m: number
  total_elevation_m: number
  total_moving_time_s: number
  total_elapsed_time_s: number
  total_calories: number
  active_days: number
  countries: number
  earth_laps: number
  moon_trips: number
  everest_count: number
  current_day_streak: number
  max_day_streak: number
}

export interface EddingtonResult {
  metric: string
  number: number
  next_levels: { level: number; remaining: number }[]
  top_values?: number[]
}

export interface Distribution {
  label: string
  count: number
  value: number
  percentage: number
  children?: Distribution[]
}

export interface YearlyOverviewRow {
  year: number
  month?: number
  distance_m: number
  elevation_m: number
  duration_s: number
  count: number
  avg_hr?: number | null
  avg_tss?: number | null
}

// === Insights Tab Types ===

export interface ReadinessInsight {
  score: number
  level: string
  factors: { name: string; value: number; status: string }[]
  explanation: string | null
  computed_at: string
}

export interface Warning {
  type: string
  severity: 'info' | 'warning' | 'danger'
  message: string
  value: number
  threshold: number
  explanation: string | null
}

export interface Projection {
  metric: string
  target: number
  current: number
  predicted_date: string | null
  on_track: boolean
  weekly_needed: number
  explanation: string | null
}

export interface ActivityCluster {
  id: number
  label: string
  size: number
  avg_distance_m: number
  avg_hr: number | null
  sport_types: string[]
  description: string | null
}

export interface CorrelationInsight {
  factor_a: string
  factor_b: string
  correlation: number
  direction: 'positive' | 'negative' | 'neutral'
  explanation: string | null
}

// === Health Data Types (from /health/* endpoints) ===

export interface HealthStressEntry {
  user_id: string
  measured_at_utc: string
  stress_level: number | null
  source_type: string
  created_at: string
}

export interface HealthSpO2Entry {
  user_id: string
  measured_at_utc: string
  spo2_pct: number | null
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

export interface HealthFitnessAgeEntry {
  user_id: string
  calendar_date: string
  fitness_age: number | null
  source_type: string
  created_at: string
}

// === API Functions ===

export async function fetchAnalyticsPMC(params: string): Promise<PMCExtended> {
  return apiClient(`/analytics/pmc?${params}`)
}

export async function fetchYearProgression(params: string): Promise<YearProgressionResponse> {
  return apiClient(`/analytics/year-progression?${params}`)
}

export async function fetchTrainingVolume(params: string): Promise<TrainingVolumePoint[]> {
  return apiClient(`/analytics/training-volume?${params}`)
}

export async function fetchZoneDistribution(params: string): Promise<ZoneDistribution[]> {
  return apiClient(`/analytics/zone-distribution?${params}`)
}

export async function fetchPersonalRecords(params: string): Promise<PersonalRecord[]> {
  const result = await apiClient<{ records: PersonalRecord[] } | PersonalRecord[]>(`/analytics/personal-records?${params}`)
  return Array.isArray(result) ? result : ((result as { records: PersonalRecord[] })?.records ?? [])
}

export async function fetchHRVAnalysis(params: string): Promise<HRVAnalysis> {
  return apiClient(`/analytics/hrv?${params}`)
}

export async function fetchSleep(params: string): Promise<SleepEntry[]> {
  return apiClient(`/analytics/sleep?${params}`)
}

export async function fetchWeight(params: string): Promise<WeightEntry[]> {
  return apiClient(`/analytics/weight?${params}`)
}

export async function fetchHealthMetrics(params: string): Promise<HealthMetric[]> {
  const result = await apiClient<{ metrics: HealthMetric[] } | HealthMetric[]>(`/analytics/health-metrics?${params}`)
  return Array.isArray(result) ? result : ((result as { metrics: HealthMetric[] })?.metrics ?? [])
}

export async function fetchLifetimeStats(params: string): Promise<LifetimeStats> {
  return apiClient(`/analytics/lifetime-stats?${params}`)
}

export async function fetchEddington(params: string): Promise<EddingtonResult> {
  return apiClient(`/analytics/eddington?${params}`)
}

export async function fetchDistributions(params: string): Promise<Distribution[]> {
  const result = await apiClient<{ rows?: Distribution[] } | Distribution[]>(`/analytics/distributions?${params}`)
  if (Array.isArray(result)) return result
  return (result as { rows?: Distribution[] })?.rows ?? []
}

export async function fetchYearlyOverview(params: string): Promise<YearlyOverviewRow[]> {
  const result = await apiClient<{ rows?: YearlyOverviewRow[] } | YearlyOverviewRow[]>(`/analytics/yearly-overview?${params}`)
  if (Array.isArray(result)) return result
  return (result as { rows?: YearlyOverviewRow[] })?.rows ?? []
}

export interface StreakPeriod {
  from: string
  to: string
  count: number
}

export interface StreaksResponse {
  current_streak: number
  max_streak: number
  current_week_streak: number
  max_week_streak: number
  streak_history: StreakPeriod[]
}

export async function fetchStreaks(params: string): Promise<StreaksResponse> {
  return apiClient(`/analytics/streaks?${params}`)
}

export async function fetchInsightsReadiness(params: string): Promise<ReadinessInsight> {
  return apiClient(`/analytics/insights/readiness?${params}`)
}

export async function fetchInsightsWarnings(params: string): Promise<Warning[]> {
  return apiClient(`/analytics/insights/warnings?${params}`)
}

export async function fetchInsightsProjection(params: string): Promise<Projection[]> {
  return apiClient(`/analytics/insights/projection?${params}`)
}

export async function fetchInsightsClusters(params: string): Promise<ActivityCluster[]> {
  return apiClient(`/analytics/insights/clusters?${params}`)
}

export async function fetchInsightsCorrelations(params: string): Promise<CorrelationInsight[]> {
  return apiClient(`/analytics/insights/correlations?${params}`)
}

export async function refreshInsights(): Promise<{ status: string }> {
  return apiClient(`/analytics/insights/refresh`, { method: 'POST' })
}

export async function fetchHRVCorrelation(params: string): Promise<HRVCorrelation> {
  return apiClient(`/analytics/hrv/correlation?${params}`)
}

export async function fetchHRVRecovery(): Promise<HRVRecovery> {
  return apiClient('/analytics/hrv/recovery')
}

export async function fetchSleepAnalysis(params: string): Promise<SleepAnalysis> {
  return apiClient(`/analytics/sleep?${params}`)
}

export async function fetchWeightAnalysis(params: string): Promise<WeightAnalysis> {
  return apiClient(`/analytics/weight?${params}`)
}

// === Health Data Fetch Functions (from /health/* endpoints) ===

export async function fetchHealthStress(from: string, to: string): Promise<HealthStressEntry[]> {
  const params = new URLSearchParams({ from, to }).toString()
  return apiClient(`/health/stress?${params}`)
}

export async function fetchHealthSpO2(from: string, to: string): Promise<HealthSpO2Entry[]> {
  const params = new URLSearchParams({ from, to }).toString()
  return apiClient(`/health/spo2?${params}`)
}

export async function fetchHealthBloodPressure(from: string, to: string): Promise<HealthBloodPressureEntry[]> {
  const params = new URLSearchParams({ from, to }).toString()
  return apiClient(`/health/blood-pressure?${params}`)
}

export async function fetchHealthFitnessAge(from: string, to: string): Promise<HealthFitnessAgeEntry[]> {
  const params = new URLSearchParams({ from, to }).toString()
  return apiClient(`/health/fitness-age?${params}`)
}
