/**
 * V6-Retrofit Tests für useRoutePlanner routeStats + mergeSegmentStatsForState.
 *
 * Testet:
 * 1. mergeSegmentStatsForState-Helper (Koordinaten-Offset-Shift, Dedup, BRouter-null)
 * 2. loadSuggestion setzt routeStats aus Suggestion-Feldern (Quelle 2)
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ToastProvider } from '@/lib/toast'
import { useRoutePlanner, mergeSegmentStatsForState } from '../useRoutePlanner'
import type { RouteStats } from '../useRoutePlanner'
import type { RouteSegment, RouteSuggestion } from '@/api/routes'

// addToastMock must be hoisted so vi.mock factory can reference it
const addToastMock = vi.hoisted(() => vi.fn())

vi.mock('@/api/routes', async () => {
  const actual = await vi.importActual<typeof import('@/api/routes')>('@/api/routes')
  return {
    ...actual,
    getCapabilities: vi.fn().mockResolvedValue({
      routing: true,
      brouter: false,
      badrouter: false,
      offline_map: false,
      surface_enrichment: false,
      dem_correction: false,
      isochrone: false,
      route_suggestion: false,
      ml_service: false,
      import_path_enabled: false,
    }),
    getProfileDefaults: vi.fn().mockResolvedValue({
      profile: 'hiking',
      uphill_cost: 7,
    }),
  }
})

vi.mock('@/lib/routingProfiles', async () => {
  const actual = await vi.importActual<typeof import('@/lib/routingProfiles')>('@/lib/routingProfiles')
  return {
    ...actual,
    getProfileDefaults: vi.fn().mockReturnValue(null),
    cacheServerDefaults: vi.fn(),
  }
})

vi.mock('@/lib/toast', async () => {
  const actual = await vi.importActual<typeof import('@/lib/toast')>('@/lib/toast')
  return {
    ...actual,
    useToast: vi.fn().mockReturnValue({ addToast: addToastMock }),
  }
})

// ── mergeSegmentStatsForState unit tests ──────────────────────────────────────

/** Helper: build a minimal RouteSegment with geometry */
function makeSegment(
  fromId: string,
  toId: string,
  distanceM: number,
  coords: number[][],
): RouteSegment {
  return {
    from_waypoint: fromId,
    to_waypoint: toId,
    mode: 'routed',
    geometry: { type: 'LineString', coordinates: coords },
    control_points: [],
    smooth: true,
    distance_m: distanceM,
    elevation_gain: 0,
    elevation_loss: 0,
  }
}

/** Helper: build minimal RouteStats */
function makeStats(overrides: Partial<RouteStats> = {}): RouteStats {
  return {
    forest_pct: null,
    protected_pct: null,
    sac_ranges: null,
    mtb_ranges: null,
    vf_ranges: null,
    vf_unknown_ranges: null,
    summits_passed: null,
    coordinates: [],
    ...overrides,
  }
}

