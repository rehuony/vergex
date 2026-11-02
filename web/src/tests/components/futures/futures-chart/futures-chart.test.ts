import { expect, it } from 'vitest'
import {
  chartQuery,
  logicalAt,
} from '@/components/futures/futures-chart/futures-chart-data'
import {
  comparisonShapes,
  holdingLines,
  selectAnnotations,
} from '@/components/futures/futures-chart/futures-chart-shapes'
import type { Drawing, Plan } from '@/api/futures'

it('heads each comparison lane with its way of entering, over prices that fit it', () => {
  const lane = (id: string, kind: string, entry: string, orders: string[]) =>
    ({
      id,
      quantity: '1',
      snapshot: { tick: '0.1' },
      orders: orders.map((price) => ({ price, quantity: '1' })),
      idea: {
        side: 'long',
        entry,
        stop: '100',
        target: '200',
        execution: { kind },
      },
    }) as unknown as Plan
  const texts = comparisonShapes(
    [
      lane('a', 'limit', '150', ['150']),
      // A market order's price is its bound, and a ladder goes by its average.
      lane('b', 'market', '160', ['152']),
      lane('c', 'ladder', '140', ['140', '141', '141']),
    ],
    undefined
  ).flatMap((shape) => (shape.type === 'text' ? [shape.text] : []))
  expect(texts).toEqual([
    'Limit',
    '200',
    '150',
    '100',
    'Market',
    '200',
    '≈160',
    '100',
    'Ladder',
    '200',
    '≈140.7',
    '100',
  ])
})

it('draws only what the account holds on this market', () => {
  const position = {
    side: 'long' as const,
    quantity: 0.5,
    entry: 80000,
    mark: 81000,
    pnl: 500,
    leverage: 5,
  }
  const lines = holdingLines(
    'binance',
    'BTCUSDT',
    [
      { ...position, symbol: 'BTCUSDT' },
      { ...position, symbol: 'ETHUSDT', entry: 3000 },
    ],
    [
      { type: 'STOP_MARKET', price: 0, stop_price: 79000, quantity: 0.5 },
      {
        type: 'TAKE_PROFIT_MARKET',
        price: 0,
        stop_price: 84000,
        quantity: 0.5,
      },
      { type: 'LIMIT', price: 78000, stop_price: 0, quantity: 0.5 },
    ]
  )
  expect(lines.map((line) => [line.tone, line.price])).toEqual([
    ['up', 80000],
    ['down', 79000],
    ['up', 84000],
  ])
})

it.each([
  ['OKX:BTC-USDT-SWAP', 'okx', 'BTC-USDT-SWAP'],
  ['BINANCE:BTCUSDT', 'binance', 'BTCUSDT'],
])('routes %s chart history to its own venue', (ticker, venue, symbol) => {
  const query = new URLSearchParams(chartQuery(ticker, '60'))
  expect(query.get('venue')).toBe(venue)
  expect(query.get('symbol')).toBe(symbol)
  expect(query.get('interval')).toBe('1h')
})

it('rejects unsupported venues and path-like symbols before fetching', () => {
  for (const ticker of ['BITGET:BTCUSDT', 'BINANCE:../secret']) {
    expect(() => chartQuery(ticker, '60')).toThrow('Invalid symbol format')
  }
})

it('maps drawing times onto the bar index, extrapolating past the data', () => {
  const times = [100, 160, 220]
  expect(logicalAt(times, 60, 160)).toBe(1)
  expect(logicalAt(times, 60, 190)).toBe(1.5)
  expect(logicalAt(times, 60, 400)).toBe(5)
  expect(logicalAt(times, 60, 40)).toBe(-1)
  expect(logicalAt([], 60, 100)).toBeNull()
})

const line = (label: string, price: string): Drawing => ({
  kind: 'horizontal_line',
  label,
  points: [{ time: 0, price }],
})

it('drops agent lines that duplicate plan levels or each other', () => {
  const picked = selectAnnotations(
    [line('Support', '100'), line('S2', '100.001'), line('R1', '120')],
    [120]
  )
  expect(picked.map((d) => d.label)).toEqual(['Support'])
})

it('keeps at most four annotations balanced across support and resistance', () => {
  const picked = selectAnnotations(
    [
      line('S1', '90'),
      line('S2', '80'),
      line('S3', '70'),
      line('S4', '60'),
      line('R1', '110'),
      line('R2', '120'),
    ],
    []
  )
  expect(picked.map((d) => d.label)).toEqual(['S1', 'S2', 'R1', 'R2'])
})
