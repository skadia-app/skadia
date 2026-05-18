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
import { useRoutePlanner, mergeSegmentStatsForState, buildCombinedProfileFromSegments } from '../useRoutePlanner'
import type { RouteStats } from '../useRoutePlanner'
import type { RouteSegment, PlannedRoute, RouteWaypoint, ElevationPoint } from '@/api/routes'
import { makeFakeRouteStats, makeFakeSuggestion } from '@/test-utils/routeFixtures'
import { clearMigrationCache } from '../migrateRoute'

// Shared mock for getElevationAtVerticesChunked — used in D.2 token tests
const getElevationAtVerticesChunkedMock = vi.hoisted(() => vi.fn())

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
      brouter: true,
      badrouter: true,
      offline_map: false,
      surface_enrichment: false,
      dem_correction: false,
      isochrone: false,
      route_suggestion: true,
      ml_service: false,
      import_path_enabled: false,
      features: {
        routing: { configured: true, available: true },
        brouter: { configured: true, available: true },
        badrouter: { configured: true, available: true },
        route_suggestion: { configured: true, available: true },
      },
    }),
    getProfileDefaults: vi.fn().mockResolvedValue({
      profile: 'hiking',
      uphill_cost: 7,
    }),
    computeInternalRoute: computeInternalRouteMock,
    // DEM endpoint — default: empty profile (no DEM available). Tests that need
    // a resolved profile override this via vi.mocked(getElevationProfile).mockResolvedValue(...)
    getElevationProfile: vi.fn().mockResolvedValue({ profile: [] }),
    // updateRoute — needed by forceMigrateAndSave tests; default returns a minimal PlannedRoute.
    updateRoute: vi.fn(async (id: string, payload: Partial<PlannedRoute>) => ({
      id,
      name: 'Gespeicherte Route',
      profile: payload.profile ?? 'hiking',
      sport_type: payload.sport_type ?? 'hiking',
      waypoints: payload.waypoints ?? [],
      segments: payload.segments ?? [],
      elevation_profile: payload.elevation_profile ?? null,
      time_override: false,
      auto_tag: false,
      route_schema_version: payload.route_schema_version ?? 1,
      photos: [],
      tags: [],
      created_at: '2026-04-27T00:00:00Z',
      updated_at: '2026-04-27T00:00:00Z',
    })),
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

vi.mock('@/api/elevation-chunked', () => ({
  getElevationAtVerticesChunked: getElevationAtVerticesChunkedMock,
}))

// ── buildCombinedProfileFromSegments unit tests (Phase D.1) ──────────────────

/**
 * Minimal segment helper for buildCombinedProfileFromSegments tests.
 * Uses from/to waypoint IDs that match the waypoints array in each test.
 */
function makeProfileSeg(
  fromId: string,
  toId: string,
  distM: number,
  profile?: Array<{ dist_m: number; ele_m: number | null }>,
): RouteSegment {
  return {
    from_waypoint: fromId,
    to_waypoint: toId,
    mode: 'routed',
    geometry: { type: 'LineString', coordinates: [[11, 47], [11.1, 47.1]] },
    control_points: [],
    smooth: true,
    distance_m: distM,
    elevation_gain: 0,
    elevation_loss: 0,
    elevation_profile: profile,
  }
}

function makeWpList(...ids: string[]): RouteWaypoint[] {
  return ids.map((id, i) => ({
    id,
    latlng: [47 + i * 0.1, 11 + i * 0.1] as [number, number],
    type: (i === 0 ? 'start' : i === ids.length - 1 ? 'end' : 'via') as RouteWaypoint['type'],
  }))
}

