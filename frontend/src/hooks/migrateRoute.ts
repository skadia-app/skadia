/**
 * Pre-T0 route migration helpers (Spec §5.3-5.5 / Phase E).
 *
 * Motivation: routes saved before T0 (route_schema_version=0) may have segments
 * without elevation_profile. This module fills those gaps lazily on load —
 * strictly read-only (Spec §5.4). The next explicit user save will persist the
 * filled profiles and set route_schema_version=1 on the backend.
 *
 * Entry point for callers: migrateRouteCached (session-cached wrapper).
 * Direct migrateRoute is exported for testing only.
 */
import type { PlannedRoute, RouteSegment, RouteElevationPoint } from '@/api/routes'
import { legacyToRouteProfile } from '@/api/routes'
import { getElevationAtVerticesChunked } from '@/api/elevation-chunked'
import { haversineM } from '@/lib/geoUtils'

// ─── Segment composite key ────────────────────────────────────────────────────
// RouteSegment has no id field — use from_waypoint:to_waypoint as stable key.
function segKey(seg: RouteSegment): string {
  return `${seg.from_waypoint}:${seg.to_waypoint}`
}

// ─── needsMigration ───────────────────────────────────────────────────────────

/**
 * Returns true if the route needs pre-T0 migration, i.e.:
 * - route_schema_version < 1 (explicit schema gate), OR
 * - any segment lacks an elevation_profile (defensive check for partial saves).
 *
 * A fresh T0 route (version=1, all segments have profiles) returns false and
 * loadRoute skips the migration path entirely.
 */
export function needsMigration(route: PlannedRoute): boolean {
  return (
    (route.route_schema_version ?? 0) < 1 ||
    route.segments.some(seg => !seg.elevation_profile || seg.elevation_profile.length === 0)
  )
}

// ─── buildProfileFromCoordZ ───────────────────────────────────────────────────
// Extracts elevation from inline-Z of GeoJSON coordinates (coord[2]).
// Returns [] when fewer than 2 valid Z-values are available.
function buildProfileFromCoordZ(coords: number[][]): RouteElevationPoint[] {
  const out: RouteElevationPoint[] = []
  let cum = 0
  for (let i = 0; i < coords.length; i++) {
    if (i > 0) {
      const [lon1, lat1] = coords[i - 1]
      const [lon2, lat2] = coords[i]
      cum += haversineM(lat1, lon1, lat2, lon2)
    }
    const c = coords[i]
    if (c.length >= 3 && typeof c[2] === 'number' && Number.isFinite(c[2])) {
      out.push({ dist_m: cum, ele_m: c[2] })
    }
  }
  return out.length >= 2 ? out : []
}

// ─── fillSegmentsViaConcatHop ─────────────────────────────────────────────────

/**
 * Fills a batch of segments that all lack elevation_profile via a single
 * DEM call (or chunked calls for very large routes) using concat-hop:
 *
 * 1. Concatenate all segment geometries into a single LineString.
 *    Junction-dedup: skip the first coord of each non-first segment
 *    (it equals the last coord of the previous segment).
 *    Convention (Spec Critic-Runde-2 #33): junction vertex lands in the
 *    EARLIER segment, dropped from the LATER.
 *
 * 2. Call getElevationAtVerticesChunked once on the combined LineString.
 *    (The function handles intra-segment chunking for > 50 000 vertices.)
 *
 * 3. Re-map the returned profile back to each segment via vertex-index ranges
 *    recorded before the call. Distances are normalised to start at 0 per segment
 *    (buildCombinedProfileFromSegments adds the cumulative offset later).
 *
 * Returns a Map keyed by segKey(seg) with updated segments.
 */
