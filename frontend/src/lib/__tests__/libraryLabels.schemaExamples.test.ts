import { describe, expect, it } from 'vitest'
import { EXAMPLE_VALUES, TYPE_VARIABLES } from '../libraryLabels'

describe('schema preview examples', () => {
  it('matches backend ShadowFS activity schema tag formatting', () => {
    expect(EXAMPLE_VALUES['{time}']).toBe('1430')
    expect(EXAMPLE_VALUES['{duration_h}']).toBe('2-0')
  })

  it('offers file format for activity schemas because backend emits it', () => {
    expect(TYPE_VARIABLES.activity.map((v) => v.key)).toContain('{format}')
    expect(EXAMPLE_VALUES['{format}']).toBe('fit')
  })
})
