import { describe, expect, it } from 'vitest'
import { gearCategoryLabel, isGearCategoryScope } from '../gearSchemaTypes'

describe('gearSchemaTypes', () => {
  it('localizes category labels without changing category keys', () => {
    expect(isGearCategoryScope('bike')).toBe(true)
    expect(gearCategoryLabel('bike', 'de')).toBe('Räder')
    expect(gearCategoryLabel('bike', 'en')).toBe('Bikes')
    expect(gearCategoryLabel('water', 'en')).toBe('Water sports')
    expect(gearCategoryLabel('other', 'en')).toBe('Other')
  })
})
