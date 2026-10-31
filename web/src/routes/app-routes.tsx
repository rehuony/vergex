import { type ReactNode, useEffect } from 'react'
import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { ROUTES } from '@/constants/route-paths'
import { useAuth } from '@/contexts/auth-context'
import { useSystemConfig } from '@/hooks/use-system-config'
// Page CSS is bundled in import order, so reordering these can change the
// cascade: AppChrome last keeps the settings styles after the page styles.
import { LoginPage } from '@/pages/login-page'
import { RegisterPage } from '@/pages/register-page'
import { ResetPasswordPage } from '@/pages/reset-password-page'
import { ResetAccountPage } from '@/pages/reset-account-page'
import { SetupPage } from '@/pages/setup-page'
import { AITradersPage } from '@/pages/ai-traders-page'
import { FAQPage } from '@/pages/faq-page'
import { ForecastBacktestsPage } from '@/pages/forecast-backtests-page'
import { ForecastDetailPage } from '@/pages/forecast-detail-page'
import { ForecastPage } from '@/pages/forecast-page'
import { ForecastStrategiesPage } from '@/pages/forecast-strategies-page'
import { StrategyStudioPage } from '@/pages/strategy-studio-page'
import { TraderDetailPage } from '@/pages/trader-detail-page'
import { TradingPlanningPage } from '@/pages/trading-planning-page'
import { DashboardPage } from '@/pages/dashboard-page'
import { AppChrome } from '@/components/common/app-chrome'
import { LoadingScreen } from '@/components/common/loading-screen'

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
  const guard = (path: string, element: ReactNode) =>
    isAuthenticated ? element : <LoginRedirect returnUrl={path} />

  if (isLoading || configLoading) {
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
        <Route path={ROUTES.login} element={<LoginPage />} />
        <Route path={ROUTES.register} element={<RegisterPage />} />
        <Route path={ROUTES.resetPassword} element={<ResetPasswordPage />} />
        <Route path={ROUTES.resetAccount} element={<ResetAccountPage />} />
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
          path={ROUTES.planning}
          element={guard(
            ROUTES.planning,
            <AppChrome currentPage="planning">
              <TradingPlanningPage />
            </AppChrome>
          )}
        />
        <Route
          path={ROUTES.autopilot}
          element={guard(
            ROUTES.autopilot,
            <AppChrome currentPage="autopilot" animateContent>
              <AITradersPage />
            </AppChrome>
          )}
        />
        <Route
          path={ROUTES.traderDetail}
          element={guard(
            pathname,
            <AppChrome currentPage="autopilot">
              <TraderDetailPage />
            </AppChrome>
          )}
        />
        <Route
          path={ROUTES.strategies}
          element={guard(
            ROUTES.strategies,
            <AppChrome currentPage="autopilot" animateContent>
              <StrategyStudioPage />
            </AppChrome>
          )}
        />
        <Route
          path={ROUTES.forecast}
          element={guard(
            ROUTES.forecast,
            <AppChrome currentPage="forecast" animateContent>
              <ForecastPage />
            </AppChrome>
          )}
        />
        <Route
          path={ROUTES.forecastStrategies}
          element={guard(
            ROUTES.forecastStrategies,
            <AppChrome currentPage="forecast" animateContent>
              <ForecastStrategiesPage />
            </AppChrome>
          )}
        />
        <Route
          path={ROUTES.forecastBacktests}
          element={guard(
            ROUTES.forecastBacktests,
            <AppChrome currentPage="forecast" animateContent>
              <ForecastBacktestsPage />
            </AppChrome>
          )}
        />
        <Route
          path={ROUTES.forecastDetail}
          element={guard(
            pathname,
            <AppChrome currentPage="forecast">
              <ForecastDetailPage />
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
    </>
  )
}
