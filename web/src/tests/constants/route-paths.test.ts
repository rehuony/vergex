import { describe, expect, it } from 'vitest'
import {
  buildTraderPath,
  getCurrentPageForPath,
  ROUTES,
} from '@/constants/route-paths'

describe('router paths helpers', () => {
  it('maps a pathname to its navigation section', () => {
    expect(getCurrentPageForPath(ROUTES.home)).toBeUndefined()
    expect(getCurrentPageForPath('/futures')).toBe('futures')
    expect(getCurrentPageForPath(ROUTES.dashboard)).toBe('dashboard')
    expect(getCurrentPageForPath(ROUTES.traders)).toBe('traders')
    expect(getCurrentPageForPath('/traders/alpha-1234')).toBe('traders')
    expect(getCurrentPageForPath('/connections')).toBeUndefined()
    expect(getCurrentPageForPath('/settings')).toBeUndefined()
    expect(getCurrentPageForPath('/help/')).toBe('help')
    expect(getCurrentPageForPath('/tradersx')).toBeUndefined()
  })

  it('opens a trader on a tab, keeping what the tab needs', () => {
    expect(buildTraderPath('t1')).toBe('/traders?id=t1')
    expect(buildTraderPath('t1', 'positions')).toBe(
      '/traders?id=t1&tab=positions'
    )
    expect(buildTraderPath('t1', 'decisions?decision=12')).toBe(
      '/traders?id=t1&tab=decisions&decision=12'
    )
  })
})
