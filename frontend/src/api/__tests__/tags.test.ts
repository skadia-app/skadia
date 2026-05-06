import { describe, expect, it, vi } from 'vitest'

const { apiClientMock } = vi.hoisted(() => ({
  apiClientMock: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: apiClientMock,
  apiUpload: vi.fn(),
}))

import { assignActivitiesToTag, unassignActivitiesFromTag } from '@/api/tags'

describe('tags API activity assignment', () => {
  it('can defer activity file refresh for batched tag edits', async () => {
    apiClientMock.mockResolvedValue(undefined)

    await assignActivitiesToTag('tag-1', ['act-1'], { deferActivityFileRefresh: true })
    await unassignActivitiesFromTag('tag-2', ['act-1'], { deferActivityFileRefresh: true })

    expect(apiClientMock).toHaveBeenNthCalledWith(1, '/tags/tag-1/assign-activities?defer_activity_file_refresh=true', {
      method: 'POST',
      body: { activity_ids: ['act-1'] },
    })
    expect(apiClientMock).toHaveBeenNthCalledWith(2, '/tags/tag-2/activities?defer_activity_file_refresh=true', {
      method: 'DELETE',
      body: { activity_ids: ['act-1'] },
    })
  })
})
