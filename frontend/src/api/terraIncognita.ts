import { apiClient } from './client'

export interface TerraIncognitaCoverage {
  /** 5 concentric ring geometries (innermost → outermost) for gradient fog. */
  rings: GeoJSON.Geometry[]
  area_km2: number
}

export function fetchTerraIncognita(): Promise<TerraIncognitaCoverage> {
  return apiClient<TerraIncognitaCoverage>('/terra-incognita/coverage')
}
