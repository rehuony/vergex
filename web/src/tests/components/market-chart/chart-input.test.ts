import { expect, it } from 'vitest'
import { wheelIntent } from '@/components/market-chart/chart-input'

const wheel = (deltaX: number, deltaY: number, keys = {}) => ({
  ctrlKey: false,
  metaKey: false,
  deltaX,
  deltaY,
  ...keys,
})

it('leaves an upright scroll to the page, even with a little sideways drift', () => {
  expect(wheelIntent(wheel(0, 100))).toBe('page')
  expect(wheelIntent(wheel(3, -40))).toBe('page')
})

it('moves along time on a sideways scroll', () => {
  expect(wheelIntent(wheel(60, 2))).toBe('pan')
})

it('zooms only with Ctrl or Command, which is how a pinch arrives too', () => {
  expect(wheelIntent(wheel(0, 100, { ctrlKey: true }))).toBe('zoom')
  expect(wheelIntent(wheel(0, -4, { metaKey: true }))).toBe('zoom')
})
