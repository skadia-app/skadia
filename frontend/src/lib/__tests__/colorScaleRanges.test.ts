// frontend/src/lib/__tests__/colorScaleRanges.test.ts
import { describe, it, expect } from 'vitest'
import {
  SPEED_PROFILES, PACE_PROFILES, CADENCE_PROFILES,
  MOTION_PROFILE_BY_SPORT_TYPE, GENERIC_ALLOWLIST,
  expandRange, TEMPERATURE_RANGE, ALTITUDE_MIN_SPREAD,
  HR_FALLBACK_MIN_SPREAD, POWER_FALLBACK_MIN_SPREAD,
} from '../colorScaleRanges'
import { SPORT_DEFS } from '../sportTaxonomy'

describe('expandRange', () => {
  it('keeps range when spread satisfied', () => {
    expect(expandRange(0, 50, 10)).toEqual([0, 50])
  })
  it('expands symmetrically when too narrow', () => {
    expect(expandRange(18, 20, 10)).toEqual([14, 24])
  })
  it('handles equal min/max', () => {
    expect(expandRange(100, 100, 30)).toEqual([85, 115])
  })
})

describe('SPEED_PROFILES', () => {
  it('all profiles start at 0 km/h', () => {
    for (const k of Object.keys(SPEED_PROFILES)) {
      const p = SPEED_PROFILES[k as keyof typeof SPEED_PROFILES]
      expect(p.minKmh).toBe(0)
      expect(p.maxKmh).toBeGreaterThan(0)
    }
  })
})

describe('PACE_PROFILES', () => {
  it('only foot-based profiles', () => {
    const keys = Object.keys(PACE_PROFILES)
    expect(keys).toContain('running')
    expect(keys).toContain('walking')
    expect(keys).toContain('hiking')
    expect(keys).toContain('generic')
  })
  it('min < max in minutes/km', () => {
    for (const k of Object.keys(PACE_PROFILES)) {
      const p = PACE_PROFILES[k as keyof typeof PACE_PROFILES]
      expect(p.minMinPerKm).toBeLessThan(p.maxMinPerKm)
    }
  })
})

describe('CI-Guard: MOTION_PROFILE_BY_SPORT_TYPE', () => {
  it('every SportDef.sportType has an explicit mapping', () => {
    const missing: string[] = []
    for (const def of SPORT_DEFS) {
      if (!(def.sportType in MOTION_PROFILE_BY_SPORT_TYPE)) {
        missing.push(def.sportType)
      }
    }
    expect(missing, `Unmapped sportTypes: ${missing.join(', ')}`).toHaveLength(0)
  })

  it('every generic-profile mapping is on the allowlist', () => {
    const unexpectedGeneric: string[] = []
    for (const [sportType, profile] of Object.entries(MOTION_PROFILE_BY_SPORT_TYPE)) {
      if (profile.speed === 'generic' && !GENERIC_ALLOWLIST.includes(sportType)) {
        unexpectedGeneric.push(sportType)
      }
    }
    expect(
      unexpectedGeneric,
      `Sports with generic profile but not on allowlist: ${unexpectedGeneric.join(', ')}`,
    ).toHaveLength(0)
  })
})

describe('constants', () => {
  it('TEMPERATURE_RANGE matches diverging palette stops', () => {
    expect(TEMPERATURE_RANGE.min).toBe(-10)
    expect(TEMPERATURE_RANGE.max).toBe(35)
  })
  it('ALTITUDE_MIN_SPREAD = 50', () => expect(ALTITUDE_MIN_SPREAD).toBe(50))
  it('HR_FALLBACK_MIN_SPREAD = 30', () => expect(HR_FALLBACK_MIN_SPREAD).toBe(30))
  it('POWER_FALLBACK_MIN_SPREAD = 50', () => expect(POWER_FALLBACK_MIN_SPREAD).toBe(50))
})
