import { describe, expect, it } from 'vitest'
import { EXAMPLE_VALUES } from '../libraryLabels'

describe('schema preview examples', () => {
  it('matches backend ShadowFS activity schema tag formatting', () => {
    expect(EXAMPLE_VALUES['{time}']).toBe('1430')
    expect(EXAMPLE_VALUES['{duration_h}']).toBe('2-0')
  })
})
