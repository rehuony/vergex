import { expect, it } from 'vitest'
import { normalizePosition } from '@/api/trader'

it('reads the field names the exchange adapters return', () => {
  const position = normalizePosition({
    symbol: 'BTCUSDT',
    side: 'short',
    positionAmt: -0.5,
    entryPrice: 100,
    markPrice: 110,
    unRealizedProfit: -5,
    leverage: 5,
  })
  expect(position.quantity).toBe(0.5)
  expect(position.entry_price).toBe(100)
  expect(position.mark_price).toBe(110)
  expect(position.unrealized_pnl).toBe(-5)
  expect(position.unrealized_pnl_pct).toBe(-50)
  expect(position.margin_used).toBe(10)
})

it('leaves a missing value undefined instead of inventing zero', () => {
  const position = normalizePosition({ symbol: 'BTCUSDT', side: 'long' })
  expect(position.quantity).toBeUndefined()
  expect(position.unrealized_pnl).toBeUndefined()
  expect(position.unrealized_pnl_pct).toBeUndefined()
})
