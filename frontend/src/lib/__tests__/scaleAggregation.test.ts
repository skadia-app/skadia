import { describe, it, expect } from 'vitest'
import { aggregateScaleLevels } from '../scaleAggregation'
import type { CoordRange } from '@/api/routes'

// NOTE: range.to is EXCLUSIVE (matches backend route_ranges.go API convention).
// The range [from, to) spans edge indices from..to-1.
// Number of segments = max(0, to - from):
//   { from: 0, to: 1 } = 1 segment  (coords[0] → coords[1])
//   { from: 0, to: 2 } = 2 segments (coords[0] → coords[1] → coords[2])
//   { from: 0, to: 3 } = 3 segments (coords[0] → coords[1] → coords[2] → coords[3])
// Equivalently: from==to also yields 0 segments (empty range).

describe('aggregateScaleLevels', () => {
  const coords: number[][] = [
    [0, 0],    // idx 0
    [0, 1],    // idx 1  — ~111 km from idx 0
    [0, 2],    // idx 2  — ~111 km from idx 1
    [0, 3],    // idx 3  — ~111 km from idx 2
  ]

  it('returns empty breakdown for undefined ranges', () => {
    const { maxLevel, breakdown } = aggregateScaleLevels(undefined, coords, 'sac')
    expect(maxLevel).toBeNull()
    expect(breakdown).toEqual([])
  })

  it('returns empty for SAC ranges of wrong kind', () => {
    const ranges: Record<string, CoordRange[]> = { T3: [{ from: 0, to: 2 }] }
    const { breakdown } = aggregateScaleLevels(ranges, coords, 'mtb')
    expect(breakdown).toEqual([])
  })

  it('computes mixed SAC levels, maxLevel = hardest present', () => {
    const ranges: Record<string, CoordRange[]> = {
      // to=3 exclusive → 2 segments (idx 0→1→2, ~222 km)
      T1: [{ from: 0, to: 3 }],
      // to=4 exclusive → coords[2]→coords[3] = 1 segment (~111 km); clamped to coords.length=4
      T5: [{ from: 2, to: 4 }],
    }
    const { maxLevel, breakdown } = aggregateScaleLevels(ranges, coords, 'sac')
    expect(maxLevel).toBe('T5')
    expect(breakdown.map(b => b.level).sort()).toEqual(['T1', 'T5'])
    // exact km not asserted here — only presence and ordering
  })

  it('no threshold — even 1-segment ranges are kept (Outdoor-Safety I8)', () => {
    const ranges: Record<string, CoordRange[]> = {
      // to=2 exclusive → 1 segment (idx 0→1), ~111 km
      T6: [{ from: 0, to: 2 }],
    }
    const { maxLevel } = aggregateScaleLevels(ranges, coords, 'sac')
    expect(maxLevel).toBe('T6')
  })

  // Anti-regression: from==to must produce rangeLength=0 (exclusive convention)
  it('from==to produces zero distance (exclusive boundary invariant)', () => {
    const ranges: Record<string, CoordRange[]> = {
      T3: [{ from: 1, to: 1 }],
    }
    const { maxLevel, breakdown } = aggregateScaleLevels(ranges, coords, 'sac')
    // rangeLength=0 → sum=0 → level is filtered out → no entry in breakdown
    expect(maxLevel).toBeNull()
    expect(breakdown).toEqual([])
  })

  // Anti-regression: to=from+2 produces exactly 2 segments (exclusive edge range)
  it('to=from+2 produces a non-empty edge range', () => {
    const ranges: Record<string, CoordRange[]> = {
      // exclusive edge range [1, 3) → coords[1]→coords[2] and coords[2]→coords[3]
      T2: [{ from: 1, to: 3 }],
    }
    const { maxLevel, breakdown } = aggregateScaleLevels(ranges, coords, 'sac')
    expect(maxLevel).toBe('T2')
    expect(breakdown).toHaveLength(1)
    expect(breakdown[0].km).toBeGreaterThan(0)
  })
})

describe('aggregateScaleLevels (vf)', () => {
  it('returns K3 as maxLevel for K1+K3 route', () => {
    const ranges = {
      // to=5 exclusive → 5 segments (idx 0–4)
      K1: [{ from: 0, to: 5 }],
      // to=10 exclusive → 5 segments (idx 5–9); clamped to coords.length=10
      K3: [{ from: 5, to: 10 }],
    }
    const coords = Array.from({ length: 10 }, (_, i) => [i * 0.001, 0]) as [number, number][]
    const got = aggregateScaleLevels(ranges, coords, 'vf')
    expect(got.maxLevel).toBe('K3')
    expect(got.breakdown.map((b) => b.level)).toEqual(['K1', 'K3'])
  })

  it('returns null maxLevel for empty VF ranges', () => {
    expect(aggregateScaleLevels(undefined, null, 'vf').maxLevel).toBeNull()
  })
})
