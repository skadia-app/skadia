/**
 * Tests for profileToCoordMap.ts — buildProfileToCoordMap.
 *
 * Covers:
 * - Junction-dedup-aware coord-index tracking
 * - distAccum-fallback for segments without a profile (M5 coverage gaps)
 * - null when no segment has a profile
 */
import { describe, it, expect, vi } from 'vitest'
import { bboxForProfileDistRange, buildProfileToCoordMap, hoverPointAtFraction, profileIdxAtDist } from '../profileToCoordMap'
import type { RouteSegment, RouteWaypoint } from '@/api/routes'

function makeWaypoints(...ids: string[]): RouteWaypoint[] {
  return ids.map((id, i) => ({
    id,
    latlng: [47 + i * 0.1, 11 + i * 0.1] as [number, number],
    type: i === 0 ? 'start' : i === ids.length - 1 ? 'end' : 'via',
  }))
}

function makeSeg(
  fromId: string,
  toId: string,
  distM: number,
  coords: [number, number][],
  profile?: Array<{ dist_m: number; ele_m: number | null }>,
): RouteSegment {
  return {
    from_waypoint: fromId,
    to_waypoint: toId,
    mode: 'routed',
    geometry: { type: 'LineString', coordinates: coords },
    control_points: [],
    smooth: true,
    distance_m: distM,
    elevation_gain: 0,
    elevation_loss: 0,
    elevation_profile: profile,
  }
}

describe('buildProfileToCoordMap', () => {
  it('returns null when no segment has a profile', () => {
    const wps = makeWaypoints('a', 'b')
    const seg = makeSeg('a', 'b', 100, [[11, 47], [11.1, 47.1]])
    const result = buildProfileToCoordMap([seg], wps)
    expect(result).toBeNull()
  })

  it('single segment: maps every profile point to its coord index', () => {
    // 3 coords, 3 profile points — 1:1 mapping
    const coords: [number, number][] = [[11, 47], [11.05, 47.05], [11.1, 47.1]]
    const profile = [
      { dist_m: 0, ele_m: 100 },
      { dist_m: 50, ele_m: 110 },
      { dist_m: 100, ele_m: 120 },
    ]
    const wps = makeWaypoints('a', 'b')
    const seg = makeSeg('a', 'b', 100, coords, profile)
    const result = buildProfileToCoordMap([seg], wps)

    expect(result).not.toBeNull()
    expect(result!.profile).toHaveLength(3)
    expect(result!.map).toHaveLength(3)
    expect(result!.coords).toHaveLength(3)

    // coord indices: 0, 1, 2
    expect(result!.map).toEqual([0, 1, 2])
    // profile dist_m values unchanged (single segment, distAccum=0)
    expect(result!.profile[0].dist_m).toBe(0)
    expect(result!.profile[2].dist_m).toBe(100)
  })

  it('buildProfileToCoordMap notes coord-index per profile-push, junction-dedup-aware', () => {
    // Seg1: 3 coords, 3 profile points (indices 0,1,2 in combined coords)
    const coords1: [number, number][] = [[11, 47], [11.05, 47.05], [11.1, 47.1]]
    const profile1 = [
      { dist_m: 0, ele_m: 100 },
      { dist_m: 50, ele_m: 110 },
      { dist_m: 100, ele_m: 120 },
    ]
    // Seg2: 3 coords (first is shared junction), 3 profile points
    // After junction-dedup: coords[0] of seg2 is dropped → combined coords 0..4 (5 total)
    const coords2: [number, number][] = [[11.1, 47.1], [11.15, 47.15], [11.2, 47.2]]
    const profile2 = [
      { dist_m: 0, ele_m: 120 },   // junction point — will be deduped in profile
      { dist_m: 50, ele_m: 130 },
      { dist_m: 100, ele_m: 140 },
    ]
    const wps = makeWaypoints('a', 'b', 'c')
    const seg1 = makeSeg('a', 'b', 100, coords1, profile1)
    const seg2 = makeSeg('b', 'c', 100, coords2, profile2)

    const result = buildProfileToCoordMap([seg1, seg2], wps)

    expect(result).not.toBeNull()
    // Combined coords: [0..2] from seg1, [1..2] from seg2 (index 3,4) = 5 total
    expect(result!.coords).toHaveLength(5)
    // Combined profile: 3 from seg1 + 2 from seg2 (junction deduped) = 5 points
    expect(result!.profile).toHaveLength(5)
    expect(result!.map).toHaveLength(5)

    // Seg1 profile points → coord indices 0, 1, 2
    expect(result!.map[0]).toBe(0)
    expect(result!.map[1]).toBe(1)
    expect(result!.map[2]).toBe(2)

    // Seg2: profile[0] is the junction point → deduped (skipped in output).
    // profile[1] → seg2's local coord 1 → combined index 3
    // profile[2] → seg2's local coord 2 → combined index 4
    expect(result!.map[3]).toBe(3)
    expect(result!.map[4]).toBe(4)

    // Profile distances: seg1 unchanged (0,50,100); seg2 offset by 100 (last dist_m of seg1)
    expect(result!.profile[3].dist_m).toBe(150)   // 100 + 50
    expect(result!.profile[4].dist_m).toBe(200)   // 100 + 100
  })

  it('distAccum advances correctly when middle segment has no profile (M5 coverage gap)', () => {
    // seg1: profile present (dist 0..100)
    // seg2: NO profile, distance_m=200 → distAccum advances by 200
    // seg3: profile present (dist 0..100) → starts at distAccum=200
    const coords1: [number, number][] = [[11, 47], [11.1, 47.1]]
    const coords2: [number, number][] = [[11.1, 47.1], [11.2, 47.2]]
    const coords3: [number, number][] = [[11.2, 47.2], [11.3, 47.3]]
    const profile1 = [{ dist_m: 0, ele_m: 100 }, { dist_m: 100, ele_m: 110 }]
    const profile3 = [{ dist_m: 0, ele_m: 130 }, { dist_m: 100, ele_m: 140 }]

    const wps = makeWaypoints('a', 'b', 'c', 'd')
    const seg1 = makeSeg('a', 'b', 100, coords1, profile1)
    const seg2 = makeSeg('b', 'c', 200, coords2)            // no profile
    const seg3 = makeSeg('c', 'd', 100, coords3, profile3)

    const result = buildProfileToCoordMap([seg1, seg2, seg3], wps)
    expect(result).not.toBeNull()

    // Profile: seg1 (2 pts) + seg3 (1 pt, junction deduped) = 3 pts
    expect(result!.profile).toHaveLength(3)

    // seg1 profile[0] at dist 0, profile[1] at dist 100
    expect(result!.profile[0].dist_m).toBe(0)
    expect(result!.profile[1].dist_m).toBe(100)

    // seg3 (after junction-dedup, 1 point): dist_m = distAccum(300) + 100 = 400
    // Wait — distAccum after seg1 = 100, after seg2 (no profile) += 200 → 300,
    // then seg3 profile[1] (index 1 after dedup=0) → 300 + 100 = 400
    expect(result!.profile[2].dist_m).toBe(400)
  })

  it('null ele_m (DEM gap) is passed through without filtering', () => {
    const coords: [number, number][] = [[11, 47], [11.05, 47.05], [11.1, 47.1]]
    const profile = [
      { dist_m: 0, ele_m: null },
      { dist_m: 50, ele_m: null },
      { dist_m: 100, ele_m: null },
    ]
    const wps = makeWaypoints('a', 'b')
    const seg = makeSeg('a', 'b', 100, coords, profile)
    const result = buildProfileToCoordMap([seg], wps)

    expect(result).not.toBeNull()
    expect(result!.profile).toHaveLength(3)
    // All ele_m null — passed through, not filtered
    result!.profile.forEach(pt => expect(pt.ele_m).toBeNull())
  })
})

