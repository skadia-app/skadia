import { describe, it, expect } from 'vitest'
import { cumulativeDistanceM, resampleStreamCoords } from '../resampleTrackCoords'

/** Reference: equirectangular distance between two points (backend formula). */
function equirectDistMeters(lat1: number, lng1: number, lat2: number, lng2: number): number {
  const M_PER_DEG = 111320
  const cosLat = Math.cos((lat1 * Math.PI) / 180)
  const dLat = (lat2 - lat1) * M_PER_DEG
  const dLng = (lng2 - lng1) * cosLat * M_PER_DEG
  return Math.sqrt(dLat * dLat + dLng * dLng)
}

// ── cumulativeDistanceM ───────────────────────────────────────────────────────

describe('cumulativeDistanceM', () => {
  it('empty input → empty output', () => {
    expect(cumulativeDistanceM([])).toEqual([])
  })

  it('single point → [0]', () => {
    expect(cumulativeDistanceM([[13.0, 47.0]])).toEqual([0])
  })

  it('two points — matches backend equirectDistMeters per-segment cos(lat)', () => {
    const a: [number, number] = [13.0, 47.0] // [lng, lat]
    const b: [number, number] = [13.0, 47.1] // ~11 km north
    const cum = cumulativeDistanceM([a, b])
    expect(cum[0]).toBe(0)
    const expected = equirectDistMeters(a[1], a[0], b[1], b[0])
    expect(cum[1]).toBeCloseTo(expected, 3)
  })

  it('three points — cumulative, per-segment cos(lat)', () => {
    const pts: [number, number][] = [
      [13.0, 47.0],
      [13.0, 47.1],
      [13.1, 47.1],
    ]
    const cum = cumulativeDistanceM(pts)
    const d01 = equirectDistMeters(pts[0][1], pts[0][0], pts[1][1], pts[1][0])
    const d12 = equirectDistMeters(pts[1][1], pts[1][0], pts[2][1], pts[2][0])
    expect(cum[0]).toBe(0)
    expect(cum[1]).toBeCloseTo(d01, 3)
    expect(cum[2]).toBeCloseTo(d01 + d12, 3)
  })
})

// ── resampleStreamCoords ──────────────────────────────────────────────────────

describe('resampleStreamCoords', () => {
  it('empty input → empty output', () => {
    expect(resampleStreamCoords([])).toEqual([])
  })

  it('single point → empty output (need at least 2)', () => {
    expect(resampleStreamCoords([[13.0, 47.0]])).toEqual([])
  })

  it('two identical points (zero length) → empty output', () => {
    const p: [number, number] = [13.0, 47.0]
    expect(resampleStreamCoords([p, p])).toEqual([])
  })

  it('output starts at first coord and ends at last coord', () => {
    const pts: [number, number][] = [
      [13.0, 47.0],
      [13.1, 47.0], // ~7.5 km east
    ]
    const result = resampleStreamCoords(pts)
    expect(result.length).toBeGreaterThanOrEqual(2)
    // First and last output points match the input endpoints
    expect(result[0][0]).toBeCloseTo(pts[0][0], 6)
    expect(result[0][1]).toBeCloseTo(pts[0][1], 6)
    expect(result[result.length - 1][0]).toBeCloseTo(pts[pts.length - 1][0], 4)
    expect(result[result.length - 1][1]).toBeCloseTo(pts[pts.length - 1][1], 4)
  })

  it('spacing between consecutive resampled points is ≈ spacingM', () => {
    const pts: [number, number][] = [
      [13.0, 47.0],
      [13.0, 47.5], // ~55 km north — produces many resampled points
    ]
    const SPACING = 10
    const result = resampleStreamCoords(pts, SPACING)
    // Consecutive output points should be ~SPACING metres apart
    const cum = cumulativeDistanceM(result)
    for (let i = 1; i < cum.length - 1; i++) {
      const gap = cum[i] - cum[i - 1]
      expect(gap).toBeGreaterThan(SPACING * 0.8)
      expect(gap).toBeLessThan(SPACING * 1.2)
    }
  })

  it('custom spacingM=100 produces fewer points than default 10', () => {
    const pts: [number, number][] = [
      [13.0, 47.0],
      [13.0, 47.5],
    ]
    const coarse = resampleStreamCoords(pts, 100)
    const fine   = resampleStreamCoords(pts, 10)
    expect(coarse.length).toBeLessThan(fine.length)
  })

  it('all output points lie on the input segment (collinear check)', () => {
    // A perfectly N–S segment at a fixed lng
    const pts: [number, number][] = [[13.0, 47.0], [13.0, 48.0]]
    const result = resampleStreamCoords(pts, 10)
    for (const [lng] of result) {
      expect(lng).toBeCloseTo(13.0, 8) // lng must not drift
    }
  })

  it('output latitudes are monotonically increasing for a northward track', () => {
    const pts: [number, number][] = [[13.0, 47.0], [13.0, 48.0]]
    const result = resampleStreamCoords(pts, 10)
    for (let i = 1; i < result.length; i++) {
      expect(result[i][1]).toBeGreaterThanOrEqual(result[i - 1][1])
    }
  })
})
