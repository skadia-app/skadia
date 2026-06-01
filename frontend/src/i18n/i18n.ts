import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import { resources } from './resources'

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

export { i18n }
