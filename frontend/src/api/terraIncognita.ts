import { apiClient } from './client'

export interface TerraIncognitaCoverage {
  terra_incognita_area: GeoJSON.Geometry
  area_km2: number
}

export function fetchTerraIncognita(bbox?: string): Promise<TerraIncognitaCoverage> {
  const params = bbox ? `?bbox=${bbox}` : ''
  return apiClient<TerraIncognitaCoverage>(`/terra-incognita/coverage${params}`)
}
