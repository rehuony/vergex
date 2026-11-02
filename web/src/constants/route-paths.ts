// Top-level sections of the app, one per item in the header navigation.
export type Page =
  | 'dashboard'
  | 'forecast'
  | 'futures'
  | 'traders'
  | 'help'
  | 'login'

export const ROUTES = {
  home: '/',
  dashboard: '/dashboard',
  futures: '/futures',
  traders: '/traders',
  tradersOverview: '/traders/overview',
  forecast: '/forecast',
  forecastStrategies: '/forecast/strategies',
  forecastBacktests: '/forecast/backtests',
  forecastSettings: '/forecast/settings',
  help: '/help',
  login: '/login',
  setup: '/setup',
  resetPassword: '/reset-password',
  resetUser: '/reset-user',
} as const

// Settings open as a dialog over the current page, named by this query
// parameter (?settings=profile, accounts, models, costs or telegram).
export const SETTINGS_PARAM = 'settings'

const SECTION_PREFIXES: [string, Page][] = [
  [ROUTES.dashboard, 'dashboard'],
  [ROUTES.forecast, 'forecast'],
  [ROUTES.futures, 'futures'],
  [ROUTES.traders, 'traders'],
  [ROUTES.help, 'help'],
  [ROUTES.login, 'login'],
]

export function getCurrentPageForPath(pathname: string): Page | undefined {
  const path = pathname.replace(/\/+$/, '') || '/'
  for (const [prefix, page] of SECTION_PREFIXES) {
    if (path === prefix || path.startsWith(`${prefix}/`)) return page
  }
  return undefined
}

export function buildForecastPath(symbol: string, interval: string): string {
  return `${ROUTES.forecast}?symbol=${encodeURIComponent(symbol)}&interval=${interval}`
}

// buildTraderPath opens a trader on a tab, which can carry its own parameters.
export function buildTraderPath(traderId: string, section = ''): string {
  const [tab, more] = section.split('?')
  const query = new URLSearchParams({ id: traderId })
  if (tab) query.set('tab', tab)
  new URLSearchParams(more).forEach((value, name) => query.set(name, value))
  return `${ROUTES.traders}?${query}`
}

export interface FuturesLink {
  venue?: string
  symbol?: string
  account?: string
  plan?: string
  tab?: string
}

// buildFuturesPath opens Futures on a market, account, plan or section.
export function buildFuturesPath(link: FuturesLink): string {
  const query = new URLSearchParams()
  for (const [name, value] of Object.entries(link)) {
    if (value) query.set(name, value)
  }
  const text = query.toString()
  return text ? `${ROUTES.futures}?${text}` : ROUTES.futures
}
