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
import usersZh from './locales/zh-CN/users.json'
import usersEn from './locales/en/users.json'
import groupsZh from './locales/zh-CN/groups.json'
import groupsEn from './locales/en/groups.json'
import rolesZh from './locales/zh-CN/roles.json'
import rolesEn from './locales/en/roles.json'
import adminClassesZh from './locales/zh-CN/adminClasses.json'
import adminClassesEn from './locales/en/adminClasses.json'
import teachingClassesZh from './locales/zh-CN/teachingClasses.json'
import teachingClassesEn from './locales/en/teachingClasses.json'
import coursesZh from './locales/zh-CN/courses.json'
import coursesEn from './locales/en/courses.json'
import scheduleConfigZh from './locales/zh-CN/scheduleConfig.json'
import scheduleConfigEn from './locales/en/scheduleConfig.json'
import timetableZh from './locales/zh-CN/timetable.json'
import timetableEn from './locales/en/timetable.json'
import attendanceZh from './locales/zh-CN/attendance.json'
import attendanceEn from './locales/en/attendance.json'
import importsZh from './locales/zh-CN/imports.json'
import importsEn from './locales/en/imports.json'
import observationsZh from './locales/zh-CN/observations.json'
import observationsEn from './locales/en/observations.json'
import repairsZh from './locales/zh-CN/repairs.json'
import repairsEn from './locales/en/repairs.json'
import logsZh from './locales/zh-CN/logs.json'
import logsEn from './locales/en/logs.json'
import settingsZh from './locales/zh-CN/settings.json'
import settingsEn from './locales/en/settings.json'
import aiChatZh from './locales/zh-CN/aiChat.json'
import aiChatEn from './locales/en/aiChat.json'

export type Language = 'zh-CN' | 'en'

const NAMESPACES = [
  'common', 'login', 'dashboard', 'classrooms', 'bookings', 'users', 'groups',
  'roles', 'adminClasses', 'teachingClasses', 'courses', 'scheduleConfig',
  'timetable', 'attendance', 'imports', 'observations', 'repairs', 'logs',
  'settings', 'aiChat',
] as const

// Static namespaces.
const resources = {
  'zh-CN': {
    common: commonZh,
    login: loginZh,
    dashboard: dashboardZh,
    classrooms: classroomsZh,
    bookings: bookingsZh,
    users: usersZh,
    groups: groupsZh,
    roles: rolesZh,
    adminClasses: adminClassesZh,
    teachingClasses: teachingClassesZh,
    courses: coursesZh,
    scheduleConfig: scheduleConfigZh,
    timetable: timetableZh,
    attendance: attendanceZh,
    imports: importsZh,
    observations: observationsZh,
    repairs: repairsZh,
    logs: logsZh,
    settings: settingsZh,
    aiChat: aiChatZh,
  },
  en: {
    common: commonEn,
    login: loginEn,
    dashboard: dashboardEn,
    classrooms: classroomsEn,
    bookings: bookingsEn,
    users: usersEn,
    groups: groupsEn,
    roles: rolesEn,
    adminClasses: adminClassesEn,
    teachingClasses: teachingClassesEn,
    courses: coursesEn,
    scheduleConfig: scheduleConfigEn,
    timetable: timetableEn,
    attendance: attendanceEn,
    imports: importsEn,
    observations: observationsEn,
    repairs: repairsEn,
    logs: logsEn,
    settings: settingsEn,
    aiChat: aiChatEn,
  },
}

const SUPPORTED_LANGUAGES: Language[] = ['zh-CN', 'en']
const FALLBACK_LANGUAGE: Language = 'zh-CN'

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng: FALLBACK_LANGUAGE,
    supportedLngs: SUPPORTED_LANGUAGES,
    lng: localStorage.getItem('ocm.lang') || undefined,
    ns: [...NAMESPACES],
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
