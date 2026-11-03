import { accountApi } from './account'
import { agentApi } from './agent'
import { assistantApi } from './assistant'
import { forecastApi } from './forecast'
import { marketApi } from './market'
import { overviewApi } from './dashboard'
import { systemApi } from './system'
import { telegramApi } from './telegram'
import { traderApi } from './trader'
import { userApi } from './user'

export const api = {
  ...accountApi,
  ...agentApi,
  ...assistantApi,
  ...forecastApi,
  ...marketApi,
  ...overviewApi,
  ...systemApi,
  ...telegramApi,
  ...traderApi,
  ...userApi,
}
