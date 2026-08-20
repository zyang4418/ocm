import type { ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'
import i18n from './index'

// LanguageProvider makes the i18next instance available to the whole React tree.
// The actual language state lives in i18next itself; this component is a thin
// wrapper so the provider can be placed next to AuthProvider in App.
export default function LanguageProvider({ children }: { children: ReactNode }) {
  return <I18nextProvider i18n={i18n}>{children}</I18nextProvider>
}
