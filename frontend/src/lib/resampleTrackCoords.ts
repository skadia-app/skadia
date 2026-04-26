/**
 * Resampling helper that mirrors the backend's resampleStreamPoints in
 * services/api/internal/enrichment/surfaces_graph.go.
 *
 * Exported as a standalone pure function so that it can be tested in
 * isolation and eventually imported by TrackMap.tsx when the R1-B API
 * wave ships resample_spacing_m in the response.
 */

/** Approximate metres per degree of latitude (≈ geo.MetersPerDegreeLat). */
const M_PER_DEG_LAT = 111320

/**
 * Compute cumulative equirectangular distances for an array of [lng, lat] coordinates.
 * Uses per-segment cos(lat) at the *start* of each segment, identical to the backend
 * equirectDistMeters implementation, to eliminate N–S drift on long tracks.
 *
 * @returns Array of cumulative distances in metres (same length as coords).
 */
export function cumulativeDistanceM(coords: ReadonlyArray<readonly [number, number]>): number[] {
  if (coords.length === 0) return []
  const cum = new Array<number>(coords.length)
  cum[0] = 0
  for (let i = 1; i < coords.length; i++) {
    const lat1Rad = (coords[i - 1][1] * Math.PI) / 180
    const cosLat = Math.cos(lat1Rad)
    const dLat = (coords[i][1] - coords[i - 1][1]) * M_PER_DEG_LAT
    const dLng = (coords[i][0] - coords[i - 1][0]) * cosLat * M_PER_DEG_LAT
    cum[i] = cum[i - 1] + Math.sqrt(dLat * dLat + dLng * dLng)
  }
  return cum
}

/**
 * Resample a [lng, lat] coordinate array to a uniform spacing of `spacingM` metres.
 * Mirrors the backend resampleStreamPoints algorithm.
 *
 * Input array MUST be pre-filtered to GPS-valid points only (matches the backend
 * `EnrichmentStreamPoint{Lat,Lng != nil}` filter in
 * services/api/internal/enrichment/surfaces_graph.go). Passing arrays with
 * synthetic/zero points (e.g. pause gaps materialised as [0, 0]) will misalign
 * the range indices returned by the API: cumulative-distance arithmetic happily
 * spans the gap, so output index N here will not correspond to backend index N.
 *
 * @param coords   Input coordinates as [lng, lat] pairs (MapLibre convention),
 *                 pre-filtered to GPS-valid points (no nulls, no [0,0] sentinels).
 * @param spacingM Target spacing between output points in metres. Defaults to 10.
 * @returns Resampled coordinate array, or empty array if input has fewer than 2 points
 *          or total length < 1 µm.
 */
export function resampleStreamCoords(
  coords: ReadonlyArray<readonly [number, number]>,
  spacingM = 10,
): [number, number][] {
  if (coords.length < 2) return []
  const cum = cumulativeDistanceM(coords)
  const total = cum[cum.length - 1]
  if (total < 1e-6) return []
  const numPts = Math.max(2, Math.floor(total / spacingM) + 1)
  const out: [number, number][] = new Array(numPts)
  let j = 0
  for (let i = 0; i < numPts; i++) {
    const target = (i * total) / (numPts - 1)
    if (target >= total) {
      out[i] = [coords[coords.length - 1][0], coords[coords.length - 1][1]]
      continue
    }
    while (j < coords.length - 2 && cum[j + 1] < target) j++
    const segLen = cum[j + 1] - cum[j]
    const t = segLen > 1e-10 ? (target - cum[j]) / segLen : 0
    out[i] = [
      coords[j][0] + t * (coords[j + 1][0] - coords[j][0]),
      coords[j][1] + t * (coords[j + 1][1] - coords[j][1]),
    ]
  }
  return out
}
