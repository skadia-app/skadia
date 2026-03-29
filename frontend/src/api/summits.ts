import { apiClient } from './client'

export interface SummitVisit {
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

export interface SummitCollectionFilter {
  country?: string
  min_elevation?: number
  max_elevation?: number
  year?: number
}

export interface SummitActivity {
  activity_id: string
  title: string
  started_at: string
  sport_type: string
  distance_m: number | null
  elevation_gain_m: number | null
  timezone?: string | null
}

export async function fetchSummitCollection(filter?: SummitCollectionFilter): Promise<SummitVisit[]> {
  const params = new URLSearchParams()
  if (filter?.country) params.set('country', filter.country)
  if (filter?.min_elevation != null) params.set('min_elevation', String(filter.min_elevation))
  if (filter?.max_elevation != null) params.set('max_elevation', String(filter.max_elevation))
  if (filter?.year != null) params.set('year', String(filter.year))
  const qs = params.toString()
  return apiClient<SummitVisit[]>(`/summits/collection${qs ? '?' + qs : ''}`)
}

export async function fetchSummitActivities(osmId: number | null, name: string): Promise<SummitActivity[]> {
  if (osmId != null) {
    return apiClient<SummitActivity[]>(`/summits/${osmId}/activities`)
  }
  const qs = new URLSearchParams({ name }).toString()
  return apiClient<SummitActivity[]>(`/summits/by-name/activities?${qs}`)
}
