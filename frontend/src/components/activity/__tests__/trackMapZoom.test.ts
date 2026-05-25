import { describe, expect, it } from 'vitest'
import type { StreamPoint } from '@/api/types'
import type { ZoomRange } from '@/components/ActivityCharts'
import { buildTrackMapFitTarget } from '../trackMapZoom'

describe('buildTrackMapFitTarget', () => {
  const streams: StreamPoint[] = [
    { time: '2026-01-01T00:00:00Z', lat: 47.0, lng: 11.0 },
    { time: '2026-01-01T00:01:00Z' },
    { time: '2026-01-01T00:02:00Z', lat: 47.2, lng: 11.2 },
    { time: '2026-01-01T00:03:00Z', lat: 47.3, lng: 11.3 },
  ]
  const coordinates: [number, number][] = [[11.0, 47.0], [11.2, 47.2], [11.3, 47.3]]

  it('uses the selected chart index range for brush and wheel zoom', () => {
    const zoomRange: ZoomRange = { startIdx: 1, endIdx: 3, startKm: 1, endKm: 3, startPct: 10, endPct: 30 }

    expect(buildTrackMapFitTarget(streams, coordinates, zoomRange)).toEqual({
      coordinates: [[11.2, 47.2], [11.3, 47.3]],
      padding: 50,
      maxZoom: 18,
    })
  })

  it('uses the full valid track when chart zoom is cleared', () => {
    expect(buildTrackMapFitTarget(streams, coordinates, null)).toEqual({
      coordinates,
      padding: 40,
      maxZoom: 15,
    })
  })

  it('returns null when the selected range has fewer than two GPS points', () => {
    const zoomRange: ZoomRange = { startIdx: 0, endIdx: 1, startKm: 0, endKm: 1, startPct: 0, endPct: 10 }

    expect(buildTrackMapFitTarget(streams, coordinates, zoomRange)).toBeNull()
  })
})
