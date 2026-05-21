import { apiClient } from './client'

export type TargetVisibility = 'private' | 'shared' | 'public'
export type TargetPlaceKind = 'custom' | 'private_place' | 'poi' | 'entrance' | 'parking' | 'summit' | 'pass' | 'water' | 'settlement' | 'amenity'
export type TargetRegionKind = 'custom' | 'private_area' | 'public_pool' | 'venue_area' | 'privacy_zone_mirror' | 'admin_area' | 'protected_area'
export type TargetCollectionKind = 'private_map' | 'public_pool' | 'venue_pool' | 'route_set' | 'course_set' | 'rule_set' | 'privacy_set' | 'custom'
export type TargetCollectionMemberKind = 'course' | 'route_publication' | 'place' | 'region' | 'collection' | 'privacy_zone' | 'tag_zone' | 'external_ref'

export type PointGeometry = { type: 'Point'; coordinates: [number, number] }
export type PolygonGeometry = { type: 'Polygon'; coordinates: number[][][] }
export type MultiPolygonGeometry = { type: 'MultiPolygon'; coordinates: number[][][][] }
export type TargetGeometry = PointGeometry | PolygonGeometry | MultiPolygonGeometry

export interface TargetObjectListParams {
  visibility_mode?: 'mine' | 'public_only' | 'shared_visible_to_me' | 'all_visible'
  kinds?: string[]
  bbox?: [number, number, number, number]
  search?: string
  include_geometry?: boolean
  limit?: number
  offset?: number
}

export interface TargetCollectionListParams extends Omit<TargetObjectListParams, 'include_geometry'> {}

export interface TargetPlace {
  id: string
  owner_user_id?: string | null
  name: string
  description?: string | null
  place_kind: TargetPlaceKind
  geometry?: string | TargetGeometry
  visibility: TargetVisibility
  access_scope_id?: string | null
  origin_key: string
  metadata?: Record<string, unknown>
  updated_at?: string
  archived_at?: string | null
}

export interface TargetRegion {
  id: string
  owner_user_id?: string | null
  name: string
  description?: string | null
  region_kind: TargetRegionKind
  geometry?: string | TargetGeometry
  visibility: TargetVisibility
  access_scope_id?: string | null
  origin_key: string
  metadata?: Record<string, unknown>
  updated_at?: string
  archived_at?: string | null
}

export interface TargetCollectionSummary {
  id: string
  owner_user_id?: string | null
  name: string
  collection_kind: TargetCollectionKind
  visibility: TargetVisibility
  access_scope_id?: string | null
  bounds?: string | TargetGeometry | null
  owner_member_count?: number
  public_member_count?: number
  updated_at?: string
  archived_at?: string | null
}

export interface TargetCollection extends TargetCollectionSummary {
  description?: string | null
  display_style?: 'map_list' | 'ordered_list' | 'directory' | 'compact'
  default_sort?: 'manual' | 'name' | 'distance' | 'updated_at' | 'role_then_manual'
  origin_key?: string
  metadata?: Record<string, unknown>
}

export interface TargetCollectionMember {
  id: string
  collection_id: string
  member_kind: TargetCollectionMemberKind
  member_id?: string | null
  member_ref?: string | null
  binding_mode: 'live_current' | 'pinned_version' | 'pinned_snapshot' | 'external_ref'
  role: string
  sort_order: number
  display_name_override?: string | null
  membership_visibility?: 'inherit_collection' | 'owner_only' | 'public_safe'
  member_visibility_state?: string
  added_at?: string
  removed_at?: string | null
}

export interface TargetCollectionDetail extends TargetCollection {
  members?: TargetCollectionMember[]
  hidden_member_count?: number
}

