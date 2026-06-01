import { describe, expect, it } from 'vitest'
import { i18n, syncDocumentLanguage } from './i18n'

describe('document language synchronization', () => {
  it('updates the html lang attribute when i18n changes language', async () => {
    await i18n.changeLanguage('fr')

    expect(document.documentElement.lang).toBe('fr')
  })

  it('falls back to German for unsupported language tags', () => {
    syncDocumentLanguage('pt-BR')

    expect(document.documentElement.lang).toBe('de')
  })
})
