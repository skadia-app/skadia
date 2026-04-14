import { apiClient } from './client'
import { apiFetch } from '@/lib/auth'
import type { GeoTag, Climb, ActivityInterval, EnrichmentSettings, ActivityStop, ActivitySurface } from './types'
import type * as GeoJSON from 'geojson'

export async function fetchGeoTags(activityId: string): Promise<GeoTag[]> {
  return apiClient<GeoTag[]>(`/activities/${activityId}/geo-tags`)
}

export async function fetchActivityClimbs(activityId: string): Promise<Climb[]> {
  return apiClient<Climb[]>(`/activities/${activityId}/climbs`)
}

export async function fetchActivityIntervals(activityId: string): Promise<ActivityInterval[]> {
  return apiClient<ActivityInterval[]>(`/activities/${activityId}/intervals`)
}

export async function fetchEnrichmentSettings(): Promise<EnrichmentSettings> {
  return apiClient<EnrichmentSettings>('/settings/enrichment')
}

export async function updateEnrichmentSettings(settings: Partial<EnrichmentSettings>): Promise<EnrichmentSettings> {
  return apiClient<EnrichmentSettings>('/settings/enrichment', { method: 'PUT', body: settings })
}

export async function backfillWeather(): Promise<{ job_id: string; status: string }> {
  return apiClient<{ job_id: string; status: string }>('/admin/backfill-weather', { method: 'POST' })
}

export async function backfillEnrichment(): Promise<{ processed: number; errors: number; error_details?: string }> {
  return apiClient<{ processed: number; errors: number; error_details?: string }>('/admin/backfill-enrichment', { method: 'POST' })
}

/**
 * Reports how many GPS activities currently have no resolved start_city.
 * These are the candidates for the user-facing "Geocoding nachholen" action
 * in Settings → Karte → Standort-Erkennung.
 */
export async function fetchGeocodingGap(): Promise<{ count: number }> {
  return apiClient<{ count: number }>('/enrichment/geocoding-gap')
}

/**
 * Starts a background job that reverse-geocodes every activity without a
 * start_city using PostGIS + the user's configured fallback provider. Requires
 * the provider to be set to something other than "off" — otherwise the server
 * returns 400 with `{ error_code: "provider_disabled" }` and the thrown
 * ApiError exposes the parsed body via `ApiError.json`.
 *
 * When another backfill is already in flight, the server returns 409 with
 * `{ error_code: "already_running", job_id }`. Callers should discriminate on
 * `err.json?.error_code` rather than pattern-matching the localized message.
 */
export async function startGeocodingBackfill(): Promise<{ job_id: string; total: number }> {
  return apiClient<{ job_id: string; total: number }>('/enrichment/geocoding-backfill', { method: 'POST' })
}

export async function fetchStops(activityId: string): Promise<ActivityStop[]> {
  return apiClient<ActivityStop[]>(`/activities/${activityId}/stops`)
}

export async function fetchSurfaces(activityId: string): Promise<ActivitySurface[]> {
  return apiClient<ActivitySurface[]>(`/activities/${activityId}/surfaces`)
}

export interface EnrichmentModuleStatus {
  name: string
  status: 'completed' | 'error' | 'no_data' | 'pending'
  error?: string
  count: number
}

export interface EnrichmentStatusResponse {
  enrichment_status: string
  modules: EnrichmentModuleStatus[]
}

export async function fetchEnrichmentStatus(activityId: string): Promise<EnrichmentStatusResponse> {
  return apiClient<EnrichmentStatusResponse>(`/activities/${activityId}/enrichment-status`)
}

export async function fetchVisitedPOIs(): Promise<GeoJSON.FeatureCollection> {
  const res = await apiFetch('/api/v1/geo/visited-pois')
  if (!res.ok) throw new Error(`fetchVisitedPOIs: ${res.status}`)
  return res.json()
}
