import { describe, expect, it, vi } from 'vitest'

const { apiClientMock } = vi.hoisted(() => ({
  apiClientMock: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: apiClientMock,
  apiDownload: vi.fn(),
  API_BASE: '/api/v1',
}))

vi.mock('@/lib/utils', () => ({
  triggerBlobDownload: vi.fn(),
}))

import { fetchActivities } from '@/api/activities'

describe('activities API filters', () => {
  it('preserves composite sport selection keys in sport_type', async () => {
    apiClientMock.mockResolvedValueOnce({ activities: [], total: 0 })

    await fetchActivities(undefined, 20, undefined, undefined, {
      sportTypes: ['cycling:mountain', 'running:trail'],
    })

    expect(apiClientMock).toHaveBeenCalledWith('/activities?limit=20&sport_type=cycling%3Amountain%2Crunning%3Atrail')
  })
})
