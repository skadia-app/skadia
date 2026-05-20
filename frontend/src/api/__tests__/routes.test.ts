import { beforeEach, describe, expect, it, vi } from 'vitest'

const { apiClientMock } = vi.hoisted(() => ({
  apiClientMock: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: apiClientMock,
  apiDownload: vi.fn(),
  apiUpload: vi.fn(),
}))

vi.mock('@/lib/utils', () => ({
  triggerBlobDownload: vi.fn(),
}))

import {
  createRouteCourseRelationship,
  createRouteRelationship,
  createRouteSnapshot,
  forkRoute,
  listRoutePublications,
  listRoutes,
  publishRouteCourse,
} from '@/api/routes'
import { createObjectAccessScope, listObjectAccessScopes, updateObjectAccessScope } from '@/api/accessScopes'
import { fetchSegments, type CreateSegmentPayload, type UpdateSegmentPayload } from '@/api/segments'

describe('routes API', () => {
  beforeEach(() => {
    apiClientMock.mockReset()
  })

  it('serializes limit and offset for paginated route lists', async () => {
    apiClientMock.mockResolvedValueOnce({ routes: [], total: 129, limit: 50, offset: 0 })

    await listRoutes({ limit: 50, offset: 0, sort: 'newest' })

    expect(apiClientMock).toHaveBeenCalledWith('/routes?limit=50&offset=0&sort=newest')
  })

  it('serializes published course search filters separately from route lists', async () => {
    apiClientMock.mockResolvedValueOnce({ courses: [], limit: 25, offset: 0 })

    await listRoutePublications({
      visibility_mode: 'public_only',
      sport_type: 'bike',
      sac_max_rank: 3,
      max_distance_m: 1000,
      has_variants: true,
    })

    expect(apiClientMock).toHaveBeenCalledWith(
      '/route-publications?visibility_mode=public_only&sport_type=bike&sac_max_rank=3&max_distance_m=1000&has_variants=true',
    )
  })

  it('posts route snapshot requests with explicit operation and source version', async () => {
    apiClientMock.mockResolvedValueOnce({ snapshot_id: 'snap-1' })

    await createRouteSnapshot('route-1', 'op-1', 12)

    expect(apiClientMock).toHaveBeenCalledWith('/routes/route-1/snapshots', {
      method: 'POST',
      body: { operation_id: 'op-1', source_route_version: 12 },
    })
  })

  it('posts route course publication requests unchanged', async () => {
    apiClientMock.mockResolvedValueOnce({ publication_id: 'pub-1' })

    await publishRouteCourse('route-1', {
      route_snapshot_id: 'snap-1',
      source_route_version: 12,
      publish_policy: 'new_course',
      equivalence_decision: 'new_group',
      visibility: 'shared',
      access_scope_id: 'scope-1',
      compatibility_preset: 'trail',
      operation_id: 'op-2',
    })

    expect(apiClientMock).toHaveBeenCalledWith('/routes/route-1/publish-course', {
      method: 'POST',
      body: {
        route_snapshot_id: 'snap-1',
        source_route_version: 12,
        publish_policy: 'new_course',
        equivalence_decision: 'new_group',
        visibility: 'shared',
        access_scope_id: 'scope-1',
        compatibility_preset: 'trail',
        operation_id: 'op-2',
      },
    })
  })

  it('posts fork and relationship operations with caller-supplied operation ids', async () => {
    apiClientMock.mockResolvedValueOnce({ route_id: 'fork-1', relationship_id: 'rel-1' })
    await forkRoute('route-1', { name: 'Variante', source_route_version: 3, operation_id: 'op-fork' })
    expect(apiClientMock).toHaveBeenLastCalledWith('/routes/route-1/fork', {
      method: 'POST',
      body: { name: 'Variante', source_route_version: 3, operation_id: 'op-fork' },
    })

    apiClientMock.mockResolvedValueOnce({ relationship_id: 'rel-2' })
    await createRouteRelationship('route-1', { target_route_id: 'route-2', relation_type: 'variant_of', operation_id: 'op-route-rel' })
    expect(apiClientMock).toHaveBeenLastCalledWith('/routes/route-1/relationships', {
      method: 'POST',
      body: { target_route_id: 'route-2', relation_type: 'variant_of', operation_id: 'op-route-rel' },
    })

    apiClientMock.mockResolvedValueOnce({ relationship_id: 'rel-3' })
    await createRouteCourseRelationship('pub-1', { target_publication_id: 'pub-2', relation_type: 'variant_of', operation_id: 'op-course-rel' })
    expect(apiClientMock).toHaveBeenLastCalledWith('/route-publications/pub-1/relationships', {
      method: 'POST',
      body: { target_publication_id: 'pub-2', relation_type: 'variant_of', operation_id: 'op-course-rel' },
    })
  })

  it('serializes shared segment visibility and exposes shared payload types', async () => {
    apiClientMock.mockResolvedValueOnce({ segments: [], has_more: false })

    await fetchSegments({ visibility: 'shared' })

    expect(apiClientMock).toHaveBeenCalledWith('/segments?visibility=shared')

    const createPayload: CreateSegmentPayload = {
      name: 'shared segment',
      sport_group: 'bike',
      segment_type: 'standard',
      geojson_track: '{"type":"LineString","coordinates":[[0,0],[1,1]]}',
      visibility: 'shared',
      access_scope_id: 'scope-1',
    }
    const updatePayload: UpdateSegmentPayload = {
      visibility: 'shared',
      access_scope_id: 'scope-1',
    }
    expect(createPayload.visibility).toBe('shared')
    expect(updatePayload.access_scope_id).toBe('scope-1')
  })

  it('wraps access-scope create list and revoke without edit permissions', async () => {
    apiClientMock.mockResolvedValueOnce({ scope: { id: 'scope-1' }, link_tokens: [{ grant_id: 'grant-1', token: 'raw' }] })

    await createObjectAccessScope({
      scope_kind: 'mixed',
      grants: [
        { principal_kind: 'user', principal_user_id: 'user-2', permission: 'view' },
        { principal_kind: 'link_token', permission: 'view' },
      ],
    })

    expect(apiClientMock).toHaveBeenCalledWith('/access-scopes', {
      method: 'POST',
      body: {
        scope_kind: 'mixed',
        grants: [
          { principal_kind: 'user', principal_user_id: 'user-2', permission: 'view' },
          { principal_kind: 'link_token', permission: 'view' },
        ],
      },
    })

    apiClientMock.mockResolvedValueOnce({ scopes: [] })
    await listObjectAccessScopes()
    expect(apiClientMock).toHaveBeenLastCalledWith('/access-scopes')

    apiClientMock.mockResolvedValueOnce({ ok: true })
    await updateObjectAccessScope('scope-1', { revoke_grant_ids: ['grant-1'], revoke_scope: true })
    expect(apiClientMock).toHaveBeenLastCalledWith('/access-scopes/scope-1', {
      method: 'PATCH',
      body: { revoke_grant_ids: ['grant-1'], revoke_scope: true },
    })
  })
})
