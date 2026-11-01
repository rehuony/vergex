import { common } from './common'
import { auth } from './auth'
import { settings } from './settings'
import { traders } from './traders'
import { dashboard } from './dashboard'
import { strategies } from './strategies'
import { frontier } from './frontier'
import { forecast } from './forecast'

// The template: every other language has exactly these keys, which the types enforce.
export const en = {
  ...common,
  ...auth,
  ...settings,
  ...traders,
  ...dashboard,
  ...strategies,
  ...frontier,
  ...forecast,
}