describe('mergeSegmentStatsForState', () => {
  it('single segment: merge liefert Stats unverändert', () => {
    const seg = makeSegment('a', 'b', 1000, Array.from({ length: 10 }, (_, i) => [10 + i * 0.001, 47]))
    const stats = makeStats({
      sac_ranges: { T3: [{ from: 2, to: 5 }] },
      forest_pct: 0.5,
      coordinates: seg.geometry!.coordinates as number[][],
    })
    const result = mergeSegmentStatsForState([seg], { 'a:b': stats })
    expect(result).not.toBeNull()
    expect(result!.sac_ranges!['T3']).toEqual([{ from: 2, to: 5 }])
    expect(result!.forest_pct).toBe(0.5)
  })

  it('two segments: Coord-Offset-Shift korrekt (T1 bleibt, T2 shiftet um offset)', () => {
    // Seg1: 10 coords (indices 0..9), T1 range {from:0, to:3}
    const coords1 = Array.from({ length: 10 }, (_, i) => [10 + i * 0.001, 47, 500])
    const seg1 = makeSegment('a', 'b', 1000, coords1)
    const stats1 = makeStats({
      sac_ranges: { T1: [{ from: 0, to: 3 }] },
      coordinates: coords1,
    })

    // Seg2: 8 coords, T2 range {from:0, to:2}
    const coords2 = Array.from({ length: 8 }, (_, i) => [10.009 + i * 0.001, 47, 510])
    const seg2 = makeSegment('b', 'c', 800, coords2)
    const stats2 = makeStats({
      sac_ranges: { T2: [{ from: 0, to: 2 }] },
      coordinates: coords2,
    })

    const result = mergeSegmentStatsForState(
      [seg1, seg2],
      { 'a:b': stats1, 'b:c': stats2 },
    )

    expect(result).not.toBeNull()
    // Merged coords: 10 (seg1) + 8-1 (seg2 without first) = 17
    expect(result!.coordinates.length).toBe(17)

    // T1 range unverändert (coordOffset für seg1 = 0)
    expect(result!.sac_ranges!['T1']).toEqual([{ from: 0, to: 3 }])

    // T2 range shifted by 9 (10 - 1 = offset after seg1)
    // coordOffset for seg2 = 10 (seg1 coords)
    // from: 0 + 9 = 9 — wait: offset when seg2 starts = 10 (seg1 length)
    // But shift happens BEFORE coordOffset is incremented for seg2.
    // At start of loop i=1: coordOffset = 10 (after seg1 added 10 coords).
    // But dedup: for i>0, seg2 first coord is skipped, so seg2 effectively starts
    // at merged index 10 (= coordOffset after seg1). Range {from:0, to:2} + 10 = {from:10, to:12}.
    expect(result!.sac_ranges!['T2']).toEqual([{ from: 10, to: 12 }])
  })

  it('two segments: Summit osm_id=42 in both → nur 1x im Ergebnis (Dedup)', () => {
    const coords1 = Array.from({ length: 5 }, (_, i) => [10 + i * 0.001, 47])
    const coords2 = Array.from({ length: 5 }, (_, i) => [10.004 + i * 0.001, 47])
    const seg1 = makeSegment('a', 'b', 500, coords1)
    const seg2 = makeSegment('b', 'c', 500, coords2)

    const summit = { osm_id: 42, name: 'Testgipfel', ele_m: 1200, category: 'summit' as const, lat: 47.0, lng: 10.003, dist_m: 5 }
    const stats1 = makeStats({ summits_passed: [summit], coordinates: coords1 })
    const stats2 = makeStats({ summits_passed: [summit], coordinates: coords2 })

    const result = mergeSegmentStatsForState(
      [seg1, seg2],
      { 'a:b': stats1, 'b:c': stats2 },
    )

    expect(result!.summits_passed).toHaveLength(1)
    expect(result!.summits_passed![0].osm_id).toBe(42)
  })

  it('alle Segments ohne Cache-Eintrag → null (BRouter-Gesamtroute)', () => {
    const coords = Array.from({ length: 5 }, (_, i) => [10 + i * 0.001, 47])
    const seg = makeSegment('a', 'b', 500, coords)
    // Cache leer → kein Segment hat Eintrag
    const result = mergeSegmentStatsForState([seg], {})
    expect(result).toBeNull()
  })

  it('leere Segments-Liste → null', () => {
    const result = mergeSegmentStatsForState([], {})
    expect(result).toBeNull()
  })
})

// ── Hook-Integration: loadSuggestion sets routeStats (Quelle 2) ───────────────

