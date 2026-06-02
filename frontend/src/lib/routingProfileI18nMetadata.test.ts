import { describe, expect, it } from 'vitest'
import { resources } from '@/i18n/resources'
import { FIELD_METADATA, SECTION_LABEL_KEYS } from './routingProfiles'
import { PROFILE_LABELS, PROFILE_ORDER } from './profileLabels'

function catalogValue(path: string): unknown {
  return path.split('.').reduce<unknown>((node, key) => {
    if (!node || typeof node !== 'object') return undefined
    return (node as Record<string, unknown>)[key]
  }, resources.en)
}

function expectStringKey(path: string): void {
  expect(catalogValue(path), path).toEqual(expect.any(String))
}

describe('routing profile i18n metadata', () => {
  it('uses catalog keys instead of visible field labels and hints', () => {
    for (const field of FIELD_METADATA) {
      expect(field).not.toHaveProperty('label')
      expect(field.labelKey).toBe(`routePlanner.profileEditor.fields.${field.key}.label`)
      expectStringKey(field.labelKey)

      if (field.hint) {
        expect(field.hint).toBe(true)
        expectStringKey(`routePlanner.profileEditor.fields.${field.key}.hint`)
      }
    }
  })

  it('uses catalog keys for section labels', () => {
    expect(SECTION_LABEL_KEYS).toEqual({
      elevation: 'routePlanner.profileEditor.sections.elevation',
      difficulty: 'routePlanner.profileEditor.sections.difficulty',
      environment: 'routePlanner.profileEditor.sections.environment',
      paths: 'routePlanner.profileEditor.sections.paths',
      behavior: 'routePlanner.profileEditor.sections.behavior',
    })

    for (const key of Object.values(SECTION_LABEL_KEYS)) {
      expectStringKey(key)
    }
  })

  it('uses catalog keys for profile labels and descriptions', () => {
    for (const profile of PROFILE_ORDER) {
      const labels = PROFILE_LABELS[profile]
      expect(labels).toEqual({
        labelKey: `routePlanner.profileEditor.profiles.${profile}.label`,
        descriptionKey: `routePlanner.profileEditor.profiles.${profile}.description`,
      })
      expectStringKey(labels.labelKey)
      expectStringKey(labels.descriptionKey)
    }
  })
})
