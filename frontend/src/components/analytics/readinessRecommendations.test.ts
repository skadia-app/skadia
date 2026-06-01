import { describe, it, expect } from 'vitest'
import { recommendationText } from './readinessRecommendations'
import { i18n } from '@/i18n/i18n'

describe('recommendationText', () => {
  it('handles known code "good_high_normal"', () => {
    expect(recommendationText('good_high_normal', i18n.t)).toContain('Planmäßige')
  })

  it('handles known code "critical_high_rhr_changepoint_recent"', () => {
    expect(recommendationText('critical_high_rhr_changepoint_recent', i18n.t)).toContain('Ruhetag')
  })

  it('handles "good_high_rhr_changepoint_recent" (explicit entry)', () => {
    expect(recommendationText('good_high_rhr_changepoint_recent', i18n.t)).toContain('Ruhepuls')
  })

  it('falls back via level for unknown confidence', () => {
    expect(recommendationText('moderate_low_normal', i18n.t)).toContain('Lockere')
  })

  it('returns empty string for null', () => {
    expect(recommendationText(null, i18n.t)).toBe('')
  })

  it('returns empty string for empty string', () => {
    expect(recommendationText('', i18n.t)).toBe('')
  })

  it('falls back to empty for fully unknown code (invalid level prefix)', () => {
    expect(recommendationText('asdf_qwer_zxcv', i18n.t)).toBe('')
  })

  it('handles all five levels with normal modifier', () => {
    for (const lvl of ['critical', 'low', 'moderate', 'good', 'optimal'] as const) {
      const out = recommendationText(`${lvl}_medium_normal`, i18n.t)
      expect(out.length).toBeGreaterThan(0)
    }
  })

  it('critical + rhr_changepoint at medium confidence still hits special pattern', () => {
    expect(recommendationText('critical_medium_rhr_changepoint_recent', i18n.t)).toContain('dringend')
  })

  it('low_high_normal returns active recovery message', () => {
    expect(recommendationText('low_high_normal', i18n.t)).toContain('Aktive Erholung')
  })

  it('optimal_medium_normal returns optimal message via fallback', () => {
    expect(recommendationText('optimal_medium_normal', i18n.t)).toContain('Sehr gute')
  })
})
