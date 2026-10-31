import { expect, it } from 'vitest'
import { ordersToCsv } from '@/components/trader/orders-csv'
import type { TraderOrder } from '@/types'

const order = (fields: Partial<TraderOrder>) =>
  ({
    id: 1,
    symbol: 'BTCUSDT',
    side: 'BUY',
    position_side: 'LONG',
    type: 'MARKET',
    quantity: 0.5,
    price: 0,
    status: 'FILLED',
    filled_quantity: 0.5,
    avg_fill_price: 70000.5,
    commission: 0.02,
    commission_asset: 'USDT',
    order_action: 'open_long',
    created_at: Date.UTC(2026, 0, 2, 3, 4, 5),
    ...fields,
  }) as TraderOrder

it('writes one row per order under a header', () => {
  const [header, row] = ordersToCsv([order({})]).split('\n')
  expect(header.startsWith('created_at,symbol,side')).toBe(true)
  expect(row).toBe(
    '2026-01-02T03:04:05.000Z,BTCUSDT,BUY,LONG,MARKET,open_long,FILLED,0.5,0.5,70000.5,0.02,USDT'
  )
})

it('quotes separators and defuses spreadsheet formulas', () => {
  const row = ordersToCsv([
    order({ symbol: 'A,B', order_action: '=SUM(A1)' }),
  ]).split('\n')[1]
  expect(row).toContain('"A,B"')
  expect(row).toContain("'=SUM(A1)")
})
