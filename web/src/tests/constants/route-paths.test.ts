import { describe, expect, it } from 'vitest'
import {
  buildTraderPath,
  getCurrentPageForPath,
  ROUTES,
} from '@/constants/route-paths'

describe('router paths helpers', () => {
  it('maps a pathname to its navigation section', () => {
    expect(getCurrentPageForPath(ROUTES.home)).toBeUndefined()
    expect(getCurrentPageForPath('/frontier')).toBe('planning')
    expect(getCurrentPageForPath(ROUTES.dashboard)).toBe('dashboard')
    expect(getCurrentPageForPath(ROUTES.traders)).toBe('traders')
    expect(getCurrentPageForPath('/traders/alpha-1234')).toBe('traders')
    expect(getCurrentPageForPath(ROUTES.strategies)).toBe('strategies')
    expect(getCurrentPageForPath('/connections')).toBeUndefined()
    expect(getCurrentPageForPath('/settings')).toBeUndefined()
    expect(getCurrentPageForPath('/help/')).toBe('help')
    expect(getCurrentPageForPath('/tradersx')).toBeUndefined()
  })

  it('builds a trader path that survives spaces in the name', () => {
    expect(buildTraderPath('alpha beta-1234')).toBe(
      '/traders/alpha%20beta-1234'
    )
  })
})
