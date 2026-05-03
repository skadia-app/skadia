import { describe, expect, it } from 'vitest'
import { ROUTE_MODIFIER_LABELS } from '../routeModifierLabels'

describe('route modifier labels', () => {
  it('uses the neutral electric-drive label for route-level is_electric', () => {
    expect(ROUTE_MODIFIER_LABELS.is_electric).toBe('E-Antrieb')
    expect(Object.values(ROUTE_MODIFIER_LABELS)).not.toContain('E-Bike')
  })
})
