// frontend/src/components/activity/__tests__/useMetricColoring.test.tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useMetricColoring, splitStreamsAtTeleports, resolveEffectiveTrackBounds } from '../useMetricColoring'
import type { StreamPoint } from '@/api/types'
import type { ZoomRange } from '@/components/ActivityCharts'

// Minimal fake Map — we only test the hook's state logic, not MapLibre integration
function makeFakeMap() {
  const listeners = new Map<string, Array<() => void>>()
  return {
    on: vi.fn((event: string, cb: () => void) => {
      if (!listeners.has(event)) listeners.set(event, [])
      listeners.get(event)!.push(cb)
    }),
    off: vi.fn(),
    once: vi.fn(),
    getSource: vi.fn(() => ({ setData: vi.fn() })),
    getLayer: vi.fn(() => ({ id: 'track-line' })),
    setPaintProperty: vi.fn(),
    setLayoutProperty: vi.fn(),
    isStyleLoaded: vi.fn(() => true),
    getBounds: vi.fn(() => ({
      getWest: () => 11.0, getSouth: () => 47.0,
      getEast: () => 11.1, getNorth: () => 47.1,
    })),
    fitBounds: vi.fn(),
    isMoving: vi.fn(() => false),
    isZooming: vi.fn(() => false),
  } as unknown as import('maplibre-gl').Map
}

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

const streamsWithHR: StreamPoint[] = Array.from({ length: 10 }, (_, i) => ({
  time: `2026-01-01T00:0${i}:00Z`,
  heart_rate: 130 + i,
  lat: 47.0 + i * 0.001, lng: 11.0 + i * 0.001,
}))
const streamsWithoutHR: StreamPoint[] = Array.from({ length: 10 }, (_, i) => ({
  time: `2026-01-01T00:0${i}:00Z`,
  lat: 47.0 + i * 0.001, lng: 11.0 + i * 0.001,
}))

describe('useMetricColoring', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('defaults metric to sport', () => {
    const map = makeFakeMap()
    const { result } = renderHook(() => useMetricColoring({
      streams: streamsWithHR,
      activity: { sport_type: 'running', sub_type: null, fit_sport_id: 1 },
      hasGps: true, zoomRange: null, map,
    }), { wrapper })
    expect(result.current.metric).toBe('sport')
  })

  it('setMetric(hr) switches metric', () => {
    const map = makeFakeMap()
    const { result } = renderHook(() => useMetricColoring({
      streams: streamsWithHR,
      activity: { sport_type: 'running', sub_type: null, fit_sport_id: 1 },
      hasGps: true, zoomRange: null, map,
    }), { wrapper })
    act(() => result.current.setMetric('hr'))
    expect(result.current.metric).toBe('hr')
  })

  it('auto-resets metric to sport when current metric data is not available', () => {
    const map = makeFakeMap()
    const { result, rerender } = renderHook(
      ({ streams }: { streams: StreamPoint[] }) => useMetricColoring({
        streams,
        activity: { sport_type: 'running', sub_type: null, fit_sport_id: 1 },
        hasGps: true, zoomRange: null, map,
      }),
      { wrapper, initialProps: { streams: streamsWithHR } },
    )
    act(() => result.current.setMetric('hr'))
    expect(result.current.metric).toBe('hr')
    // Switch to streams without HR → should reset
    rerender({ streams: streamsWithoutHR })
    expect(result.current.metric).toBe('sport')
  })

  it('scale is null when hasGps=false', () => {
    const map = makeFakeMap()
    const { result } = renderHook(() => useMetricColoring({
      streams: streamsWithHR,
      activity: { sport_type: 'running', sub_type: null, fit_sport_id: 1 },
      hasGps: false, zoomRange: null, map,
    }), { wrapper })
    expect(result.current.scale).toBeNull()
  })

  it('scale is constant for metric=sport', () => {
    const map = makeFakeMap()
    const { result } = renderHook(() => useMetricColoring({
      streams: streamsWithHR,
      activity: { sport_type: 'running', sub_type: null, fit_sport_id: 1 },
      hasGps: true, zoomRange: null, map,
    }), { wrapper })
    expect(result.current.scale?.type).toBe('constant')
  })

  it('registers moveend listener on mount, removes on unmount', () => {
    const map = makeFakeMap()
    const { unmount } = renderHook(() => useMetricColoring({
      streams: streamsWithHR,
      activity: { sport_type: 'running', sub_type: null, fit_sport_id: 1 },
      hasGps: true, zoomRange: null, map,
    }), { wrapper })
    expect(map.on).toHaveBeenCalledWith('moveend', expect.any(Function))
    unmount()
    expect(map.off).toHaveBeenCalledWith('moveend', expect.any(Function))
  })
})

