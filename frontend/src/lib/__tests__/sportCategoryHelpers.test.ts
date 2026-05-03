import { describe, expect, it } from 'vitest'
import {
  expandCategoryToSportTypes,
  whitelistForSegmentType,
} from '../sportCategoryHelpers'

describe('segment sport category helpers', () => {
  it('leaves standard segments on the full non-winter/non-water choice surface', () => {
    expect(whitelistForSegmentType('standard')).toBeUndefined()
  })

  it('filters fallback and system sports only when a segment type supplies a whitelist', () => {
    const types = expandCategoryToSportTypes('sonstige', whitelistForSegmentType('standard'))
    expect(types).toContain('strength_training')
    expect(types).toContain('training')
    expect(types).toContain('fitness_equipment')
    expect(types).toContain('generic')
    expect(types).toContain('unknown')
  })

  it('uses projection eligibility for winter and water segment types', () => {
    expect(whitelistForSegmentType('wintersport')).toContain('snowshoeing')
    expect(whitelistForSegmentType('wassersport')).toContain('swimming')
    expect(whitelistForSegmentType('lift')).toBeUndefined()
  })
})
