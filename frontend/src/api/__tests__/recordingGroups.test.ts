import { describe, expect, it, vi } from 'vitest'

const { apiClientMock } = vi.hoisted(() => ({
  apiClientMock: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: apiClientMock,
}))

import { setGroupSportType } from '@/api/recordingGroups'

describe('recording group API', () => {
  it('sends complete sport tuple and modifiers when setting group sport', async () => {
    apiClientMock.mockResolvedValueOnce(undefined)

    await setGroupSportType('group-1', {
      sport_type: 'cycling',
      sub_type: 'mountain',
      is_electric: true,
      is_indoor: false,
      is_virtual: true,
    })

    expect(apiClientMock).toHaveBeenCalledWith('/recording-groups/group-1/sport-type', {
      method: 'PATCH',
      body: {
        sport_type: 'cycling',
        sub_type: 'mountain',
        is_electric: true,
        is_indoor: false,
        is_virtual: true,
      },
    })
  })
})