describe('buildCombinedProfileFromSegments (Phase D.1)', () => {
  it('concatenates with junction-dedup', () => {
    // Seg1: [0..100], Seg2: [0..100]. Junction-dedup drops seg2's point at dist_m=0.
    const segs = [
      makeProfileSeg('w1', 'w2', 100, [
        { dist_m: 0, ele_m: 100 },
        { dist_m: 100, ele_m: 110 },
      ]),
      makeProfileSeg('w2', 'w3', 100, [
        { dist_m: 0, ele_m: 110 },   // junction — deduped
        { dist_m: 100, ele_m: 120 },
      ]),
    ]
    const wps = makeWpList('w1', 'w2', 'w3')
    const out = buildCombinedProfileFromSegments(segs, wps)
    expect(out).not.toBeNull()
    // Output: 3 points (seg1[0], seg1[1], seg2[1])
    expect(out).toEqual([
      { dist_m: 0,   ele_m: 100 },
      { dist_m: 100, ele_m: 110 },
      { dist_m: 200, ele_m: 120 },
    ])
  })

  it('preserves distAccum for empty-profile segments (M5 case)', () => {
    // Seg1: profile [0..100], seg2: NO profile (distance_m=200), seg3: [0..100]
    // seg3 must end at dist=400 (100 from seg1 + 200 gap fallback + 100 from seg3).
    //
    // Junction-dedup note: hadAny is true after seg1 emits points. When seg3 runs,
    // startIdx=1 → seg3's first point (dist_m=0) is junction-deduped. So the first
    // emitted point from seg3 is profile[1] at distAccum(300) + p[1].dist_m(100) = 400.
    const segs = [
      makeProfileSeg('w1', 'w2', 100, [
        { dist_m: 0, ele_m: 100 },
        { dist_m: 100, ele_m: 110 },
      ]),
      makeProfileSeg('w2', 'w3', 200, []),   // M5: DEM gap — empty profile
      makeProfileSeg('w3', 'w4', 100, [
        { dist_m: 0, ele_m: 130 },
        { dist_m: 100, ele_m: 140 },
      ]),
    ]
    const wps = makeWpList('w1', 'w2', 'w3', 'w4')
    const out = buildCombinedProfileFromSegments(segs, wps)
    expect(out).not.toBeNull()
    // Output: seg1[0,1] + seg3[1] (seg3[0] junction-deduped by hadAny=true)
    expect(out).toHaveLength(3)
    // seg3's last point: distAccum(300) + p[1].dist_m(100) = 400
    const last = out![out!.length - 1]
    expect(last.dist_m).toBe(400)
    expect(last.ele_m).toBe(140)
    // seg3's first emitted point is at 400 (junction-deduped from seg3[0]=300).
    // Verify seg2 gap is correctly reflected: seg1 ends at 100, then 200 gap → next at 400.
    expect(out![0].dist_m).toBe(0)
    expect(out![1].dist_m).toBe(100)
    expect(out![2].dist_m).toBe(400)
  })

  it('handles null ele_m points (DEM gap — Phase A.0 nullable)', () => {
    // ele_m may be null for DEM coverage gaps; they must be passed through.
    const segs = [
      makeProfileSeg('w1', 'w2', 100, [
        { dist_m: 0, ele_m: null },
        { dist_m: 50, ele_m: null },
        { dist_m: 100, ele_m: 100 },
      ]),
    ]
    const wps = makeWpList('w1', 'w2')
    const out = buildCombinedProfileFromSegments(segs, wps)
    expect(out).not.toBeNull()
    expect(out).toHaveLength(3)
    expect(out![0].ele_m).toBeNull()
    expect(out![1].ele_m).toBeNull()
    expect(out![2].ele_m).toBe(100)
  })

  it('returns null when no segment has a profile', () => {
    const segs = [makeProfileSeg('w1', 'w2', 100)]
    const wps = makeWpList('w1', 'w2')
    expect(buildCombinedProfileFromSegments(segs, wps)).toBeNull()
  })
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

  it('Core mit BRouter-only faellt beim frischen Planen auf BRouter zurueck', async () => {
    const { getCapabilities } = await import('@/api/routes')
    vi.mocked(getCapabilities).mockResolvedValueOnce({
      routing: true,
      brouter: true,
      badrouter: false,
      route_suggestion: false,
      features: {
        routing: { configured: true, available: true },
        brouter: { configured: true, available: true },
        badrouter: { configured: false, available: false, reason: 'core_profile' },
        route_suggestion: { configured: false, available: false, reason: 'core_profile' },
      },
    } as any)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    await waitFor(() => {
      expect(result.current.routingEngine).toBe('brouter')
    })
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

  it('loadSuggestion uebernimmt Sub-Segmente fuer Wegearten und Oberflaechen', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => { expect(result.current.profile.status).toBe('ready') })

    act(() => {
      result.current.loadSuggestion(makeFakeSuggestion({
        geojson: {
          type: 'FeatureCollection',
          features: [{
            type: 'Feature',
            geometry: {
              type: 'LineString',
              coordinates: [
                [12.0, 49.0, 500],
                [12.01, 49.01, 510],
                [12.02, 49.02, 505],
              ],
            },
            properties: {
              segments: [
                {
                  from_idx: 0,
                  to_idx: 2,
                  highway: 'path',
                  surface: 'gravel',
                  sac_scale: 'T2',
                },
              ],
            },
          }],
        },
      }))
    })

    expect(result.current.segments).toHaveLength(1)
    expect(result.current.segments[0].sub_segments).toEqual([
      {
        from_idx: 0,
        to_idx: 2,
        highway: 'path',
        surface: 'gravel',
        sac_scale: 'T2',
      },
    ])
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
    // Reset migration session cache so each test starts clean.
    clearMigrationCache()
    // Reset getElevationAtVerticesChunked mock to a safe default (returns empty array).
    getElevationAtVerticesChunkedMock.mockResolvedValue([])

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
    // T0 steady-state route (route_schema_version=1, all segments have profiles)
    // → neither getElevationProfile nor getElevationAtVerticesChunked called.
    const { getElevationProfile } = await import('@/api/routes')
    const getElevSpy = vi.mocked(getElevationProfile)
    getElevSpy.mockClear()
    getElevationAtVerticesChunkedMock.mockClear()

    // T0 route: version=1 AND per-segment profile present → needsMigration=false.
    const route = makeTestRoute({
      route_schema_version: 1,
      elevation_profile: [
        { dist_m: 0, ele_m: 100 },
        { dist_m: 50, ele_m: 110 },
      ],
      enrichment: { enrichment_version: 1, forest_pct: 0.5 },
      segments: [{
        from_waypoint: 'wp1',
        to_waypoint: 'wp2',
        mode: 'routed' as const,
        geometry: { type: 'LineString', coordinates: [[11.0, 47.0, 500], [11.1, 47.1, 520]] },
        control_points: [],
        smooth: true,
        distance_m: 12000,
        elevation_gain: 200,
        elevation_loss: 50,
        elevation_profile: [{ dist_m: 0, ele_m: 500 }, { dist_m: 12000, ele_m: 520 }],
      }],
    })

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    act(() => {
      result.current.loadRoute(route)
    })

    // elevationProfile is hydrated synchronously from route.elevation_profile
    await waitFor(() => {
      expect(result.current.elevationProfile).not.toBeNull()
    })

    // Neither DEM endpoint must have been called (T0 route — no migration)
    expect(getElevSpy).not.toHaveBeenCalled()
    expect(getElevationAtVerticesChunkedMock).not.toHaveBeenCalled()
    // No lazy-fill state active
    expect(result.current.lazyFillPendingSave).toBe(false)
    expect(result.current.lazyFillInProgress).toBe(false)
  })

  it('TestLoadRoute_LazyFillWhenProfileNull_DEMOnly', async () => {
    // Phase E / Spec §5.3: pre-T0 route (route_schema_version=0, segment without profile)
    // → migration is triggered. Stage 1 (coord-Z) fills the profile from inline-Z.
    // computeInternalRoute must NOT be called (no re-routing).
    // Migration is read-only: lazyFillPendingSave stays false.
    const computeSpy = vi.mocked(computeInternalRouteMock)
    computeSpy.mockClear()

    // Default segment has 3D coords [lon, lat, Z] → Stage 1 fills from Z
    const route = makeTestRoute({ elevation_profile: null, enrichment: null })
    const initialSegments = route.segments

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    act(() => {
      result.current.loadRoute(route)
    })

    // Wait for migration to complete
    await waitFor(() => {
      expect(result.current.lazyFillInProgress).toBe(false)
    })

    // computeInternalRoute (re-routing) was NOT called
    expect(computeSpy).not.toHaveBeenCalled()

    // elevationProfile is populated (Stage 1 coord-Z filled it)
    expect(result.current.elevationProfile).not.toBeNull()

    // Geometry (waypoints, coordinates) is identical — no route drift.
    // Note: segments now have elevation_profile added by migration (expected).
    expect(result.current.segments.length).toBe(initialSegments.length)
    expect(result.current.segments[0].from_waypoint).toBe(initialSegments[0].from_waypoint)
    expect(result.current.segments[0].to_waypoint).toBe(initialSegments[0].to_waypoint)
    expect(result.current.segments[0].geometry).toEqual(initialSegments[0].geometry)

    // Migration is read-only (Spec §5.4): lazyFillPendingSave remains false
    expect(result.current.lazyFillPendingSave).toBe(false)
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
    // Phase E: pre-T0 route with no inline-Z coords AND DEM returns empty →
    // all 3 migration stages produce nothing → silent graceful degradation,
    // no toast, no crash, lazyFillPendingSave remains false.
    getElevationAtVerticesChunkedMock.mockResolvedValue([])

    // Segment WITHOUT 3D coords (no inline-Z) and no DEM coverage
    const route = makeTestRoute({
      elevation_profile: null,
      segments: [{
        from_waypoint: 'wp1',
        to_waypoint: 'wp2',
        mode: 'routed' as const,
        geometry: {
          type: 'LineString',
          // 2D coords only (no Z) — Stage 1 returns []
          coordinates: [[11.0, 47.0], [11.1, 47.1]],
        },
        control_points: [],
        smooth: true,
        distance_m: 12000,
        elevation_gain: 0,
        elevation_loss: 0,
      }],
    })
    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    act(() => {
      result.current.loadRoute(route)
    })

    await waitFor(() => {
      expect(result.current.lazyFillInProgress).toBe(false)
    })

    // All stages produced nothing — profile stays null / segment profile stays empty
    // No toast (silent graceful degradation per Spec §5.4)
    // Note: error toast is shown on exception; empty results are not an error.
    expect(addToastMock).not.toHaveBeenCalled()
    expect(result.current.lazyFillPendingSave).toBe(false)
  })

  it('TestLoadRoute_LazyFillError_SilentGracefulDegradation', async () => {
    // Phase E: pre-T0 route where Stage 1 (coord-Z) fills from inline-Z.
    // migrateRouteCached does not throw in this case.
    // Separate error case: DEM exception on 2D-coord route where Stage 1+2 produce nothing
    // → toast shown, state is not crashed, lazyFillPendingSave remains false.
    getElevationAtVerticesChunkedMock.mockRejectedValue(new Error('PostGIS-DEM not available'))

    // 2D coords (no Z) → Stage 1 returns [], Stage 2 returns [] (no top-level profile),
    // Stage 3 (DEM) throws → error path: toast shown.
    const route = makeTestRoute({
      elevation_profile: null,
      segments: [{
        from_waypoint: 'wp1',
        to_waypoint: 'wp2',
        mode: 'routed' as const,
        geometry: {
          type: 'LineString',
          coordinates: [[11.0, 47.0], [11.1, 47.1]],
        },
        control_points: [],
        smooth: true,
        distance_m: 12000,
        elevation_gain: 0,
        elevation_loss: 0,
      }],
    })
    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    act(() => {
      result.current.loadRoute(route)
    })

    await waitFor(() => {
      expect(result.current.lazyFillInProgress).toBe(false)
    })

    // Phase E error path: toast is shown when migrateRouteCached throws
    expect(addToastMock).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }))
    expect(result.current.lazyFillPendingSave).toBe(false)
  })

  it('TestLazyFillToast_OnlyShownOncePerSession', async () => {
    // Phase E: pre-T0 migration is silent (no toast on successful coord-Z fill).
    // The showLazyFillNoticeOncePerSession path is removed in Phase E.
    // This test verifies: multiple pre-T0 route loads → NO toast (migration is silent).
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)

    sessionStorage.removeItem('skadia.lazy_fill_notice_shown')
    addToastMock.mockClear()

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => { expect(result.current.profile.status).toBe('ready') })

    // First pre-T0 load — coord-Z fills the profile silently
    act(() => {
      result.current.loadRoute(makeTestRoute({ id: 'r1', elevation_profile: null }))
    })
    await waitFor(() => { expect(result.current.lazyFillInProgress).toBe(false) })
    // Migration is silent — no toast for successful coord-Z fill
    expect(addToastMock).not.toHaveBeenCalled()

    // Second pre-T0 load — still no toast
    act(() => {
      result.current.loadRoute(makeTestRoute({ id: 'r2', elevation_profile: null }))
    })
    await waitFor(() => { expect(result.current.lazyFillInProgress).toBe(false) })
    expect(addToastMock).not.toHaveBeenCalled()
  })

  it('TestLoadRoute_FromActivityCreated_TriggersLazyFill', async () => {
    // Phase E: route_from_activity.go creates routes with elevation_profile=nil (pre-T0).
    // loadRoute MUST trigger pre-T0 migration, NOT computeInternalRoute.
    // Stage 1 fills from coord-Z (3D geometry). Migration is read-only.
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)

    computeInternalRouteMock.mockClear()

    // Default segment has 3D coords → Stage 1 fills
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

    // computeInternalRoute (re-routing) was NOT called (migration is DEM/coord-Z only)
    expect(computeInternalRouteMock).not.toHaveBeenCalled()
    // elevationProfile filled from Stage 1 coord-Z
    expect(result.current.elevationProfile).not.toBeNull()
    // Migration is read-only (Spec §5.4)
    expect(result.current.lazyFillPendingSave).toBe(false)
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
    clearMigrationCache()
    getElevationAtVerticesChunkedMock.mockResolvedValue([])

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

// ── Critic finding #10 (MAJOR): Lazy-fill geometry-inclusive distance axis ───
//
// Plan §10 Q1 acknowledges: lazy-fill uses cumulativeHaversineMeters over the
// WHOLE geometry (approach-inclusive), while retrackUnified emits profiles that
// are approach-exclusive. This is a known Phase-1 acceptance: lazy-fill is
// best-effort for legacy routes where no per-segment profile_dist_m is available.
//
// This test PINS the current geometry-inclusive behavior so that Phase 4
// (Hover-Sync via per-segment profile_dist_m) introduces a deliberate, detectable
// change rather than a silent regression.

describe('useRoutePlanner lazy-fill distance-axis (Critic #10, §10 Q1 Phase-1)', () => {
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

  it('TestLazyFill_DistanceAxis_GeometryInclusive_KnownDriftFromBackend', async () => {
    // Phase E version: pre-T0 migration fills per-segment profiles from coord-Z
    // (Stage 1). buildCombinedProfileFromSegments then produces the effective profile.
    //
    // For a route with a 2-segment geometry:
    //   Segment 1: coords [11.0,47.0,500], [11.05,47.05,510], [11.1,47.1,520] (3 pts)
    //   Segment 2: coords [11.1,47.1,520], [11.15,47.15,530]                  (2 pts)
    //
    // After migration: seg1 gets elevation_profile with 3 points (dist_m from Z),
    //   seg2 gets elevation_profile with 2 points.
    // buildCombinedProfileFromSegments: junction-dedup → 3 + 2 - 1 = 4 points total.
    //
    // Phase 4 (Hover-Sync) will change the axis to approach-exclusive.
    // When that happens, update the expected point count and dist_m values.

    const seg1Coords = [
      [11.0, 47.0, 500],
      [11.05, 47.05, 510],
      [11.1, 47.1, 520],
    ]
    const seg2Coords = [
      [11.1, 47.1, 520],
      [11.15, 47.15, 530],
    ]

    const wp1: RouteWaypoint = { id: 'w1', latlng: [47.0, 11.0], type: 'start' }
    const wp2: RouteWaypoint = { id: 'w2', latlng: [47.1, 11.1], type: 'via' }
    const wp3: RouteWaypoint = { id: 'w3', latlng: [47.15, 11.15], type: 'end' }

    const seg1: RouteSegment = {
      from_waypoint: 'w1',
      to_waypoint: 'w2',
      mode: 'routed',
      geometry: { type: 'LineString', coordinates: seg1Coords },
      control_points: [],
      smooth: true,
      distance_m: 8000,
      elevation_gain: 20,
      elevation_loss: 0,
    }
    const seg2: RouteSegment = {
      from_waypoint: 'w2',
      to_waypoint: 'w3',
      mode: 'routed',
      geometry: { type: 'LineString', coordinates: seg2Coords },
      control_points: [],
      smooth: true,
      distance_m: 5000,
      elevation_gain: 10,
      elevation_loss: 0,
    }

    const route: PlannedRoute = {
      id: 'geom-inclusive-test',
      name: 'Geometry Inclusive Test',
      sport_type: 'hiking',
      profile: 'hiking',
      waypoints: [wp1, wp2, wp3],
      segments: [seg1, seg2],
      time_override: false,
      auto_tag: false,
      photos: [],
      tags: [],
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      elevation_profile: null,
      enrichment: null,
    }

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    act(() => {
      result.current.loadRoute(route)
    })

    await waitFor(() => {
      expect(result.current.lazyFillInProgress).toBe(false)
    })

    const profile = result.current.elevationProfile
    expect(profile).not.toBeNull()
    // Phase E: coord-Z fills both segments → combined profile has
    // seg1 (3 pts) + seg2 (2 pts) - 1 junction dedup = 4 points.
    // Phase 4 will change the axis — when this test breaks, update intentionally.
    expect(profile!.length).toBe(4)

    // dist_m[0] must be 0
    expect(profile![0].dist_m).toBe(0)

    // dist_m values must be monotonically non-decreasing
    for (let i = 1; i < profile!.length; i++) {
      expect(profile![i].dist_m).toBeGreaterThanOrEqual(profile![i - 1].dist_m)
    }

    // Ele values match the inline-Z (500, 510, 520, 530)
    expect(profile![0].ele_m).toBe(500)
    expect(profile![1].ele_m).toBe(510)
    expect(profile![2].ele_m).toBe(520)
    expect(profile![3].ele_m).toBe(530)

    // Last dist_m > 0 (non-trivial distance axis)
    expect(profile![profile!.length - 1].dist_m).toBeGreaterThan(0)
  })
})

// ── Phase D.2: fillOffroadProfile + per-segment fill tokens ──────────────────
//
// Tests cover:
// 1. Happy path: fillOffroadProfile result applies when token is current.
// 2. Parallel edits to different segments all apply (key differentiator vs global token).
// 3. Re-edit same segment: earlier in-flight result is invalidated.
// 4. Segment replaced (identity check): stale result is discarded.

describe('fillOffroadProfile + per-segment fill tokens (Phase D.2)', () => {
  let qc: QueryClient

  // Minimal offroad segment factory
  function makeOffroadSeg(fromId: string, toId: string): RouteSegment {
    return {
      from_waypoint: fromId,
      to_waypoint: toId,
      mode: 'offroad',
      geometry: {
        type: 'LineString',
        coordinates: [[11, 47], [11.05, 47.05], [11.1, 47.1]],
      },
      control_points: [[47, 11], [47.05, 11.05], [47.1, 11.1]],
      smooth: true,
      distance_m: 5000,
      elevation_gain: 50,
      elevation_loss: 10,
      // Pre-populate elevation_profile so needsMigration() returns false for these
      // test routes — tests here target fillOffroadAndApply, not lazy-fill migration.
      elevation_profile: [{ dist_m: 0, ele_m: 500 }, { dist_m: 5000, ele_m: 510 }],
    }
  }

  // Minimal routed-segment-based PlannedRoute with one offroad segment
  function makeOffroadRoute(fromId: string, toId: string): PlannedRoute {
    const wp1: RouteWaypoint = { id: fromId, latlng: [47, 11], type: 'start' }
    const wp2: RouteWaypoint = { id: toId, latlng: [47.1, 11.1], type: 'end' }
    return {
      id: 'offroad-route',
      name: 'Offroad Route',
      sport_type: 'hiking',
      profile: 'hiking',
      waypoints: [wp1, wp2],
      segments: [makeOffroadSeg(fromId, toId)],
      time_override: false,
      auto_tag: false,
      photos: [],
      tags: [],
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      elevation_profile: null,
      enrichment: null,
      // route_schema_version=1: both conditions in needsMigration() are false
      // (version gate passes; segments already have elevation_profile via makeOffroadSeg).
      route_schema_version: 1,
    }
  }

  beforeEach(async () => {
    qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    addToastMock.mockClear()
    getElevationAtVerticesChunkedMock.mockClear()
    // Prevent cross-test cache hits in migrateRouteCached (module-level Map).
    clearMigrationCache()

    const { getProfileDefaults, cacheServerDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)
    vi.mocked(cacheServerDefaults).mockReset()

    const { getProfileDefaults: fetchProfileDefaults } = await import('@/api/routes')
    vi.mocked(fetchProfileDefaults).mockResolvedValue({ profile: 'hiking', uphill_cost: 7 } as any)

    // Default: returns a simple 3-point profile
    getElevationAtVerticesChunkedMock.mockResolvedValue([
      { dist_m: 0, ele_m: 500 },
      { dist_m: 2500, ele_m: 520 },
      { dist_m: 5000, ele_m: 510 },
    ])
  })

  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <ToastProvider>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </ToastProvider>
  )

  it('fillOffroadProfile applies result when token is current (happy path)', async () => {
    // Wire: updateControlPoint triggers fillOffroadAndApply → getElevationAtVerticesChunked.
    // After completion, the segment's elevation_profile must be set.
    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => expect(result.current.profile.status).toBe('ready'))

    const route = makeOffroadRoute('wp1', 'wp2')
    act(() => { result.current.loadRoute(route) })

    // Trigger: update a control point on the offroad segment
    await act(async () => {
      result.current.updateControlPoint({ from: 'wp1', to: 'wp2' }, 0, [47.02, 11.02])
    })

    // Wait for the DEM fill to complete
    await waitFor(() => {
      const seg = result.current.segments.find(
        s => s.from_waypoint === 'wp1' && s.to_waypoint === 'wp2',
      )
      return seg?.elevation_profile != null && seg.elevation_profile.length > 0
    })

    const seg = result.current.segments.find(
      s => s.from_waypoint === 'wp1' && s.to_waypoint === 'wp2',
    )
    expect(seg?.elevation_profile).not.toBeNull()
    expect(seg?.elevation_profile).toHaveLength(3)
    expect(seg?.elevation_profile![0].ele_m).toBe(500)
    expect(getElevationAtVerticesChunkedMock).toHaveBeenCalled()
  })

  it('fillOffroadProfile per-segment-token: fills for different segments apply independently', async () => {
    // Per-segment tokens ensure fills for DIFFERENT segments do not invalidate each other.
    // A global token would: filling segment A increments the global counter, so the token
    // that segment B's in-flight call captured is now stale → B's result discarded.
    // With per-segment map: fillTokens.get('w1:w2') and fillTokens.get('w2:w3') are
    // independent counters, so both results land correctly.
    //
    // Test: two-segment offroad route loaded, fill A triggered (hanging), fill B triggered
    // (hanging — A still in-flight). Both resolved → both apply independently.

    let resolveA!: (v: Array<{ dist_m: number; ele_m: number }>) => void
    let resolveB!: (v: Array<{ dist_m: number; ele_m: number }>) => void
    let callCount = 0
    getElevationAtVerticesChunkedMock.mockImplementation(() => {
      callCount += 1
      if (callCount === 1) return new Promise(res => { resolveA = res })
      return new Promise(res => { resolveB = res })
    })

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => expect(result.current.profile.status).toBe('ready'))

    // Two-segment offroad route (w1→w2, w2→w3).
    // elevation_profile: non-null, non-empty → needsLazyFill=false.
    const segAB = makeOffroadSeg('w1', 'w2')
    const segBC = makeOffroadSeg('w2', 'w3')
    const twoSegRoute: PlannedRoute = {
      id: 'two-seg-offroad',
      name: 'Two-segment offroad',
      sport_type: 'hiking',
      profile: 'hiking',
      waypoints: [
        { id: 'w1', latlng: [47, 11], type: 'start' },
        { id: 'w2', latlng: [47.1, 11.1], type: 'via' },
        { id: 'w3', latlng: [47.2, 11.2], type: 'end' },
      ],
      segments: [segAB, segBC],
      time_override: false,
      auto_tag: false,
      photos: [],
      tags: [],
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      elevation_profile: [{ dist_m: 0, ele_m: 0 }],
      enrichment: null,
      // route_schema_version=1 + segments with elevation_profile → needsMigration()=false.
      route_schema_version: 1,
    }

    await act(async () => { result.current.loadRoute(twoSegRoute) })

    // Fill for segment A (w1→w2) — in-flight call 1 (hanging).
    await act(async () => {
      result.current.updateControlPoint({ from: 'w1', to: 'w2' }, 0, [47.01, 11.01])
    })

    // Wait until DEM call 1 has started (fill A in-flight).
    await waitFor(() => {
      expect(getElevationAtVerticesChunkedMock).toHaveBeenCalledTimes(1)
    })

    // Fill for segment B (w2→w3) — in-flight call 2 (hanging).
    // At this point fill A is still in-flight; both fills are concurrent.
    await act(async () => {
      result.current.updateControlPoint({ from: 'w2', to: 'w3' }, 0, [47.11, 11.11])
    })

    // Wait until DEM call 2 has also started.
    await waitFor(() => {
      expect(getElevationAtVerticesChunkedMock).toHaveBeenCalledTimes(2)
    })

    // Resolve both fills: A → ele_m=100, B → ele_m=200.
    // With per-segment tokens: both should apply because their token counters are
    // independent (fillTokens.get('w1:w2')=1 and fillTokens.get('w2:w3')=1 are separate).
    await act(async () => {
      resolveA([{ dist_m: 0, ele_m: 100 }, { dist_m: 5000, ele_m: 110 }])
      resolveB([{ dist_m: 0, ele_m: 200 }, { dist_m: 5000, ele_m: 210 }])
    })

    // Both fills must have applied independently (per-segment tokens)
    await waitFor(() => {
      const segA = result.current.segments.find(s => s.from_waypoint === 'w1' && s.to_waypoint === 'w2')
      const segB = result.current.segments.find(s => s.from_waypoint === 'w2' && s.to_waypoint === 'w3')
      expect(segA?.elevation_profile?.[0]?.ele_m).toBe(100)
      expect(segB?.elevation_profile?.[0]?.ele_m).toBe(200)
    })
  })

  it('fillOffroadProfile per-segment-token: re-edit same segment invalidates earlier in-flight', async () => {
    // Edit seg A → in-flight call 1 (ele=999 stale).
    // Edit seg A again → in-flight call 2 (ele=777 fresh).
    // When call 1 resolves AFTER call 2: must be discarded (token mismatch).

    let resolveCall1!: (v: Array<{ dist_m: number; ele_m: number }>) => void
    let callCount = 0
    getElevationAtVerticesChunkedMock.mockImplementation(() => {
      callCount += 1
      if (callCount === 1) return new Promise(res => { resolveCall1 = res })
      // Call 2 resolves immediately with ele=777
      return Promise.resolve([{ dist_m: 0, ele_m: 777 }, { dist_m: 5000, ele_m: 780 }])
    })

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => expect(result.current.profile.status).toBe('ready'))

    const route = makeOffroadRoute('wp1', 'wp2')
    act(() => { result.current.loadRoute(route) })

    // First edit: triggers call 1 (in-flight, stale)
    act(() => {
      result.current.updateControlPoint({ from: 'wp1', to: 'wp2' }, 0, [47.01, 11.01])
    })

    // Second edit: triggers call 2 (resolves immediately with 777)
    act(() => {
      result.current.updateControlPoint({ from: 'wp1', to: 'wp2' }, 1, [47.09, 11.09])
    })

    // Wait for call 2 to land (ele=777)
    await waitFor(() => {
      const seg = result.current.segments.find(
        s => s.from_waypoint === 'wp1' && s.to_waypoint === 'wp2',
      )
      return seg?.elevation_profile != null && seg.elevation_profile[0].ele_m === 777
    })

    // Now resolve the stale call 1 with ele=999
    act(() => {
      resolveCall1([{ dist_m: 0, ele_m: 999 }, { dist_m: 5000, ele_m: 999 }])
    })

    // Allow microtasks to flush
    await act(async () => { await Promise.resolve() })

    // The stale 999 result must have been discarded (token counter mismatch).
    const seg = result.current.segments.find(
      s => s.from_waypoint === 'wp1' && s.to_waypoint === 'wp2',
    )
    expect(seg?.elevation_profile![0].ele_m).toBe(777)
    expect(seg?.elevation_profile![0].ele_m).not.toBe(999)
  })

  it('fillOffroadProfile segment-identity-check: segment replaced during fetch is skipped', async () => {
    // Edit seg (triggers DEM fetch, in-flight). While in-flight, loadRoute() replaces
    // all segments with new identities. When fetch resolves, identity check finds no
    // segment with the original from/to keys → result silently discarded.

    let resolveElev!: (v: Array<{ dist_m: number; ele_m: number }>) => void
    getElevationAtVerticesChunkedMock.mockImplementation(
      () => new Promise(res => { resolveElev = res }),
    )

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => expect(result.current.profile.status).toBe('ready'))

    const route = makeOffroadRoute('wp1', 'wp2')
    act(() => { result.current.loadRoute(route) })

    // Trigger in-flight DEM fetch
    act(() => {
      result.current.updateControlPoint({ from: 'wp1', to: 'wp2' }, 0, [47.01, 11.01])
    })

    // Wait until the DEM call has been initiated (resolver set)
    await waitFor(() => {
      expect(getElevationAtVerticesChunkedMock).toHaveBeenCalledTimes(1)
    })

    // Replace segments with a fresh loadRoute that uses different waypoint IDs
    const freshRoute = makeOffroadRoute('x1', 'x2')
    act(() => { result.current.loadRoute(freshRoute) })

    // Now resolve the stale DEM fetch
    act(() => {
      resolveElev([{ dist_m: 0, ele_m: 888 }, { dist_m: 5000, ele_m: 890 }])
    })

    // Allow microtasks to flush
    await act(async () => { await Promise.resolve() })

    // The new route's segment (x1→x2) must not have been touched by the stale result.
    const seg = result.current.segments.find(
      s => s.from_waypoint === 'x1' && s.to_waypoint === 'x2',
    )
    // elevation_profile may be null or already filled by its own DEM call.
    // The key assertion: the old segment (wp1→wp2) must not exist.
    const oldSeg = result.current.segments.find(
      s => s.from_waypoint === 'wp1' && s.to_waypoint === 'wp2',
    )
    expect(oldSeg).toBeUndefined()
    // The new segment was not written with the stale ele_m=888
    if (seg?.elevation_profile != null && seg.elevation_profile.length > 0) {
      expect(seg.elevation_profile[0].ele_m).not.toBe(888)
    }
  })
})

