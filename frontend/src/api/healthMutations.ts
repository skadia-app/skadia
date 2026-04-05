import { apiClient } from './client'
import type { HealthWeightEntry, HealthBloodPressureEntry } from './health'

// ── Blood Pressure ────────────────────────────────────────────────────────────

export interface BloodPressureCreateInput {
  measured_at: string // RFC3339
  systolic: number
  diastolic: number
  pulse?: number
}

export async function createBloodPressure(data: BloodPressureCreateInput): Promise<HealthBloodPressureEntry> {
  return apiClient<HealthBloodPressureEntry>('/health/blood-pressure', { method: 'POST', body: data })
}

// ── Weight ────────────────────────────────────────────────────────────────────

export interface WeightCreateInput {
  measured_at_utc: string // RFC3339
  weight_g: number
  bmi?: number | null
  body_fat_pct?: number | null
  muscle_mass_g?: number | null
  bone_mass_g?: number | null
  subcutaneous_fat_pct?: number | null
  protein_pct?: number | null
  skeletal_muscle_pct?: number | null
  lean_body_mass_g?: number | null
  total_body_water_l?: number | null
}

export async function createWeightEntry(data: WeightCreateInput): Promise<HealthWeightEntry> {
  return apiClient<HealthWeightEntry>('/health/weight', { method: 'POST', body: data })
}
