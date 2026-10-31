// Top-level sections of the app, one per item in the header navigation.
export type Page =
  | 'dashboard'
  | 'forecast'
  | 'planning'
  | 'traders'
  | 'strategies'
  | 'help'
  | 'login'
  | 'register'

export const ROUTES = {
  home: '/',
  dashboard: '/dashboard',
  planning: '/frontier',
  traders: '/traders',
  traderDetail: '/traders/:slug/*',
  strategies: '/strategies',
  forecast: '/forecast',
  forecastStrategies: '/forecast/strategies',
  forecastBacktests: '/forecast/backtests',
  forecastSettings: '/forecast/settings',
  forecastDetail: '/forecast/:symbol',
  help: '/help',
  login: '/login',
  register: '/register',
  setup: '/setup',
  resetPassword: '/reset-password',
  resetAccount: '/reset-account',
} as const

// Settings open as a dialog over the current page, named by this query
// parameter (?settings=account, models, exchanges, costs or telegram).
export const SETTINGS_PARAM = 'settings'

const SECTION_PREFIXES: [string, Page][] = [
  [ROUTES.dashboard, 'dashboard'],
  [ROUTES.forecast, 'forecast'],
  [ROUTES.planning, 'planning'],
  [ROUTES.traders, 'traders'],
  [ROUTES.strategies, 'strategies'],
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

export function buildStrategyPath(strategyId: string): string {
  return `${ROUTES.strategies}?strategy=${encodeURIComponent(strategyId)}`
}

export function buildTraderPath(traderSlug: string): string {
  return `${ROUTES.traders}/${encodeURIComponent(traderSlug)}`
}

export interface FrontierLink {
  venue?: string
  symbol?: string
  account?: string
  plan?: string
  tab?: string
}

// buildFrontierPath opens Frontier on a market, account, plan or section.
export function buildFrontierPath(link: FrontierLink): string {
  const query = new URLSearchParams()
  for (const [name, value] of Object.entries(link)) {
    if (value) query.set(name, value)
  }
  const text = query.toString()
  return text ? `${ROUTES.planning}?${text}` : ROUTES.planning
}