export type TargetPlaceCreateRequest = Omit<Partial<TargetPlace>, 'id' | 'geometry' | 'place_kind' | 'visibility'> & {
  name: string
  place_kind: TargetPlaceKind
  geometry: TargetGeometry
  visibility: TargetVisibility
}
export type TargetPlaceUpdateRequest = Partial<TargetPlaceCreateRequest>
export type TargetRegionCreateRequest = Omit<Partial<TargetRegion>, 'id' | 'geometry' | 'region_kind' | 'visibility'> & {
  name: string
  region_kind: TargetRegionKind
  geometry: PolygonGeometry | MultiPolygonGeometry
  visibility: TargetVisibility
}
export type TargetRegionUpdateRequest = Partial<TargetRegionCreateRequest>
export type TargetCollectionCreateRequest = Omit<Partial<TargetCollection>, 'id' | 'collection_kind' | 'visibility'> & {
  name: string
  collection_kind: TargetCollectionKind
  visibility: TargetVisibility
}
export type TargetCollectionUpdateRequest = Partial<TargetCollectionCreateRequest>
export type TargetCollectionMemberCreateRequest = Partial<TargetCollectionMember> & {
  member_kind: TargetCollectionMemberKind
  member_id?: string
  member_ref?: string
}

export interface GovernanceScopeCreateRequest {
  kind: 'user_space' | 'organization' | 'region_scope' | 'collection_scope' | 'system_scope'
  anchor_target_kind?: string
  anchor_target_id?: string
  anchor_collection_id?: string
  owner_scope: 'user' | 'shared' | 'global' | 'system'
  visibility?: TargetVisibility
  origin_key?: string
  metadata?: Record<string, unknown>
}

export interface GovernanceActorRoleCreateRequest {
  actor_type: 'user' | 'group' | 'system' | 'external_party_credit'
  actor_user_id?: string
  actor_ref?: string
  role: string
  permissions?: string[]
  public_credit?: 'hidden' | 'listed' | 'badge'
  metadata?: Record<string, unknown>
}

export interface GovernanceBindingCreateRequest {
  target_kind: string
  target_id: string
  binding_reason?: string
  permissions_scope?: string
  source?: string
  metadata?: Record<string, unknown>
}

export interface GovernanceEffectiveTargetParams {
  target_kind?: string
  target_id?: string
  permission?: string
  permissions_scope?: string
  limit?: number
  offset?: number
}

export interface GovernanceScopeResponse { scope: unknown }
export interface GovernanceActorRoleResponse { role: unknown }
export interface GovernanceBindingResponse { binding: unknown }
export interface GovernanceEffectiveTargetsResponse { targets: unknown[] }
export interface TargetPlaceResponse { place: TargetPlace }
export interface TargetRegionResponse { region: TargetRegion }
export interface TargetCollectionResponse { collection: TargetCollection }
export interface TargetCollectionDetailResponse { collection: TargetCollectionDetail }
export interface TargetCollectionMemberResponse { member: TargetCollectionMember }
export interface TargetPlaceListResponse { places: TargetPlace[] }
export interface TargetRegionListResponse { regions: TargetRegion[] }
export interface TargetCollectionListResponse { collections: TargetCollectionSummary[] }

export function listTargetPlaces(params: TargetObjectListParams = {}): Promise<TargetPlaceListResponse> {
  return apiClient<TargetPlaceListResponse>(withQuery('/target-places', targetObjectListQuery(params)))
}

export function createTargetPlace(request: TargetPlaceCreateRequest): Promise<TargetPlaceResponse> {
  return apiClient<TargetPlaceResponse>('/target-places', { method: 'POST', body: request })
}

export function updateTargetPlace(id: string, request: TargetPlaceUpdateRequest): Promise<TargetPlaceResponse> {
  return apiClient<TargetPlaceResponse>(`/target-places/${id}`, { method: 'PATCH', body: request })
}

export function deleteTargetPlace(id: string): Promise<{ ok: boolean }> {
  return apiClient<{ ok: boolean }>(`/target-places/${id}`, { method: 'DELETE' })
}

