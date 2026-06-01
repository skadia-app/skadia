import { afterEach, describe, expect, it } from 'vitest'
import { i18n } from '@/i18n/i18n'
import { USAGE_METRIC_LABELS, formatUsageMetricValue } from './gearUsageMetrics'

describe('gearUsageMetrics', () => {
  afterEach(async () => {
    await i18n.changeLanguage('de')
  })

  it('localizes metric labels without changing stable metric keys', async () => {
    await i18n.changeLanguage('en')

    expect(USAGE_METRIC_LABELS.distance_m).toBe('Distance')
    expect(USAGE_METRIC_LABELS.elapsed_days).toBe('Days since service')
  })

  it('formats metric values with the active language number format', async () => {
    await i18n.changeLanguage('de')
    expect(formatUsageMetricValue('distance_m', 1500)).toBe('1,5 km')

    await i18n.changeLanguage('en')
    expect(formatUsageMetricValue('distance_m', 1500)).toBe('1.5 km')
  })

  it('formats stored SI distance and elevation values with imperial display units when requested', async () => {
    await i18n.changeLanguage('en')

    expect(formatUsageMetricValue('distance_m', 1609.344, { locale: 'en-US', units: 'imperial' })).toBe('1.0 mi')
    expect(formatUsageMetricValue('elevation_gain_m', 100, { locale: 'en-US', units: 'imperial' })).toBe('328 ft')
  })
})
