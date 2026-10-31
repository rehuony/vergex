import { expect, it } from 'vitest'
import {
  chartQuery,
  logicalAt,
} from '@/components/planning/planning-chart/planning-chart-data'
import { selectAnnotations } from '@/components/planning/planning-chart/planning-chart-shapes'
import type { Drawing } from '@/api/planning'

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

it('drops AI lines that duplicate plan levels or each other', () => {
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
