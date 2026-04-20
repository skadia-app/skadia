import { describe, it, expect } from 'vitest'
import {
  BIDIRECTIONAL_DEFAULT, SAC_COLOR_MAP, MTB_COLOR_MAP,
  deriveTrailDifficultyColor, WINTER_SPORT_WHITELIST,
} from './constants'

describe('deriveTrailDifficultyColor', () => {
  it('returns null when both unset', () => {
    expect(deriveTrailDifficultyColor(null, null)).toBeNull()
  })
  it('SAC only → SAC color', () => {
    expect(deriveTrailDifficultyColor('T1', null)).toBe('blue')
    expect(deriveTrailDifficultyColor('T4', null)).toBe('black')
  })
  it('MTB only → MTB color', () => {
    expect(deriveTrailDifficultyColor(null, 'S0')).toBe('blue')
    expect(deriveTrailDifficultyColor(null, 'S3')).toBe('black')
  })
  it('both → max color', () => {
    expect(deriveTrailDifficultyColor('T3', 'S1')).toBe('red')  // red > blue
    expect(deriveTrailDifficultyColor('T1', 'S3')).toBe('black') // black > blue
    expect(deriveTrailDifficultyColor('T3', 'S3')).toBe('black') // black > red
  })
})

describe('BIDIRECTIONAL_DEFAULT', () => {
  it('trail and wintersport default to false', () => {
    expect(BIDIRECTIONAL_DEFAULT.trail).toBe(false)
    expect(BIDIRECTIONAL_DEFAULT.wintersport).toBe(false)
  })
  it('standard and lap default to true', () => {
    expect(BIDIRECTIONAL_DEFAULT.standard).toBe(true)
    expect(BIDIRECTIONAL_DEFAULT.lap).toBe(true)
  })
})

describe('WINTER_SPORT_WHITELIST', () => {
  it('includes snowshoeing even though taxonomy puts it in run-category', () => {
    expect(WINTER_SPORT_WHITELIST).toContain('snowshoeing')
  })
})

describe('SAC_COLOR_MAP', () => {
  it('T1 maps to blue', () => {
    expect(SAC_COLOR_MAP.T1).toBe('blue')
  })
  it('T6 maps to black', () => {
    expect(SAC_COLOR_MAP.T6).toBe('black')
  })
})

describe('MTB_COLOR_MAP', () => {
  it('S0 maps to blue', () => {
    expect(MTB_COLOR_MAP.S0).toBe('blue')
  })
  it('S5 maps to black', () => {
    expect(MTB_COLOR_MAP.S5).toBe('black')
  })
})
