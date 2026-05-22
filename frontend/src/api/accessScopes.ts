import { apiClient } from './client'

export type ObjectAccessScopeKind = 'manual' | 'relationship_rule' | 'link' | 'mixed' | 'system'
export type ObjectAccessPrincipalKind =
  | 'user'
  | 'group'
  | 'relationship_rule'
  | 'link_token'
  | 'team'
  | 'organization'
  | 'role'
  | 'server'
  | 'delegated_audience'
export type CreateObjectAccessPrincipalKind = 'user' | 'link_token'

export interface ObjectAccessGrant {
  id: string
  access_scope_id: string
  principal_kind: ObjectAccessPrincipalKind
  principal_user_id?: string | null
  principal_ref?: string | null
  principal_selector?: Record<string, unknown>
  principal_selector_hash?: string
  permission: 'view'
  created_at: string
  revoked_at?: string | null
}

export interface ObjectAccessScope {
  id: string
  owner_kind: string
  owner_user_id?: string | null
  owner_ref?: string | null
  scope_kind: ObjectAccessScopeKind
  label?: string | null
  created_by?: string | null
  created_at: string
  revoked_at?: string | null
  metadata?: Record<string, unknown>
  grants?: ObjectAccessGrant[]
}

export type CreateObjectAccessGrantRequest =
  | {
    principal_kind: 'user'
    principal_user_id: string
    permission?: 'view'
  }
  | {
    principal_kind: 'link_token'
    link_token?: string | null
    permission?: 'view'
  }

export interface CreateObjectAccessScopeRequest {
  scope_kind?: ObjectAccessScopeKind
  label?: string
  grants: CreateObjectAccessGrantRequest[]
}

export interface ObjectAccessLinkTokenValue {
  grant_id: string
  token: string
}

export interface ObjectAccessScopeResponse {
  scope: ObjectAccessScope
  link_tokens?: ObjectAccessLinkTokenValue[]
}

export interface ListObjectAccessScopesResponse {
  scopes: ObjectAccessScope[]
}

export interface UpdateObjectAccessScopeRequest {
  revoke_scope?: boolean
  revoke_grant_ids?: string[]
}

export function createObjectAccessScope(request: CreateObjectAccessScopeRequest): Promise<ObjectAccessScopeResponse> {
  return apiClient<ObjectAccessScopeResponse>('/access-scopes', { method: 'POST', body: request })
}

export function listObjectAccessScopes(): Promise<ListObjectAccessScopesResponse> {
  return apiClient<ListObjectAccessScopesResponse>('/access-scopes')
}

export function updateObjectAccessScope(scopeId: string, request: UpdateObjectAccessScopeRequest): Promise<{ ok: boolean }> {
  return apiClient<{ ok: boolean }>(`/access-scopes/${scopeId}`, { method: 'PATCH', body: request })
}
