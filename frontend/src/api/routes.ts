import { apiClient, apiUpload, apiDownload } from './client'
import type { Tag } from './tags'

// === Types ===

export interface RouteWaypoint {
  id: string                      // UUID (frontend-generated)
  latlng: [number, number]
  type: 'start' | 'via' | 'end'
  poi_id?: number                 // Reference to osm_pois
  poi_name?: string               // e.g. "Bettelwurfhütte (2077m)"
  poi_type?: string               // e.g. "alpine_hut", "peak"
}

export interface RouteSegment {
  from_waypoint: string           // Waypoint ID
  to_waypoint: string             // Waypoint ID
  mode: 'routed' | 'offroad'
  geometry?: GeoJSON.LineString   // BRouter or offroad polyline
  control_points: [number, number][] // Offroad: shape points
  smooth: boolean                 // Catmull-Rom spline on/off
  distance_m: number
  elevation_gain: number          // DEM-based
  elevation_loss: number
}

export interface RoutePhoto {
  id: string
  s3_key_thumb: string
  s3_key_preview?: string
  original_name?: string
  sort_order: number
}

export interface PlannedRoute {
  id: string
  name: string
  description?: string            // Markdown
  sport_type: string
  difficulty?: string
  profile: string                 // BRouter profile
  waypoints: RouteWaypoint[]
  segments: RouteSegment[]
  total_distance_m?: number
  total_elevation_gain?: number
  total_elevation_loss?: number
  estimated_time_min?: number
  time_override: boolean
  auto_tag: boolean
  geometry_geojson?: string        // ST_AsGeoJSON — present in list (simplified) and detail
  photos: RoutePhoto[]
  tags: Tag[]
  created_at: string
  updated_at: string
}

export interface Capabilities {
  routing: boolean
  offline_map: boolean
  surface_enrichment: boolean
  dem_correction: boolean
  isochrone: boolean
  route_suggestion: boolean
  ml_service: boolean
}

export interface POIResult {
  name: string
  type: string
  ele?: number
  latlng: [number, number]
  osm_id: number
}

export interface ElevationPoint {
  fraction: number
  elevation: number
}

export interface IsochronePolygon {
  minutes: number
  geometry: GeoJSON.Polygon
}

export interface IsochronePOI {
  name: string
  type: string
  minutes: number
  latlng: [number, number]
}

export interface IsochroneResult {
  polygons: IsochronePolygon[]
  pois: IsochronePOI[]
}

export interface RouteSuggestionParams {
  start: [number, number]
  end?: [number, number]
  round_trip: boolean
  direction_point?: [number, number]
  target_distance_m: number
  target_elevation_gain: number
  sport_type: string
  profile: string
  prefer_unknown?: boolean  // Terra Incognita bias
  landscape_prefs: {
    forest: number    // 0-10
    water: number     // 0-10
    meadow: number    // 0-10
  }
  surface_prefs: {
    trail: number        // 0-10
    forest_road: number  // 0-10
    asphalt: number      // 0-10
  }
}

export interface ListRoutesResponse {
  routes: PlannedRoute[]
  total: number
  limit: number
  offset: number
}

export interface ListRoutesParams {
  sort?: string
  sport_type?: string
}

export interface BRouterRequest {
  waypoints: [number, number][]
  profile: string
}

// === Capabilities ===

export async function getCapabilities(): Promise<Capabilities> {
  return apiClient('/capabilities')
}

// === Routes CRUD ===

export async function listRoutes(params?: ListRoutesParams): Promise<ListRoutesResponse> {
  const qs = params ? '?' + new URLSearchParams(
    Object.fromEntries(Object.entries(params).filter(([, v]) => v !== undefined)) as Record<string, string>
  ).toString() : ''
  return apiClient(`/routes${qs}`)
}

export async function getRoute(id: string): Promise<PlannedRoute> {
  return apiClient(`/routes/${id}`)
}

export async function createRoute(route: Partial<PlannedRoute>): Promise<PlannedRoute> {
  return apiClient('/routes', { method: 'POST', body: route })
}

export async function updateRoute(id: string, route: Partial<PlannedRoute>): Promise<PlannedRoute> {
  return apiClient(`/routes/${id}`, { method: 'PATCH', body: route })
}

