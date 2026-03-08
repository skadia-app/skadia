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
}

export async function fetchPeakCollection(): Promise<PeakVisit[]> {
  return apiClient<PeakVisit[]>('/peaks/collection')
}
