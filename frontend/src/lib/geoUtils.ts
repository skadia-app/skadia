import type { Tag } from '@/api/tags'

// ── Haversine-Distanz in Metern ───────────────────────────────────────────
export function haversineM(lat1: number, lng1: number, lat2: number, lng2: number): number {
  const R = 6_371_000
  const dLat = (lat2 - lat1) * (Math.PI / 180)
  const dLng = (lng2 - lng1) * (Math.PI / 180)
  const a = Math.sin(dLat / 2) ** 2 + Math.cos(lat1 * (Math.PI / 180)) * Math.cos(lat2 * (Math.PI / 180)) * Math.sin(dLng / 2) ** 2
  return R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a))
}

export function findNearbyLocationTag(lat: number, lng: number, tags: Tag[], thresholdM: number): Tag | null {
  let nearest: Tag | null = null
  let nearestDist = Infinity
  for (const tag of tags) {
    if (tag.lat == null || tag.lng == null) continue
    const d = haversineM(lat, lng, tag.lat, tag.lng)
    if (d <= thresholdM && d < nearestDist) { nearest = tag; nearestDist = d }
  }
  return nearest
}

// ── Geo shape helpers ─────────────────────────────────────────────────────

/** Approximate circle as a 64-vertex GeoJSON Polygon */
export function circlePolygon(
  center: [number, number],
  radiusM: number,
  steps = 64,
): GeoJSON.Feature<GeoJSON.Polygon> {
  const coords: [number, number][] = []
  const [lng, lat] = center
  for (let i = 0; i <= steps; i++) {
    const angle = (i / steps) * 2 * Math.PI
    const dx = radiusM * Math.cos(angle)
    const dy = radiusM * Math.sin(angle)
    const dlat = dy / 111320
    const dlng = dx / (111320 * Math.cos((lat * Math.PI) / 180))
    coords.push([lng + dlng, lat + dlat])
  }
  return {
    type: 'Feature',
    properties: {},
    geometry: { type: 'Polygon', coordinates: [coords] },
  }
}

/** Format metres as "200 m" or "1.5 km" */
export function formatRadius(m: number): string {
  if (m >= 1000) return `${(m / 1000).toFixed(1).replace('.0', '')} km`
  return `${m} m`
}

/** Clamp a point to within radiusM of center (0.95 shrink factor) */
export function clampToCircle(
  point: [number, number],
  center: [number, number],
  radiusM: number,
): [number, number] {
  const [plng, plat] = point
  const [clng, clat] = center
  const dx = (plng - clng) * 111320 * Math.cos((clat * Math.PI) / 180)
  const dy = (plat - clat) * 111320
  const dist = Math.sqrt(dx * dx + dy * dy)
  if (dist <= radiusM) return point
  const scale = (radiusM * 0.95) / dist
  return [clng + (dx * scale) / (111320 * Math.cos((clat * Math.PI) / 180)), clat + (dy * scale) / 111320]
}

/** Rough point-in-polygon (ray casting) */
export function pointInPolygon(pt: [number, number], ring: [number, number][]): boolean {
  let inside = false
  const [x, y] = pt
  for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
    const [xi, yi] = ring[i]
    const [xj, yj] = ring[j]
    const intersect =
      yi > y !== yj > y && x < ((xj - xi) * (y - yi)) / (yj - yi) + xi
    if (intersect) inside = !inside
  }
  return inside
}

/** Centroid of a point array */
export function centroid(points: [number, number][]): [number, number] {
  const sum = points.reduce(([ax, ay], [x, y]) => [ax + x, ay + y], [0, 0])
  return [sum[0] / points.length, sum[1] / points.length]
}
