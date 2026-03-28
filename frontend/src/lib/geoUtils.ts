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
