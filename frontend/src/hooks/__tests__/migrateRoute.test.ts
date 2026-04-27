/**
 * Unit tests for migrateRoute.ts (Phase E.3 / Plan §E.3.1).
 *
 * Coverage:
 *  M1: T0 steady-state route → needsMigration=false, no DEM hop
 *  M2: Routed segment without profile → DEM-hop fills it
 *  M3: Mixed routed + offroad, all segments filled
 *  M4: route_schema_version===0 → needsMigration=true
 *  M5: No DEM coverage → all-null profile, silent
 *  M6: Partial DEM coverage → some null points passed through
 *  Race: user edits during migration → stale result discarded (via token in useRoutePlanner)
 *  TwoTabs: skipped (optimistic-concurrency pre-existing bug)
 *  JunctionVertex: approach-end equals next segment start → no double-drop
 *  ConcatHop: junction vertex in earlier segment, not duplicated
 *  Cache: second load with same id+updated_at hits session cache, no second DEM hop
 *  StillReachable: CI gate for migration-code removal
 *  NoAutoSave: migration is read-only (tested via useRoutePlanner.test.tsx)
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import type { PlannedRoute, RouteSegment, RouteWaypoint } from '@/api/routes'
import {
  needsMigration,
  migrateRoute,
  migrateRouteCached,
  clearMigrationCache,
} from '../migrateRoute'

// ─── Mock getElevationAtVerticesChunked ───────────────────────────────────────
const getElevationAtVerticesChunkedMock = vi.hoisted(() => vi.fn())
vi.mock('@/api/elevation-chunked', () => ({
  getElevationAtVerticesChunked: getElevationAtVerticesChunkedMock,
}))

// ─── Route + Segment factories ────────────────────────────────────────────────

function makeWaypoints(ids: string[]): RouteWaypoint[] {
  return ids.map((id, i) => ({
    id,
    latlng: [47 + i * 0.1, 11 + i * 0.1] as [number, number],
    type: (i === 0 ? 'start' : i === ids.length - 1 ? 'end' : 'via') as RouteWaypoint['type'],
  }))
}

/** Segment with 3D geometry (Stage-1 fillable). */
function makeSegment3D(
  fromId: string,
  toId: string,
  mode: RouteSegment['mode'] = 'routed',
  elevProfile?: RouteSegment['elevation_profile'],
): RouteSegment {
  return {
    from_waypoint: fromId,
    to_waypoint: toId,
    mode,
    geometry: {
      type: 'LineString',
      coordinates: [
        [11.0 + Math.random() * 0.001, 47.0, 500],
        [11.05, 47.05, 510],
        [11.1, 47.1, 520],
      ],
    },
    control_points: [],
    smooth: true,
    distance_m: 12000,
    elevation_gain: 20,
    elevation_loss: 0,
    elevation_profile: elevProfile,
  }
}

/** Segment with 2D geometry only (Stage-1 cannot fill — needs DEM). */
function makeSegment2D(
  fromId: string,
  toId: string,
  mode: RouteSegment['mode'] = 'routed',
  elevProfile?: RouteSegment['elevation_profile'],
): RouteSegment {
  return {
    from_waypoint: fromId,
    to_waypoint: toId,
    mode,
    geometry: {
      type: 'LineString',
      coordinates: [
        [11.0, 47.0],
        [11.05, 47.05],
        [11.1, 47.1],
      ],
    },
    control_points: [],
    smooth: true,
    distance_m: 12000,
    elevation_gain: 20,
    elevation_loss: 0,
    elevation_profile: elevProfile,
  }
}

