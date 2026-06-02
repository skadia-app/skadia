import { describe, expect, it } from 'vitest'
import { formatBestEffortDisplay, formatBestEffortValue } from './bestEffortLabels'
import type { UnitFormatOptions } from './units'

const imperial: UnitFormatOptions = { locale: 'en-US', units: 'imperial' }
const frenchMetric: UnitFormatOptions = { locale: 'fr-FR', units: 'metric' }

describe('best effort value formatting', () => {
  it('formats superlative distances and elevations with the active unit system', () => {
    expect(formatBestEffortValue('sup_longest_distance', null, 16_093.44, imperial)).toBe('10.0 mi')
    expect(formatBestEffortValue('sup_most_elevation', null, 1_000, imperial)).toBe('3,281 ft')
  })

  it('formats cycling distance records as active-unit speed', () => {
    expect(formatBestEffortDisplay('dist_10km', 1_800, null, 'cycling', imperial)).toEqual({
      primary: '12.4 mph',
      secondary: '30:00',
    })
  })

  it('formats power records with locale-aware grouping', () => {
    const expectedWatts = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 0 }).format(1235)
    expect(formatBestEffortValue('power_5min', null, 1234.6, frenchMetric)).toBe(`${expectedWatts} W`)
  })
})
