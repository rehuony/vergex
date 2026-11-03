import { common } from './common'
import { auth } from './auth'
import { settings } from './settings'
import { traders } from './traders'
import { dashboard } from './dashboard'
import { futures } from './futures'
import { forecast } from './forecast'
import { assistant } from './assistant'

// The template: every other language has exactly these keys, which the types enforce.
export const en = {
  ...common,
  ...auth,
  ...settings,
  ...traders,
  ...dashboard,
  ...futures,
  ...forecast,
  ...assistant,
}
