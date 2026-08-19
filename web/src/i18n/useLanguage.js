import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { SUPPORTED_LANGUAGES } from './index.js'

const STORAGE_KEY = 'ocm.lang'

// useLanguage exposes the active language and a setter that persists the
// choice to localStorage. In the future it can also POST to a user-preference
// endpoint after login.
export default function useLanguage() {
  const { i18n } = useTranslation()

  const setLanguage = useCallback(
    (lang) => {
      if (!SUPPORTED_LANGUAGES.includes(lang)) return
      i18n.changeLanguage(lang)
      localStorage.setItem(STORAGE_KEY, lang)
    },
    [i18n],
  )

  return {
    language: i18n.language,
    languages: SUPPORTED_LANGUAGES,
    setLanguage,
    isSupported: (lang) => SUPPORTED_LANGUAGES.includes(lang),
  }
}
