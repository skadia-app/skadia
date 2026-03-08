import { apiClient } from './client'

export interface ExplorationCoverage {
  explored_area: GeoJSON.Geometry
  area_km2: number
}

export function fetchExplorationCoverage(bbox?: string): Promise<ExplorationCoverage> {
  const params = bbox ? `?bbox=${bbox}` : ''
  return apiClient<ExplorationCoverage>(`/exploration/coverage${params}`)
}
