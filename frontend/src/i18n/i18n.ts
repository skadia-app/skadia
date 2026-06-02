import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import { resources } from './resources'
import { isSupportedLanguage } from './languages'

export function syncDocumentLanguage(language: string): void {
  if (typeof document === 'undefined') return

  const baseLanguage = language.split('-')[0] ?? ''
  document.documentElement.lang = isSupportedLanguage(baseLanguage) ? baseLanguage : 'de'
}

void i18n
  .use(initReactI18next)
  .init({
    fallbackLng: 'de',
    lng: 'de',
    interpolation: {
      escapeValue: false,
    },
    resources: {
      de: { translation: resources.de },
      en: { translation: resources.en },
      fr: { translation: resources.fr },
      it: { translation: resources.it },
      es: { translation: resources.es },
    },
  })
  .then(() => syncDocumentLanguage(i18n.language))

i18n.on('languageChanged', syncDocumentLanguage)

export { i18n }