// ── P6 null-initial-state + profile lifecycle tests ───────────────────────────
// Migrated from the flat hooks/useRoutePlanner.test.tsx (P6 block).

describe('useRoutePlanner (P6 null-initial-state)', () => {
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

  it('starts with profile.status === loading before server fetch', () => {
    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    expect(result.current.profile.status).toBe('loading')
  })

  it('populates profile to ready after server fetch resolves', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    const hikingProfile = { profile: 'hiking', uphill_cost: 7 } as any
    vi.mocked(getProfileDefaults).mockReturnValue(hikingProfile)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => {
      expect(result.current.profile.status).toBe('ready')
    })
    if (result.current.profile.status === 'ready') {
      expect(result.current.profile.name).toBe('hiking')
    }
  })

  it('shows error toast when mount-fetch fails; profile.status stays loading', async () => {
    const { getProfileDefaults: fetchProfileDefaults } = await import('@/api/routes')
    vi.mocked(fetchProfileDefaults).mockRejectedValueOnce(new Error('network'))

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    await waitFor(() => {
      expect(addToastMock).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'error',
          title: expect.stringContaining('Profil-Defaults'),
        })
      )
    })
    expect(result.current.profile.status).toBe('loading')
  })

  it('addWaypoint before mount-fetch resolves does not crash (profile still loading)', async () => {
    let resolveMount: (v: any) => void
    const mountPromise = new Promise<any>(r => { resolveMount = r })
    const { getProfileDefaults: fetchProfileDefaults } = await import('@/api/routes')
    vi.mocked(fetchProfileDefaults).mockImplementationOnce(() => mountPromise)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    expect(result.current.profile.status).toBe('loading')

    await act(async () => {
      await result.current.addWaypoint([47.0, 11.0])
    })

    expect(result.current.waypoints.length).toBeGreaterThan(0)

    await act(async () => {
      resolveMount({ profile: 'hiking', uphill_cost: 7 })
      await new Promise(r => setTimeout(r, 0))
    })
  })

  it('loadSuggestion setzt 2 Waypoints + 1 Segment mit voller Geometrie', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    const hikingProfile = { profile: 'hiking', uphill_cost: 7 } as any
    vi.mocked(getProfileDefaults).mockReturnValue(hikingProfile)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    await waitFor(() => {
      expect(result.current.profile.status).toBe('ready')
    })

    const fakeSuggestion = makeFakeSuggestion({
      geojson: {
        type: 'FeatureCollection',
        features: [{
          type: 'Feature',
          geometry: {
            type: 'LineString',
            coordinates: [
              [12.0, 49.0, 500],
              [12.01, 49.01, 510],
              [12.02, 49.0, 505],
              [12.0, 49.0, 500],
            ],
          },
          properties: {},
        }],
      },
    })

    act(() => {
      result.current.loadSuggestion(fakeSuggestion)
    })

    expect(result.current.waypoints).toHaveLength(2)
    expect(result.current.waypoints[0].latlng[0]).toBeCloseTo(49.0)
    expect(result.current.waypoints[0].latlng[1]).toBeCloseTo(12.0)
    expect(result.current.waypoints[1].latlng[0]).toBeCloseTo(49.0)
    expect(result.current.waypoints[1].latlng[1]).toBeCloseTo(12.0)

    expect(result.current.segments).toHaveLength(1)
    expect(result.current.segments[0].distance_m).toBe(11700)
    expect(result.current.segments[0].elevation_gain).toBe(208)
    expect(result.current.segments[0].elevation_loss).toBe(208)
    expect(result.current.segments[0].geometry?.coordinates).toHaveLength(4)
  })

  it('loadRouteDraft uebernimmt volle Segment-Geometrie ohne gespeicherte Route zu binden', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    await waitFor(() => {
      expect(result.current.profile.status).toBe('ready')
    })

    const coords = [
      [12.0, 49.0, 500],
      [12.01, 49.01, 510],
      [12.02, 49.0, 505],
      [12.0, 49.0, 500],
    ]
    const draft = makeTestRoute({
      id: 'activity-draft',
      waypoints: [
        { id: 'start', latlng: [49.0, 12.0], type: 'start' },
        { id: 'end', latlng: [49.0, 12.0], type: 'end' },
      ],
      segments: [{
        from_waypoint: 'start',
        to_waypoint: 'end',
        mode: 'routed',
        geometry: { type: 'LineString', coordinates: coords },
        control_points: [],
        smooth: false,
        distance_m: 1234,
        elevation_gain: 56,
        elevation_loss: 34,
      }],
      total_distance_m: 1234,
      total_elevation_gain: 56,
      total_elevation_loss: 34,
    })

    act(() => {
      result.current.loadRouteDraft(draft)
    })

    expect(result.current.editingRouteId).toBeNull()
    expect(result.current.waypoints).toHaveLength(2)
    expect(result.current.segments).toHaveLength(1)
    expect(result.current.segments[0].geometry?.coordinates).toEqual(coords)
    expect(result.current.totals.distance_m).toBe(1234)
  })

  it('mount-fetch does not overwrite profileConfig set by loadRoute', async () => {
    let resolveMountFetch!: (v: any) => void
    const mountFetchPromise = new Promise<any>(r => { resolveMountFetch = r })

    const { getProfileDefaults: fetchProfileDefaults } = await import('@/api/routes')
    vi.mocked(fetchProfileDefaults).mockReturnValueOnce(mountFetchPromise)

    const mtbProfileConfig = { profile: 'mtb', uphill_cost: 5 } as any
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockImplementation((name: string) => {
      if (name === 'mtb') return mtbProfileConfig
      return null
    })

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    expect(result.current.profile.status).toBe('loading')

    act(() => {
      result.current.loadRoute({
        id: 'route-1',
        profile: 'mtb',
        sport_type: 'mountain_biking',
        waypoints: [],
        segments: [],
        total_distance_m: 0,
        total_elevation_gain: 0,
        total_elevation_loss: 0,
        name: 'Test Route',
        created_at: '',
        updated_at: '',
      } as any)
    })

    expect(result.current.profile.status).toBe('ready')
    if (result.current.profile.status === 'ready') {
      expect(result.current.profile.name).toBe('mtb')
    }

    await act(async () => {
      resolveMountFetch({ profile: 'hiking', uphill_cost: 7 })
      await new Promise(r => setTimeout(r, 0))
    })

    expect(result.current.profile.status).toBe('ready')
    if (result.current.profile.status === 'ready') {
      expect(result.current.profile.name).toBe('mtb')
    }
  })

  it('loadSuggestion setzt scalePin auf null zurück (I3-Invariant)', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => { expect(result.current.profile.status).toBe('ready') })

    act(() => { result.current.togglePin('sac') })
    expect(result.current.scalePin).toBe('sac')

    act(() => { result.current.loadSuggestion(makeFakeSuggestion()) })
    expect(result.current.scalePin).toBeNull()
  })

  it('recalculateAll setzt scalePin auf null zurück', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => { expect(result.current.profile.status).toBe('ready') })

    act(() => { result.current.togglePin('mtb') })
    expect(result.current.scalePin).toBe('mtb')

    await act(async () => {
      if (result.current.profile.status === 'ready') {
        await result.current.profile.recalculate()
      }
    })
    expect(result.current.scalePin).toBeNull()
  })

  it('loadRoute (cache-hit) setzt scalePin auf null zurück', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    const hikingProfile = { profile: 'hiking', uphill_cost: 7 } as any
    vi.mocked(getProfileDefaults).mockReturnValue(hikingProfile)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => { expect(result.current.profile.status).toBe('ready') })

    act(() => { result.current.togglePin('vf') })
    expect(result.current.scalePin).toBe('vf')

    act(() => {
      result.current.loadRoute({
        id: 'route-cache-hit',
        profile: 'hiking',
        sport_type: 'hiking',
        waypoints: [],
        segments: [],
        total_distance_m: 0,
        total_elevation_gain: 0,
        total_elevation_loss: 0,
        name: 'Cache-Hit Route',
        created_at: '',
        updated_at: '',
      } as any)
    })
    expect(result.current.scalePin).toBeNull()
  })

  it('loadRoute (cache-miss) setzt scalePin auf null zurück', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue(null)

    const { getProfileDefaults: fetchProfileDefaults } = await import('@/api/routes')
    vi.mocked(fetchProfileDefaults).mockResolvedValue({ profile: 'hiking', uphill_cost: 7 } as any)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    act(() => { result.current.togglePin('sac') })
    expect(result.current.scalePin).toBe('sac')

    act(() => {
      result.current.loadRoute({
        id: 'route-cache-miss',
        profile: 'hiking',
        sport_type: 'hiking',
        waypoints: [],
        segments: [],
        total_distance_m: 0,
        total_elevation_gain: 0,
        total_elevation_loss: 0,
        name: 'Cache-Miss Route',
        created_at: '',
        updated_at: '',
      } as any)
    })
    expect(result.current.scalePin).toBeNull()
  })

  it('loadRoute leitet heutige Defaults fuer Legacy-Profil outdoor aus der Sportart ab', async () => {
    const { getProfileDefaults, cacheServerDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockImplementation((profile: string) => (
      profile === 'hiking' ? { profile: 'hiking', uphill_cost: 7 } as any : null
    ))

    const { getProfileDefaults: fetchProfileDefaults } = await import('@/api/routes')
    vi.mocked(fetchProfileDefaults).mockResolvedValue({ profile: 'mtb', uphill_cost: 5 } as any)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => { expect(result.current.profile.status).toBe('ready') })

    vi.mocked(fetchProfileDefaults).mockClear()
    vi.mocked(cacheServerDefaults).mockClear()
    vi.mocked(getProfileDefaults).mockReturnValue(null)

    act(() => {
      result.current.loadRoute({
        id: 'legacy-outdoor-route',
        profile: 'outdoor',
        sport_type: 'cycling',
        sub_type: 'mountain',
        waypoints: [],
        segments: [],
        total_distance_m: 0,
        total_elevation_gain: 0,
        total_elevation_loss: 0,
        name: 'Legacy Outdoor Route',
        route_schema_version: 1,
        created_at: '',
        updated_at: '',
      } as any)
    })

    await waitFor(() => {
      expect(fetchProfileDefaults).toHaveBeenCalledWith('mtb')
    })
    expect(fetchProfileDefaults).not.toHaveBeenCalledWith('outdoor')
    expect(cacheServerDefaults).toHaveBeenCalledWith('mtb', expect.objectContaining({ profile: 'mtb' }))
    expect(addToastMock).not.toHaveBeenCalledWith(expect.objectContaining({
      title: 'Profil-Defaults konnten nicht geladen werden',
    }))
  })

  it('forceMigrateAndSave saves a loaded pre-T0 route with schema version 1', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)
    const { updateRoute } = await import('@/api/routes')

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => {
      expect(result.current.profile.status).toBe('ready')
    })

    const route: PlannedRoute = {
      id: 'route-pre-t0',
      name: 'Alte Route',
      profile: 'hiking',
      sport_type: 'hiking',
      waypoints: [
        { id: 'a', latlng: [47, 11] as [number, number], type: 'start' as const },
        { id: 'b', latlng: [47.1, 11.1] as [number, number], type: 'end' as const },
      ],
      segments: [{
        from_waypoint: 'a',
        to_waypoint: 'b',
        mode: 'routed' as const,
        geometry: { type: 'LineString' as const, coordinates: [[11, 47], [11.1, 47.1]] },
        control_points: [],
        smooth: true,
        distance_m: 100,
        elevation_gain: 10,
        elevation_loss: 0,
        elevation_profile: [{ dist_m: 0, ele_m: 100 }, { dist_m: 100, ele_m: 110 }],
      }],
      elevation_profile: [{ dist_m: 0, ele_m: 100 }, { dist_m: 100, ele_m: 110 }],
      time_override: false,
      auto_tag: false,
      route_schema_version: 0,
      photos: [],
      tags: [],
      created_at: '2026-04-27T00:00:00Z',
      updated_at: '2026-04-27T00:00:00Z',
    }

    act(() => {
      result.current.loadRoute(route)
    })
    expect(result.current.routeSchemaVersion).toBe(0)

    await act(async () => {
      await result.current.forceMigrateAndSave()
    })

    expect(updateRoute).toHaveBeenCalledWith('route-pre-t0', expect.objectContaining({
      route_schema_version: 1,
    }))
    await waitFor(() => {
      expect(result.current.routeSchemaVersion).toBe(1)
    })
  })
})
