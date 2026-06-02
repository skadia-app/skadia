import { describe, expect, it } from 'vitest'
import { i18n } from '@/i18n/i18n'
import { analysisModuleDescriptionKeyForScope, analysisModuleLabelKeyForScope } from '../ScopeCheckboxList'

describe('ScopeCheckboxList module labels', () => {
  it('maps known scope keys to settings analysis modules', () => {
    expect(analysisModuleLabelKeyForScope('auto_title')).toBe('settingsPage.analysis.modules.auto_naming.label')
    expect(analysisModuleDescriptionKeyForScope('auto_title')).toBe('settingsPage.analysis.modules.auto_naming.description')
    expect(analysisModuleLabelKeyForScope('summits')).toBe('settingsPage.analysis.modules.summits.label')
  })

  it('uses localized unknown module labels instead of exposing raw scope keys', async () => {
    expect(analysisModuleLabelKeyForScope('private_backend_scope')).toBe('settingsPage.analysis.modules.unknown.label')
    expect(analysisModuleDescriptionKeyForScope('private_backend_scope')).toBe('settingsPage.analysis.modules.unknown.description')

    for (const locale of ['de', 'en', 'fr', 'it', 'es']) {
      await i18n.changeLanguage(locale)
      expect(i18n.exists('settingsPage.analysis.modules.unknown.label')).toBe(true)
      expect(i18n.exists('settingsPage.analysis.modules.unknown.description')).toBe(true)
      expect(i18n.t(analysisModuleLabelKeyForScope('private_backend_scope'))).not.toContain('private_backend_scope')
      expect(i18n.t(analysisModuleDescriptionKeyForScope('private_backend_scope'))).not.toContain('private_backend_scope')
    }
    await i18n.changeLanguage('de')
  })
})
