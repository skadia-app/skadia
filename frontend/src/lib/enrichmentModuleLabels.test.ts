import { describe, expect, it } from 'vitest'
import { i18n } from '@/i18n/i18n'
import { getEnrichmentModuleLabel } from './enrichmentModuleLabels'

describe('getEnrichmentModuleLabel', () => {
  it('keeps backend enrichment module ids out of visible German labels', async () => {
    await i18n.changeLanguage('de')

    expect(getEnrichmentModuleLabel('summits')).toBe('Gipfel')
    expect(getEnrichmentModuleLabel('embeddings')).toBe('Ähnlichkeitsindex')
    expect(getEnrichmentModuleLabel('summits')).not.toBe('summits')
  })

  it('localizes backend enrichment module ids in English', async () => {
    await i18n.changeLanguage('en')

    expect(getEnrichmentModuleLabel('settlements')).toBe('Settlements')
    expect(getEnrichmentModuleLabel('embeddings')).toBe('Similarity index')
    expect(getEnrichmentModuleLabel('settlements')).not.toBe('settlements')
  })
})
