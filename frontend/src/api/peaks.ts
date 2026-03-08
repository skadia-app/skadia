import { apiClient } from './client'

export interface PeakVisit {
  osm_id: number | null
  name: string
  elevation_m: number | null
  visit_count: number
  first_visit: string
  last_visit: string
  lat: number | null
  lng: number | null
  country_codes: string[]
}

export interface PeakCollectionFilter {
  country?: string
  min_elevation?: number
  max_elevation?: number
  year?: number
}

export async function fetchPeakCollection(filter?: PeakCollectionFilter): Promise<PeakVisit[]> {
  const params = new URLSearchParams()
  if (filter?.country) params.set('country', filter.country)
  if (filter?.min_elevation != null) params.set('min_elevation', String(filter.min_elevation))
  if (filter?.max_elevation != null) params.set('max_elevation', String(filter.max_elevation))
  if (filter?.year != null) params.set('year', String(filter.year))
  const qs = params.toString()
  return apiClient<PeakVisit[]>(`/peaks/collection${qs ? '?' + qs : ''}`)
}
