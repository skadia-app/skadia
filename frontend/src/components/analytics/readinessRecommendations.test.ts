import { describe, it, expect } from 'vitest'
import { recommendationText } from './readinessRecommendations'

describe('recommendationText', () => {
  it('handles known code "good_high_normal"', () => {
    expect(recommendationText('good_high_normal')).toContain('Planmäßige')
  })

  it('handles known code "critical_high_rhr_changepoint_recent"', () => {
    expect(recommendationText('critical_high_rhr_changepoint_recent')).toContain('Ruhetag')
  })

  it('handles "good_high_rhr_changepoint_recent" (explicit entry)', () => {
    expect(recommendationText('good_high_rhr_changepoint_recent')).toContain('Ruhepuls')
  })

  it('falls back via level for unknown confidence', () => {
    expect(recommendationText('moderate_low_normal')).toContain('Lockere')
  })

  it('returns empty string for null', () => {
    expect(recommendationText(null)).toBe('')
  })

  it('returns empty string for empty string', () => {
    expect(recommendationText('')).toBe('')
  })

  it('falls back to empty for fully unknown code (invalid level prefix)', () => {
    expect(recommendationText('asdf_qwer_zxcv')).toBe('')
  })

  it('handles all five levels with normal modifier', () => {
    for (const lvl of ['critical', 'low', 'moderate', 'good', 'optimal'] as const) {
      const out = recommendationText(`${lvl}_medium_normal`)
      expect(out.length).toBeGreaterThan(0)
    }
  })

  it('critical + rhr_changepoint at medium confidence still hits special pattern', () => {
    expect(recommendationText('critical_medium_rhr_changepoint_recent')).toContain('dringend')
  })

  it('low_high_normal returns active recovery message', () => {
    expect(recommendationText('low_high_normal')).toContain('Aktive Erholung')
  })

  it('optimal_medium_normal returns optimal message via fallback', () => {
    expect(recommendationText('optimal_medium_normal')).toContain('Sehr gute')
  })
})
