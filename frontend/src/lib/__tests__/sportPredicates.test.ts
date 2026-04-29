import { describe, expect, it } from 'vitest'
import {
  modifiersMatch,
  predicatesMatch,
  type ModifierFilters,
  type SportPredicate,
} from '../sportPredicates'

describe('sport predicate helpers', () => {
  it('matches open sport predicates across subtypes', () => {
    expect(predicatesMatch([{ sport: 'cycling' }], 'cycling', 'mountain')).toBe(true)
    expect(predicatesMatch([{ sport: 'cycling' }], 'cycling')).toBe(true)
    expect(predicatesMatch([{ sport: 'cycling' }], 'running')).toBe(false)
  })

  it('matches exact subtype predicates only for that subtype', () => {
    const predicates: SportPredicate[] = [{ sport: 'cycling', sub_type: 'mountain' }]

    expect(predicatesMatch(predicates, 'cycling', 'mountain')).toBe(true)
    expect(predicatesMatch(predicates, 'cycling', 'road')).toBe(false)
    expect(predicatesMatch(predicates, 'cycling')).toBe(false)
  })

  it('matches the empty subtype sentinel only for no subtype', () => {
    const predicates: SportPredicate[] = [{ sport: 'cycling', sub_type: '' }]

    expect(predicatesMatch(predicates, 'cycling')).toBe(true)
    expect(predicatesMatch(predicates, 'cycling', '')).toBe(true)
    expect(predicatesMatch(predicates, 'cycling', 'mountain')).toBe(false)
  })

  it('treats an empty predicate list as match-never', () => {
    expect(predicatesMatch([], 'cycling', 'mountain')).toBe(false)
  })

  it('matches modifiers exactly only for present keys', () => {
    const state = { is_electric: true, is_indoor: false, is_virtual: true, is_commute: false }

    expect(modifiersMatch({}, state)).toBe(true)
    expect(modifiersMatch({ is_electric: true }, state)).toBe(true)
    expect(modifiersMatch({ is_indoor: true }, state)).toBe(false)
    expect(modifiersMatch({ is_virtual: true, is_commute: false } satisfies ModifierFilters, state)).toBe(true)
  })
})
