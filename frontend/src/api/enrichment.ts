import { apiClient } from './client'
import type { GeoTag, EnrichmentSettings } from './types'

export async function fetchGeoTags(activityId: string): Promise<GeoTag[]> {
  return apiClient<GeoTag[]>(`/activities/${activityId}/geo-tags`)
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
