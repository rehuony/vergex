import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from 'react'
import type { Language } from '@/i18n/translations'

const STORAGE_KEY = 'vergex-language'

// The other languages are unfinished, so the UI stays in English and the
// account menu has no Language entry. Turn this on to bring back the saved
// choice, the browser language and the menu.
export const LANGUAGE_CHOICE_ENABLED = false

// Each option is labelled in its own language so it can be found from any UI.
export const LANGUAGE_OPTIONS: { code: Language; label: string }[] = [
  { code: 'en', label: 'English' },
  { code: 'zh', label: '简体中文' },
  { code: 'id', label: 'Bahasa Indonesia' },
]

function isLanguage(value: string | null): value is Language {
  return LANGUAGE_OPTIONS.some((option) => option.code === value)
}

// A saved choice wins; otherwise follow the browser.
function initialLanguage(): Language {
  if (!LANGUAGE_CHOICE_ENABLED) return 'en'
  const saved = localStorage.getItem(STORAGE_KEY)
  if (isLanguage(saved)) return saved
  const browser = navigator.language.toLowerCase()
  if (browser.startsWith('zh')) return 'zh'
  if (browser.startsWith('id')) return 'id'
  return 'en'
}

interface LanguageContextType {
  language: Language
  setLanguage: (lang: Language) => void
}

const LanguageContext = createContext<LanguageContextType | undefined>(
  undefined
)

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [language, setLanguageState] = useState<Language>(initialLanguage)

  useEffect(() => {
    document.documentElement.lang = language === 'zh' ? 'zh-CN' : language
  }, [language])

  const setLanguage = (lang: Language) => {
    setLanguageState(lang)
    localStorage.setItem(STORAGE_KEY, lang)
  }

  return (
    <LanguageContext.Provider value={{ language, setLanguage }}>
      {children}
    </LanguageContext.Provider>
  )
}

export function useLanguage() {
  const context = useContext(LanguageContext)
  if (!context) {
    throw new Error('useLanguage must be used within LanguageProvider')
  }
  return context
}
