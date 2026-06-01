import { describe, expect, it, vi } from 'vitest'
import { formatFactorName } from './CorrelationsPanel'
import { formatProjectionMetricLabel } from './FitnessProjection'
import { formatWarningType } from './WarningsPanel'

const t = vi.fn((key: string) => key)

describe('analytics insight fallback labels', () => {
  it('uses structured warning type labels and hides unknown warning codes behind a generic label', () => {
    expect(formatWarningType('high_acwr', t)).toBe('analyticsHealthInsights.warnings.types.highAcwr')
    expect(formatWarningType('new_warning_code', t)).toBe('analyticsHealthInsights.warnings.types.unknown')
  })

  it('uses structured factor labels and hides unknown factor codes behind a generic label', () => {
    expect(formatFactorName('sleep_score', t)).toBe('analyticsHealthInsights.factorNames.sleepScore')
    expect(formatFactorName('custom_factor_code', t)).toBe('common.unknown')
  })

  it('uses structured projection metric labels and hides unknown metric codes behind a generic label', () => {
    expect(formatProjectionMetricLabel('distance', t)).toBe('analyticsHealthInsights.factorNames.distance')
    expect(formatProjectionMetricLabel('novel_metric_code', t)).toBe('common.unknown')
  })
})
