import { describe, expect, it } from 'vitest'
import { SPORT_COLORS, sportChartColor } from '../sportColors'

describe('sportColors', () => {
  it('does not keep legacy top-level sport aliases as runtime color keys', () => {
    expect(Object.keys(SPORT_COLORS)).not.toEqual(expect.arrayContaining([
      'trail_running',
      'mountain_biking',
      'open_water_swimming',
    ]))
  })

  it('still normalizes legacy aliases before color lookup', () => {
    expect(sportChartColor('mountain_biking')).toBe(sportChartColor('cycling', 'mountain'))
    expect(sportChartColor('open_water_swimming')).toBe(sportChartColor('swimming', 'open_water'))
  })
})
