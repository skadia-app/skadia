import { apiClient } from './client'

// ── Zone type constants ───────────────────────────────────────────────────────

export const ZONE_TYPES = {
  HR: 'hr',
  POWER: 'power',
  SPEED: 'speed',
} as const

export type ZoneType = typeof ZONE_TYPES[keyof typeof ZONE_TYPES]

// ── Types ─────────────────────────────────────────────────────────────────────

export interface ZoneDetail {
  zone_names: string[]
  zone_highs: number[]
  threshold_heart_rate?: number
  functional_threshold_power?: number
  threshold_speed_mps?: number
  source_type: string
}

export interface SportZones {
  sport: number
  sport_label: string
  zones: Record<string, ZoneDetail | null> // keys: "hr", "power", "speed"
}

export interface TrainingZonesResponse {
  sports: SportZones[]
}

export interface TrainingZoneUpdate {
  sport: number
  sub_sport: number
  zone_type: string
  functional_threshold_power?: number
  threshold_heart_rate?: number
  threshold_speed_mps?: number
  zone_names: string[]
  zone_highs: number[]
}

// ── API Functions ─────────────────────────────────────────────────────────────

export async function fetchTrainingZones(): Promise<TrainingZonesResponse> {
  return apiClient<TrainingZonesResponse>('/training-zones')
}

export async function updateTrainingZones(data: TrainingZoneUpdate): Promise<void> {
  await apiClient<unknown>('/training-zones', { method: 'PUT', body: data })
}
