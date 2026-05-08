import { describe, expect, it } from 'vitest'
import { buildExistingSegmentSportPatch, buildNewSegmentSportPatch } from './segmentSave'

describe('segment sport save helpers', () => {
  it('expands new water all selection to explicit predicates', () => {
    const patch = buildNewSegmentSportPatch('wassersport', null)
    expect(patch.allowed_predicates).toContainEqual({ sport: 'swimming' })
    expect(patch.allowed_predicates).toContainEqual({ sport: 'fishing' })
    expect(patch.allowed_predicates).not.toContainEqual({ sport: 'diving' })
    expect(patch.sport_group).toBe('generic')
  })

  it('preserves legacy null on unrelated existing segment edits', () => {
    expect(buildExistingSegmentSportPatch({
      segType: 'standard',
      initialSegType: 'standard',
      selection: null,
      initialAllowedPredicates: null,
      touched: false,
    })).toEqual({})
  })

  it('preserves explicit predicates on unrelated existing segment edits', () => {
    expect(buildExistingSegmentSportPatch({
      segType: 'standard',
      initialSegType: 'standard',
      selection: ['running'],
      initialAllowedPredicates: [{ sport: 'running' }],
      touched: false,
    })).toEqual({})
  })

  it('converts touched existing all selection to explicit predicates', () => {
    const patch = buildExistingSegmentSportPatch({
      segType: 'wintersport',
      initialSegType: 'wintersport',
      selection: null,
      initialAllowedPredicates: null,
      touched: true,
    })
    expect(patch.allowed_predicates).toContainEqual({ sport: 'snowshoeing' })
    expect(patch.allowed_predicates).not.toBeNull()
    expect(patch).not.toHaveProperty('sport_group')
  })

  it('keeps lift on current legacy behavior', () => {
    expect(buildNewSegmentSportPatch('lift', null)).toEqual({ allowed_predicates: null, sport_group: 'generic' })
  })

  it('treats segment type changes as eligibility changes', () => {
    const patch = buildExistingSegmentSportPatch({
      segType: 'wintersport',
      initialSegType: 'standard',
      selection: null,
      initialAllowedPredicates: null,
      touched: false,
    })
    expect(patch.allowed_predicates).toContainEqual({ sport: 'snowshoeing' })
  })
})
