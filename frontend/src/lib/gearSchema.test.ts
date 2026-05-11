import { describe, it, expect } from 'vitest'
import { gearSchema as generatedSchema } from './generatedGearSchema'
import { lookupAttribute, lookupGearType, getActiveSchema } from './gearSchema'

describe('gearSchema barrel exports', () => {
  it('TestLookupAttributeReturnsFromGeneratedSchema', () => {
    const attr = lookupAttribute('bike.frame_material')
    expect(attr).toBeDefined()
    expect(attr?.key).toBe('bike.frame_material')
    expect(attr?.labelDe).toBe(
      generatedSchema.attributes.find(a => a.key === 'bike.frame_material')?.labelDe,
    )
  })

  it('TestLookupAttributeWheelSizeReturnsEntry', () => {
    const attr = lookupAttribute('bike.wheel_size')
    expect(attr).toBeDefined()
    expect(attr?.key).toBe('bike.wheel_size')
  })

  it('TestLookupGearTypeReturnsEntry', () => {
    const gt = lookupGearType('bike')
    expect(gt).toBeDefined()
    expect(gt?.key).toBe('bike')
  })

  it('TestGetActiveSchemaIsIdenticalToGenerated', () => {
    expect(getActiveSchema()).toBe(generatedSchema)
  })
})
