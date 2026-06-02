import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { RoutingApplicableModifierKeys } from '@/lib/generatedRoutingApplicableModifiers'
import { resources, type TranslationTree } from '@/i18n/resources'
import type { SupportedLanguage } from '@/i18n/languages'

const routesPageSource = readFileSync(
  join(process.cwd(), 'src/pages/RoutesPage.tsx'),
  'utf8',
)

describe('RoutesPage modifier labels', () => {
  it('does not use component-local defaultValue fallbacks for route modifier labels', () => {
    expect(routesPageSource).not.toMatch(/routesPage\.modifiers\.[^`'"]+[`'"],\s*\{\s*defaultValue:/)
  })

  it('has localized labels for every routing modifier in every locale catalog', () => {
    for (const language of Object.keys(resources) as SupportedLanguage[]) {
      const routesPage = resources[language].routesPage as TranslationTree | undefined
      const modifiers = routesPage?.modifiers
      expect(modifiers, `${language} routesPage.modifiers`).toBeTypeOf('object')
      const modifierLabels = modifiers as TranslationTree

      for (const key of RoutingApplicableModifierKeys) {
        expect(
          modifierLabels[key],
          `${language} routesPage.modifiers.${key}`,
        ).toBeTypeOf('string')
      }
    }
  })
})
