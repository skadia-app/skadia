// Haversine distance in meters
function haversineM(lat1: number, lng1: number, lat2: number, lng2: number): number {
  const R = 6371000
  const dLat = (lat2 - lat1) * Math.PI / 180
  const dLng = (lng2 - lng1) * Math.PI / 180
  const a = Math.sin(dLat/2)**2 +
    Math.cos(lat1 * Math.PI/180) * Math.cos(lat2 * Math.PI/180) * Math.sin(dLng/2)**2
  return R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1-a))
}

export function nearestIndex(
  points: Array<{ lat: number; lng: number }>,
  clickLat: number,
  clickLng: number,
): number {
  let best = 0
  let bestDist = Infinity
  for (let i = 0; i < points.length; i++) {
    const d = haversineM(points[i].lat, points[i].lng, clickLat, clickLng)
    if (d < bestDist) { bestDist = d; best = i }
  }
  return best
}

export function extractSubTrack(
  points: Array<{ lat: number; lng: number }>,
  startIdx: number,
  endIdx: number,
): string {
  const coords = points.slice(startIdx, endIdx + 1).map(p => [p.lng, p.lat])
  return JSON.stringify({ type: 'LineString', coordinates: coords })
}

export function detectLapCrossings(
  points: Array<{ lat: number; lng: number }>,
  finishIdx: number,
  proximityM = 30,
  minGapM = 100,
): number[] {
  const finish = points[finishIdx]
  const near = points.reduce<number[]>((acc, p, i) => {
    if (haversineM(p.lat, p.lng, finish.lat, finish.lng) <= proximityM) acc.push(i)
    return acc
  }, [])
  if (near.length < 2) return []

  const crossings: number[] = [near[0]]
  for (const idx of near.slice(1)) {
    const last = crossings[crossings.length - 1]
    let dist = 0
    for (let i = last; i < idx; i++) {
      dist += haversineM(points[i].lat, points[i].lng, points[i+1].lat, points[i+1].lng)
    }
    if (dist >= minGapM) crossings.push(idx)
  }
  return crossings
}
