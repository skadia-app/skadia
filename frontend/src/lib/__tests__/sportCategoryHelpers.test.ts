import { describe, expect, it } from 'vitest'
import {
  getVisibleCategories,
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

  it('uses generated six-bucket water groups for water segment category chips', () => {
    const groups = getVisibleCategories('wassersport')
    expect(groups.map(group => group.key)).toEqual([
      'water_swim',
      'water_paddle',
      'water_board',
      'water_boat',
      'water_dive',
      'water_other',
    ])
    expect(groups.map(group => group.label)).not.toContain('Surfen & Segeln')
  })

  it('expands generated water buckets to their projected sport keys', () => {
    const whitelist = whitelistForSegmentType('wassersport')

    expect(expandCategoryToSportTypes('water_board', whitelist)).toEqual(expect.arrayContaining([
      'surfing',
      'kitesurfing',
      'windsurfing',
      'wakeboarding',
      'wakesurfing',
      'water_skiing',
      'water_tubing',
    ]))
    expect(expandCategoryToSportTypes('water_board', whitelist)).not.toContain('sailing')
    expect(expandCategoryToSportTypes('water_boat', whitelist)).toEqual(['boating', 'sailing'])
    expect(expandCategoryToSportTypes('water_dive', whitelist)).toEqual(['snorkeling'])
    expect(expandCategoryToSportTypes('water_other', whitelist)).toEqual(['fishing'])
  })
})
