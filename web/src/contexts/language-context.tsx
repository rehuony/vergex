import { createContext, useContext, useState, type ReactNode } from 'react'
import { DEFAULT_LANGUAGE, LANGUAGES, type Language } from '@/i18n'

const STORAGE_KEY = 'vergex-language'

function isLanguage(value: string | null): value is Language {
  return LANGUAGES.some((option) => option.code === value)
}

// A saved choice wins; otherwise follow the browser.
function initialLanguage(): Language {
  const saved = localStorage.getItem(STORAGE_KEY)
  if (isLanguage(saved)) return saved
  return navigator.language.toLowerCase().startsWith('zh')
    ? 'zh-CN'
    : DEFAULT_LANGUAGE
}

interface LanguageContextType {
  language: Language
  setLanguage: (lang: Language) => void
}

// Outside a provider, as in a component tested alone, the page is in the default.
const LanguageContext = createContext<LanguageContextType>({
  language: DEFAULT_LANGUAGE,
  setLanguage: () => {},
})

export function LanguageProvider({ children }: { children: ReactNode }) {
  // The page's language is set before anything renders, since dates are written in it.
  const [language, setLanguageState] = useState<Language>(() => {
    const initial = initialLanguage()
    document.documentElement.lang = initial
    return initial
  })

  const setLanguage = (lang: Language) => {
    document.documentElement.lang = lang
    localStorage.setItem(STORAGE_KEY, lang)
    setLanguageState(lang)
  }

  return (
    <LanguageContext.Provider value={{ language, setLanguage }}>
      {children}
    </LanguageContext.Provider>
  )
}

export function useLanguage() {
  return useContext(LanguageContext)
}
