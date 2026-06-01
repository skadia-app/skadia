import { generateActivityFilterOptions, generateMapFilterOptions, type SportOption } from '@/lib/sportOptions'
import type { SupportedLanguage } from '@/i18n/languages'

export type SportFilterPreset = 'MAP_PANEL_GPS' | 'ACTIVITY_ALL' | 'RECORDS_RECORDABLE'

const RECORDABLE_SPORT_TYPES = new Set(['running', 'cycling', 'hiking', 'swimming'])

function filterRecordable(options: SportOption[]): SportOption[] {
  return options.flatMap((opt) => {
    if (opt.children) {
      const children = opt.children.filter((child) => RECORDABLE_SPORT_TYPES.has(child.value))
      return children.length > 0 ? [{ ...opt, children }] : []
    }
    return RECORDABLE_SPORT_TYPES.has(opt.value) ? [opt] : []
  })
}

export function sportFilterOptionsForPreset(preset: SportFilterPreset, language?: SupportedLanguage): SportOption[] {
  if (preset === 'MAP_PANEL_GPS') return generateMapFilterOptions(language)
  if (preset === 'RECORDS_RECORDABLE') return filterRecordable(generateActivityFilterOptions(language))
  return generateActivityFilterOptions(language)
}