export function listTargetRegions(params: TargetObjectListParams = {}): Promise<TargetRegionListResponse> {
  return apiClient<TargetRegionListResponse>(withQuery('/target-regions', targetObjectListQuery(params)))
}

export function createTargetRegion(request: TargetRegionCreateRequest): Promise<TargetRegionResponse> {
  return apiClient<TargetRegionResponse>('/target-regions', { method: 'POST', body: request })
}

export function updateTargetRegion(id: string, request: TargetRegionUpdateRequest): Promise<TargetRegionResponse> {
  return apiClient<TargetRegionResponse>(`/target-regions/${id}`, { method: 'PATCH', body: request })
}

export function deleteTargetRegion(id: string): Promise<{ ok: boolean }> {
  return apiClient<{ ok: boolean }>(`/target-regions/${id}`, { method: 'DELETE' })
}

export function listTargetCollections(params: TargetCollectionListParams = {}): Promise<TargetCollectionListResponse> {
  return apiClient<TargetCollectionListResponse>(withQuery('/target-collections', targetObjectListQuery(params)))
}

export function createTargetCollection(request: TargetCollectionCreateRequest): Promise<TargetCollectionResponse> {
  return apiClient<TargetCollectionResponse>('/target-collections', { method: 'POST', body: request })
}

export function getTargetCollection(id: string, options: { include_members?: boolean } = {}): Promise<TargetCollectionDetailResponse> {
  return apiClient<TargetCollectionDetailResponse>(withQuery(`/target-collections/${id}`, options.include_members ? { include_members: 'true' } : {}))
}

export function addTargetCollectionMember(collectionId: string, request: TargetCollectionMemberCreateRequest): Promise<TargetCollectionMemberResponse> {
  return apiClient<TargetCollectionMemberResponse>(`/target-collections/${collectionId}/members`, { method: 'POST', body: request })
}

export function removeTargetCollectionMember(collectionId: string, memberId: string): Promise<{ ok: boolean }> {
  return apiClient<{ ok: boolean }>(`/target-collections/${collectionId}/members/${memberId}`, { method: 'DELETE' })
}

export function createGovernanceScope(request: GovernanceScopeCreateRequest): Promise<GovernanceScopeResponse> {
  return apiClient<GovernanceScopeResponse>('/governance-scopes', { method: 'POST', body: { origin_key: 'manual', ...request } })
}

export function addGovernanceRole(scopeId: string, request: GovernanceActorRoleCreateRequest): Promise<GovernanceActorRoleResponse> {
  return apiClient<GovernanceActorRoleResponse>(`/governance-scopes/${scopeId}/roles`, { method: 'POST', body: request })
}

export function addGovernanceBinding(scopeId: string, request: GovernanceBindingCreateRequest): Promise<GovernanceBindingResponse> {
  return apiClient<GovernanceBindingResponse>(`/governance-scopes/${scopeId}/bindings`, { method: 'POST', body: request })
}

export function listGovernanceEffectiveTargets(params: GovernanceEffectiveTargetParams = {}): Promise<GovernanceEffectiveTargetsResponse> {
  return apiClient<GovernanceEffectiveTargetsResponse>(withQuery('/governance/effective-targets', cleanQuery(params)))
}

function targetObjectListQuery(params: TargetObjectListParams): Record<string, string> {
  return cleanQuery({
    visibility_mode: params.visibility_mode,
    kinds: params.kinds?.join(','),
    bbox: params.bbox?.join(','),
    search: params.search,
    include_geometry: params.include_geometry,
    limit: params.limit,
    offset: params.offset,
  })
}

function cleanQuery(params: object): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '') continue
    out[key] = String(value)
  }
  return out
}

function withQuery(path: string, params: Record<string, string>): string {
  const query = new URLSearchParams(params)
  const raw = query.toString()
  return raw ? `${path}?${raw}` : path
}
