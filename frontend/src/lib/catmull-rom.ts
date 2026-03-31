/**
 * Catmull-Rom spline interpolation and polyline geometry utilities.
 *
 * Extracted from OffroadEditor.tsx for reuse in useRoutePlanner and useRouteEditor.
 */

// ── Catmull-Rom Spline ──────────────────────────────────────────────────────

/**
 * Compute a dense Catmull-Rom spline through the given control points.
 * Returns approximately `samplesPerSegment` interpolated points per span.
 * Phantom points are added at both ends to ensure the spline passes through
 * the first and last control point.
 *
 * Points are [lat, lng] pairs (internal convention).
 */
export function catmullRom(
  points: [number, number][],
  samplesPerSegment = 20,
): [number, number][] {
  if (points.length < 2) return [...points]

  if (points.length === 2) {
    // Straight line — no curve needed
    const result: [number, number][] = []
    for (let t = 0; t <= 1; t += 1 / samplesPerSegment) {
      result.push([
        points[0][0] + (points[1][0] - points[0][0]) * t,
        points[0][1] + (points[1][1] - points[0][1]) * t,
      ])
    }
    result.push(points[points.length - 1])
    return result
  }

  // Extend with phantom points (reflection of first/last segment)
  const pts: [number, number][] = [
    [2 * points[0][0] - points[1][0], 2 * points[0][1] - points[1][1]],
    ...points,
    [
      2 * points[points.length - 1][0] - points[points.length - 2][0],
      2 * points[points.length - 1][1] - points[points.length - 2][1],
    ],
  ]

  const result: [number, number][] = []
  const alpha = 0.5 // tension

  for (let i = 1; i < pts.length - 2; i++) {
    const p0 = pts[i - 1]
    const p1 = pts[i]
    const p2 = pts[i + 1]
    const p3 = pts[i + 2]

    for (let s = 0; s <= samplesPerSegment; s++) {
      const t = s / samplesPerSegment
      const t2 = t * t
      const t3 = t2 * t

      const x =
        alpha * (
          (2 * p1[0]) +
          (-p0[0] + p2[0]) * t +
          (2 * p0[0] - 5 * p1[0] + 4 * p2[0] - p3[0]) * t2 +
          (-p0[0] + 3 * p1[0] - 3 * p2[0] + p3[0]) * t3
        )
      const y =
        alpha * (
          (2 * p1[1]) +
          (-p0[1] + p2[1]) * t +
          (2 * p0[1] - 5 * p1[1] + 4 * p2[1] - p3[1]) * t2 +
          (-p0[1] + 3 * p1[1] - 3 * p2[1] + p3[1]) * t3
        )

      if (s === 0 && result.length > 0) continue // avoid duplicate join points
      result.push([x, y])
    }
  }

  return result
}

// ── Nearest point on polyline ───────────────────────────────────────────────

export interface NearestPointResult {
  /** Index of the segment start point (insert after this index). */
  insertAfter: number
  /** Parametric position along the segment [0, 1]. */
  t: number
  /** The projected point on the segment. */
  point: [number, number]
}

/**
 * Find the nearest point on a polyline to a target point.
 * Works in flat coordinate space (suitable for small-scale map interactions).
 */
export function nearestPointOnPolyline(
  target: [number, number],
  points: [number, number][],
): NearestPointResult {
  let bestDist = Infinity
  let bestIdx = 0
  let bestT = 0

  for (let i = 0; i < points.length - 1; i++) {
    const ax = points[i][0], ay = points[i][1]
    const bx = points[i + 1][0], by = points[i + 1][1]
    const dx = bx - ax, dy = by - ay
    const lenSq = dx * dx + dy * dy
    let t = lenSq === 0 ? 0 : ((target[0] - ax) * dx + (target[1] - ay) * dy) / lenSq
    t = Math.max(0, Math.min(1, t))
    const px = ax + t * dx - target[0]
    const py = ay + t * dy - target[1]
    const dist = px * px + py * py
    if (dist < bestDist) {
      bestDist = dist
      bestIdx = i
      bestT = t
    }
  }

  // Compute the actual projected point
  const a = points[bestIdx]
  const b = points[Math.min(bestIdx + 1, points.length - 1)]
  const projectedPoint: [number, number] = [
    a[0] + bestT * (b[0] - a[0]),
    a[1] + bestT * (b[1] - a[1]),
  ]

  return { insertAfter: bestIdx, t: bestT, point: projectedPoint }
}

// ── Polyline distance (Haversine) ───────────────────────────────────────────

const EARTH_RADIUS_M = 6371000

function haversineM(a: [number, number], b: [number, number]): number {
  const dLat = (b[0] - a[0]) * Math.PI / 180
  const dLng = (b[1] - a[1]) * Math.PI / 180
  const sinDLat = Math.sin(dLat / 2)
  const sinDLng = Math.sin(dLng / 2)
  const h = sinDLat * sinDLat +
    Math.cos(a[0] * Math.PI / 180) * Math.cos(b[0] * Math.PI / 180) * sinDLng * sinDLng
  return 2 * EARTH_RADIUS_M * Math.asin(Math.sqrt(h))
}

/**
 * Total Haversine distance along a polyline.
 * Points are [lat, lng] pairs.
 */
export function polylineDistanceM(points: [number, number][]): number {
  let total = 0
  for (let i = 1; i < points.length; i++) {
    total += haversineM(points[i - 1], points[i])
  }
  return total
}
