/**
 * ProfileToCoordMap — side-channel mapping from combined-profile index to
 * combined-coordinate index. Prerequisite for Appendix-B hover sync
 * (plan Phase D.1.4 / Phase E hover brush).
 *
 * The mapping mirrors the exact same sort + junction-dedup logic as
 * buildCombinedProfileFromSegments so the two arrays are always in sync.
 */
import type { RouteSegment, RouteWaypoint, RouteElevationPoint } from '@/api/routes'

/** Index i → coordIdx: the combined-coordinate index corresponding to
 *  combined-profile point i. Length equals the combined profile length. */
export type ProfileToCoordMap = number[]

/**
 * Builds the combined profile, the ProfileToCoordMap, and the combined
 * coordinate array in a single pass — all three use the same sort + dedup
 * logic so they are always aligned.
 *
 * Junction-dedup: for segment index > 0, the first profile point AND the
 * first coordinate are both skipped (they duplicate the last point of the
 * previous segment).
 *
 * Returns null when no segment carries an elevation_profile (same contract
 * as buildCombinedProfileFromSegments — null means "no profile data yet").
 */
export function buildProfileToCoordMap(
  segments: RouteSegment[],
  waypoints: RouteWaypoint[],
): {
  profile: RouteElevationPoint[]
  map: ProfileToCoordMap
  coords: [number, number][]
} | null {
  // Sort by waypoint order (defensive, mirrors buildCombinedProfileFromSegments).
  const wpOrder = new Map(waypoints.map((wp, i) => [wp.id, i]))
  const sorted = [...segments].sort(
    (a, b) =>
      (wpOrder.get(a.from_waypoint) ?? Infinity) -
      (wpOrder.get(b.from_waypoint) ?? Infinity),
  )

  const profile: RouteElevationPoint[] = []
  const map: ProfileToCoordMap = []
  const coords: [number, number][] = []

  let distAccum = 0
  let hadAny = false
  let globalCoordIdx = 0

  for (const seg of sorted) {
    const p = seg.elevation_profile
    const segCoords = (seg.geometry?.coordinates ?? []) as [number, number][]

    // Accumulate coords (junction-dedup: skip first coord after the first segment).
    const coordStartIdx = hadAny ? 1 : 0
    for (let ci = coordStartIdx; ci < segCoords.length; ci++) {
      coords.push(segCoords[ci])
    }

    if (!p || p.length === 0) {
      // No profile: advance distAccum (M5 coverage-gap / pre-T0 segment).
      // Coords were already appended; advance coord counter accordingly.
      const coordsAdded = segCoords.length - coordStartIdx
      globalCoordIdx += coordsAdded
      distAccum += seg.distance_m ?? 0
      continue
    }

    // Profile points: junction-dedup mirrors buildCombinedProfileFromSegments.
    const profileStartIdx = hadAny ? 1 : 0

    // Profile point j maps to coord point j (at-vertices, 1-to-1 in T0 form).
    // Lengths can differ in mixed-form routes (silently-migrated pre-T0 segments,
    // synthetic prepended {dist_m:0}-points). Best-effort mapping: clamp the
    // coord index to the available coord window so hover degrades gracefully
    // (slight position offset) instead of disabling for the whole route.
    const segCoordLast = segCoords.length - 1
    for (let j = profileStartIdx; j < p.length; j++) {
      profile.push({ dist_m: distAccum + p[j].dist_m, ele_m: p[j].ele_m })
      const localIdx = Math.min(Math.max(0, j - profileStartIdx), Math.max(0, segCoordLast - coordStartIdx))
      map.push(globalCoordIdx + localIdx)
    }

    const coordsAdded = segCoords.length - coordStartIdx
    globalCoordIdx += coordsAdded
    distAccum += p[p.length - 1].dist_m
    hadAny = true
  }

  if (profile.length === 0) return null
  return { profile, map, coords }
}

export function profileIdxAtDist(distM: number, profile: RouteElevationPoint[]): number {
  if (profile.length === 0) return -1
  if (distM <= profile[0].dist_m) return 0
  const lastIdx = profile.length - 1
  if (distM >= profile[lastIdx].dist_m) return lastIdx

  let lo = 0
  let hi = lastIdx
  while (lo < hi) {
    const mid = Math.floor((lo + hi) / 2)
    if (profile[mid].dist_m < distM) {
      lo = mid + 1
    } else {
      hi = mid
    }
  }

  const nextIdx = lo
  const prevIdx = lo - 1
  const prevDelta = Math.abs(distM - profile[prevIdx].dist_m)
  const nextDelta = Math.abs(profile[nextIdx].dist_m - distM)
  return prevDelta <= nextDelta ? prevIdx : nextIdx
}

export function hoverPointAtFraction(
  fraction: number,
  totalDistanceM: number,
  profile: RouteElevationPoint[],
  profileToCoordMap: ProfileToCoordMap,
  coords: [number, number][],
): { lat: number; lng: number } | null {
  if (totalDistanceM <= 0 || profile.length === 0) return null
  const profileIdx = profileIdxAtDist(fraction * totalDistanceM, profile)
  if (profileIdx < 0) return null
  const coordIdx = profileToCoordMap[profileIdx]
  const coord = coords[coordIdx]
  if (!coord) return null
  const [lng, lat] = coord
  return { lat, lng }
}

export function bboxForProfileDistRange(
  startDistM: number,
  endDistM: number,
  profile: RouteElevationPoint[],
  profileToCoordMap: ProfileToCoordMap,
  coords: [number, number][],
): [number, number, number, number] | null {
  if (profile.length === 0 || coords.length === 0) return null
  const startProfileIdx = profileIdxAtDist(Math.min(startDistM, endDistM), profile)
  const endProfileIdx = profileIdxAtDist(Math.max(startDistM, endDistM), profile)
  if (startProfileIdx < 0 || endProfileIdx < 0) return null

  const startCoordIdx = profileToCoordMap[startProfileIdx]
  const endCoordIdx = profileToCoordMap[endProfileIdx]
  if (startCoordIdx == null || endCoordIdx == null) return null
  // Both dist endpoints snapped to the same coord → degenerate single-point bbox.
  // A single [lon,lat,lon,lat] bbox would cause fitBounds to zoom to maxZoom=15
  // on a single point. Return null so the caller skips the fitBounds call.
  if (startCoordIdx === endCoordIdx) return null

  const from = Math.max(0, Math.min(startCoordIdx, endCoordIdx))
  const to = Math.min(coords.length - 1, Math.max(startCoordIdx, endCoordIdx))
  let minLng = Infinity
  let minLat = Infinity
  let maxLng = -Infinity
  let maxLat = -Infinity

  for (let idx = from; idx <= to; idx++) {
    const coord = coords[idx]
    if (!coord) continue
    const [lng, lat] = coord
    minLng = Math.min(minLng, lng)
    minLat = Math.min(minLat, lat)
    maxLng = Math.max(maxLng, lng)
    maxLat = Math.max(maxLat, lat)
  }

  if (!Number.isFinite(minLng) || !Number.isFinite(minLat) || !Number.isFinite(maxLng) || !Number.isFinite(maxLat)) {
    return null
  }
  return [minLng, minLat, maxLng, maxLat]
}
