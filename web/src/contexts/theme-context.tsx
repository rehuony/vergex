import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from 'react'

const STORAGE_KEY = 'vergex-theme'

export type ThemePreference = 'system' | 'light' | 'dark'
export type Theme = 'light' | 'dark'

export const THEME_OPTIONS: ThemePreference[] = ['system', 'light', 'dark']

// Fired on window after the page switches theme, so imperative widgets such as
// the charts can repaint with the new colors.
export const THEME_EVENT = 'vergex-theme-change'

function isPreference(value: string | null): value is ThemePreference {
  return THEME_OPTIONS.some((option) => option === value)
}

function readPreference(): ThemePreference {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    return isPreference(saved) ? saved : 'system'
  } catch {
    return 'system'
  }
}

const darkQuery = () =>
  typeof window.matchMedia === 'function'
    ? window.matchMedia('(prefers-color-scheme: dark)')
    : null

function resolve(preference: ThemePreference): Theme {
  if (preference !== 'system') return preference
  return darkQuery()?.matches ? 'dark' : 'light'
}

/** Applies a theme to <html>; index.html runs the same logic before React. */
export function applyTheme(theme: Theme) {
  const root = document.documentElement
  if (root.dataset.theme === theme) return
  root.dataset.theme = theme
  root.style.colorScheme = theme
  window.dispatchEvent(new Event(THEME_EVENT))
}

interface ThemeContextType {
  preference: ThemePreference
  theme: Theme
  setPreference: (preference: ThemePreference) => void
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined)

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreferenceState] =
    useState<ThemePreference>(readPreference)
  const [theme, setTheme] = useState<Theme>(() => resolve(preference))

  useEffect(() => {
    const update = () => setTheme(resolve(preference))
    update()
    if (preference !== 'system') return
    const query = darkQuery()
    query?.addEventListener('change', update)
    return () => query?.removeEventListener('change', update)
  }, [preference])

  useEffect(() => applyTheme(theme), [theme])

  const setPreference = (next: ThemePreference) => {
    setPreferenceState(next)
    try {
      localStorage.setItem(STORAGE_KEY, next)
    } catch {
      /* the choice still applies for this visit */
    }
  }

  return (
    <ThemeContext.Provider value={{ preference, theme, setPreference }}>
      {children}
    </ThemeContext.Provider>
  )
}

export function useTheme() {
  const context = useContext(ThemeContext)
  if (!context) {
    throw new Error('useTheme must be used within ThemeProvider')
  }
  return context
}
