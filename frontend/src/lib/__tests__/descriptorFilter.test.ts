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

  it('returns empty array for empty segments', () => {
    expect(filterSubSegmentsByDescriptor([], 'mtb_technical')).toEqual([])
    expect(filterSubSegmentsByDescriptor([], 'alpine_hiking')).toEqual([])
    expect(filterSubSegmentsByDescriptor([], 'exposed')).toEqual([])
    expect(filterSubSegmentsByDescriptor([], 'low_visibility')).toEqual([])
  })

  it('handles undefined fields gracefully (no tags at all)', () => {
    const segs = [
      seg({ highway: 'path' }),  // keine scale/visibility-fields
      seg({ highway: 'track' }),
    ]
    expect(filterSubSegmentsByDescriptor(segs, 'mtb_technical')).toEqual([])
    expect(filterSubSegmentsByDescriptor(segs, 'alpine_hiking')).toEqual([])
    expect(filterSubSegmentsByDescriptor(segs, 'exposed')).toEqual([])
    expect(filterSubSegmentsByDescriptor(segs, 'low_visibility')).toEqual([])
  })

  it('mtb_technical matches via mtb_scale_uphill alone (no mtb_scale)', () => {
    const out = filterSubSegmentsByDescriptor(
      [seg({ mtb_scale_uphill: 'S4' })],
      'mtb_technical',
    )
    expect(out).toHaveLength(1)
  })

  it('mtb_technical matches when mtb_scale is S0/S1 but uphill is S2+', () => {
    const out = filterSubSegmentsByDescriptor(
      [
        seg({ mtb_scale: 'S0', mtb_scale_uphill: 'S3' }),  // uphill qualifies
        seg({ mtb_scale: 'S1', mtb_scale_uphill: 'S0' }),  // neither qualifies
        seg({ mtb_scale: 'S1' }),                          // neither set (S1 < S2)
      ],
      'mtb_technical',
    )
    expect(out).toHaveLength(1)
  })

  it('alpine_hiking does NOT match T1/T2/T3', () => {
    const out = filterSubSegmentsByDescriptor(
      [seg({ sac_scale: 'T1' }), seg({ sac_scale: 'T2' }), seg({ sac_scale: 'T3' })],
      'alpine_hiking',
    )
    expect(out).toEqual([])
  })

  it('exposed matches via_ferrata_scale even without highway=via_ferrata', () => {
    const out = filterSubSegmentsByDescriptor(
      [seg({ highway: 'path', via_ferrata_scale: 'K1' })],
      'exposed',
    )
    expect(out).toHaveLength(1)
  })

  it('low_visibility does NOT match "good" or "excellent"', () => {
    const out = filterSubSegmentsByDescriptor(
      [
        seg({ trail_visibility: 'excellent' }),
        seg({ trail_visibility: 'good' }),
      ],
      'low_visibility',
    )
    expect(out).toEqual([])
  })
})
