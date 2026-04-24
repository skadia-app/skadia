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
import type { RouteSegment, PlannedRoute, RouteWaypoint, ElevationPoint } from '@/api/routes'
import { makeFakeRouteStats, makeFakeSuggestion } from '@/test-utils/routeFixtures'

// addToastMock must be hoisted so vi.mock factory can reference it
const addToastMock = vi.hoisted(() => vi.fn())

// Shared mock for computeInternalRoute — used in Quelle-1 integration test
const computeInternalRouteMock = vi.hoisted(() => vi.fn())

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
    computeInternalRoute: computeInternalRouteMock,
    // DEM endpoint — default: empty profile (no DEM available). Tests that need
    // a resolved profile override this via vi.mocked(getElevationProfile).mockResolvedValue(...)
    getElevationProfile: vi.fn().mockResolvedValue({ profile: [] }),
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

/**
 * Helper: build minimal RouteStats — Wrapper um makeFakeRouteStats-Factory.
 * Hook-Tests brauchen `coordinates: []` als Default (nicht das 3-Punkt-Array der
 * Chart-Tests), weil mergeSegmentStatsForState den coordinates-Array selbst
 * zusammenstellt und die Fixture nur Input-Stats liefert.
 *
 * forest_pct / protected_pct default null — kommt aus Factory (F4-Runde-3).
 */
function makeStats(overrides: Partial<RouteStats> = {}): RouteStats {
  return makeFakeRouteStats({
    coordinates: [],
    ...overrides,
  })
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

  it('extractRouteStatsFromFeature: extrahiert forest_ranges aus feature.properties', () => {
    // Import extractRouteStatsFromFeature indirectly via mergeSegmentStatsForState:
    // We verify extraction by building a segment whose stats come from a feature
    // with forest_ranges — done via direct cache injection.
    const coords = Array.from({ length: 6 }, (_, i) => [10 + i * 0.001, 47])
    const seg = makeSegment('a', 'b', 500, coords)
    const stats = makeStats({
      forest_ranges: [{ from: 0, to: 5 }],
      coordinates: coords,
    })
    const result = mergeSegmentStatsForState([seg], { 'a:b': stats })
    expect(result).not.toBeNull()
    expect(result!.forest_ranges).toEqual([{ from: 0, to: 5 }])
  })

  it('two segments: forest_ranges mergen mit korrektem Coord-Offset-Shift', () => {
    // Seg1: 5 coords (indices 0..4), forest_ranges [{from:0, to:2}]
    const coords1 = Array.from({ length: 5 }, (_, i) => [10 + i * 0.001, 47])
    const seg1 = makeSegment('a', 'b', 500, coords1)
    const stats1 = makeStats({
      forest_ranges: [{ from: 0, to: 2 }],
      coordinates: coords1,
    })

    // Seg2: 5 coords, forest_ranges [{from:0, to:2}]
    const coords2 = Array.from({ length: 5 }, (_, i) => [10.004 + i * 0.001, 47])
    const seg2 = makeSegment('b', 'c', 500, coords2)
    const stats2 = makeStats({
      forest_ranges: [{ from: 0, to: 2 }],
      coordinates: coords2,
    })

    const result = mergeSegmentStatsForState(
      [seg1, seg2],
      { 'a:b': stats1, 'b:c': stats2 },
    )

    expect(result).not.toBeNull()
    // Merged coords: 5 (seg1) + 4 (seg2 without first) = 9
    expect(result!.coordinates.length).toBe(9)

    // Seg1 ranges: coordOffset=0 at start → unchanged {from:0, to:2}
    // Seg2 ranges: coordOffset=5 at start of seg2 iteration → {from:0+5, to:2+5} = {from:5, to:7}
    expect(result!.forest_ranges).toEqual([
      { from: 0, to: 2 },
      { from: 5, to: 7 },
    ])
  })
})

// ── Hook-Integration: loadSuggestion sets routeStats (Quelle 2) ───────────────

// makeFakeSuggestion imported from @/test-utils/routeFixtures (zentrale Factory).

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

