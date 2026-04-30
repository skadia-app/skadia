import { describe, expect, it } from 'vitest'
import { sportPredicatesFromSelectionKeys } from '../sportSelection'

describe('sportPredicatesFromSelectionKeys', () => {
  it('preserves tuple selections without cross-matching unrelated subtypes', () => {
    expect(sportPredicatesFromSelectionKeys(['cycling:mountain', 'running:trail'])).toEqual([
      { sport: 'cycling', sub_type: 'mountain' },
      { sport: 'running', sub_type: 'trail' },
    ])
  })

  it('keeps broad and exact predicates independently for the same sport', () => {
    expect(sportPredicatesFromSelectionKeys(['cycling', 'cycling:mountain'])).toEqual([
      { sport: 'cycling' },
      { sport: 'cycling', sub_type: 'mountain' },
    ])
  })
})
