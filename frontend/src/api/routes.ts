import { apiClient, apiUpload, apiDownload } from './client'
import type { Tag } from './tags'
import type { ProfileConfig } from '@/lib/routingProfiles'
import type { RouteSubSegment } from '@/lib/routeSurfaceAggregation'

export type { RouteSubSegment }

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
  elevation_estimated?: boolean   // true for offroad segments (no DEM lookup)
  approach_from?: number[][]      // [lon, lat] pairs: click point → edge projection
  approach_to?: number[][]        // [lon, lat] pairs: edge projection → click point
  sub_segments?: RouteSubSegment[] // From BadRouter GeoJSON properties.segments
}

export interface RoutePhoto {
  id: string
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
  elevation: number | null
}

export interface IsochronePolygon {
  minutes: number
  geometry: GeoJSON.Polygon
}

export interface IsochronePOI {
  name: string
  type: string
  lat: number
  lng: number
  osm_id?: number
  minutes: number
}

export interface IsochroneResult {
  polygons: IsochronePolygon[]
  pois: IsochronePOI[]
  truncated?: boolean
}

export interface LandscapePreferences {
  forest: number      // 0-10
  water: number       // 0-10
  protected: number   // 0-10
  low_noise: number   // 0-10
  low_traffic: number // 0-10
}

export interface RouteSuggestionParams {
  start: [number, number]
  end?: [number, number]
  round_trip: boolean
  direction_point?: [number, number]
  target_distance_m: number
  sport_type: string
  profile: string
  prefer_unknown?: boolean  // Terra Incognita bias
  landscape_prefs?: LandscapePreferences
  surface_tendency?: 'trails' | 'forest_roads' | 'mixed' | 'paved'
  max_sac_scale?: number
  max_mtb_scale?: number
  target_ascent_m?: number
  ascent_tolerance_pct?: number
  max_variants?: number
  variant_ids?: string[]
}

export interface RouteSuggestionResponse {
  suggestions: RouteSuggestion[]
  coverage_available: boolean
  partial_search: boolean
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

// === Profile Defaults ===

/** Server-side profile defaults. null = blocked highway type or unset field. */
export interface ProfileDefaultsResponse {
  profile: string
  // Elevation
  uphill_cost: number | null
  downhill_cost: number | null
  uphill_cutoff_pct: number | null
  downhill_cutoff_pct: number | null
  elev_penalty_buffer_m: number | null
  elev_max_buffer_m: number | null
  elev_buffer_reduce: number | null
  // Difficulty
  max_sac_scale: number | null
  sac_preferred: number | null
  max_mtb_scale: number | null
  max_via_ferrata: number | null
  allow_steps: boolean | null
  allow_ferries: boolean | null
  // Enrichment weights
  forest_bonus: number | null
  noise_penalty: number | null
  traffic_penalty: number | null
  protected_bonus: number | null
  // Route adherence
  cycleroute_boost: number | null
  hikeroute_boost: number | null
  // Behavior
  wet_weather: boolean | null
  path_preference: number | null
  turn_cost: number | null
  initial_cost_road: number | null
  pass0_coefficient: number | null
  // Highway costs — null means permanently blocked for this profile
  path_cost: number | null
  track_cost: number | null
  track_grade1_cost: number | null
  track_grade2_cost: number | null
  track_grade3_cost: number | null
  track_grade4_cost: number | null
  track_grade5_cost: number | null
  footway_cost: number | null
  cycleway_cost: number | null
  bridleway_cost: number | null
  pedestrian_cost: number | null
  residential_cost: number | null
  service_cost: number | null
  unclassified_cost: number | null
  tertiary_cost: number | null
  secondary_cost: number | null
  primary_cost: number | null
}

export async function getProfileDefaults(name: string): Promise<ProfileDefaultsResponse> {
  return apiClient(`/routing/profiles/defaults/${encodeURIComponent(name)}`)
}

// === Internal Router (BadRouter) ===

export async function computeInternalRoute(
  from: [number, number],
  to: [number, number],
  config?: ProfileConfig,
): Promise<GeoJSON.FeatureCollection> {
  return apiClient('/routing/route-internal', {
    method: 'POST',
    body: {
      from,
      to,
      config: config ?? { profile: 'hiking' },
    },
  })
}

// === Route Suggestion ===

export interface RouteSuggestion {
  variant: number
  profile: string
  via_point: [number, number]

  variant_id: string
  variant_label_key: string

  distance_m: number
  ascent_m: number
  descent_m: number
  avg_flat: number

  loop_quality: 'loop' | 'partial_loop' | 'out_and_back'
  overlap_pct: number

  distance_target_achieved: boolean
  ascent_target_achieved: boolean

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  geojson: any  // Backend sends json.RawMessage — access as geojson.features directly
  is_fallback: boolean
}

export async function suggestRoute(
  params: RouteSuggestionParams,
): Promise<RouteSuggestionResponse> {
  return apiClient('/routes/suggest', { method: 'POST', body: params })
}
