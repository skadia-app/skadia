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

  it('serializes commute modifier filters to the activities endpoint', async () => {
    apiClientMock.mockResolvedValueOnce({ activities: [], total: 0 })

    await fetchActivities(undefined, 20, undefined, undefined, {
      isCommute: false,
    })

    expect(apiClientMock).toHaveBeenCalledWith('/activities?limit=20&is_commute=false')
  })

  it('serializes pending lift review filters to the activities endpoint', async () => {
    apiClientMock.mockResolvedValueOnce({ activities: [], total: 0 })

    await fetchActivities(undefined, 20, undefined, undefined, {
      liftReview: 'pending',
    })

    expect(apiClientMock).toHaveBeenCalledWith('/activities?limit=20&lift_review=pending')
  })
})
