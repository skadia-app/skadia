import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mock apiClient before importing the module under test.
vi.mock('./client', () => ({
  apiClient: vi.fn(),
}))

import { apiClient } from './client'
import {
  getLiftDetectionConfig,
  putLiftDetectionConfig,
  deleteLiftDetectionConfig,
  type LiftDetectionConfigResponse,
} from './liftDetectionConfig'

const mockApiClient = apiClient as ReturnType<typeof vi.fn>

const mockResponse: LiftDetectionConfigResponse = {
  effective: { osm: true, heuristic: true },
  user_setting: { osm: true, heuristic: true },
  override: { osm: null, heuristic: null },
  master_enabled: true,
}

describe('getLiftDetectionConfig', () => {
  beforeEach(() => vi.clearAllMocks())

  it('calls GET /activities/{id}/lifts/detection-config', async () => {
    mockApiClient.mockResolvedValueOnce(mockResponse)

    const result = await getLiftDetectionConfig('act-123')

    expect(mockApiClient).toHaveBeenCalledWith('/activities/act-123/lifts/detection-config')
    expect(result).toEqual(mockResponse)
  })
})

describe('putLiftDetectionConfig', () => {
  beforeEach(() => vi.clearAllMocks())

  it('calls PUT with the correct body', async () => {
    // 204 → apiClient returns null; putLiftDetectionConfig returns void
    mockApiClient.mockResolvedValueOnce(null)

    await putLiftDetectionConfig('act-123', { osm: false, heuristic: true })

    expect(mockApiClient).toHaveBeenCalledWith('/activities/act-123/lifts/detection-config', {
      method: 'PUT',
      body: { osm: false, heuristic: true },
    })
  })

  it('returns void (does not throw) on 204', async () => {
    mockApiClient.mockResolvedValueOnce(null)
    await expect(putLiftDetectionConfig('act-1', { osm: true })).resolves.toBeUndefined()
  })

  it('propagates ApiError when request fails', async () => {
    mockApiClient.mockRejectedValueOnce(new Error('Network error'))
    await expect(putLiftDetectionConfig('act-1', {})).rejects.toThrow('Network error')
  })
})

describe('deleteLiftDetectionConfig', () => {
  beforeEach(() => vi.clearAllMocks())

  it('calls DELETE on the detection-config endpoint', async () => {
    mockApiClient.mockResolvedValueOnce(null)

    await deleteLiftDetectionConfig('act-456')

    expect(mockApiClient).toHaveBeenCalledWith('/activities/act-456/lifts/detection-config', {
      method: 'DELETE',
    })
  })

  it('returns void on 204', async () => {
    mockApiClient.mockResolvedValueOnce(null)
    await expect(deleteLiftDetectionConfig('act-1')).resolves.toBeUndefined()
  })
})
