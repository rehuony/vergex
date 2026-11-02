import { expect, it } from 'vitest'
import { accountLabel } from '@/components/trader/trader-dialog'
import type { Account } from '@/types'

const real = {
  id: 'aaaa1111-1',
  venue: 'binance',
  venue_name: 'Binance',
  name: 'Main',
} as Account
const test = { ...real, id: 'bbbb2222-2', testnet: true } as Account

it('adds nothing to a testnet account but tells it from a real one of the same name', () => {
  expect(accountLabel(test, [test])).toBe('Binance · Main')
  expect(accountLabel(real, [real, test])).toBe('Binance · Main · aaaa1111')
  expect(accountLabel(test, [real, test])).toBe('Binance · Main · bbbb2222')
})
