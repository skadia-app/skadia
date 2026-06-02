import { describe, expect, it } from 'vitest'
import { i18n } from '@/i18n/i18n'
import { dateFnsLocaleForLanguage, fmtDateCompact, fmtPower } from './format'

describe('format helpers locale defaults', () => {
  it('uses the active i18n language when no explicit locale is supplied', async () => {
    await i18n.changeLanguage('en')

    expect(fmtDateCompact('2026-05-31T12:00:00Z', 'UTC')).toBe('05/31/2026')
    expect(fmtPower(1234.6)).toBe('1,235 W')
  })

  it('resolves date-fns locales from supported app languages', () => {
    expect(dateFnsLocaleForLanguage('en').code).toBe('en-US')
    expect(dateFnsLocaleForLanguage('de').code).toBe('de')
  })
})
