import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { resources } from '@/i18n/resources'
import type { SupportedLanguage } from '@/i18n/languages'

const frontendRoot = resolve(__dirname, '../../..')

function readFrontendSource(path: string): string {
  return readFileSync(resolve(frontendRoot, path), 'utf8')
}

function getTranslation(path: string, language: SupportedLanguage): string | undefined {
  let current: unknown = resources[language]
  for (const segment of path.split('.')) {
    if (!current || typeof current !== 'object' || !(segment in current)) return undefined
    current = (current as Record<string, unknown>)[segment]
  }
  return typeof current === 'string' ? current : undefined
}

describe('admin label fallbacks', () => {
  it('does not expose raw backend label tokens as translation fallbacks', () => {
    const files = [
      'src/components/AdminLibrarySettingsTab.tsx',
      'src/components/AdminStorageDashboard.tsx',
      'src/pages/admin/AdminAuditLogPage.tsx',
    ]

    for (const file of files) {
      const source = readFrontendSource(file)
      expect(source).not.toContain('defaultValue: type')
      expect(source).not.toContain('defaultValue: eventType')
      expect(source).not.toContain('?? fileType')
    }
  })

  it('defines localized unknown labels for admin file types and audit events', () => {
    const languages = Object.keys(resources) as SupportedLanguage[]

    for (const language of languages) {
      expect(getTranslation('admin.storage.fileTypes.unknown', language)).toBeTruthy()
      expect(getTranslation('adminAuditLog.events.unknown', language)).toBeTruthy()
    }
  })
})
