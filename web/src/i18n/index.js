import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'

import commonZh from './locales/zh-CN/common.json'
import commonEn from './locales/en/common.json'
import loginZh from './locales/zh-CN/login.json'
import loginEn from './locales/en/login.json'
import dashboardZh from './locales/zh-CN/dashboard.json'
import dashboardEn from './locales/en/dashboard.json'
import classroomsZh from './locales/zh-CN/classrooms.json'
import classroomsEn from './locales/en/classrooms.json'
import bookingsZh from './locales/zh-CN/bookings.json'
import bookingsEn from './locales/en/bookings.json'

// Static namespaces. More page-level namespaces (users, roles, ...)
// are added here as they are extracted.
const resources = {
  'zh-CN': { common: commonZh, login: loginZh, dashboard: dashboardZh, classrooms: classroomsZh, bookings: bookingsZh },
  en: { common: commonEn, login: loginEn, dashboard: dashboardEn, classrooms: classroomsEn, bookings: bookingsEn },
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
    ns: ['common', 'login', 'dashboard', 'classrooms', 'bookings'],
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
