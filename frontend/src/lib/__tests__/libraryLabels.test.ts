import { describe, expect, it } from 'vitest'

import { EXAMPLE_VALUES, TYPE_VARIABLES } from '@/lib/libraryLabels'

describe('libraryLabels file schema variables', () => {
  it('exposes activity modifiers with the stable preview token chain', () => {
    expect(TYPE_VARIABLES.activity.map((v) => v.key)).toContain('{modifiers}')
    expect(EXAMPLE_VALUES['{modifiers}']).toBe('electric_virtual')
  })
})
