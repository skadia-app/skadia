import { describe, it, expect } from 'vitest'
import { catmullRom, nearestPointOnPolyline, polylineDistanceM } from './catmull-rom'

describe('catmullRom', () => {
  it('returns a straight line for 2 points', () => {
    const result = catmullRom([[0, 0], [1, 1]], 10)
    // First point should be [0, 0], last should be [1, 1]
    expect(result[0][0]).toBeCloseTo(0, 5)
    expect(result[0][1]).toBeCloseTo(0, 5)
    expect(result[result.length - 1][0]).toBeCloseTo(1, 5)
    expect(result[result.length - 1][1]).toBeCloseTo(1, 5)
    // All points should lie on the line y = x
    for (const [x, y] of result) {
      expect(x).toBeCloseTo(y, 5)
    }
  })

  it('returns a smooth curve through all 4 control points', () => {
    const input: [number, number][] = [[0, 0], [1, 2], [3, 3], [4, 0]]
    const result = catmullRom(input, 20)
    // Should have many interpolated points
    expect(result.length).toBeGreaterThan(input.length)
    // The spline must pass through each original control point
    for (const cp of input) {
      const closest = result.reduce((best, pt) => {
        const d = Math.hypot(pt[0] - cp[0], pt[1] - cp[1])
        return d < best.d ? { d, pt } : best
      }, { d: Infinity, pt: result[0] })
      expect(closest.d).toBeLessThan(0.01)
    }
  })

  it('returns a copy of input for 0 or 1 points', () => {
    expect(catmullRom([])).toEqual([])
    expect(catmullRom([[5, 10]])).toEqual([[5, 10]])
  })
})

describe('nearestPointOnPolyline', () => {
  it('finds the projection on a segment', () => {
    // Horizontal line from (0,0) to (10,0)
    const points: [number, number][] = [[0, 0], [10, 0]]
    const target: [number, number] = [5, 3]
    const result = nearestPointOnPolyline(target, points)
    expect(result.insertAfter).toBe(0)
    expect(result.t).toBeCloseTo(0.5, 5)
    expect(result.point[0]).toBeCloseTo(5, 5)
    expect(result.point[1]).toBeCloseTo(0, 5)
  })

  it('clamps to segment endpoints', () => {
    const points: [number, number][] = [[0, 0], [10, 0]]
    // Target far beyond the end
    const result = nearestPointOnPolyline([20, 0], points)
    expect(result.t).toBeCloseTo(1, 5)
    expect(result.point[0]).toBeCloseTo(10, 5)
  })

  it('finds the correct segment in a multi-segment polyline', () => {
    const points: [number, number][] = [[0, 0], [10, 0], [10, 10]]
    const target: [number, number] = [10, 5]
    const result = nearestPointOnPolyline(target, points)
    expect(result.insertAfter).toBe(1) // second segment
    expect(result.point[0]).toBeCloseTo(10, 5)
    expect(result.point[1]).toBeCloseTo(5, 5)
  })
})

describe('polylineDistanceM', () => {
  it('computes ~111 km between 1 degree latitude', () => {
    // 1 degree of latitude at the equator ≈ 111.195 km
    const points: [number, number][] = [[0, 0], [1, 0]]
    const dist = polylineDistanceM(points)
    expect(dist).toBeGreaterThan(110_000)
    expect(dist).toBeLessThan(112_000)
  })

  it('returns 0 for empty or single-point polyline', () => {
    expect(polylineDistanceM([])).toBe(0)
    expect(polylineDistanceM([[47, 11]])).toBe(0)
  })

  it('sums multi-segment distances', () => {
    const points: [number, number][] = [[0, 0], [1, 0], [2, 0]]
    const dist = polylineDistanceM(points)
    // Should be ~222 km (2 degrees)
    expect(dist).toBeGreaterThan(220_000)
    expect(dist).toBeLessThan(224_000)
  })
})