async function fillSegmentsViaConcatHop(
  segs: RouteSegment[],
): Promise<Map<string, RouteSegment>> {
  const allCoords: number[][] = []
  const segVertexRanges: { key: string; seg: RouteSegment; start: number; end: number }[] = []

  for (const seg of segs) {
    if (!seg.geometry) {
      // Segment without geometry — cannot sample DEM. Keep as-is.
      segVertexRanges.push({ key: segKey(seg), seg, start: allCoords.length, end: allCoords.length })
      continue
    }
    const coords = seg.geometry.coordinates as number[][]
    // No cross-segment junction-dedup here: segs is the stillNeedsDem batch, which
    // may be non-contiguous (e.g. only positions 1, 2, 4 of the original route).
    // Deduping across batch-index boundaries would drop junction vertices from
    // physically non-adjacent segments, corrupting per-segment vertex ranges and
    // breaking buildProfileToCoordMap's 1:1 assumption. Each segment is sampled
    // independently at its full geometry.
    const startIdx = allCoords.length
    for (let j = 0; j < coords.length; j++) {
      allCoords.push(coords[j])
    }
    segVertexRanges.push({ key: segKey(seg), seg, start: startIdx, end: allCoords.length })
  }

  if (allCoords.length < 2) {
    // Not enough coords for DEM lookup — return segments unchanged.
    const out = new Map<string, RouteSegment>()
    for (const r of segVertexRanges) out.set(r.key, r.seg)
    return out
  }

  const combinedLine: GeoJSON.LineString = { type: 'LineString', coordinates: allCoords }
  // addToast=null: suppress DEM-coverage toast in migration context (batch context,
  // not directly user-triggered). The existing lazy-fill path handles user-visible toasts.
  const allProfile = await getElevationAtVerticesChunked(combinedLine, null)

  const out = new Map<string, RouteSegment>()
  for (const range of segVertexRanges) {
    if (range.start === range.end) {
      // No geometry — keep unchanged.
      out.set(range.key, range.seg)
      continue
    }
    const segProfile = allProfile.slice(range.start, range.end)
    if (segProfile.length === 0) {
      out.set(range.key, range.seg)
      continue
    }
    // Normalise dist_m to start at 0 for this segment.
    // buildCombinedProfileFromSegments adds cumulative offset during rendering.
    const baseDist = segProfile[0].dist_m
    const normalised = segProfile.map(p => ({ dist_m: p.dist_m - baseDist, ele_m: p.ele_m }))
    out.set(range.key, { ...range.seg, elevation_profile: normalised })
  }
  return out
}

// ─── Per-segment fallback hierarchy ──────────────────────────────────────────

/**
 * Attempt to fill a single segment's elevation_profile using the 3-stage
 * fallback hierarchy (Spec §E.3.4 / Critic-Runde-2 #5):
 *
 * Stage 1: buildProfileFromCoordZ — use inline-Z from geometry coords.
 * Stage 2: legacyToRouteProfile — top-level fraction profile (BRouter fallback).
 * Stage 3: DEM concat-hop (deferred to fillSegmentsViaConcatHop caller).
 *
 * Returns the segment with elevation_profile set when a stage succeeds,
 * or the segment unchanged when no stage produced a result (Stage 3 not tried here).
 */
function tryFillSegmentFromLocalSources(
  seg: RouteSegment,
  topLevelProfile: Array<{ fraction: number; elevation: number | null }> | null,
  totalDistM: number,
): RouteSegment {
  const coords = seg.geometry?.coordinates as number[][] | undefined

  // Stage 1: inline-Z
  if (coords && coords.length >= 2) {
    const fromZ = buildProfileFromCoordZ(coords)
    if (fromZ.length >= 2) {
      return { ...seg, elevation_profile: fromZ }
    }
  }

  // Stage 2: top-level fraction profile (valid for single-segment routes only,
  // where distance attribution is unambiguous).
  if (topLevelProfile && topLevelProfile.length >= 2 && totalDistM > 0) {
    const converted = legacyToRouteProfile(topLevelProfile, totalDistM)
    const nonNull = converted.filter(p => p.ele_m !== null)
    if (nonNull.length >= 2) {
      // Guard: if the first converted point has dist_m > 0 (BRouter routes whose
      // fraction axis starts above 0), prepend a synthetic origin point so that
      // the profile satisfies the T0 form requirement (first point at dist_m=0).
      // Without this, IsT0FormCompliant rejects the profile and the route stays
      // at version=0 indefinitely → silent migration loop.
      const profile: RouteElevationPoint[] =
        converted[0].dist_m > 0
          ? [{ dist_m: 0, ele_m: converted[0].ele_m }, ...converted]
          : converted
      return { ...seg, elevation_profile: profile }
    }
  }

  // Stage 3 is handled in the caller (DEM concat-hop).
  return seg
}

