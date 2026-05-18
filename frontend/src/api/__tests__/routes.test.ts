import { describe, expect, it, vi } from 'vitest'

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

import { listRoutes } from '@/api/routes'

describe('routes API', () => {
  it('serializes limit and offset for paginated route lists', async () => {
    apiClientMock.mockResolvedValueOnce({ routes: [], total: 129, limit: 50, offset: 0 })

    await listRoutes({ limit: 50, offset: 0, sort: 'newest' })

    expect(apiClientMock).toHaveBeenCalledWith('/routes?limit=50&offset=0&sort=newest')
  })
})
