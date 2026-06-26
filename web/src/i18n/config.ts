import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import zh from './locales/zh.json'
import en from './locales/en.json'

export const SUPPORTED_LANGUAGES = ['zh', 'en'] as const
export type AppLanguage = (typeof SUPPORTED_LANGUAGES)[number]

export const DEFAULT_LANGUAGE: AppLanguage = 'en'

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      zh: { translation: zh },
      en: { translation: en },
    },
    supportedLngs: [...SUPPORTED_LANGUAGES],
    // Strip the region so e.g. zh-CN -> zh, en-US -> en and matches a supported
    // language; otherwise regional variants fall back to DEFAULT_LANGUAGE.
    load: 'languageOnly',
    nonExplicitSupportedLngs: true,
    fallbackLng: DEFAULT_LANGUAGE,
    detection: {
      // First visit has nothing cached, so the browser/system language
      // (navigator) decides. After the user picks a language it is cached to
      // localStorage and reused on later visits.
      order: ['localStorage', 'navigator', 'htmlTag'],
      lookupLocalStorage: 'i18nextLng',
      caches: ['localStorage'],
    },
    interpolation: { escapeValue: false },
  })

// Keep <html lang> in sync with the active language so it does not stay stuck
// on the hardcoded value in index.html (also helps screen readers / SEO).
i18n.on('languageChanged', (lng) => {
  if (typeof document !== 'undefined') {
    document.documentElement.lang = lng
  }
})

export default i18n
