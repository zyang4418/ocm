import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'

import commonZh from './locales/zh-CN/common.json'
import commonEn from './locales/en/common.json'

// Static namespaces for the first phase. More page-level namespaces (login,
// dashboard, classrooms, ...) are added here as they are extracted.
const resources = {
  'zh-CN': { common: commonZh },
  en: { common: commonEn },
}

const SUPPORTED_LANGUAGES = ['zh-CN', 'en']
const FALLBACK_LANGUAGE = 'zh-CN'

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng: FALLBACK_LANGUAGE,
    supportedLngs: SUPPORTED_LANGUAGES,
    lng: localStorage.getItem('ocm.lang') || undefined,
    ns: ['common'],
    defaultNS: 'common',
    interpolation: {
      escapeValue: false, // React already escapes
    },
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
      lookupLocalStorage: 'ocm.lang',
    },
  })

export default i18n
export { SUPPORTED_LANGUAGES, FALLBACK_LANGUAGE }
