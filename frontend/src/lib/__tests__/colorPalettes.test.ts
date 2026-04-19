import { describe, it, expect } from 'vitest'
import { interpolateHex, MISSING_COLOR, VIRIDIS_9, DIVERGING_TEMP_STOPS, HR_ZONE_BASE, POWER_ZONE_BASE, SPEED_ZONE_BASE } from '../colorPalettes'

describe('interpolateHex', () => {
  it('returns start at t=0', () => {
    expect(interpolateHex('#ff0000', '#00ff00', 0).toLowerCase()).toBe('#ff0000')
  })
  it('returns end at t=1', () => {
    expect(interpolateHex('#ff0000', '#00ff00', 1).toLowerCase()).toBe('#00ff00')
  })
  it('mixes channels at t=0.5', () => {
    expect(interpolateHex('#ff0000', '#00ff00', 0.5).toLowerCase()).toBe('#808000')
  })
  it('handles uppercase hex input', () => {
    expect(interpolateHex('#FF0000', '#00FF00', 0.5).toLowerCase()).toBe('#808000')
  })
})

describe('palette constants', () => {
  it('MISSING_COLOR is a hex string', () => {
    expect(MISSING_COLOR).toMatch(/^#[0-9a-f]{6}$/i)
  })
  it('VIRIDIS_9 has 9 stops', () => {
    expect(VIRIDIS_9).toHaveLength(9)
    VIRIDIS_9.forEach(c => expect(c).toMatch(/^#[0-9a-f]{6}$/i))
  })
  it('DIVERGING_TEMP_STOPS is sorted by value', () => {
    const values = DIVERGING_TEMP_STOPS.map(s => s.value)
    for (let i = 1; i < values.length; i++) {
      expect(values[i]).toBeGreaterThan(values[i - 1])
    }
  })
  it('HR_ZONE_BASE has at least 6 stops', () => {
    expect(HR_ZONE_BASE.length).toBeGreaterThanOrEqual(6)
  })
  it('POWER_ZONE_BASE has at least 8 stops (Coggan 7+margin)', () => {
    expect(POWER_ZONE_BASE.length).toBeGreaterThanOrEqual(8)
  })
  it('SPEED_ZONE_BASE is distinct (aqua palette, not HR)', () => {
    // First stop should differ from HR_ZONE_BASE[0]
    expect(SPEED_ZONE_BASE[0]).not.toBe(HR_ZONE_BASE[0])
  })
})
