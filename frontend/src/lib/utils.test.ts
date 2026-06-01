import { describe, expect, it } from 'vitest'
import { formatElevationGain, formatElevationLoss } from './utils'

describe('elevation delta formatting', () => {
  it('formats gain and loss with active unit preferences when provided', () => {
    const imperial = { units: 'imperial' as const, locale: 'en-US' }

    expect(formatElevationGain(100, imperial)).toBe('+328 ft')
    expect(formatElevationLoss(100, imperial)).toBe('−328 ft')
  })
})
