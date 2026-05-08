import { describe, expect, it } from 'vitest'
import { activityModifierLabels } from '../activityModifierLabels'

describe('activityModifierLabels', () => {
  it('uses the generic electric-drive wording for is_electric', () => {
    expect(activityModifierLabels({ isElectric: true, isIndoor: true, isVirtual: true })).toEqual([
      'E-Antrieb',
      'Indoor',
      'Virtuell',
    ])
  })
})
