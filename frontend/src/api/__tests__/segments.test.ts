import { beforeEach, describe, expect, it, vi } from 'vitest'

const { apiClientMock } = vi.hoisted(() => ({
  apiClientMock: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: apiClientMock,
  apiDownload: vi.fn(),
}))

import {
  createCheckpointCourse,
  fetchActivitySegments,
  fetchSegment,
  fetchSegments,
  importCheckpointCourseGeoJSON,
  suggestSegmentEnrichment,
} from '@/api/segments'

describe('segment API', () => {
  beforeEach(() => {
    apiClientMock.mockReset()
  })

  it('adds elevation_kind to segment search query parameters', async () => {
    apiClientMock.mockResolvedValueOnce({ segments: [], has_more: false })

    await fetchSegments({ elevation_kind: 'uphill' })

    expect(apiClientMock).toHaveBeenCalledWith('/segments?elevation_kind=uphill')
  })

  it('adds checkpoint course list query parameters', async () => {
    apiClientMock.mockResolvedValueOnce({ segments: [], has_more: false })

    await fetchSegments({ course_kind: 'checkpoint_sequence', include_control_map_features: true })

    expect(apiClientMock).toHaveBeenCalledWith('/segments?course_kind=checkpoint_sequence&include_control_map_features=true')
  })

  it('adds multi course kind query parameters', async () => {
    apiClientMock.mockResolvedValueOnce({ segments: [], has_more: false })

    await fetchSegments({ course_kinds: ['line', 'checkpoint_sequence'] })

    expect(apiClientMock).toHaveBeenCalledWith('/segments?course_kinds=line%2Ccheckpoint_sequence')
  })

  it('adds include_controls on segment detail requests', async () => {
    apiClientMock.mockResolvedValueOnce({ id: 'seg-1' })

    await fetchSegment('seg-1', { includeGeoJSON: true, includeControls: true })

    expect(apiClientMock).toHaveBeenCalledWith('/segments/seg-1?include_geojson=true&include_controls=true')
  })

  it('adds include_overlay_summary on segment detail requests', async () => {
    apiClientMock.mockResolvedValueOnce({ id: 'seg-1' })

    await fetchSegment('seg-1', { includeOverlaySummary: true })

    expect(apiClientMock).toHaveBeenCalledWith('/segments/seg-1?include_overlay_summary=true')
  })

  it('posts checkpoint course creation payloads through the segment endpoint', async () => {
    apiClientMock.mockResolvedValueOnce({ id: 'seg-1' })
    const control = {
      sequence_index: 0,
      role: 'start' as const,
      primitive: 'point' as const,
      geometry_shape: 'point' as const,
      geometry: { type: 'Point', coordinates: [8, 49] },
      hit_radius_m: 30,
      required: true,
    }

    await createCheckpointCourse({
      name: 'Checkpoint',
      sport_group: 'bike',
      controls: [control],
    })

    expect(apiClientMock).toHaveBeenCalledWith('/segments', {
      method: 'POST',
      body: {
        name: 'Checkpoint',
        sport_group: 'bike',
        controls: [control],
        course_kind: 'checkpoint_sequence',
        segment_type: 'standard',
        geojson_track: '',
      },
    })
  })

  it('posts checkpoint GeoJSON imports to the segment import endpoint', async () => {
    apiClientMock.mockResolvedValueOnce({ id: 'seg-1' })
    const payload = { type: 'FeatureCollection', features: [] }

    await importCheckpointCourseGeoJSON(payload)

    expect(apiClientMock).toHaveBeenCalledWith('/segments/import', {
      method: 'POST',
      body: payload,
    })
  })

  it('fetches activity segment efforts with checkpoint metadata support', async () => {
    apiClientMock.mockResolvedValueOnce([])

    await fetchActivitySegments('activity-1')

    expect(apiClientMock).toHaveBeenCalledWith('/activities/activity-1/segments')
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
