import { describe, it, expect } from 'vitest'
import { filterSubSegmentsByDescriptor } from '../descriptorFilter'
import type { RouteSubSegment } from '../routeSurfaceAggregation'

function seg(x: Partial<RouteSubSegment>): RouteSubSegment {
  return { from_idx: 0, to_idx: 1, highway: 'path', surface: 'ground', ...x }
}

describe('filterSubSegmentsByDescriptor', () => {
  it('mtb_technical matches S2+ in either field', () => {
    const out = filterSubSegmentsByDescriptor(
      [seg({ mtb_scale: 'S1' }), seg({ mtb_scale: 'S2' }), seg({ mtb_scale_uphill: 'S3' })],
      'mtb_technical',
    )
    expect(out).toHaveLength(2)
  })
  it('alpine_hiking matches T4+', () => {
    const out = filterSubSegmentsByDescriptor(
      [seg({ sac_scale: 'T3' }), seg({ sac_scale: 'T4' })],
      'alpine_hiking',
    )
    expect(out).toHaveLength(1)
  })
  it('exposed matches via_ferrata highway or scale', () => {
    const out = filterSubSegmentsByDescriptor(
      [seg({ highway: 'path' }), seg({ highway: 'via_ferrata' }), seg({ via_ferrata_scale: 'K2' })],
      'exposed',
    )
    expect(out).toHaveLength(2)
  })
  it('low_visibility matches intermediate+', () => {
    const out = filterSubSegmentsByDescriptor(
      [seg({ trail_visibility: 'good' }), seg({ trail_visibility: 'bad' })],
      'low_visibility',
    )
    expect(out).toHaveLength(1)
  })
})