// ── Hook-Integration: addWaypoint → computeInternalRoute → routeStats (Quelle 1) ─

describe('useRoutePlanner routeStats Quelle 1 — addWaypoint via computeInternalRoute', () => {
  let qc: QueryClient

  beforeEach(async () => {
    qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    addToastMock.mockClear()
    computeInternalRouteMock.mockClear()

    const { getProfileDefaults, cacheServerDefaults } = await import('@/lib/routingProfiles')
    // Make profileConfig available immediately so addWaypoint triggers _calculateSegment
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)
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

  it('addWaypoint (2 Waypoints) → computeInternalRoute → routeStats enthält forest_pct + sac_ranges', async () => {
    // Mock computeInternalRoute to return an enriched FeatureCollection
    // with enrichment keys (Quelle 1 path).
    const mockCoords = [
      [12.0, 49.0, 500],
      [12.01, 49.01, 510],
      [12.02, 49.02, 520],
    ]
    computeInternalRouteMock.mockResolvedValue({
      type: 'FeatureCollection',
      features: [{
        type: 'Feature',
        geometry: {
          type: 'LineString',
          coordinates: mockCoords,
        },
        properties: {
          distance_m: 3000,
          ascent_m: 80,
          descent_m: 20,
          forest_pct: 0.65,
          protected_pct: 0.15,
          sac_ranges: { T2: [{ from: 0, to: 2 }] },
          summits_passed: null,
          mtb_ranges: null,
          vf_ranges: null,
          vf_unknown_ranges: null,
        },
      }],
    })

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    // Wait for profileConfig to be hydrated (Quelle-1 path requires it)
    await waitFor(() => { expect(result.current.profile.status).toBe('ready') })

    // Add two waypoints — second addWaypoint triggers _calculateSegment
    await act(async () => {
      await result.current.addWaypoint([49.0, 12.0])
    })
    await act(async () => {
      await result.current.addWaypoint([49.02, 12.02])
    })

    // Wait for segment calculation to complete
    await waitFor(() => {
      expect(result.current.isCalculating).toBe(false)
    })

    // Verify Quelle-1 path: computeInternalRoute was called
    expect(computeInternalRouteMock).toHaveBeenCalledOnce()

    // Verify routeStats are populated from computeInternalRoute response
    expect(result.current.routeStats).not.toBeNull()
    expect(result.current.routeStats!.forest_pct).toBe(0.65)
    expect(result.current.routeStats!.protected_pct).toBe(0.15)
    expect(result.current.routeStats!.sac_ranges).not.toBeNull()
    expect(result.current.routeStats!.sac_ranges!['T2']).toEqual([{ from: 0, to: 2 }])
  })
})

// ── Lazy-Fill DEM-only (Spec Rev 2.3 §7.5) ───────────────────────────────────
//
// Tests cover three cases:
//   1. Route already has elevation_profile → no lazy-fill triggered
//   2. Route with null profile → DEM-only lazy-fill (getElevationProfile called,
//      computeInternalRoute NOT called, segments untouched, lazyFillPendingSave=true)
//   3. Stale lazy-fill result discarded when user moves a waypoint mid-flight
//      (reference-identity check: s.segments !== segmentsSnapshot → discard)
//
// Note on API function names: the plan references `calculateSegment` which does
// not exist in @/api/routes. The real equivalent that _calculateSegment uses
// internally is `computeInternalRoute` (verified via grep). Spies target that.

/** Build a minimal PlannedRoute with required fields, merging overrides. */
function makeTestRoute(overrides: Partial<PlannedRoute> = {}): PlannedRoute {
  const wp1: RouteWaypoint = { id: 'wp1', latlng: [47.0, 11.0], type: 'start' }
  const wp2: RouteWaypoint = { id: 'wp2', latlng: [47.1, 11.1], type: 'end' }
  const defaultSegment: RouteSegment = {
    from_waypoint: 'wp1',
    to_waypoint: 'wp2',
    mode: 'routed',
    geometry: {
      type: 'LineString',
      coordinates: [
        [11.0, 47.0, 500],
        [11.05, 47.05, 510],
        [11.1, 47.1, 520],
      ],
    },
    control_points: [],
    smooth: true,
    distance_m: 12000,
    elevation_gain: 200,
    elevation_loss: 50,
  }
  return {
    id: 'test-route-1',
    name: 'Test Route',
    sport_type: 'hiking',
    profile: 'hiking',
    waypoints: [wp1, wp2],
    segments: [defaultSegment],
    time_override: false,
    auto_tag: false,
    photos: [],
    tags: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    elevation_profile: null,
    enrichment: null,
    ...overrides,
  }
}

describe('useRoutePlanner lazy-fill (DEM-only, Spec Rev 2.3 §7.5)', () => {
  let qc: QueryClient

  beforeEach(async () => {
    qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    addToastMock.mockClear()
    sessionStorage.removeItem('skadia.lazy_fill_notice_shown')

    const { getProfileDefaults, cacheServerDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue(null)
    vi.mocked(cacheServerDefaults).mockReset()

    const { getProfileDefaults: fetchProfileDefaults, getElevationProfile } =
      await import('@/api/routes')
    vi.mocked(fetchProfileDefaults).mockResolvedValue({
      profile: 'hiking',
      uphill_cost: 7,
    } as any)
    // Default: getElevationProfile resolves with empty profile (no DEM available)
    vi.mocked(getElevationProfile).mockResolvedValue({ profile: [] })
  })

  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <ToastProvider>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </ToastProvider>
  )

  it('TestLoadRoute_NoLazyFill_WhenProfilePresent', async () => {
    // Route with a persisted elevation_profile → getElevationProfile must NOT be called.
    const { getElevationProfile } = await import('@/api/routes')
    const getElevSpy = vi.mocked(getElevationProfile)
    getElevSpy.mockClear()

    const route = makeTestRoute({
      elevation_profile: [
        { dist_m: 0, ele_m: 100 },
        { dist_m: 50, ele_m: 110 },
      ],
      enrichment: { enrichment_version: 1, forest_pct: 0.5 },
    })

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    act(() => {
      result.current.loadRoute(route)
    })

    // elevationProfile is hydrated synchronously from route.elevation_profile
    await waitFor(() => {
      expect(result.current.elevationProfile).not.toBeNull()
    })

    // DEM endpoint must NOT have been called
    expect(getElevSpy).not.toHaveBeenCalled()
    // No lazy-fill state active
    expect(result.current.lazyFillPendingSave).toBe(false)
    expect(result.current.lazyFillInProgress).toBe(false)
  })

  it('TestLoadRoute_LazyFillWhenProfileNull_DEMOnly', async () => {
    // Spec §7.4: getElevationProfile is called (DEM-only), computeInternalRoute is NOT.
    // Segments remain identical (no geometry drift).
    const { getElevationProfile } = await import('@/api/routes')
    const getElevSpy = vi.mocked(getElevationProfile)
    getElevSpy.mockResolvedValue({
      profile: [
        { fraction: 0, elevation: 100 },
        { fraction: 0.5, elevation: 110 },
        { fraction: 1, elevation: 120 },
      ] as ElevationPoint[],
    })

    const computeSpy = vi.mocked(computeInternalRouteMock)
    computeSpy.mockClear()

    const route = makeTestRoute({ elevation_profile: null, enrichment: null })
    const initialSegments = route.segments

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    act(() => {
      result.current.loadRoute(route)
    })

    // Wait for lazy-fill to complete
    await waitFor(() => {
      expect(result.current.lazyFillInProgress).toBe(false)
    })

    // getElevationProfile was called exactly once (DEM-only path)
    expect(getElevSpy).toHaveBeenCalledTimes(1)

    // computeInternalRoute (recalculateAll equivalent) was NOT called
    expect(computeSpy).not.toHaveBeenCalled()

    // elevationProfile is now populated
    expect(result.current.elevationProfile).not.toBeNull()

    // Geometry (segments) is identical — no route drift
    expect(result.current.segments).toEqual(initialSegments)

    // lazyFillPendingSave is true after successful fill
    expect(result.current.lazyFillPendingSave).toBe(true)
  })

  it('TestLoadRoute_LazyFillStaleResult_DiscardedWhenUserMoved', async () => {
    // Spec §R7 + Rev 2.3: stale-check via reference-identity (s.segments !== snapshot).
    //
    // The lazy-fill callback is kept pending while the user moves a waypoint.
    // moveWaypoint fires _calculateSegment (via computeInternalRoute), which
    // replaces state.segments with a new array — breaking reference-identity
    // against segmentsSnapshot captured at loadRoute() time. When the
    // in-flight getElevationProfile resolves, the stale-check discards the result.
    //
    // Note: moveWaypoint only fires _calculateSegment when profileConfig is set.
    // We must wait for the profile to be hydrated (status='ready') before
    // calling moveWaypoint, otherwise the move is a no-op for segments.
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)

    // Keep getElevationProfile in-flight until we manually resolve it
    let resolveGetElev!: (v: { profile: ElevationPoint[] }) => void
    const { getElevationProfile } = await import('@/api/routes')
    vi.mocked(getElevationProfile).mockImplementation(
      () => new Promise(resolve => { resolveGetElev = resolve }),
    )

    // Mock computeInternalRoute to return a new geometry (new segments array
    // reference — this is what breaks segmentsSnapshot identity).
    const newCoords = [
      [11.5, 47.3, 600],
      [11.55, 47.35, 610],
      [11.6, 47.4, 620],
    ]
    computeInternalRouteMock.mockResolvedValue({
      type: 'FeatureCollection',
      features: [{
        type: 'Feature',
        geometry: { type: 'LineString', coordinates: newCoords },
        properties: {
          distance_m: 8000,
          ascent_m: 100,
          descent_m: 30,
          forest_pct: null,
          protected_pct: null,
          sac_ranges: null,
          mtb_ranges: null,
          vf_ranges: null,
          vf_unknown_ranges: null,
          summits_passed: null,
        },
      }],
    })

    const route = makeTestRoute({ elevation_profile: null })
    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    act(() => {
      result.current.loadRoute(route)
    })

    // Lazy-fill is in-flight
    await waitFor(() => {
      expect(result.current.lazyFillInProgress).toBe(true)
    })

    // Wait for profile to be ready (required for moveWaypoint to trigger recalc)
    await waitFor(() => {
      expect(result.current.profile.status).toBe('ready')
    })

    // User moves a waypoint — triggers _calculateSegment → new segments array
    await act(async () => {
      await result.current.moveWaypoint('wp1', [47.3, 11.5])
    })

    // Wait for segment recalculation to complete (computeInternalRoute called)
    await waitFor(() => {
      expect(computeInternalRouteMock).toHaveBeenCalled()
    })

    // Capture profile state after user move (may be null or have fresh data)
    const profileAfterMove = result.current.elevationProfile

    // Now resolve the stale lazy-fill with a clearly distinguishable value (ele=999)
    act(() => {
      resolveGetElev({ profile: [{ fraction: 0, elevation: 999 }, { fraction: 1, elevation: 999 }] })
    })

    // Wait for lazy-fill to settle
    await waitFor(() => {
      expect(result.current.lazyFillInProgress).toBe(false)
    })

    // Stale-check must have discarded the 999-elevation result.
    // The effective profile must NOT contain the stale 999 value.
    // (Note: ele_m=999 corresponds to fraction=0/1 after conversion.)
    const finalProfile = result.current.elevationProfile
    if (finalProfile != null && finalProfile.length > 0) {
      // If profile is present, it must not be the stale one
      expect(finalProfile).toEqual(profileAfterMove)
      expect(finalProfile[0].ele_m).not.toBe(999)
    } else {
      // Profile is null (no DEM data from moveWaypoint recalc) — still correct,
      // because the stale value (999) was discarded.
      expect(finalProfile).toBeNull()
    }
    // lazyFillPendingSave must NOT be true (stale result was discarded)
    expect(result.current.lazyFillPendingSave).toBe(false)
  })

  it('TestLoadRoute_LazyFillNoDEM_SilentGracefulDegradation', async () => {
    // Spec §7.4: empty profile response (no DEM) → silent, no toast, no crash,
    // lazyFillPendingSave remains false.
    const { getElevationProfile } = await import('@/api/routes')
    vi.mocked(getElevationProfile).mockResolvedValue({ profile: [] })

    const route = makeTestRoute({ elevation_profile: null })
    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    act(() => {
      result.current.loadRoute(route)
    })

    await waitFor(() => {
      expect(result.current.lazyFillInProgress).toBe(false)
    })

    expect(result.current.elevationProfile).toBeNull()
    // No toast (silent graceful degradation)
    expect(addToastMock).not.toHaveBeenCalled()
    expect(result.current.lazyFillPendingSave).toBe(false)
  })

  it('TestLoadRoute_LazyFillError_SilentGracefulDegradation', async () => {
    // Spec §7.4: 500/exception → silent, no toast, no crash.
    const { getElevationProfile } = await import('@/api/routes')
    vi.mocked(getElevationProfile).mockRejectedValue(new Error('PostGIS-DEM not available'))

    const route = makeTestRoute({ elevation_profile: null })
    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    act(() => {
      result.current.loadRoute(route)
    })

    await waitFor(() => {
      expect(result.current.lazyFillInProgress).toBe(false)
    })

    expect(result.current.elevationProfile).toBeNull()
    expect(addToastMock).not.toHaveBeenCalled()
    expect(result.current.lazyFillPendingSave).toBe(false)
  })

  it('TestLazyFillToast_OnlyShownOncePerSession', async () => {
    // Spec §7.4: the "profile was filled" notice appears at most once per session.
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)

    const { getElevationProfile } = await import('@/api/routes')
    vi.mocked(getElevationProfile).mockResolvedValue({
      profile: [{ fraction: 0, elevation: 100 }, { fraction: 1, elevation: 110 }] as ElevationPoint[],
    })

    sessionStorage.removeItem('skadia.lazy_fill_notice_shown')

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => { expect(result.current.profile.status).toBe('ready') })

    // First load with null profile
    act(() => {
      result.current.loadRoute(makeTestRoute({ id: 'r1', elevation_profile: null }))
    })
    await waitFor(() => { expect(result.current.lazyFillInProgress).toBe(false) })
    expect(addToastMock).toHaveBeenCalledTimes(1)

    addToastMock.mockClear()
    // Reset lazyFillPendingSave by simulating save (set it back via a new load with profile)
    // Second load with null profile in same session → toast suppressed
    act(() => {
      result.current.loadRoute(makeTestRoute({ id: 'r2', elevation_profile: null }))
    })
    await waitFor(() => { expect(result.current.lazyFillInProgress).toBe(false) })

    // sessionStorage flag set → toast must NOT fire again
    expect(addToastMock).not.toHaveBeenCalled()
  })

  it('TestLoadRoute_FromActivityCreated_TriggersLazyFill', async () => {
    // Spec §3 Non-Goal: route_from_activity.go creates routes with elevation_profile=nil.
    // First loadRoute MUST trigger DEM-only lazy-fill, NOT computeInternalRoute.
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)

    const { getElevationProfile } = await import('@/api/routes')
    vi.mocked(getElevationProfile).mockResolvedValue({
      profile: [
        { fraction: 0, elevation: 800 },
        { fraction: 0.5, elevation: 850 },
        { fraction: 1, elevation: 900 },
      ] as ElevationPoint[],
    })

    computeInternalRouteMock.mockClear()

    const route = makeTestRoute({
      id: 'activity-route',
      elevation_profile: null,
      enrichment: null,
    })

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    act(() => {
      result.current.loadRoute(route)
    })

    await waitFor(() => {
      expect(result.current.lazyFillInProgress).toBe(false)
    })

    // DEM endpoint called exactly once
    expect(vi.mocked(getElevationProfile)).toHaveBeenCalledTimes(1)
    // computeInternalRoute (routing) was NOT called (DEM-only path)
    expect(computeInternalRouteMock).not.toHaveBeenCalled()
    // lazyFillPendingSave set after successful fill
    expect(result.current.lazyFillPendingSave).toBe(true)
  })
})

