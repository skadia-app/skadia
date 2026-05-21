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
  listTargetPlaces,
} from '@/api/targetObjects'

describe('target object api', () => {
  beforeEach(() => {
    apiClientMock.mockReset()
  })

  it('creates target places through /target-places', async () => {
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
        geometry: { type: 'Point', coordinates: [10.1, 47.7] },
        visibility: 'private',
      },
    })
  })

  it('serializes target place list filters', async () => {
    apiClientMock.mockResolvedValueOnce({ places: [] })

    await listTargetPlaces({
      visibility_mode: 'shared_visible_to_me',
      kinds: ['custom', 'parking'],
      bbox: [10, 47, 11, 48],
      search: 'Start',
      include_geometry: true,
      limit: 25,
      offset: 50,
    })

    expect(apiClientMock).toHaveBeenCalledWith(
      '/target-places?visibility_mode=shared_visible_to_me&kinds=custom%2Cparking&bbox=10%2C47%2C11%2C48&search=Start&include_geometry=true&limit=25&offset=50',
    )
  })

  it('creates target regions through /target-regions', async () => {
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
        body: expect.objectContaining({ region_kind: 'custom' }),
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
