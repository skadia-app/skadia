import { afterEach, describe, expect, it } from 'vitest'
import { i18n } from '@/i18n/i18n'
import { DROPDOWN_LABELS, METRIC_LABELS, MODE_LABELS } from '../colorMetricLabels'

describe('colorMetricLabels i18n', () => {
  afterEach(async () => {
    await i18n.changeLanguage('de')
  })

  it('resolves metric and dropdown labels from the active locale', async () => {
    await i18n.changeLanguage('en')

    expect(METRIC_LABELS.hr).toBe('Heart rate')
    expect(DROPDOWN_LABELS.chooseColoring).toBe('Choose coloring')

    await i18n.changeLanguage('fr')

    expect(METRIC_LABELS.hr).toBe('Fréquence cardiaque')
    expect(MODE_LABELS.linear).toBe('Linéaire')
  })
})