describe('resolveEffectiveTrackBounds', () => {
  const streams: StreamPoint[] = [
    { time: '2026-01-01T00:00:00Z', lat: 47.0, lng: 11.0 },
    { time: '2026-01-01T00:01:00Z', lat: 47.2, lng: 11.2 },
    { time: '2026-01-01T00:02:00Z', lat: 47.4, lng: 11.4 },
  ]
  const viewportBounds = { west: 11.0, south: 47.0, east: 11.05, north: 47.05 }
  const zoomRange: ZoomRange = { startIdx: 1, endIdx: 2, startKm: 1, endKm: 2, startPct: 20, endPct: 40 }

  it('does not viewport-cut track features while a chart zoom range is active', () => {
    expect(resolveEffectiveTrackBounds(streams, viewportBounds, zoomRange)).toBeUndefined()
  })

  it('keeps the viewport filter available when no chart zoom range is active', () => {
    expect(resolveEffectiveTrackBounds(streams, viewportBounds, null)).toEqual(viewportBounds)
  })
})

// ─── splitStreamsAtTeleports — semantic boundary for the GPS-teleport filter ────
//
// Regression coverage for the bug where the user's heatmap-setting threshold
// (typical 100m) was applied to *downsampled* track segments instead of the
// *raw GPS stream*. With downsampling, two consecutive sampled points can
// legitimately span hundreds of metres on long activities — the 100m filter
// then dropped them as "teleports" and produced visible gaps in the track.
//
// The fix moves the filter back to the original splitAtGaps semantics:
// applied to full GPS resolution, where 100m is appropriate (consecutive
// raw GPS samples are never that far apart unless the device truly lost
// lock and re-acquired far away).
describe('splitStreamsAtTeleports', () => {
  const mkPoint = (i: number, lat: number, lng: number): StreamPoint => ({
    time: `2026-01-01T00:${String(i).padStart(2, '0')}:00Z`,
    lat, lng,
  })

  it('returns single chunk when no consecutive GPS pair exceeds threshold', () => {
    // 50 points along a near-straight line, ~10m apart each.
    const streams = Array.from({ length: 50 }, (_, i) => mkPoint(i, 47.0, 11.0 + i * 0.0001))
    const chunks = splitStreamsAtTeleports(streams, 100)
    expect(chunks).toHaveLength(1)
    expect(chunks[0]).toHaveLength(50)
  })

  it('splits at a real teleport gap exceeding the threshold', () => {
    // Three pre-teleport points, then a >100km jump, then three post-teleport points.
    const streams: StreamPoint[] = [
      mkPoint(0, 47.0, 11.000),
      mkPoint(1, 47.0, 11.001),
      mkPoint(2, 47.0, 11.002),
      mkPoint(3, 48.0, 12.000), // ~140 km jump
      mkPoint(4, 48.0, 12.001),
      mkPoint(5, 48.0, 12.002),
    ]
    const chunks = splitStreamsAtTeleports(streams, 1000)
    expect(chunks).toHaveLength(2)
    expect(chunks[0]).toHaveLength(3)
    expect(chunks[1]).toHaveLength(3)
  })

  it('does not split when threshold is 0 (filter disabled)', () => {
    const streams: StreamPoint[] = [
      mkPoint(0, 47.0, 11.0),
      mkPoint(1, 48.0, 12.0), // big jump, but filter off
    ]
    const chunks = splitStreamsAtTeleports(streams, 0)
    expect(chunks).toHaveLength(1)
    expect(chunks[0]).toHaveLength(2)
  })

  it('keeps null-GPS samples attached to the surrounding chunk (only real geo jumps split)', () => {
    // Pause-with-no-GPS scenarios: device kept recording but GPS dropped briefly.
    // Those should NOT cause a chunk split — a chunk split implies a teleport,
    // and a missing-GPS sample doesn't have coordinates to compare against.
    const streams: StreamPoint[] = [
      mkPoint(0, 47.0, 11.0),
      { time: '2026-01-01T00:01:00Z' }, // null lat/lng
      { time: '2026-01-01T00:02:00Z' }, // null lat/lng
      mkPoint(3, 47.0, 11.0001), // ~7m later — well within threshold
    ]
    const chunks = splitStreamsAtTeleports(streams, 100)
    expect(chunks).toHaveLength(1)
    expect(chunks[0]).toHaveLength(4)
  })

  it('handles back-to-back teleports (multiple chunks)', () => {
    const streams: StreamPoint[] = [
      mkPoint(0, 47.0, 11.0),
      mkPoint(1, 48.0, 12.0), // teleport 1
      mkPoint(2, 49.0, 13.0), // teleport 2
      mkPoint(3, 49.0, 13.0001),
    ]
    const chunks = splitStreamsAtTeleports(streams, 1000)
    expect(chunks).toHaveLength(3)
    expect(chunks[0]).toHaveLength(1) // only point 0
    expect(chunks[1]).toHaveLength(1) // only point 1
    expect(chunks[2]).toHaveLength(2) // points 2 + 3
  })

  it('treats short input as single chunk regardless of threshold', () => {
    expect(splitStreamsAtTeleports([], 100)).toEqual([[]])
    expect(splitStreamsAtTeleports([mkPoint(0, 47.0, 11.0)], 100)).toEqual([
      [mkPoint(0, 47.0, 11.0)],
    ])
  })
})

