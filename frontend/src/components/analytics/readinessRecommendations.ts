// Readiness recommendation text mapping (DE).
// Code format from worker: `${level}_${confidence}_${signal_modifier|'normal'}`.
// See services/ml/analytics/readiness.py — _compute_readiness_locked.

type Level = 'critical' | 'low' | 'moderate' | 'good' | 'optimal'

const EXPLICIT: Record<string, string> = {
  optimal_high_normal:
    'Sehr gute Bereitschaft. Intensive Einheiten oder Wettkampf möglich.',
  good_high_normal:
    'Gute Bereitschaft. Planmäßige Einheit empfehlenswert.',
  good_high_rhr_changepoint_recent:
    'Gute Bereitschaft, aber Ruhepuls hat sich kürzlich verändert — auf zusätzliche Erholung achten.',
  critical_high_normal:
    'Bereitschaft kritisch. Ruhetag oder sehr lockere Bewegung.',
}

const LEVEL_FALLBACK: Record<Level, string> = {
  critical: 'Bereitschaft kritisch. Ruhetag oder sehr lockere Bewegung.',
  low: 'Bereitschaft niedrig. Aktive Erholung empfohlen.',
  moderate: 'Moderate Bereitschaft. Lockere Einheit empfehlenswert.',
  good: 'Gute Bereitschaft. Planmäßige Einheit empfehlenswert.',
  optimal: 'Sehr gute Bereitschaft. Intensive Einheiten oder Wettkampf möglich.',
}

const LEVELS: ReadonlySet<Level> = new Set<Level>([
  'critical', 'low', 'moderate', 'good', 'optimal',
])

const CONFIDENCES: ReadonlySet<string> = new Set(['high', 'medium', 'low'])

function isLevel(s: string): s is Level {
  return LEVELS.has(s as Level)
}

export function recommendationText(code: string | null): string {
  if (code === null || code === undefined || code === '') return ''

  // Explicit hit wins.
  const direct = EXPLICIT[code]
  if (direct) return direct

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
    return LEVEL_FALLBACK[level]
  }

  // Special pattern: critical with rhr changepoint at any confidence.
  if (level === 'critical' && modifier === 'rhr_changepoint_recent') {
    return 'Bereitschaft kritisch, Ruhepuls auffällig. Ruhetag dringend empfohlen.'
  }

  // Generic level fallback (covers moderate/low any confidence + normal,
  // and any other level/confidence combo without an explicit entry).
  return LEVEL_FALLBACK[level]
}
