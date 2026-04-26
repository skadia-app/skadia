import type GeoJSON from 'geojson'
import { haversineM } from '@/lib/geoUtils'
import { getElevationProfile, type ElevationPoint } from './routes'
import type { RouteElevationPoint } from './routes'
import type { Toast } from '@/lib/toast'

export const MAX_VERTICES_PER_CALL = 50_000

// Module-level dedup flag: triggers at most one DEM-coverage warning per
// browser-tab lifetime (Spec §6 Class B). No React state needed — this is
// a pure utility module.
let demCoverageWarnedThisSession = false

/**
 * Resets the DEM coverage warned flag. Exported for unit-testing only — do
 * not call in application code.
 * @internal
 */
export function __resetDemCoverageWarnedForTesting(): void {
  demCoverageWarnedThisSession = false
}

/**
 * Samples DEM elevation at every vertex of `line`.
 *
 * - Single API call when line.coordinates.length <= MAX_VERTICES_PER_CALL.
 * - For larger LineStrings: splits into overlapping chunks (1-vertex overlap
 *   at each boundary), calls sequentially, and reassembles with junction-dedup.
 * - Sequential calls guard against burst load on the DEM service.
 * - Returns RouteElevationPoint[] (dist_m, ele_m). Distances are derived from
 *   the input geometry coordinates via Haversine — not from the fraction field
 *   of the API response — so chunking is transparent to callers.
 * - If the entire result has all ele_m === null, triggers one info-toast per
 *   session via the injected addToast callback (Spec §6 Class B).
 *   Pass null to suppress (e.g. tests, background contexts without a provider).
 */
export async function getElevationAtVerticesChunked(
  line: GeoJSON.LineString,
  addToast: ((toast: Omit<Toast, 'id'>) => unknown) | null = null,
): Promise<RouteElevationPoint[]> {
  const vertexCount = line.coordinates.length

  let points: RouteElevationPoint[]

  if (vertexCount <= MAX_VERTICES_PER_CALL) {
    const { profile } = await getElevationProfile(line, { atVertices: true })
    points = fractionProfileToRoutePoints(profile, line.coordinates)
  } else {
    // Intra-segment chunking: 1-vertex overlap per chunk for junction-dedup.
    const chunks = splitLineStringAtVertices(line, MAX_VERTICES_PER_CALL)
    const rawProfiles: ElevationPoint[][] = []
    for (const chunk of chunks) {
      const { profile } = await getElevationProfile(chunk, { atVertices: true })
      rawProfiles.push(profile)
    }
    points = concatProfilesWithDedup(rawProfiles, chunks)
  }

  // Spec §6 Class B: warn once per session when DEM has zero data for this area.
  if (
    addToast !== null &&
    !demCoverageWarnedThisSession &&
    points.length > 0 &&
    points.every(p => p.ele_m === null)
  ) {
    demCoverageWarnedThisSession = true
    addToast({
      type: 'info',
      title: 'Höhendaten nicht verfügbar',
      message: 'Für diese Region sind keine Höhendaten vorhanden.',
      group: 'dem-coverage',
      duration: 8000,
    })
  }

  return points
}

/**
 * Splits a LineString into overlapping chunks of at most maxVertices vertices.
 * Adjacent chunks share exactly one vertex at the boundary (1-vertex overlap)
 * so that junction-dedup in concatProfilesWithDedup can drop the duplicate
 * without losing elevation coverage.
 */
export function splitLineStringAtVertices(
  line: GeoJSON.LineString,
  maxVertices: number,
): GeoJSON.LineString[] {
  const chunks: GeoJSON.LineString[] = []
  const stride = maxVertices - 1 // overlap of 1 vertex per chunk
  for (let start = 0; start < line.coordinates.length - 1; start += stride) {
    const end = Math.min(start + maxVertices, line.coordinates.length)
    chunks.push({ type: 'LineString', coordinates: line.coordinates.slice(start, end) })
  }
  return chunks
}

/**
 * Reassembles per-chunk ElevationPoint[] arrays into a single RouteElevationPoint[]
 * with cumulative dist_m and junction-dedup.
 *
 * Junction-dedup rule: the first vertex of each non-first chunk is identical to
 * the last vertex of the previous chunk (1-vertex overlap from splitLineStringAtVertices).
 * That overlap point is dropped from the output.
 *
 * dist_m is computed from the chunk coordinates (Haversine) — not from the
 * API's fraction field — so the accumulator stays accurate across chunk boundaries.
 *
 * Accumulation:
 *   chunk[0]:   startIdx=0, points get dist_m = cumDist[j].
 *               After chunk: distAccum = cumDist[last].
 *   chunk[i>0]: startIdx=1 (overlap vertex dropped).
 *               Point j gets dist_m = distAccum + cumDist[j].
 *               cumDist[0]=0 (the overlap vertex), cumDist[1] = dist to next
 *               vertex within this chunk. distAccum already represents the
 *               global position of that overlap vertex.
 *               After chunk: distAccum += cumDist[last].
 */
export function concatProfilesWithDedup(
  profiles: ElevationPoint[][],
  chunks: GeoJSON.LineString[],
): RouteElevationPoint[] {
  const out: RouteElevationPoint[] = []
  let distAccum = 0

  for (const [i, p] of profiles.entries()) {
    if (p.length === 0) continue

    const chunkCoords = chunks[i].coordinates
    const startIdx = i > 0 ? 1 : 0

    // Cumulative Haversine distances within this chunk (index 0 always = 0).
    const cumDist = computeCumulativeDistancesFromCoords(chunkCoords)

    for (let j = startIdx; j < p.length; j++) {
      out.push({
        dist_m: distAccum + cumDist[j],
        ele_m: p[j].elevation,
      })
    }

    // Advance global accumulator to the end of this chunk.
    distAccum += cumDist[chunkCoords.length - 1]
  }

  return out
}

/**
 * Converts a single ElevationPoint[] (from one non-chunked API call) into
 * RouteElevationPoint[] with Haversine-based cumulative distances derived
 * from the input coordinates.
 */
function fractionProfileToRoutePoints(
  profile: ElevationPoint[],
  coords: GeoJSON.Position[],
): RouteElevationPoint[] {
  if (profile.length === 0) return []
  const cumDist = computeCumulativeDistancesFromCoords(coords)
  return profile.map((p, i) => ({
    dist_m: cumDist[i] ?? 0,
    ele_m: p.elevation,
  }))
}

/**
 * Builds a cumulative-distance array (metres) from GeoJSON [lng, lat] coordinate
 * pairs. Index 0 is always 0.
 */
export function computeCumulativeDistancesFromCoords(coords: GeoJSON.Position[]): number[] {
  const d = new Array<number>(coords.length).fill(0)
  for (let i = 1; i < coords.length; i++) {
    // GeoJSON Position is [lng, lat, ?ele]
    d[i] = d[i - 1] + haversineM(coords[i - 1][1], coords[i - 1][0], coords[i][1], coords[i][0])
  }
  return d
}
