// frontend/src/components/activity/__tests__/useMetricColoring.test.tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useMetricColoring } from '../useMetricColoring'
import type { StreamPoint } from '@/api/types'

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
      activity: { sport_type: 'running', fit_sport_id: 1 },
      hasGps: true, zoomRange: null, map,
    }), { wrapper })
    expect(result.current.metric).toBe('sport')
  })

  it('setMetric(hr) switches metric', () => {
    const map = makeFakeMap()
    const { result } = renderHook(() => useMetricColoring({
      streams: streamsWithHR,
      activity: { sport_type: 'running', fit_sport_id: 1 },
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
        activity: { sport_type: 'running', fit_sport_id: 1 },
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
      activity: { sport_type: 'running', fit_sport_id: 1 },
      hasGps: false, zoomRange: null, map,
    }), { wrapper })
    expect(result.current.scale).toBeNull()
  })

  it('scale is constant for metric=sport', () => {
    const map = makeFakeMap()
    const { result } = renderHook(() => useMetricColoring({
      streams: streamsWithHR,
      activity: { sport_type: 'running', fit_sport_id: 1 },
      hasGps: true, zoomRange: null, map,
    }), { wrapper })
    expect(result.current.scale?.type).toBe('constant')
  })

  it('registers moveend listener on mount, removes on unmount', () => {
    const map = makeFakeMap()
    const { unmount } = renderHook(() => useMetricColoring({
      streams: streamsWithHR,
      activity: { sport_type: 'running', fit_sport_id: 1 },
      hasGps: true, zoomRange: null, map,
    }), { wrapper })
    expect(map.on).toHaveBeenCalledWith('moveend', expect.any(Function))
    unmount()
    expect(map.off).toHaveBeenCalledWith('moveend', expect.any(Function))
  })
})
