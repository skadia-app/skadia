import { describe, expect, it, vi } from 'vitest'
import { renderHook, act } from '@testing-library/react'

// Mock useMapLayers (used transitively via useSystemLayerVisibility)
vi.mock('./useMapLayers', () => ({
  useMapLayers: vi.fn(),
}))

// Mock useQuery so coverage-info fetch doesn't fire
vi.mock('@tanstack/react-query', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-query')>()
  return {
    ...actual,
    useQuery: vi.fn().mockReturnValue({ data: undefined }),
  }
})

import { useMapLayers } from './useMapLayers'
import { useCoverageOverlays } from './useCoverageOverlays'
import type { UseMapLayersResult } from './useMapLayers'
import type { SystemLayerVisibility } from '@/api/mapLayers'

function mockVisibility(keys: Record<string, { enabled: boolean }>) {
  const systemVisibility: Record<string, SystemLayerVisibility> = {}
  for (const [k, v] of Object.entries(keys)) {
    systemVisibility[k] = {
      layer_key: k,
      enabled: v.enabled,
      default_enabled_for_new_users: false,
      updated_at: '2026-04-26T00:00:00Z',
    }
  }
  ;(useMapLayers as ReturnType<typeof vi.fn>).mockReturnValue({
    isLoading: false,
    base: [],
    overlay: [],
    disabled: [],
    raw: [],
    systemVisibility,
    capabilitiesLoading: false,
  } satisfies UseMapLayersResult)
}

function makeMapMock(presentLayers: string[]) {
  const setLayoutProperty = vi.fn()
  const setPaintProperty = vi.fn()
  return {
    map: {
      getSource: vi.fn().mockReturnValue({ id: 'fake' }),
      getLayer: vi.fn((id: string) => (presentLayers.includes(id) ? { id } : null)),
      addSource: vi.fn(),
      addLayer: vi.fn(),
      removeLayer: vi.fn(),
      removeSource: vi.fn(),
      setLayoutProperty,
      setPaintProperty,
      getStyle: vi.fn().mockReturnValue({ layers: [] }),
    },
    setLayoutProperty,
    setPaintProperty,
  }
}

describe('useCoverageOverlays — visibility filtering', () => {
  it('hides noise sublayer from MapLibre when admin disables coverage_noise', () => {
    // Admin disabled coverage_noise globally
    mockVisibility({ coverage_noise: { enabled: false } })

    const { map, setLayoutProperty } = makeMapMock(['coverage-noise-layer'])

    const { result } = renderHook(() =>
      useCoverageOverlays(map as unknown as import('maplibre-gl').Map, 1),
    )

    // User enables the noise composite — without visibility gate this would call setLayoutProperty('visible')
    act(() => {
      result.current.updateComposite('noise', { enabled: true })
    })

    // The 'noise' sublayer should have been forced to 'none' (not 'visible')
    // because coverage_noise is admin-disabled
    const visibleCalls = setLayoutProperty.mock.calls.filter(
      (args) => args[0] === 'coverage-noise-layer' && args[2] === 'visible',
    )
    expect(visibleCalls).toHaveLength(0)
  })

  it('shows noise sublayer when admin has not disabled it (default enabled)', () => {
    // No override → defaults to enabled
    mockVisibility({})

    const { map, setLayoutProperty } = makeMapMock(['coverage-noise-layer'])

    const { result } = renderHook(() =>
      useCoverageOverlays(map as unknown as import('maplibre-gl').Map, 1),
    )

    act(() => {
      result.current.updateComposite('noise', { enabled: true })
    })

    const visibleCalls = setLayoutProperty.mock.calls.filter(
      (args) => args[0] === 'coverage-noise-layer' && args[2] === 'visible',
    )
    expect(visibleCalls.length).toBeGreaterThan(0)
  })

  it('returns all three composite groups in state regardless of visibility', () => {
    mockVisibility({})
    const { result } = renderHook(() => useCoverageOverlays(null, 0))
    expect(result.current.coverageState).toHaveProperty('nature')
    expect(result.current.coverageState).toHaveProperty('noise')
    expect(result.current.coverageState).toHaveProperty('trafficUrban')
  })

  it('updateComposite and toggleLayer work as before', () => {
    mockVisibility({})
    const { result } = renderHook(() => useCoverageOverlays(null, 0))

    act(() => {
      result.current.updateComposite('nature', { enabled: true, opacity: 75 })
    })
    expect(result.current.coverageState.nature.enabled).toBe(true)
    expect(result.current.coverageState.nature.opacity).toBe(75)

    act(() => {
      result.current.toggleLayer('nature', 'forest', false)
    })
    expect(result.current.coverageState.nature.layers.forest).toBe(false)
  })
})