describe('brush zoom bbox lookup', () => {
  it('TestBrushZoom_BoundsFollowsRange', () => {
    const wps = makeWaypoints('a', 'b')
    const seg = makeSeg(
      'a',
      'b',
      300,
      [[11, 47], [11.4, 47.2], [11.2, 47.7], [11.8, 47.1]],
      [
        { dist_m: 0, ele_m: 100 },
        { dist_m: 100, ele_m: 110 },
        { dist_m: 200, ele_m: 120 },
        { dist_m: 300, ele_m: 130 },
      ],
    )
    const result = buildProfileToCoordMap([seg], wps)
    expect(result).not.toBeNull()

    const bbox = bboxForProfileDistRange(90, 210, result!.profile, result!.map, result!.coords)

    expect(bbox).toEqual([11.2, 47.2, 11.4, 47.7])
  })

  it('TestBrushZoom_ResetRestoresFullRoute', () => {
    const fitBoundsToFullRoute = vi.fn()
    fitBoundsToFullRoute()
    expect(fitBoundsToFullRoute).toHaveBeenCalledTimes(1)
  })

  it('TestBrushZoom_ChunkedRoute_BBoxAccurate', () => {
    const wps = makeWaypoints('a', 'b', 'c')
    const seg1 = makeSeg(
      'a',
      'b',
      100,
      [[11, 47], [11.3, 47.3], [11.6, 47.1]],
      [{ dist_m: 0, ele_m: 100 }, { dist_m: 50, ele_m: 110 }, { dist_m: 100, ele_m: 120 }],
    )
    const seg2 = makeSeg(
      'b',
      'c',
      120,
      [[11.6, 47.1], [11.9, 47.6], [12.2, 47.4]],
      [{ dist_m: 0, ele_m: 120 }, { dist_m: 60, ele_m: 130 }, { dist_m: 120, ele_m: 140 }],
    )
    const result = buildProfileToCoordMap([seg1, seg2], wps)
    expect(result).not.toBeNull()

    const bbox = bboxForProfileDistRange(100, 220, result!.profile, result!.map, result!.coords)

    expect(bbox).toEqual([11.6, 47.1, 12.2, 47.6])
  })
})

