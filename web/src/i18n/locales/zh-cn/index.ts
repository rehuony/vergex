import type { en } from '@/i18n/locales/en'
import { common } from './common'
import { auth } from './auth'
import { settings } from './settings'
import { traders } from './traders'
import { dashboard } from './dashboard'
import { futures } from './futures'
import { forecast } from './forecast'
import { assistant } from './assistant'

export const zhCN: typeof en = {
  ...common,
  ...auth,
  ...settings,
  ...traders,
  ...dashboard,
  ...futures,
  ...forecast,
  ...assistant,
}