// ─── migrateRoute ─────────────────────────────────────────────────────────────

/**
 * Fills all segments that lack elevation_profile using the 3-stage hierarchy.
 * Returns a new array with those segments replaced (remaining segments
 * are returned as-is — reference-equal to the input). Pure/side-effect-free.
 *
 * Callers should use migrateRouteCached to avoid re-running DEM hops on
 * every tab-switch for the same route.
 */
export async function migrateRoute(route: PlannedRoute): Promise<RouteSegment[]> {
  const segsNeedingFill = route.segments.filter(
    s => !s.elevation_profile || s.elevation_profile.length === 0,
  )

  if (segsNeedingFill.length === 0) return route.segments

  // Top-level profile context for Stage 2 (single-segment routes only).
  // The top-level elevation_profile of a route spans 0..totalDistM of the full
  // route. For multi-segment routes there is no per-segment slice of it —
  // applying it wholesale to each gap-segment would give every segment the
  // same full-route profile curve. Stage 2 is therefore only valid for
  // single-segment routes where distance attribution is unambiguous.
  const isSingleSegment = route.segments.length === 1
  const topLevel = isSingleSegment
    ? ((route.elevation_profile ?? null) as Array<{ fraction: number; elevation: number | null }> | null)
    : null
  const totalDistM = route.segments.reduce((acc, s) => acc + (s.distance_m ?? 0), 0)

  // Stage 1+2: try local sources first. Collect remaining (no-profile) for DEM.
  let stageFilledMap = new Map<string, RouteSegment>()
  const stillNeedsDem: RouteSegment[] = []

  for (const seg of segsNeedingFill) {
    const filled = tryFillSegmentFromLocalSources(seg, topLevel, totalDistM)
    if (filled.elevation_profile && filled.elevation_profile.length >= 2) {
      stageFilledMap.set(segKey(filled), filled)
    } else {
      stillNeedsDem.push(seg)
    }
  }

  // Stage 3: DEM concat-hop for remaining segments.
  let demFilledMap = new Map<string, RouteSegment>()
  if (stillNeedsDem.length > 0) {
    demFilledMap = await fillSegmentsViaConcatHop(stillNeedsDem)
  }

  // Merge: original segments array, with filled replacements from both stages.
  return route.segments.map(seg => {
    const key = segKey(seg)
    return stageFilledMap.get(key) ?? demFilledMap.get(key) ?? seg
  })
}

// ─── Session cache ────────────────────────────────────────────────────────────
// Prevents re-running the DEM hop on every tab-switch for the same route.
// Key = `${id}:${updated_at}` — ties the cache to the server version.
// Per-tab (module-level Map), never shared across users (single-tab lifecycle).

const sessionMigratedCache = new Map<string, RouteSegment[]>()

/**
 * Session-cached wrapper around migrateRoute.
 * Second call for the same `${id}:${updated_at}` key returns the cached result
 * without a DEM hop.
 */
export async function migrateRouteCached(route: PlannedRoute): Promise<RouteSegment[]> {
  const key = `${route.id}:${route.updated_at}`
  const cached = sessionMigratedCache.get(key)
  if (cached !== undefined) return cached
  const result = await migrateRoute(route)
  sessionMigratedCache.set(key, result)
  return result
}

/**
 * Clears the session migration cache.
 *
 * Called on logout to prevent cross-user cache leakage: when User A logs out
 * and User B logs in within the same tab, the cache must be flushed so User B
 * never sees results derived from User A's route data.
 *
 * Also used in unit tests (via beforeEach) — the production logout path and
 * the test reset path share this function intentionally.
 */
export function clearMigrationCache(): void {
  sessionMigratedCache.clear()
}

// ─── Logout-event subscriber ──────────────────────────────────────────────────
// Multi-user safety (CLAUDE.md): clear the module-level cache when the user
// logs out. Without this, User B inherits User A's cached segment profiles
// in the same browser tab.
if (typeof BroadcastChannel !== 'undefined') {
  const _authChannel = new BroadcastChannel('skadia-auth')
  _authChannel.addEventListener('message', (e: MessageEvent) => {
    if (e.data?.type === 'logout') {
      clearMigrationCache()
    }
  })
}