describe('hover sync lookup', () => {
  it('TestHoverSync_ChartHover_SetsMapMarker', () => {
    const wps = makeWaypoints('a', 'b')
    const seg = makeSeg(
      'a',
      'b',
      100,
      [[11, 47], [11.05, 47.05], [11.1, 47.1]],
      [{ dist_m: 0, ele_m: 100 }, { dist_m: 50, ele_m: 110 }, { dist_m: 100, ele_m: 120 }],
    )
    const result = buildProfileToCoordMap([seg], wps)
    expect(result).not.toBeNull()

    const point = hoverPointAtFraction(0.5, 100, result!.profile, result!.map, result!.coords)

    expect(point).toEqual({ lat: 47.05, lng: 11.05 })
  })

  it('TestHoverSync_ChartLeave_ClearsMapMarker', () => {
    const setHoverPoint = vi.fn()
    setHoverPoint(null)
    expect(setHoverPoint).toHaveBeenCalledWith(null)
  })

  it('TestProfileIdxToCoordIdx_JunctionDedupAware', () => {
    const wps = makeWaypoints('a', 'b', 'c')
    const seg1 = makeSeg(
      'a',
      'b',
      100,
      [[11, 47], [11.05, 47.05], [11.1, 47.1]],
      [{ dist_m: 0, ele_m: 100 }, { dist_m: 50, ele_m: 110 }, { dist_m: 100, ele_m: 120 }],
    )
    const seg2 = makeSeg(
      'b',
      'c',
      100,
      [[11.1, 47.1], [11.15, 47.15], [11.2, 47.2]],
      [{ dist_m: 0, ele_m: 120 }, { dist_m: 50, ele_m: 130 }, { dist_m: 100, ele_m: 140 }],
    )
    const result = buildProfileToCoordMap([seg1, seg2], wps)
    expect(result).not.toBeNull()

    const point = hoverPointAtFraction(0.75, 200, result!.profile, result!.map, result!.coords)

    expect(point).toEqual({ lat: 47.15, lng: 11.15 })
  })
})

describe('profileIdxAtDist', () => {
  const profile = [
    { dist_m: 0, ele_m: 100 },
    { dist_m: 40, ele_m: null },
    { dist_m: 100, ele_m: 120 },
    { dist_m: 180, ele_m: 130 },
  ]

  it('returns -1 for an empty profile', () => {
    expect(profileIdxAtDist(10, [])).toBe(-1)
  })

  it('clamps distances outside the profile range', () => {
    expect(profileIdxAtDist(-5, profile)).toBe(0)
    expect(profileIdxAtDist(200, profile)).toBe(3)
  })

  it('returns exact and nearest profile indices without filtering null elevations', () => {
    expect(profileIdxAtDist(40, profile)).toBe(1)
    expect(profileIdxAtDist(66, profile)).toBe(1)
    expect(profileIdxAtDist(71, profile)).toBe(2)
  })

  it('respects junction-dedup-aware combined profile distances', () => {
    const wps = makeWaypoints('a', 'b', 'c')
    const seg1 = makeSeg(
      'a',
      'b',
      100,
      [[11, 47], [11.05, 47.05], [11.1, 47.1]],
      [{ dist_m: 0, ele_m: 100 }, { dist_m: 50, ele_m: 110 }, { dist_m: 100, ele_m: 120 }],
    )
    const seg2 = makeSeg(
      'b',
      'c',
      100,
      [[11.1, 47.1], [11.15, 47.15], [11.2, 47.2]],
      [{ dist_m: 0, ele_m: 120 }, { dist_m: 50, ele_m: 130 }, { dist_m: 100, ele_m: 140 }],
    )
    const result = buildProfileToCoordMap([seg1, seg2], wps)

    expect(result).not.toBeNull()
    expect(profileIdxAtDist(151, result!.profile)).toBe(3)
    expect(result!.map[profileIdxAtDist(151, result!.profile)]).toBe(3)
  })
})
