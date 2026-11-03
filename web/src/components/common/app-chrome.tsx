import { type ReactNode, Suspense, lazy, useState } from 'react'
import { useLocation, useSearchParams } from 'react-router-dom'
import { HeaderBar } from './header-bar'
import { LoginRequiredOverlay } from '@/components/auth/login-required-overlay'
import {
  isSettingsSection,
  type SettingsSection,
} from '@/components/settings/settings-sections'
import { useOpenSettings } from '@/components/settings/use-open-settings'
import { SETTINGS_PARAM, type Page } from '@/constants/route-paths'
import { useAuth } from '@/contexts/auth-context'
import { useLanguage } from '@/contexts/language-context'
import { useTheme } from '@/contexts/theme-context'
import { useProfile } from '@/hooks/use-profile'

// Settings open over any page, only when asked, so their code loads apart from the page's.
const SettingsDialog = lazy(() =>
  import('@/components/settings/settings-dialog').then((module) => ({
    default: module.SettingsDialog,
  }))
)

interface AppChromeProps {
  children: ReactNode
  currentPage?: Page
  animateContent?: boolean
  extraContent?: ReactNode
}

export function AppChrome({
  children,
  currentPage,
  animateContent = false,
  extraContent,
}: AppChromeProps) {
  const location = useLocation()
  const { language, setLanguage } = useLanguage()
  const { preference: themePreference, setPreference: setThemePreference } =
    useTheme()
  const { user, logout } = useAuth()
  const { data: profile } = useProfile()
  const [loginOverlayOpen, setLoginOverlayOpen] = useState(false)
  const [loginOverlayFeature, setLoginOverlayFeature] = useState('')
  const [searchParams] = useSearchParams()
  const showSettings = useOpenSettings()
  const requestedSection = searchParams.get(SETTINGS_PARAM)
  const settingsSection: SettingsSection | null =
    !user || requestedSection === null
      ? null
      : isSettingsSection(requestedSection)
        ? requestedSection
        : 'profile'

  const handleLoginRequired = (featureName: string) => {
    setLoginOverlayFeature(featureName)
    setLoginOverlayOpen(true)
  }

  // Settings float over the page but live in its URL; opening them or
  // switching their tabs must not remount (and reload) the page underneath.
  const pageSearch = new URLSearchParams(location.search)
  pageSearch.delete(SETTINGS_PARAM)
  const content = animateContent ? (
    <div key={`${location.pathname}?${pageSearch}`} className="animate-page-in">
      {children}
    </div>
  ) : (
    children
  )

  return (
    <div className="vergex-shell min-h-screen bg-vergex-canvas text-vergex-ink">
      <HeaderBar
        isLoggedIn={!!user}
        currentPage={currentPage}
        language={language}
        onLanguageChange={setLanguage}
        themePreference={themePreference}
        onThemeChange={setThemePreference}
        user={
          user && {
            email: user.email,
            displayName: profile?.display_name,
            avatar: profile?.avatar,
          }
        }
        onLogout={logout}
        onLoginRequired={handleLoginRequired}
        onOpenSettings={() => showSettings('profile')}
      />

      <main className="min-h-screen pt-[72px]">
        {/* A page's code loads when it is first opened, under the top bar. */}
        <Suspense fallback={null}>{content}</Suspense>
      </main>

      {settingsSection && (
        <Suspense fallback={null}>
          <SettingsDialog
            section={settingsSection}
            onSectionChange={showSettings}
            onClose={() => showSettings(null)}
          />
        </Suspense>
      )}

      <LoginRequiredOverlay
        isOpen={loginOverlayOpen}
        onClose={() => setLoginOverlayOpen(false)}
        featureName={loginOverlayFeature}
      />

      {extraContent}
    </div>
  )
}