// ─── Full pipeline: splitStreamsAtTeleports → downsampleUniform → buildSegmentFeatures ─
//
// Outcome-level regression check: even when downsampling produces 2-point
// segments whose straight-line distance exceeds the user's `teleport_threshold_m`
// setting, the resulting features must still form a continuous, gap-free chain.
//
// The pre-fix bug was: `buildSegmentFeatures` received the threshold and
// dropped any downsampled segment exceeding it — a 60km cycling activity at
// 25 km/h produces ~50m straight-line spans per downsampled segment under the
// default 2000 maxSegments, easily exceeding a 100m threshold over straight
// stretches → false-positive "teleport" → visible gaps in the rendered track.
describe('teleport-filter pipeline (no-false-gaps regression)', () => {
  it('produces a gap-free feature chain for a long straight stream + 100m threshold', async () => {
    // Lazy-import the lib so we don't pull MapLibre into the unit-test module graph
    const { downsampleUniform, buildSegmentFeatures, resolveScale } = await import('@/lib/mapColorUtils')
    const { motionProfileForSport } = await import('@/lib/colorScaleRanges')

    // 5000 GPS points along a roughly straight line covering ~50km — typical
    // for a long road-bike activity. Each consecutive pair is ~10m apart
    // (well within any reasonable teleport threshold).
    const streams: StreamPoint[] = Array.from({ length: 5000 }, (_, i) => ({
      time: `2026-01-01T00:00:${String(i).padStart(2, '0')}Z`,
      lat: 47.0 + i * 0.0001, // ~11.1m per step in latitude
      lng: 11.0,
      heart_rate: 130 + (i % 30),
    }))

    const teleportThresholdM = 100 // user's typical heatmap setting
    const chunks = splitStreamsAtTeleports(streams, teleportThresholdM)
    // No real teleports in this stream → must be exactly one chunk.
    expect(chunks).toHaveLength(1)

    // Run the same pipeline useMetricColoring runs internally.
    const motionProfile = motionProfileForSport('running', null)
    const scale = resolveScale('hr', motionProfile, streams, null, 'linear', { sportColor: '#000' })
    const allFeatures: GeoJSON.Feature[] = []
    for (const chunk of chunks) {
      if (chunk.length < 2) continue
      const ds = downsampleUniform(chunk, 'hr', 2000, undefined, 'linear')
      // CRITICAL: no gapThresholdM passed — the filter belongs on full GPS
      // resolution (in splitStreamsAtTeleports above), not on downsampled
      // segments. Passing it here is the bug we're guarding against.
      allFeatures.push(...buildSegmentFeatures(ds, 'hr', scale))
    }

    // The features must form a contiguous chain over the source stream
    // indices: feature[i].segEndIdx + 1 === feature[i+1].segStartIdx.
    expect(allFeatures.length).toBeGreaterThan(100) // sanity: we got real output
    for (let i = 0; i < allFeatures.length - 1; i++) {
      const prev = allFeatures[i].properties as { segEndIdx: number }
      const next = allFeatures[i + 1].properties as { segStartIdx: number }
      // Adjacent segments share an endpoint in the source stream — the
      // downsampled stride means segEndIdx + 1 === next segStartIdx.
      expect(next.segStartIdx).toBe(prev.segEndIdx + 1)
    }
  })

  it('preserves real teleport gaps as actual chunk boundaries', async () => {
    const { downsampleUniform, buildSegmentFeatures, resolveScale } = await import('@/lib/mapColorUtils')
    const { motionProfileForSport } = await import('@/lib/colorScaleRanges')

    // Two distinct geographic clusters separated by a real teleport.
    const cluster1 = Array.from({ length: 100 }, (_, i) => ({
      time: `2026-01-01T00:00:${String(i).padStart(2, '0')}Z`,
      lat: 47.0 + i * 0.0001, lng: 11.0,
      heart_rate: 130 + (i % 30),
    }))
    const cluster2 = Array.from({ length: 100 }, (_, i) => ({
      time: `2026-01-01T01:00:${String(i).padStart(2, '0')}Z`,
      lat: 48.0 + i * 0.0001, lng: 12.0,
      heart_rate: 140 + (i % 30),
    }))
    const streams: StreamPoint[] = [...cluster1, ...cluster2]

    const chunks = splitStreamsAtTeleports(streams, 1000) // 1km threshold catches the ~140km jump
    expect(chunks).toHaveLength(2)

    const motionProfile = motionProfileForSport('running', null)
    const scale = resolveScale('hr', motionProfile, streams, null, 'linear', { sportColor: '#000' })
    const allFeatures: GeoJSON.Feature[] = []
    for (const chunk of chunks) {
      if (chunk.length < 2) continue
      const ds = downsampleUniform(chunk, 'hr', 2000, undefined, 'linear')
      allFeatures.push(...buildSegmentFeatures(ds, 'hr', scale))
    }

    // Within each chunk: contiguous. Between chunks: the index jumps over
    // the teleport (cluster 1 ends, cluster 2 begins ~ at index 100).
    // Find the discontinuity: there must be exactly ONE index where
    // segStartIdx > prev.segEndIdx + 1.
    let discontinuities = 0
    for (let i = 0; i < allFeatures.length - 1; i++) {
      const prev = allFeatures[i].properties as { segEndIdx: number }
      const next = allFeatures[i + 1].properties as { segStartIdx: number }
      if (next.segStartIdx !== prev.segEndIdx + 1) discontinuities++
    }
    expect(discontinuities).toBe(1) // exactly one teleport boundary
  })

  it('does not connect viewport-visible track parts across an outside-viewport gap', async () => {
    const { downsampleUniform, buildSegmentFeatures, resolveScale } = await import('@/lib/mapColorUtils')
    const { motionProfileForSport } = await import('@/lib/colorScaleRanges')

    const streams: StreamPoint[] = [
      { time: '2026-01-01T00:00:00Z', lat: 47.001, lng: 11.001, heart_rate: 120 },
      { time: '2026-01-01T00:00:01Z', lat: 47.002, lng: 11.002, heart_rate: 121 },
      { time: '2026-01-01T00:00:02Z', lat: 47.100, lng: 11.100, heart_rate: 122 },
      { time: '2026-01-01T00:00:03Z', lat: 47.003, lng: 11.003, heart_rate: 123 },
      { time: '2026-01-01T00:00:04Z', lat: 47.004, lng: 11.004, heart_rate: 124 },
    ]

    const motionProfile = motionProfileForSport('running', null)
    const scale = resolveScale('hr', motionProfile, streams, null, 'linear', { sportColor: '#000' })
    const ds = downsampleUniform(
      streams,
      'hr',
      2000,
      { west: 11.0, south: 47.0, east: 11.01, north: 47.01 },
      'linear',
    )

    const features = buildSegmentFeatures(ds, 'hr', scale)

    expect(features.map(f => {
      const props = f.properties as { segStartIdx: number; segEndIdx: number }
      return { segStartIdx: props.segStartIdx, segEndIdx: props.segEndIdx }
    })).toEqual([
      { segStartIdx: 0, segEndIdx: 0 },
      { segStartIdx: 3, segEndIdx: 3 },
    ])
    for (const feature of features) {
      const coords = (feature.geometry as GeoJSON.LineString).coordinates
      expect(coords).not.toEqual([[11.002, 47.002], [11.003, 47.003]])
    }
  })
})
