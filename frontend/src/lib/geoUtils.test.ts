import { describe, expect, it } from 'vitest'
import { formatRadius } from './geoUtils'

describe('geo radius formatting', () => {
  it('formats radius with active unit preferences when provided', () => {
    expect(formatRadius(1609.344, { units: 'imperial', locale: 'en-US' })).toBe('1.0 mi')
    expect(formatRadius(250, { units: 'imperial', locale: 'en-US' })).toBe('820 ft')
  })
})
