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

export async function backfillWeather(): Promise<{ processed: number; errors: number; error_details?: string }> {
  return apiClient<{ processed: number; errors: number; error_details?: string }>('/admin/backfill-weather', { method: 'POST' })
}

export async function backfillEnrichment(): Promise<{ processed: number; errors: number; error_details?: string }> {
  return apiClient<{ processed: number; errors: number; error_details?: string }>('/admin/backfill-enrichment', { method: 'POST' })
}

export async function fetchStops(activityId: string): Promise<ActivityStop[]> {
  return apiClient<ActivityStop[]>(`/activities/${activityId}/stops`)
}

export async function fetchSurfaces(activityId: string): Promise<ActivitySurface[]> {
  return apiClient<ActivitySurface[]>(`/activities/${activityId}/surfaces`)
}

export async function fetchVisitedPOIs(): Promise<GeoJSON.FeatureCollection> {
  const res = await apiFetch('/api/v1/geo/visited-pois')
  if (!res.ok) throw new Error(`fetchVisitedPOIs: ${res.status}`)
  return res.json()
}
