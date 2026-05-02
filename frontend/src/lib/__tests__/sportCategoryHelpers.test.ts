import { describe, expect, it } from 'vitest'
import {
  expandCategoryToSportTypes,
  whitelistForSegmentType,
} from '../sportCategoryHelpers'

describe('segment sport category helpers', () => {
  it('uses projection eligibility for standard segments too', () => {
    const whitelist = whitelistForSegmentType('standard')
    expect(whitelist).toContain('running')
    expect(whitelist).not.toContain('training')
    expect(whitelist).not.toContain('fitness_equipment')
    expect(whitelist).not.toContain('generic')
    expect(whitelist).not.toContain('unknown')
  })

  it('filters fallback and system sports out of standard category expansion', () => {
    const types = expandCategoryToSportTypes('sonstige', whitelistForSegmentType('standard'))
    expect(types).toContain('strength_training')
    expect(types).not.toContain('training')
    expect(types).not.toContain('fitness_equipment')
    expect(types).not.toContain('generic')
    expect(types).not.toContain('unknown')
  })
})
