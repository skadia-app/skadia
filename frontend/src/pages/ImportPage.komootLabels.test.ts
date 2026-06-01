import { describe, expect, it } from 'vitest'

import { i18n } from '@/i18n/i18n'
import {
  importCategoryLabel,
  importConflictStrategyOptions,
  importFoundLabel,
  importJobStatusLabel,
  komootAvailabilityText,
  komootCompletionLog,
  komootPhaseLabel,
  komootProgressLabel,
} from './ImportPage'

describe('Komoot import labels', () => {
  it('uses route wording for route-only imports', () => {
    const selection = { importRecorded: false, importPlanned: true, importPhotos: false }

    expect(komootProgressLabel(129, 129, selection)).toBe('Route 129 / 129')
    expect(komootCompletionLog(129, selection)).toBe('Abgeschlossen — 129 Routen importiert')
    expect(komootAvailabilityText(selection)).toBe('Die importierten Routen sind nun verfügbar.')
  })

  it('localizes route-only progress and phases in English', async () => {
    await i18n.changeLanguage('en')
    const t = i18n.t.bind(i18n)
    const selection = { importRecorded: false, importPlanned: true, importPhotos: false }

    expect(komootProgressLabel(129, 129, selection, t)).toBe('Route 129 / 129')
    expect(komootCompletionLog(129, selection, t)).toBe('Completed — 129 routes imported')
    expect(komootAvailabilityText(selection, t)).toBe('The imported routes are now available.')
    expect(komootPhaseLabel('import', t)).toBe('Import')
    expect(komootPhaseLabel('unknown_backend_phase', t)).toBe('Unknown phase')
  })
})

describe('generic import labels', () => {
  it('localizes import categories, counts, strategies and job statuses in English', async () => {
    await i18n.changeLanguage('en')
    const t = i18n.t.bind(i18n)

    expect(importCategoryLabel('gear', t)).toBe('Gear')
    expect(importCategoryLabel('new_backend_category', t)).toBe('Unknown import category')
    expect(importFoundLabel(2, t)).toBe('2 found')
    expect(importJobStatusLabel('completed', t)).toBe('Completed')
    expect(importConflictStrategyOptions(t).map(option => option.value)).toEqual([
      'enrich_missing',
      'replace_all',
      'skip',
    ])
    expect(importConflictStrategyOptions(t)[0].label).toBe('Fill missing fields')
  })

  it('uses a localized generic label for unknown import job statuses', async () => {
    await i18n.changeLanguage('en')
    const t = i18n.t.bind(i18n)

    expect(importJobStatusLabel('private_backend_status', t)).toBe('Unknown status')
  })
})
