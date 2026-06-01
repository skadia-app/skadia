export const SUPPORTED_LANGUAGES = ['de', 'en', 'fr', 'it', 'es'] as const

export type SupportedLanguage = typeof SUPPORTED_LANGUAGES[number]

export interface LanguageOption {
  code: SupportedLanguage
  locale: string
  label: string
  nativeLabel: string
  flag: string
}

export const LANGUAGE_OPTIONS: LanguageOption[] = [
  { code: 'de', locale: 'de-DE', label: 'German', nativeLabel: 'Deutsch', flag: '🇩🇪' },
  { code: 'en', locale: 'en-US', label: 'English', nativeLabel: 'English', flag: '🇺🇸' },
  { code: 'fr', locale: 'fr-FR', label: 'French', nativeLabel: 'Français', flag: '🇫🇷' },
  { code: 'it', locale: 'it-IT', label: 'Italian', nativeLabel: 'Italiano', flag: '🇮🇹' },
  { code: 'es', locale: 'es-ES', label: 'Spanish', nativeLabel: 'Español', flag: '🇪🇸' },
]

export function isSupportedLanguage(value: string): value is SupportedLanguage {
  return (SUPPORTED_LANGUAGES as readonly string[]).includes(value)
}

export function localeForLanguage(language: SupportedLanguage): string {
  return LANGUAGE_OPTIONS.find(option => option.code === language)?.locale ?? 'de-DE'
}
