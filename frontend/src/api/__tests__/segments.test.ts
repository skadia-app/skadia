import { beforeEach, describe, expect, it, vi } from 'vitest'

const { apiClientMock } = vi.hoisted(() => ({
  apiClientMock: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: apiClientMock,
  apiDownload: vi.fn(),
}))

import { fetchSegments, suggestSegmentEnrichment } from '@/api/segments'

describe('segment API', () => {
  beforeEach(() => {
    apiClientMock.mockReset()
  })

  it('adds elevation_kind to segment search query parameters', async () => {
    apiClientMock.mockResolvedValueOnce({ segments: [], has_more: false })

    await fetchSegments({ elevation_kind: 'uphill' })

    expect(apiClientMock).toHaveBeenCalledWith('/segments?elevation_kind=uphill')
  })

  it('posts segment geometry to the enrichment suggestion endpoint', async () => {
    apiClientMock.mockResolvedValueOnce({
      surface: { dominant: 'Asphalt', distribution: { Asphalt: 1 }, mixed: false },
      sac_scale: null,
      mtb_scale: { dominant: 'S1', max: 'S2', distribution: { S1: 0.8, S2: 0.2 } },
      coverage_pct: 0.91,
    })

    const response = await suggestSegmentEnrichment({
      coordinates: [[0, 0], [0, 1]],
      seg_type: 'trail',
      dominant_sport: 'cycling:mountain',
    })

    expect(response.coverage_pct).toBe(0.91)
    expect(apiClientMock).toHaveBeenCalledWith('/segments/enrichment-suggest', {
      method: 'POST',
      body: {
        coordinates: [[0, 0], [0, 1]],
        seg_type: 'trail',
        dominant_sport: 'cycling:mountain',
      },
    })
  })
})
