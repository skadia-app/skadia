import { describe, expect, it } from 'vitest'
import { splitSportSelectionKeys } from '../sportSelection'

describe('splitSportSelectionKeys', () => {
  it('preserves subtype-only selections as sport_type plus sub_type filters', () => {
    expect(splitSportSelectionKeys(['cycling:mountain', 'running:trail'])).toEqual({
      sportTypes: ['cycling', 'running'],
      subTypes: ['mountain', 'trail'],
    })
  })

  it('drops subtype filters when a bare key would otherwise be excluded', () => {
    expect(splitSportSelectionKeys(['cycling', 'cycling:mountain'])).toEqual({
      sportTypes: ['cycling'],
      subTypes: [],
    })
  })
})
