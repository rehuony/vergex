import { traderApi } from './traders'
import { strategyApi } from './strategies'
import { configApi } from './config'
import { dataApi } from './data'
import { forecastApi } from './forecast'
import { marketDataApi } from './market-data'
import { overviewApi } from './overview'
import { telegramApi } from './telegram'
import { userApi } from './user'

export const api = {
  ...traderApi,
  ...strategyApi,
  ...configApi,
  ...dataApi,
  ...forecastApi,
  ...marketDataApi,
  ...overviewApi,
  ...telegramApi,
  ...userApi,
}