function makeFakeSuggestion(overrides: Partial<RouteSuggestion> = {}): RouteSuggestion {
  return {
    variant: 0,
    profile: 'hiking',
    via_point: [12.0, 49.0],
    variant_id: 'shortest',
    variant_label_key: 'shortest',
    distance_m: 11700,
    ascent_m: 208,
    descent_m: 208,
    avg_flat: 0.5,
    loop_quality: 'loop',
    overlap_pct: 0.1,
    distance_target_achieved: true,
    ascent_target_achieved: true,
    is_fallback: false,
    forest_pct: 0,
    protected_pct: 0,
    geojson: {
      type: 'FeatureCollection',
      features: [{
        type: 'Feature',
        geometry: {
          type: 'LineString',
          coordinates: [
            [12.0, 49.0, 500],
            [12.01, 49.01, 510],
            [12.0, 49.0, 500],
          ],
        },
        properties: {},
      }],
    },
    ...overrides,
  }
}

describe('useRoutePlanner routeStats V6-Retrofit', () => {
  let qc: QueryClient

  beforeEach(async () => {
    qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    addToastMock.mockClear()

    const { getProfileDefaults, cacheServerDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue(null)
    vi.mocked(cacheServerDefaults).mockReset()

    const { getProfileDefaults: fetchProfileDefaults } = await import('@/api/routes')
    vi.mocked(fetchProfileDefaults).mockResolvedValue({
      profile: 'hiking',
      uphill_cost: 7,
    } as any)
  })

  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <ToastProvider>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </ToastProvider>
  )

  it('routeStats startet als null', () => {
    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    expect(result.current.routeStats).toBeNull()
  })

  it('loadSuggestion setzt routeStats mit forest_pct + sac_ranges (Quelle 2)', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => { expect(result.current.profile.status).toBe('ready') })

    act(() => {
      result.current.loadSuggestion(makeFakeSuggestion({
        forest_pct: 0.42,
        protected_pct: 0.10,
        sac_ranges: { T3: [{ from: 0, to: 3 }] },
        summits_passed: [
          { osm_id: 99, name: 'Gipfel', ele_m: 1800, category: 'summit', lat: 49.0, lng: 12.0, dist_m: 10 },
        ],
      }))
    })

    expect(result.current.routeStats).not.toBeNull()
    expect(result.current.routeStats!.forest_pct).toBe(0.42)
    expect(result.current.routeStats!.protected_pct).toBe(0.10)
    expect(result.current.routeStats!.sac_ranges).toEqual({ T3: [{ from: 0, to: 3 }] })
    expect(result.current.routeStats!.summits_passed).toHaveLength(1)
  })

  it('routeStats wird null nach recalculateAll (Quelle 4 — reset)', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => { expect(result.current.profile.status).toBe('ready') })

    // Set routeStats via loadSuggestion
    act(() => {
      result.current.loadSuggestion(makeFakeSuggestion({ forest_pct: 0.5 }))
    })
    expect(result.current.routeStats).not.toBeNull()

    // recalculateAll ohne Waypoints → Segments werden gecleart, routeStats = null
    await act(async () => {
      if (result.current.profile.status === 'ready') {
        await result.current.profile.recalculate()
      }
    })
    expect(result.current.routeStats).toBeNull()
  })

  it('routeStats wird null nach clear()', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => { expect(result.current.profile.status).toBe('ready') })

    act(() => {
      result.current.loadSuggestion(makeFakeSuggestion({ forest_pct: 0.3 }))
    })
    expect(result.current.routeStats).not.toBeNull()

    act(() => {
      result.current.clear()
    })
    expect(result.current.routeStats).toBeNull()
  })

  it('routeStats wird null nach setRoutingEngine (BRouter-Switch, Quelle 4)', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => { expect(result.current.profile.status).toBe('ready') })

    act(() => {
      result.current.loadSuggestion(makeFakeSuggestion({ forest_pct: 0.3 }))
    })
    expect(result.current.routeStats).not.toBeNull()

    await act(async () => {
      await result.current.setRoutingEngine('brouter')
    })
    expect(result.current.routeStats).toBeNull()
  })
})