export async function deleteRoute(id: string): Promise<void> {
  return apiClient(`/routes/${id}`, { method: 'DELETE' })
}

export async function duplicateRoute(id: string): Promise<{ id: string }> {
  return apiClient(`/routes/${id}/duplicate`, { method: 'POST' })
}

// === GPX Export ===

export async function exportRouteGPX(id: string): Promise<{ blob: Blob; filename?: string }> {
  return apiDownload(`/routes/${id}/gpx`)
}

// === Route Photos ===

export async function uploadRoutePhoto(routeId: string, file: File): Promise<RoutePhoto> {
  const fd = new FormData()
  fd.append('photo', file)
  return apiUpload(`/routes/${routeId}/photos`, fd)
}

export async function deleteRoutePhoto(routeId: string, photoId: string): Promise<void> {
  return apiClient(`/routes/${routeId}/photos/${photoId}`, { method: 'DELETE' })
}

export async function reorderRoutePhotos(routeId: string, photoIds: string[]): Promise<void> {
  return apiClient(`/routes/${routeId}/photos/reorder`, { method: 'PUT', body: { photo_ids: photoIds } })
}

// === Route Tags ===

export async function setRouteTags(routeId: string, tagIds: string[]): Promise<void> {
  return apiClient(`/routes/${routeId}/tags`, { method: 'PUT', body: { tag_ids: tagIds } })
}

export async function addRouteTag(routeId: string, tagId: string): Promise<void> {
  return apiClient(`/routes/${routeId}/tags/${tagId}`, { method: 'POST' })
}

export async function removeRouteTag(routeId: string, tagId: string): Promise<void> {
  return apiClient(`/routes/${routeId}/tags/${tagId}`, { method: 'DELETE' })
}

// === BRouter Proxy (Compute Route) ===

export async function computeRoute(
  waypoints: [number, number][],
  profile: string,
): Promise<GeoJSON.FeatureCollection> {
  return apiClient('/routing/route', { method: 'POST', body: { waypoints, profile } })
}

// === Elevation ===

export async function queryElevation(
  points: [number, number][],
): Promise<{ elevations: number[]; source: string }> {
  return apiClient('/elevation/query', { method: 'POST', body: { points } })
}

export async function getElevationProfile(
  geometry: GeoJSON.LineString,
  numPoints: number = 100,
): Promise<{ profile: ElevationPoint[] }> {
  return apiClient('/elevation/profile', { method: 'POST', body: { geometry, num_points: numPoints } })
}

// === POI Search ===

export async function searchPOIs(
  q: string,
  types?: string,
  limit?: number,
): Promise<POIResult[]> {
  const params = new URLSearchParams({ q })
  if (types) params.set('types', types)
  if (limit !== undefined) params.set('limit', String(limit))
  return apiClient(`/search/pois?${params.toString()}`)
}

// === Isochrone ===

export async function getIsochrone(
  start: [number, number],
  minutes: number[],
  profile: string,
  poiTypes?: string[],
): Promise<IsochroneResult> {
  return apiClient('/isochrone', {
    method: 'POST',
    body: { start, minutes, profile, poi_types: poiTypes },
  })
}

// === Route from Activity ===

export async function createRouteFromActivity(
  activityId: string | number,
): Promise<PlannedRoute> {
  return apiClient(`/routes/from-activity/${activityId}`, { method: 'POST' })
}

// Returns waypoints derived from an activity GPS track WITHOUT saving a route to DB.
export async function getWaypointsFromActivity(
  activityId: string | number,
): Promise<{ waypoints: RouteWaypoint[] }> {
  return apiClient(`/routes/from-activity/${activityId}/waypoints`)
}

// === Route Suggestion ===

export interface RouteSuggestion {
  variant: number
  profile: string
  via_point?: [number, number]
  geojson: GeoJSON.FeatureCollection
  is_fallback: boolean
}

export async function suggestRoute(
  params: RouteSuggestionParams,
): Promise<{ suggestions: RouteSuggestion[] }> {
  return apiClient('/routes/suggest', { method: 'POST', body: params })
}
