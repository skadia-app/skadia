import { beforeEach, describe, expect, it, vi } from 'vitest'

const { apiClientMock } = vi.hoisted(() => ({
  apiClientMock: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: apiClientMock,
}))

import {
  addGovernanceRole,
  addTargetCollectionMember,
  createGovernanceScope,
  createTargetCollection,
  createTargetPlace,
  createTargetRegion,
  deleteTargetCollection,
  listTargetPlaces,
  updateTargetCollection,
  updateTargetCollectionMember,
  updateTargetPlace,
} from '@/api/targetObjects'

describe('target object api', () => {
  beforeEach(() => {
    apiClientMock.mockReset()
  })

  it('serializes target place geometry for the backend create contract', async () => {
    apiClientMock.mockResolvedValueOnce({ place: { id: 'place-1' } })

    await createTargetPlace({
      name: 'Treffpunkt',
      place_kind: 'custom',
      geometry: { type: 'Point', coordinates: [10.1, 47.7] },
      visibility: 'private',
    })

    expect(apiClientMock).toHaveBeenCalledWith('/target-places', {
      method: 'POST',
      body: {
        name: 'Treffpunkt',
        place_kind: 'custom',
        geometry: '{"type":"Point","coordinates":[10.1,47.7]}',
        visibility: 'private',
      },
    })
  })

  it('serializes target place update geometry for the backend patch contract', async () => {
    apiClientMock.mockResolvedValueOnce({ place: { id: 'place-1' } })

    await updateTargetPlace('place-1', {
      geometry: { type: 'Point', coordinates: [10.2, 47.8] },
      visibility: 'shared',
    })

    expect(apiClientMock).toHaveBeenCalledWith('/target-places/place-1', {
      method: 'PATCH',
      body: {
        geometry: '{"type":"Point","coordinates":[10.2,47.8]}',
        visibility: 'shared',
      },
    })
  })

  it('omits unsupported target place list filters from query strings', async () => {
    apiClientMock.mockResolvedValueOnce({ places: [] })

    await listTargetPlaces({
      kinds: ['custom', 'parking'],
      bbox: [10, 47, 11, 48],
      search: 'Start',
      limit: 25,
      offset: 50,
    })

    expect(apiClientMock).toHaveBeenCalledWith(
      '/target-places?kinds=custom%2Cparking&bbox=10%2C47%2C11%2C48&search=Start&limit=25&offset=50',
    )
  })

  it('serializes target region geometry for the backend create contract', async () => {
    apiClientMock.mockResolvedValueOnce({ region: { id: 'region-1' } })

    await createTargetRegion({
      name: 'Bereich',
      region_kind: 'custom',
      geometry: {
        type: 'Polygon',
        coordinates: [
          [
            [10, 47],
            [11, 47],
            [11, 48],
            [10, 48],
            [10, 47],
          ],
        ],
      },
      visibility: 'public',
    })

    expect(apiClientMock).toHaveBeenCalledWith(
      '/target-regions',
      expect.objectContaining({
        method: 'POST',
        body: expect.objectContaining({
          region_kind: 'custom',
          geometry: '{"type":"Polygon","coordinates":[[[10,47],[11,47],[11,48],[10,48],[10,47]]]}',
        }),
      }),
    )
  })

  it('creates collections and members through /target-collections', async () => {
    apiClientMock.mockResolvedValueOnce({ collection: { id: 'collection-1' } }).mockResolvedValueOnce({ member: { id: 'member-1' } })

    await createTargetCollection({ name: 'Sammlung', collection_kind: 'private_map', visibility: 'private' })
    await addTargetCollectionMember('collection-1', { member_kind: 'place', member_id: 'place-1', role: 'poi' })

    expect(apiClientMock).toHaveBeenNthCalledWith(1, '/target-collections', expect.objectContaining({ method: 'POST' }))
    expect(apiClientMock).toHaveBeenNthCalledWith(2, '/target-collections/collection-1/members', {
      method: 'POST',
      body: { member_kind: 'place', member_id: 'place-1', role: 'poi' },
    })
  })

  it('updates and archives collections through typed helpers', async () => {
    apiClientMock.mockResolvedValueOnce({ collection: { id: 'collection-1' } }).mockResolvedValueOnce({ ok: true })

    await updateTargetCollection('collection-1', { name: 'Neue Sammlung', visibility: 'public' })
    await deleteTargetCollection('collection-1')

    expect(apiClientMock).toHaveBeenNthCalledWith(1, '/target-collections/collection-1', {
      method: 'PATCH',
      body: { name: 'Neue Sammlung', visibility: 'public' },
    })
    expect(apiClientMock).toHaveBeenNthCalledWith(2, '/target-collections/collection-1', { method: 'DELETE' })
  })

  it('patches collection members through typed helpers', async () => {
    apiClientMock.mockResolvedValueOnce({ member: { id: 'member-1' } })

    await updateTargetCollectionMember('collection-1', 'member-1', {
      role: 'start',
      sort_order: 3,
      display_name_override: 'Startpunkt',
    })

    expect(apiClientMock).toHaveBeenCalledWith('/target-collections/collection-1/members/member-1', {
      method: 'PATCH',
      body: {
        role: 'start',
        sort_order: 3,
        display_name_override: 'Startpunkt',
      },
    })
  })

  it('creates governance scopes and roles through /governance-scopes', async () => {
    apiClientMock.mockResolvedValueOnce({ scope: { id: 'scope-1' } }).mockResolvedValueOnce({ role: { id: 'role-1' } })

    await createGovernanceScope({ kind: 'collection_scope', anchor_collection_id: 'collection-1', owner_scope: 'user', visibility: 'private' })
    await addGovernanceRole('scope-1', { actor_type: 'user', actor_user_id: 'user-1', role: 'maintainer', permissions: ['manage_members'] })

    expect(apiClientMock).toHaveBeenNthCalledWith(1, '/governance-scopes', expect.objectContaining({ method: 'POST' }))
    expect(apiClientMock).toHaveBeenNthCalledWith(2, '/governance-scopes/scope-1/roles', {
      method: 'POST',
      body: { actor_type: 'user', actor_user_id: 'user-1', role: 'maintainer', permissions: ['manage_members'] },
    })
  })
})