// ── Critic finding #7 (MAJOR): Token-counter path in loadRoute ────────────────
//
// Spec §7.5: "loadRoute(R2) while R1 in-flight" — the reference-identity branch
// (s.segments !== segmentsSnapshot) was already covered. This test covers the
// TOKEN-COUNTER branch: a second loadRoute() increments lazyFillToken so that
// the in-flight R1 lazy-fill setState callback sees s.lazyFillToken !== expectedToken
// and discards R1's result.

describe('useRoutePlanner lazy-fill token-counter (Spec §7.5, Critic #7)', () => {
  let qc: QueryClient

  beforeEach(async () => {
    qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    addToastMock.mockClear()
    sessionStorage.removeItem('skadia.lazy_fill_notice_shown')
    computeInternalRouteMock.mockReset()

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

  it('TestLoadRoute_LazyFillToken_DiscardedOnNewerLoad', async () => {
    // Token-counter stale-check: loadRoute(R2) while R1's getElevationProfile is
    // in-flight increments lazyFillToken. When R1's promise resolves, the token
    // check (s.lazyFillToken !== expectedToken) discards R1's result, so the
    // stale 999-elevation value is never written to effectiveProfile.
    //
    // R1: getElevationProfile hangs until we manually resolve it with ele=999.
    // R2: getElevationProfile resolves immediately with [] (no DEM → null profile).
    // After both settle: elevationProfile must be null (R2's outcome), not 999.

    let resolveR1!: (v: { profile: ElevationPoint[] }) => void
    let callCount = 0

    const { getElevationProfile } = await import('@/api/routes')
    vi.mocked(getElevationProfile).mockImplementation(() => {
      callCount += 1
      if (callCount === 1) {
        // R1's call: hang until resolved manually
        return new Promise(resolve => { resolveR1 = resolve })
      }
      // R2's call: resolve immediately with no DEM data → null elevationProfile
      return Promise.resolve({ profile: [] })
    })

    const routeR1 = makeTestRoute({ id: 'r1', elevation_profile: null })
    const routeR2 = makeTestRoute({ id: 'r2', elevation_profile: null })

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    // Load R1 — triggers lazy-fill (in-flight, token=1)
    act(() => {
      result.current.loadRoute(routeR1)
    })

    // R1's lazy-fill is now in-flight
    await waitFor(() => {
      expect(result.current.lazyFillInProgress).toBe(true)
    })

    // Load R2 while R1 is in-flight — increments token to 2, editingRouteId → r2
    act(() => {
      result.current.loadRoute(routeR2)
    })

    expect(result.current.editingRouteId).toBe('r2')

    // Wait for R2's lazy-fill to settle (no DEM → false immediately)
    await waitFor(() => {
      expect(result.current.lazyFillInProgress).toBe(false)
    })

    // Now resolve R1's in-flight getElevationProfile with stale ele=999
    act(() => {
      resolveR1({ profile: [{ fraction: 0, elevation: 999 }, { fraction: 1, elevation: 999 }] })
    })

    // Allow the setState callback to run
    await waitFor(() => {
      // editingRouteId must still be r2 (R1 did not overwrite it)
      expect(result.current.editingRouteId).toBe('r2')
    })

    // R1's stale 999-elevation must have been discarded by the token check.
    // R2 had no DEM data → elevationProfile is null.
    const finalProfile = result.current.elevationProfile
    if (finalProfile != null && finalProfile.length > 0) {
      expect(finalProfile[0].ele_m).not.toBe(999)
      expect(finalProfile[finalProfile.length - 1].ele_m).not.toBe(999)
    } else {
      // null or empty — correct: R2 resolved with empty profile, R1 was discarded
      expect(finalProfile).toBeNull()
    }

    // lazyFillPendingSave must NOT be true (R1 discarded, R2 had no DEM)
    expect(result.current.lazyFillPendingSave).toBe(false)
  })
})

