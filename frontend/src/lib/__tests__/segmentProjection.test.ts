import { describe, expect, it } from 'vitest'
import {
  segmentEligibleSelectionKeys,
  segmentPredicatesForSelection,
  segmentProjectionEntriesForType,
  segmentSportGroupForSelection,
  segmentTypeLabel,
} from '../segmentProjection'

describe('segment projection helpers', () => {
  it('keeps snowshoeing in winter with foot movement group', () => {
    expect(segmentEligibleSelectionKeys('wintersport')).toContain('snowshoeing')
    expect(segmentSportGroupForSelection('wintersport', ['snowshoeing'])).toBe('foot')
  })

  it('keeps water whitelist semantics and excludes diving', () => {
    const water = segmentEligibleSelectionKeys('wassersport')
    expect(water).toContain('snorkeling')
    expect(water).toContain('fishing')
    expect(water).not.toContain('diving')
  })

  it('expands null selection to explicit active predicates for new saves', () => {
    const predicates = segmentPredicatesForSelection('wassersport', null)
    expect(predicates).toContainEqual({ sport: 'swimming' })
    expect(predicates).toContainEqual({ sport: 'fishing' })
    expect(predicates).not.toContainEqual({ sport: 'diving' })
  })

  it('returns generic when a selection spans multiple segment groups', () => {
    expect(segmentSportGroupForSelection('wintersport', ['alpine_skiing', 'snowshoeing'])).toBe('generic')
  })

  it('uses parent projection entries for subtype selections', () => {
    expect(segmentSportGroupForSelection('standard', ['running:trail'])).toBe('foot')
    expect(segmentPredicatesForSelection('standard', ['running:trail'])).toContainEqual({ sport: 'running', sub_type: 'trail' })
  })

  it('keeps skateboarding available as its own outdoor segment sport', () => {
    expect(segmentEligibleSelectionKeys('standard')).toContain('skateboarding')
    expect(segmentSportGroupForSelection('standard', ['skateboarding'])).toBe('generic')
  })

  it('keeps new outdoor taxonomy sports available for standard segments', () => {
    const standard = segmentEligibleSelectionKeys('standard')

    expect(standard).toEqual(expect.arrayContaining(['geocaching', 'archery', 'shooting']))
    expect(segmentSportGroupForSelection('standard', ['geocaching'])).toBe('foot')
    expect(segmentSportGroupForSelection('standard', ['archery'])).toBe('generic')
    expect(segmentSportGroupForSelection('standard', ['shooting'])).toBe('generic')
  })

  it('does not expose non-selectable or system sports for new segment expansion', () => {
    const standard = segmentEligibleSelectionKeys('standard')
    expect(standard).not.toContain('training')
    expect(standard).not.toContain('fitness_equipment')
    expect(standard).not.toContain('generic')
    expect(standard).not.toContain('unknown')
  })

  it('keeps lift outside generated eligibility expansion', () => {
    expect(segmentEligibleSelectionKeys('lift')).toEqual([])
    expect(segmentPredicatesForSelection('lift', null)).toEqual([])
  })

  it('does not expose reserved virtual domains through active helpers', () => {
    const standard = segmentProjectionEntriesForType('standard')
    expect(standard.every(entry => entry.domain === 'real_world_gps')).toBe(true)
  })

  it('reads segment type labels from generated projection metadata', () => {
    expect(segmentTypeLabel('wintersport')).toBe('Winter')
    expect(segmentTypeLabel('lift')).toBe('Lift')
    expect(segmentTypeLabel('wassersport', 'en')).toBe('Water sports')
    expect(segmentTypeLabel('lap', 'es')).toBe('Vuelta')
  })
})
