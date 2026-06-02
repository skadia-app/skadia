import type { TFunction } from 'i18next'

// Readiness recommendation text mapping.
// Code format from worker: `${level}_${confidence}_${signal_modifier|'normal'}`.
// See services/ml/analytics/readiness.py — _compute_readiness_locked.

type Level = 'critical' | 'low' | 'moderate' | 'good' | 'optimal'

const EXPLICIT: Record<string, string> = {
  optimal_high_normal: 'optimalHighNormal',
  good_high_normal: 'goodHighNormal',
  good_high_rhr_changepoint_recent: 'goodHighRhrChangepointRecent',
  critical_high_normal: 'criticalHighNormal',
}

const LEVEL_FALLBACK: Record<Level, string> = {
  critical: 'critical',
  low: 'low',
  moderate: 'moderate',
  good: 'good',
  optimal: 'optimal',
}

const LEVELS: ReadonlySet<Level> = new Set<Level>([
  'critical', 'low', 'moderate', 'good', 'optimal',
])

const CONFIDENCES: ReadonlySet<string> = new Set(['high', 'medium', 'low'])

function isLevel(s: string): s is Level {
  return LEVELS.has(s as Level)
}

export function recommendationText(code: string | null, t: TFunction): string {
  if (code === null || code === undefined || code === '') return ''

  // Explicit hit wins.
  const directKey = EXPLICIT[code]
  if (directKey) return t(`analyticsReadiness.recommendations.${directKey}`)

  // Pattern-level matches: critical_*_rhr_changepoint_recent (any confidence).
  // Parse by extracting the leading level token, the confidence token, and the
  // remainder as the modifier.
  const parts = code.split('_')
  if (parts.length < 3) return ''
  const level = parts[0]
  const confidence = parts[1]
  const modifier = parts.slice(2).join('_')

  if (!isLevel(level)) return ''
  if (!CONFIDENCES.has(confidence)) {
    // Confidence token unrecognised → still honour level fallback.
    return t(`analyticsReadiness.recommendations.${LEVEL_FALLBACK[level]}`)
  }

  // Special pattern: critical with rhr changepoint at any confidence.
  if (level === 'critical' && modifier === 'rhr_changepoint_recent') {
    return t('analyticsReadiness.recommendations.criticalRhrChangepointRecent')
  }

  // Generic level fallback (covers moderate/low any confidence + normal,
  // and any other level/confidence combo without an explicit entry).
  return t(`analyticsReadiness.recommendations.${LEVEL_FALLBACK[level]}`)
}
