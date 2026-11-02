import { beforeEach, expect, it } from 'vitest'
import { getAccountDisplayName } from '@/constants/model-providers'
import type { Account } from '@/types'
import {
  accountName,
  preferredAccount,
  rememberAccount,
} from '@/utils/futures/futures-accounts'
const accounts = [
  {
    id: 'aaaa1111-1',
    venue_name: 'Binance Futures',
    name: 'Main',
    venue: 'binance',
  },
  {
    id: 'bbbb2222-2',
    venue_name: 'Binance Futures',
    name: 'Main',
    venue: 'binance',
  },
] as Account[]
beforeEach(() => localStorage.clear())
it('disambiguates duplicate account names without displaying credentials', () => {
  expect(accountName(accounts[0], accounts)).toBe('Main · aaaa1111')
  expect(accountName(accounts[1], accounts)).toBe('Main · bbbb2222')
})
it('names an account on a test environment as it is', () => {
  const demo = { ...accounts[0], name: 'Demo', testnet: true }
  expect(accountName(demo, [demo])).toBe('Demo')
  expect(getAccountDisplayName(demo.id, [demo])).toBe('BINANCE - Demo')
  expect(getAccountDisplayName(accounts[0].id, accounts)).toBe('BINANCE - Main')
})
it('isolates saved selections by user and falls back when an account is removed', () => {
  localStorage.setItem('user_id', 'alice')
  rememberAccount('binance', accounts[1].id)
  expect(preferredAccount(accounts, 'binance')).toBe(accounts[1].id)
  localStorage.setItem('user_id', 'bob')
  expect(preferredAccount(accounts, 'binance')).toBe(accounts[0].id)
  localStorage.setItem('user_id', 'alice')
  expect(preferredAccount([accounts[0]], 'binance')).toBe(accounts[0].id)
})
it('picks an account whenever the venue has one', () => {
  rememberAccount('binance', '')
  expect(preferredAccount(accounts, 'binance')).toBe(accounts[0].id)
  expect(preferredAccount(accounts, 'okx')).toBe('')
})
