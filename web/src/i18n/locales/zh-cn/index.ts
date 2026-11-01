import type { en } from '@/i18n/locales/en'
import { common } from './common'
import { auth } from './auth'
import { settings } from './settings'
import { traders } from './traders'
import { dashboard } from './dashboard'
import { strategies } from './strategies'
import { frontier } from './frontier'
import { forecast } from './forecast'

export const zhCN: typeof en = {
  ...common,
  ...auth,
  ...settings,
  ...traders,
  ...dashboard,
  ...strategies,
  ...frontier,
  ...forecast,
}
