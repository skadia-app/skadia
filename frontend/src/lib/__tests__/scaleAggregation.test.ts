import { describe, it, expect } from 'vitest'
import { aggregateScaleLevels } from '../scaleAggregation'
import type { CoordRange } from '@/api/routes'

describe('aggregateScaleLevels', () => {
  const coords: number[][] = [
    [0, 0],    // idx 0
    [0, 1],    // idx 1  — ~111 km
    [0, 2],    // idx 2  — ~111 km
    [0, 3],    // idx 3  — ~111 km
  ]

  it('returns empty breakdown for undefined ranges', () => {
    const { maxLevel, breakdown } = aggregateScaleLevels(undefined, coords, 'sac')
    expect(maxLevel).toBeNull()
    expect(breakdown).toEqual([])
  })

  it('returns empty for SAC ranges of wrong kind', () => {
    const ranges: Record<string, CoordRange[]> = { T3: [{ from: 0, to: 1 }] }
    const { breakdown } = aggregateScaleLevels(ranges, coords, 'mtb')
    expect(breakdown).toEqual([])
  })

  it('computes mixed SAC levels, maxLevel = hardest present', () => {
    const ranges: Record<string, CoordRange[]> = {
      T1: [{ from: 0, to: 2 }],   // idx 0→1→2 = 2 segments (~222km)
      T5: [{ from: 2, to: 4 }],   // idx 2→3 = 1 segment inclusive of destination
    }
    const { maxLevel, breakdown } = aggregateScaleLevels(ranges, coords, 'sac')
    expect(maxLevel).toBe('T5')
    expect(breakdown.map(b => b.level).sort()).toEqual(['T1', 'T5'])
    // exact km not asserted here — only presence and ordering
  })

  it('no threshold — even 1-coord-pair ranges are kept (Outdoor-Safety I8)', () => {
    const ranges: Record<string, CoordRange[]> = {
      T6: [{ from: 0, to: 1 }],   // tiny segment, ~111km in this fixture
    }
    const { maxLevel } = aggregateScaleLevels(ranges, coords, 'sac')
    expect(maxLevel).toBe('T6')
  })
})

describe('aggregateScaleLevels (vf)', () => {
  it('returns K3 as maxLevel for K1+K3 route', () => {
    const ranges = {
      K1: [{ from: 0, to: 5 }],
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