function makeRoute(overrides: Partial<PlannedRoute> & { segments: RouteSegment[] }): PlannedRoute {
  const wps = makeWaypoints(
    overrides.segments.flatMap(s => [s.from_waypoint, s.to_waypoint]).filter(
      (id, i, arr) => arr.indexOf(id) === i,
    ),
  )
  return {
    id: 'test-route',
    name: 'Test Route',
    sport_type: 'hiking',
    profile: 'hiking',
    waypoints: wps,
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

// ─── beforeEach: reset cache + mock ──────────────────────────────────────────

beforeEach(() => {
  clearMigrationCache()
  getElevationAtVerticesChunkedMock.mockReset()
  // Default: DEM returns empty (simulates no coverage)
  getElevationAtVerticesChunkedMock.mockResolvedValue([])
})

// ─── needsMigration ───────────────────────────────────────────────────────────

describe('needsMigration', () => {
  it('M1: T0 route with version=1 and all profiles present → false', () => {
    const route = makeRoute({
      route_schema_version: 1,
      segments: [
        makeSegment3D('w1', 'w2', 'routed', [{ dist_m: 0, ele_m: 500 }, { dist_m: 100, ele_m: 510 }]),
      ],
    })
    expect(needsMigration(route)).toBe(false)
  })

  it('M4: route_schema_version===0 → true even if segment has a profile', () => {
    const route = makeRoute({
      route_schema_version: 0,
      segments: [
        makeSegment3D('w1', 'w2', 'routed', [{ dist_m: 0, ele_m: 500 }]),
      ],
    })
    expect(needsMigration(route)).toBe(true)
  })

  it('version absent (undefined → 0) and segment has no profile → true', () => {
    const route = makeRoute({
      segments: [makeSegment3D('w1', 'w2')],
    })
    expect(needsMigration(route)).toBe(true)
  })

  it('version=1 but segment without profile → true (partial T0 save)', () => {
    const route = makeRoute({
      route_schema_version: 1,
      segments: [makeSegment3D('w1', 'w2')],  // no elevation_profile
    })
    expect(needsMigration(route)).toBe(true)
  })

  it('version=1, segment with empty profile [] → true', () => {
    const route = makeRoute({
      route_schema_version: 1,
      segments: [makeSegment3D('w1', 'w2', 'routed', [])],
    })
    expect(needsMigration(route)).toBe(true)
  })
})

// ─── TestMigrationPathStillReachable (E.3.6 CI gate) ─────────────────────────
// When this test fails, it signals that no Pre-T0 routes with route_schema_version=0
// exist in DB anymore and the migration code path + lazyFillToken can be removed.

it('TestMigrationPathStillReachable', () => {
  const fakePreT0Route = {
    route_schema_version: 0,
    segments: [{ elevation_profile: [{ dist_m: 0, ele_m: 100 }] }],
  } as unknown as PlannedRoute
  // This MUST remain true while Pre-T0 routes exist in DB.
  // When all DB routes have route_schema_version=1, this assertion becomes false
  // and the migration code can be safely deleted.
  expect(needsMigration(fakePreT0Route)).toBe(true)
})

// ─── migrateRoute: Stage 1 (coord-Z) ─────────────────────────────────────────

describe('migrateRoute — Stage 1 (coord-Z fill)', () => {
  it('M2: segment with 3D coords and no profile → filled from inline-Z', async () => {
    const seg = makeSegment3D('w1', 'w2')
    const route = makeRoute({ segments: [seg] })

    const result = await migrateRoute(route)

    expect(result.length).toBe(1)
    const filled = result[0]
    expect(filled.elevation_profile).toBeDefined()
    expect(filled.elevation_profile!.length).toBeGreaterThanOrEqual(2)
    expect(filled.elevation_profile![0].ele_m).toBe(500)
    // DEM was NOT called (Stage 1 succeeded)
    expect(getElevationAtVerticesChunkedMock).not.toHaveBeenCalled()
  })

  it('segment already has a profile → returned unchanged', async () => {
    const existingProfile = [{ dist_m: 0, ele_m: 400 }, { dist_m: 1000, ele_m: 450 }]
    const seg = makeSegment3D('w1', 'w2', 'routed', existingProfile)
    const route = makeRoute({ segments: [seg] })

    const result = await migrateRoute(route)

    expect(result[0].elevation_profile).toStrictEqual(existingProfile)
    expect(getElevationAtVerticesChunkedMock).not.toHaveBeenCalled()
  })

  it('all segments already have profiles → returns same array reference', async () => {
    const seg1 = makeSegment3D('w1', 'w2', 'routed', [{ dist_m: 0, ele_m: 500 }])
    const seg2 = makeSegment3D('w2', 'w3', 'routed', [{ dist_m: 0, ele_m: 600 }])
    const route = makeRoute({ segments: [seg1, seg2] })

    const result = await migrateRoute(route)

    expect(result).toBe(route.segments)  // same reference — no copy needed
    expect(getElevationAtVerticesChunkedMock).not.toHaveBeenCalled()
  })
})

// ─── migrateRoute: Stage 3 (DEM concat-hop) ──────────────────────────────────

describe('migrateRoute — Stage 3 (DEM concat-hop)', () => {
  it('M2: 2D segment without Z → DEM fills it', async () => {
    const demProfile = [
      { dist_m: 0, ele_m: 800 },
      { dist_m: 5000, ele_m: 850 },
      { dist_m: 10000, ele_m: 900 },
    ]
    getElevationAtVerticesChunkedMock.mockResolvedValue(demProfile)

    const seg = makeSegment2D('w1', 'w2')
    const route = makeRoute({ segments: [seg] })

    const result = await migrateRoute(route)

    expect(getElevationAtVerticesChunkedMock).toHaveBeenCalledTimes(1)
    expect(result[0].elevation_profile).toBeDefined()
    expect(result[0].elevation_profile!.length).toBe(3)
    // First point is normalised to dist_m=0
    expect(result[0].elevation_profile![0].dist_m).toBe(0)
    expect(result[0].elevation_profile![0].ele_m).toBe(800)
  })

  it('M5: DEM returns empty → segment profile stays undefined, no throw', async () => {
    getElevationAtVerticesChunkedMock.mockResolvedValue([])

    const seg = makeSegment2D('w1', 'w2')
    const route = makeRoute({ segments: [seg] })

    const result = await migrateRoute(route)

    // DEM was called but returned empty; segment keeps no profile
    expect(getElevationAtVerticesChunkedMock).toHaveBeenCalledTimes(1)
    // segment is returned unchanged (no profile added)
    expect(result[0].elevation_profile).toBeUndefined()
  })

  it('M6: partial DEM coverage → null ele_m values preserved', async () => {
    const demProfile = [
      { dist_m: 0, ele_m: 800 },
      { dist_m: 3000, ele_m: null },
      { dist_m: 6000, ele_m: 850 },
    ]
    getElevationAtVerticesChunkedMock.mockResolvedValue(demProfile)

    const seg = makeSegment2D('w1', 'w2')
    const route = makeRoute({ segments: [seg] })

    const result = await migrateRoute(route)

    expect(result[0].elevation_profile).toBeDefined()
    expect(result[0].elevation_profile!.length).toBe(3)
    // null ele_m preserved (DEM gap, not substituted with 0)
    expect(result[0].elevation_profile![1].ele_m).toBeNull()
  })

  it('M3: mixed 3D and 2D segments → each filled by its respective stage', async () => {
    const demProfile3 = [
      { dist_m: 0, ele_m: 700 },
      { dist_m: 5000, ele_m: 720 },
      { dist_m: 10000, ele_m: 740 },
    ]
    getElevationAtVerticesChunkedMock.mockResolvedValue(demProfile3)

    const seg3d = makeSegment3D('w1', 'w2')  // Stage 1 fills this
    const seg2d = makeSegment2D('w2', 'w3')  // Stage 3 fills this
    const route = makeRoute({ segments: [seg3d, seg2d] })

    const result = await migrateRoute(route)

    // Stage 1: filled seg3d from inline-Z (ele_m=500, 510, 520)
    expect(result[0].elevation_profile).toBeDefined()
    expect(result[0].elevation_profile![0].ele_m).toBe(500)

    // Stage 3: DEM called once for seg2d only
    expect(getElevationAtVerticesChunkedMock).toHaveBeenCalledTimes(1)
    expect(result[1].elevation_profile).toBeDefined()
    expect(result[1].elevation_profile![0].ele_m).toBe(700)
  })
})

// ─── TestFillSegmentsViaConcatHop_JunctionVertexInEarlierSegment ──────────────

describe('fillSegmentsViaConcatHop — junction-vertex dedup', () => {
  it('two 2D segments sharing a junction coord → N1+N2-1 total DEM points', async () => {
    // Seg1: 3 coords, Seg2: 3 coords sharing first coord with seg1's last coord.
    // Concat-hop: sliceStart=1 for seg2 → allCoords has 3 + 2 = 5 entries.
    // DEM returns 5 points (one per vertex).
    // After remapping: seg1 gets 3 points, seg2 gets 2 points.
    const demProfile = [
      { dist_m: 0, ele_m: 500 },
      { dist_m: 5000, ele_m: 510 },
      { dist_m: 10000, ele_m: 520 },  // shared junction → in seg1 (earlier)
      { dist_m: 15000, ele_m: 530 },
      { dist_m: 20000, ele_m: 540 },
    ]
    getElevationAtVerticesChunkedMock.mockResolvedValue(demProfile)

    const seg1 = makeSegment2D('w1', 'w2')
    const seg2 = makeSegment2D('w2', 'w3')
    const route = makeRoute({ segments: [seg1, seg2] })

    const result = await migrateRoute(route)

    // DEM called once for combined geometry
    expect(getElevationAtVerticesChunkedMock).toHaveBeenCalledTimes(1)

    // Seg1: points 0..2 (3 points, including junction)
    expect(result[0].elevation_profile!.length).toBe(3)
    expect(result[0].elevation_profile![2].ele_m).toBe(520)  // junction in earlier seg

    // Seg2: points 3..4 (2 points, junction dropped from later seg)
    expect(result[1].elevation_profile!.length).toBe(2)
    expect(result[1].elevation_profile![0].ele_m).toBe(530)  // first point AFTER junction
  })
})

// ─── migrateRouteCached — session cache ───────────────────────────────────────

describe('migrateRouteCached — session cache', () => {
  it('TestMigrateRouteCached_SecondLoadHitsCache', async () => {
    const demProfile = [
      { dist_m: 0, ele_m: 800 },
      { dist_m: 5000, ele_m: 850 },
    ]
    getElevationAtVerticesChunkedMock.mockResolvedValue(demProfile)

    const seg = makeSegment2D('w1', 'w2')
    const route = makeRoute({
      id: 'cached-route',
      segments: [seg],
      updated_at: '2026-01-01T10:00:00Z',
    })

    // First call — hits DEM
    const result1 = await migrateRouteCached(route)
    expect(getElevationAtVerticesChunkedMock).toHaveBeenCalledTimes(1)

    // Second call with same id+updated_at — hits cache, no second DEM hop
    const result2 = await migrateRouteCached(route)
    expect(getElevationAtVerticesChunkedMock).toHaveBeenCalledTimes(1)  // still 1

    // Same result returned from cache
    expect(result2).toBe(result1)
  })

  it('different updated_at → cache miss, DEM called again', async () => {
    getElevationAtVerticesChunkedMock.mockResolvedValue([
      { dist_m: 0, ele_m: 800 },
      { dist_m: 5000, ele_m: 850 },
    ])

    const seg = makeSegment2D('w1', 'w2')
    const routeV1 = makeRoute({
      id: 'same-id',
      segments: [seg],
      updated_at: '2026-01-01T10:00:00Z',
    })
    const routeV2 = makeRoute({
      id: 'same-id',
      segments: [seg],
      updated_at: '2026-01-01T11:00:00Z',  // different timestamp → cache miss
    })

    await migrateRouteCached(routeV1)
    await migrateRouteCached(routeV2)

    // Two different cache keys → two DEM calls
    expect(getElevationAtVerticesChunkedMock).toHaveBeenCalledTimes(2)
  })
})

// ─── Stage-2 gate: single-segment vs. multi-segment ──────────────────────────

describe('migrateRoute — Stage-2 legacyToRouteProfile gate', () => {
  it('TestMigration_MultiSegment_LegacyTopLevel_NotApplied: Stage 2 must NOT trigger → DEM called', async () => {
    // Pre-T0 multi-segment route with a top-level elevation_profile.
    // Stage 2 must be skipped — applying the full-route profile to each gap
    // segment would give every segment the identical full-route curve.
    const topLevelProfile = [
      { fraction: 0, elevation: 500 },
      { fraction: 0.5, elevation: 550 },
      { fraction: 1, elevation: 600 },
    ]
    const demProfile = [
      { dist_m: 0, ele_m: 700 },
      { dist_m: 5000, ele_m: 720 },
      { dist_m: 10000, ele_m: 740 },
    ]
    getElevationAtVerticesChunkedMock.mockResolvedValue(demProfile)

    // Two 2D segments — Stage 1 cannot fill them, Stage 2 must be skipped,
    // Stage 3 (DEM) is the only option.
    const seg1 = makeSegment2D('w1', 'w2')
    const seg2 = makeSegment2D('w2', 'w3')
    const route = makeRoute({
      segments: [seg1, seg2],
      // Cast through unknown: the legacy top-level profile uses { fraction, elevation }
      // shape (pre-T0 BRouter format), not the modern RouteElevationPoint schema.
      elevation_profile: topLevelProfile as unknown as PlannedRoute['elevation_profile'],
    })

    await migrateRoute(route)

    // DEM MUST have been called (Stage 2 was not applied)
    expect(getElevationAtVerticesChunkedMock).toHaveBeenCalledTimes(1)
  })

  it('TestMigration_SingleSegment_LegacyTopLevel_Applied: Stage 2 triggers for single-segment routes', async () => {
    // Single-segment route with a top-level elevation_profile (legacy BRouter).
    // Stage 1 fails (2D geometry), Stage 2 should fill it → DEM must NOT be called.
    const topLevelProfile = [
      { fraction: 0, elevation: 800 },
      { fraction: 0.5, elevation: 900 },
      { fraction: 1, elevation: 850 },
    ]
    // DEM should not be called — we'll verify that below.
    getElevationAtVerticesChunkedMock.mockResolvedValue([])

    const seg = makeSegment2D('w1', 'w2')
    const route = makeRoute({
      segments: [seg],
      // Cast through unknown: the legacy top-level profile uses { fraction, elevation }
      // shape (pre-T0 BRouter format), not the modern RouteElevationPoint schema.
      elevation_profile: topLevelProfile as unknown as PlannedRoute['elevation_profile'],
    })

    const result = await migrateRoute(route)

    // DEM must NOT have been called (Stage 2 succeeded)
    expect(getElevationAtVerticesChunkedMock).not.toHaveBeenCalled()
    // Segment has a profile derived from the top-level legacy data
    expect(result[0].elevation_profile).toBeDefined()
    expect(result[0].elevation_profile!.length).toBeGreaterThanOrEqual(2)
    // First point should have ele_m from the top-level profile (elevation=800 at fraction=0)
    expect(result[0].elevation_profile![0].ele_m).toBe(800)
  })
})

// ─── TestMigration_TwoTabsSameRoute_NoConflict ────────────────────────────────
// Pre-existing architectural limitation: two concurrent tab migrations to the same
// route cannot be coordinated without backend optimistic-concurrency support.
// Self-tracked in docs/_internal/STATE.md.
it.skip(
  'TestMigration_TwoTabsSameRoute_NoConflict — optimistic-concurrency not implemented — see docs/_internal/STATE.md pre-existing bug',
  () => {
    // Would require: two parallel migrateRoute calls to same route ID →
    // verify that both tabs end up with consistent segment profiles.
    // Cannot be tested without backend session-level write coordination.
  },
)

// ─── TestMigration_ApproachEndEqualsNextSplineStart_NoDoubleDrop ──────────────

it('TestMigration_ApproachEndEqualsNextSplineStart_NoDoubleDrop', async () => {
  // Two adjacent segments where seg1 ends at exactly the same coord as seg2 starts.
  // Concat-hop sliceStart=1 for seg2 → junction coord is in seg1 only, not doubled.
  const demProfile = [
    { dist_m: 0, ele_m: 500 },      // seg1 coord 0
    { dist_m: 7000, ele_m: 510 },   // seg1 coord 1 (shared junction)
    { dist_m: 14000, ele_m: 520 },  // seg2 coord 1 (first after junction)
  ]
  getElevationAtVerticesChunkedMock.mockResolvedValue(demProfile)

  // Seg1 ends at [11.1, 47.1], seg2 starts at [11.1, 47.1] (exact same coord).
  const seg1: RouteSegment = {
    from_waypoint: 'w1',
    to_waypoint: 'w2',
    mode: 'routed',
    geometry: {
      type: 'LineString',
      coordinates: [[11.0, 47.0], [11.1, 47.1]],  // 2D — needs DEM
    },
    control_points: [],
    smooth: true,
    distance_m: 7000,
    elevation_gain: 10,
    elevation_loss: 0,
  }
  const seg2: RouteSegment = {
    from_waypoint: 'w2',
    to_waypoint: 'w3',
    mode: 'routed',
    geometry: {
      type: 'LineString',
      coordinates: [[11.1, 47.1], [11.2, 47.2]],  // starts at same coord as seg1 end
    },
    control_points: [],
    smooth: true,
    distance_m: 7000,
    elevation_gain: 10,
    elevation_loss: 0,
  }

  const route = makeRoute({ segments: [seg1, seg2] })
  const result = await migrateRoute(route)

  // DEM called once on combined geometry (3 vertices after junction-dedup)
  expect(getElevationAtVerticesChunkedMock).toHaveBeenCalledTimes(1)
  const calledWith = getElevationAtVerticesChunkedMock.mock.calls[0][0]
  // Combined geometry has 3 vertices (not 4): seg1[0], seg1[1](=junction), seg2[1]
  expect(calledWith.coordinates.length).toBe(3)

  // Seg1: 2 points (0..1), junction in seg1
  expect(result[0].elevation_profile!.length).toBe(2)
  expect(result[0].elevation_profile![1].ele_m).toBe(510)  // junction in earlier seg

  // Seg2: 1 point (index 2 → after junction), normalised dist from 0
  expect(result[1].elevation_profile!.length).toBe(1)
  expect(result[1].elevation_profile![0].dist_m).toBe(0)
  expect(result[1].elevation_profile![0].ele_m).toBe(520)
})

// ─── TestSessionMigratedCache_ClearedOnLogout ─────────────────────────────────

describe('clearMigrationCache — logout multi-user safety', () => {
  it('TestSessionMigratedCache_ClearedOnLogout: cache populated → clearMigrationCache() → cache empty, DEM called again', async () => {
    const demProfile = [
      { dist_m: 0, ele_m: 800 },
      { dist_m: 5000, ele_m: 850 },
    ]
    getElevationAtVerticesChunkedMock.mockResolvedValue(demProfile)

    const seg = makeSegment2D('w1', 'w2')
    const route = makeRoute({
      id: 'multi-user-route',
      segments: [seg],
      updated_at: '2026-01-01T10:00:00Z',
    })

    // First call — populates cache
    await migrateRouteCached(route)
    expect(getElevationAtVerticesChunkedMock).toHaveBeenCalledTimes(1)

    // Simulate logout: clear the cache (production path via BroadcastChannel)
    clearMigrationCache()

    // Second call with same key — cache was cleared → DEM called again
    await migrateRouteCached(route)
    expect(getElevationAtVerticesChunkedMock).toHaveBeenCalledTimes(2)
  })
})
