import { apiClient } from './client'

export type MapLayerRole = 'base' | 'overlay'
export type MapLayerSourceKind = 'raster_tiles' | 'vector_style' | 'protomaps_offline'

export interface MapLayer {
  id: number
  key: string
  role: MapLayerRole
  name: string
  source_kind: MapLayerSourceKind
  url_template?: string
  style_url?: string
  attribution?: string
  min_zoom?: number
  max_zoom?: number
  default_opacity: number
  is_system: boolean
  enabled: boolean
  sort_order: number
  proxy_through_backend: boolean
  requires_capability?: string
  created_at: string
  updated_at: string
}

export interface SystemLayerVisibility {
  layer_key: string
  enabled: boolean
  default_enabled_for_new_users: boolean
  default_opacity?: number
  label_override?: string
  updated_at: string
}

export interface SystemLayerDescriptor {
  key: string
  category: 'vector' | 'coverage'
  composite_group?: 'nature' | 'noise' | 'trafficUrban'
  default_label: string
}

export interface UserMapLayer {
  id: number
  name: string
  url_template: string
  attribution?: string
  min_zoom?: number
  max_zoom?: number
  default_opacity: number
  enabled: boolean
  sort_order: number
  created_at: string
  updated_at: string
}

// Public-authenticated: GET /api/v1/map-layers
export async function fetchMapLayers(): Promise<{
  layers: MapLayer[]
  system_visibility: Record<string, SystemLayerVisibility>
}> {
  return apiClient('/map-layers')
}

// Public (no auth): GET /api/v1/system-layers/keys
export async function fetchSystemLayerKeys(): Promise<SystemLayerDescriptor[]> {
  return apiClient('/system-layers/keys')
}

// Admin CRUD — routes under /api/v1/admin/map-layers
export async function createMapLayer(body: Partial<MapLayer>): Promise<MapLayer> {
  return apiClient('/admin/map-layers', { method: 'POST', body })
}

/**
 * Patch an admin-managed map layer.
 *
 * @param ifMatchUpdatedAt RFC3339Nano timestamp of the layer's current updated_at;
 *   required when patching url_template or style_url (server returns 428 otherwise).
 *   Uses X-If-Match-Updated-At to avoid RFC 7232 quoting issues with raw If-Match.
 */
export async function patchMapLayer(
  id: number,
  body: Partial<MapLayer>,
  ifMatchUpdatedAt?: string,
): Promise<void> {
  const headers: Record<string, string> = {}
  if (ifMatchUpdatedAt) headers['X-If-Match-Updated-At'] = ifMatchUpdatedAt
  await apiClient(`/admin/map-layers/${id}`, { method: 'PATCH', body, headers })
}

export async function deleteMapLayer(id: number): Promise<void> {
  await apiClient(`/admin/map-layers/${id}`, { method: 'DELETE' })
}

export async function bulkSortMapLayers(items: { id: number; sort_order: number }[]): Promise<void> {
  await apiClient('/admin/map-layers/sort', { method: 'POST', body: items })
}

export async function patchSystemLayerVisibility(
  key: string,
  body: Partial<SystemLayerVisibility>,
): Promise<void> {
  await apiClient(`/admin/system-layer-visibility/${key}`, { method: 'PATCH', body })
}

// User-managed custom tile-URL layers — routes under /api/v1/map-layers/user/
export async function fetchUserMapLayers(): Promise<UserMapLayer[]> {
  return apiClient('/map-layers/user/')
}

export async function createUserMapLayer(body: Partial<UserMapLayer>): Promise<UserMapLayer> {
  return apiClient('/map-layers/user/', { method: 'POST', body })
}

export async function patchUserMapLayer(id: number, body: Partial<UserMapLayer>): Promise<void> {
  await apiClient(`/map-layers/user/${id}`, { method: 'PATCH', body })
}

export async function deleteUserMapLayer(id: number): Promise<void> {
  await apiClient(`/map-layers/user/${id}`, { method: 'DELETE' })
}

export async function bulkSortUserMapLayers(
  items: { id: number; sort_order: number }[],
): Promise<void> {
  await apiClient('/map-layers/user/sort', { method: 'POST', body: items })
}
