// Top-level sections of the app, one per item in the header navigation.
export type Page =
  | 'dashboard'
  | 'planning'
  | 'autopilot'
  | 'forecast'
  | 'help'
  | 'login'
  | 'register'

export const ROUTES = {
  home: '/',
  dashboard: '/dashboard',
  planning: '/frontier',
  autopilot: '/autopilot',
  strategies: '/autopilot/strategies',
  traderDetail: '/autopilot/traders/:slug/*',
  forecast: '/forecast',
  forecastStrategies: '/forecast/strategies',
  forecastBacktests: '/forecast/backtests',
  forecastDetail: '/forecast/:symbol',
  help: '/help',
  login: '/login',
  register: '/register',
  setup: '/setup',
  resetPassword: '/reset-password',
  resetAccount: '/reset-account',
} as const

// Settings open as a dialog over the current page, named by this query
// parameter (?settings=account, models, exchanges, costs, telegram or forecast).
export const SETTINGS_PARAM = 'settings'

const SECTION_PREFIXES: [string, Page][] = [
  [ROUTES.dashboard, 'dashboard'],
  [ROUTES.planning, 'planning'],
  [ROUTES.autopilot, 'autopilot'],
  [ROUTES.forecast, 'forecast'],
  [ROUTES.help, 'help'],
  [ROUTES.login, 'login'],
  [ROUTES.register, 'register'],
]

export function getCurrentPageForPath(pathname: string): Page | undefined {
  const path = pathname.replace(/\/+$/, '') || '/'
  for (const [prefix, page] of SECTION_PREFIXES) {
    if (path === prefix || path.startsWith(`${prefix}/`)) return page
  }
  return undefined
}

export function buildForecastPath(symbol: string, interval: string): string {
  return `${ROUTES.forecast}/${encodeURIComponent(symbol)}?interval=${interval}`
}

export function buildTraderPath(traderSlug: string): string {
  return `${ROUTES.autopilot}/traders/${encodeURIComponent(traderSlug)}`
}
