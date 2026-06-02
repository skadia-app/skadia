import { i18n } from './i18n'
import { isSupportedLanguage, type SupportedLanguage } from './languages'

export type LocalizedString = Partial<Record<SupportedLanguage, string>> & { de: string }

export function supportedLanguageFrom(value: string | undefined): SupportedLanguage {
  const base = value?.split('-')[0] ?? ''
  return isSupportedLanguage(base) ? base : 'de'
}

export function activeLanguage(): SupportedLanguage {
  return supportedLanguageFrom(i18n.language)
}

export function localizeString(value: LocalizedString | undefined, language?: SupportedLanguage): string {
  if (!value) return ''
  const lang = language ?? activeLanguage()
  return value[lang] || value.de
}
