import { type ComponentType, type ReactNode, lazy, useEffect } from 'react'
import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { ROUTES } from '@/constants/route-paths'
import { useAuth } from '@/contexts/auth-context'
import { useSystemConfig } from '@/hooks/use-system-config'
import { LoginPage } from '@/pages/login-page'
import { ResetPasswordPage } from '@/pages/reset-password-page'
import { ResetUserPage } from '@/pages/reset-user-page'
import { SetupPage } from '@/pages/setup-page'
import { DashboardPage } from '@/pages/dashboard-page'
import { AppChrome } from '@/components/common/app-chrome'
import { AssistantDock } from '@/components/assistant/assistant-dock'
import { LoadingScreen } from '@/components/common/loading-screen'

// The Dashboard is the first screen, so only it and the sign-in pages come with the
// first download. Every other page loads on its own, and once the first screen has
// settled the browser fetches them all, so opening one later does not wait.
const later: (() => Promise<unknown>)[] = [
  () => import('@/components/settings/settings-dialog'),
]
const FETCH_LATER_MS = 3000

function page<T>(load: () => Promise<T>, pick: (module: T) => ComponentType) {
  later.push(load)
  return lazy(() => load().then((module) => ({ default: pick(module) })))
}

const TradersPage = page(
  () => import('@/pages/traders-page'),
  (module) => module.TradersPage
)
const TradersOverviewPage = page(
  () => import('@/pages/traders-overview-page'),
  (module) => module.TradersOverviewPage
)
const FuturesPage = page(
  () => import('@/pages/futures-page'),
  (module) => module.FuturesPage
)
const ForecastPage = page(
  () => import('@/pages/forecast-page'),
  (module) => module.ForecastPage
)
const ForecastStrategiesPage = page(
  () => import('@/pages/forecast-strategies-page'),
  (module) => module.ForecastStrategiesPage
)
const ForecastBacktestsPage = page(
  () => import('@/pages/forecast-backtests-page'),
  (module) => module.ForecastBacktestsPage
)
const ForecastSettingsPage = page(
  () => import('@/pages/forecast-settings-page'),
  (module) => module.ForecastSettingsPage
)
const FAQPage = page(
  () => import('@/pages/faq-page'),
  (module) => module.FAQPage
)

function LoginRedirect({ returnUrl }: { returnUrl?: string }) {
  useEffect(() => {
    if (returnUrl) sessionStorage.setItem('returnUrl', returnUrl)
  }, [returnUrl])
  return <Navigate to={ROUTES.login} replace />
}

export function AppRoutes() {
  const { user, token, isLoading } = useAuth()
  const { config: systemConfig, loading: configLoading } = useSystemConfig()
  const { pathname } = useLocation()
  useEffect(() => {
    window.scrollTo({ top: 0, left: 0, behavior: 'instant' })
  }, [pathname])
  const isAuthenticated = !!user && !!token
  useEffect(() => {
    if (!isAuthenticated) return
    const timer = setTimeout(
      () => later.forEach((load) => void load().catch(() => {})),
      FETCH_LATER_MS
    )
    return () => clearTimeout(timer)
  }, [isAuthenticated])
  const guard = (path: string, element: ReactNode) =>
    isAuthenticated ? element : <LoginRedirect returnUrl={path} />

  // The config only chooses between setup and sign-in, so whoever is signed in goes
  // straight to their page rather than waiting for it.
  if (isLoading || (configLoading && !isAuthenticated)) {
    return <LoadingScreen />
  }

  if (systemConfig && !systemConfig.initialized && !user) {
    return <SetupPage />
  }

  return (
    <>
      <Routes>
        <Route
          path={ROUTES.home}
          element={
            <Navigate
              to={isAuthenticated ? ROUTES.dashboard : ROUTES.login}
              replace
            />
          }
        />
        <Route
          path={ROUTES.login}
          element={
            isAuthenticated ? (
              <Navigate to={ROUTES.dashboard} replace />
            ) : (
              <LoginPage />
            )
          }
        />
        <Route path={ROUTES.resetPassword} element={<ResetPasswordPage />} />
        <Route path={ROUTES.resetUser} element={<ResetUserPage />} />
        <Route
          path={ROUTES.setup}
          element={
            user ? (
              <Navigate to={ROUTES.dashboard} replace />
            ) : systemConfig?.initialized ? (
              <Navigate to={ROUTES.login} replace />
            ) : (
              <SetupPage />
            )
          }
        />
        <Route
          path={ROUTES.dashboard}
          element={guard(ROUTES.dashboard, <DashboardPage />)}
        />
        <Route
          path={ROUTES.futures}
          element={guard(
            ROUTES.futures,
            <AppChrome currentPage="futures">
              <FuturesPage />
            </AppChrome>
          )}
        />
        <Route
          path={ROUTES.traders}
          element={guard(
            ROUTES.traders,
            <AppChrome currentPage="traders">
              <TradersPage />
            </AppChrome>
          )}
        />
        <Route
          path={ROUTES.tradersOverview}
          element={guard(
            ROUTES.traders,
            <AppChrome currentPage="traders">
              <TradersOverviewPage />
            </AppChrome>
          )}
        />
        <Route
          path={ROUTES.forecast}
          element={guard(
            ROUTES.forecast,
            <AppChrome currentPage="forecast">
              <ForecastPage />
            </AppChrome>
          )}
        />
        <Route
          path={ROUTES.forecastStrategies}
          element={guard(
            ROUTES.forecastStrategies,
            <AppChrome currentPage="forecast">
              <ForecastStrategiesPage />
            </AppChrome>
          )}
        />
        <Route
          path={ROUTES.forecastBacktests}
          element={guard(
            ROUTES.forecastBacktests,
            <AppChrome currentPage="forecast">
              <ForecastBacktestsPage />
            </AppChrome>
          )}
        />
        <Route
          path={ROUTES.forecastSettings}
          element={guard(
            ROUTES.forecastSettings,
            <AppChrome currentPage="forecast">
              <ForecastSettingsPage />
            </AppChrome>
          )}
        />
        <Route
          path={ROUTES.help}
          element={
            <AppChrome currentPage="help">
              <FAQPage />
            </AppChrome>
          }
        />
        <Route path="*" element={<Navigate to={ROUTES.home} replace />} />
      </Routes>
      {/* Beside the routes, so moving between pages keeps the conversation. */}
      {isAuthenticated &&
        pathname !== ROUTES.resetPassword &&
        pathname !== ROUTES.resetUser && <AssistantDock />}
    </>
  )
}
