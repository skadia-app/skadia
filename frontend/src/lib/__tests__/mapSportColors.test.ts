import { describe, expect, it } from 'vitest'

import { buildMapLineColor } from '../mapSportColors'

describe('mapSportColors', () => {
  it('uses global sport colors for subtype track tuples', () => {
    const expression = buildMapLineColor({
      mode: 'per-type',
      uniformColor: '#F97316',
      colors: {},
    }, { cycling: '#123456' })

    expect(expression).toContain('cycling:mountain')
    expect(expression).toContain('#123456')
  })

  it('ignores stale subtype-specific map colors in favor of sport colors', () => {
    const expression = buildMapLineColor({
      mode: 'per-type',
      uniformColor: '#F97316',
      colors: {
        'cycling:mountain': '#654321',
        cycling: '#123456',
      },
    }, {})

    expect(expression).toContain('cycling:mountain')
    expect(expression).toContain('#123456')
    expect(expression).not.toContain('#654321')
  })

  it('keeps uniform mode independent from per-sport colors', () => {
    expect(buildMapLineColor({
      mode: 'uniform',
      uniformColor: '#ABCDEF',
      colors: {},
    }, { cycling: '#123456' })).toBe('#ABCDEF')
  })
})
