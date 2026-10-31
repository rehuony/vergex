import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, expect, it } from 'vitest'
import type { ForecastRecord, ForecastSignal } from '@/api/forecast'
import { CoinList } from '@/components/forecast/coin-list'

const signal = (symbol: string, prob: number | null, price = 100) =>
  ({
    symbol,
    interval: '1h',
    tier: 1,
    price,
    stale: false,
    record:
      prob === null
        ? null
        : ({
            display_prob: prob,
            calibrated: false,
            direction: prob >= 0.5 ? 'up' : 'down',
          } as ForecastRecord),
  }) as ForecastSignal

const list = (signals: ForecastSignal[]) => (
  <MemoryRouter>
    <CoinList
      signals={signals}
      interval="1h"
      selected="AAAUSDT"
      loading={false}
      failed={false}
      language="en"
    />
  </MemoryRouter>
)

const coins = () =>
  screen
    .getAllByRole('row')
    .slice(1)
    .map((row) => row.querySelector('button')?.textContent)

afterEach(cleanup)

it('keeps rows in place while values refresh, until the sort changes', () => {
  const { rerender } = render(
    list([signal('AAAUSDT', 0.9), signal('BBBUSDT', 0.6)])
  )
  expect(coins()).toEqual(['AAA', 'BBB'])

  // BBB is now the stronger call, but it should not jump ahead by itself.
  rerender(
    list([
      signal('AAAUSDT', 0.51),
      signal('BBBUSDT', 0.95),
      signal('CCCUSDT', 0.99),
    ])
  )
  expect(coins()).toEqual(['AAA', 'BBB', 'CCC'])

  fireEvent.click(screen.getByRole('button', { name: 'Score' }))
  expect(coins()).toEqual(['CCC', 'BBB', 'AAA'])
  fireEvent.click(screen.getByRole('button', { name: 'Coin' }))
  expect(coins()).toEqual(['AAA', 'BBB', 'CCC'])

  rerender(list([signal('AAAUSDT', 0.5), signal('CCCUSDT', 0.5)]))
  expect(coins()).toEqual(['AAA', 'CCC'])
})

it('sorts a coin with no value last whichever way the column runs', () => {
  render(
    list([
      signal('WAITUSDT', null),
      signal('LOWUSDT', 0.3),
      signal('HIGHUSDT', 0.8),
    ])
  )
  const score = screen.getByRole('button', { name: 'Score' })
  fireEvent.click(score)
  expect(coins()).toEqual(['HIGH', 'LOW', 'WAIT'])
  fireEvent.click(score)
  expect(coins()).toEqual(['LOW', 'HIGH', 'WAIT'])
})

it('marks the coin in view and says why another has no forecast', () => {
  render(
    list([
      signal('AAAUSDT', 0.9),
      { ...signal('NEWUSDT', null), pending: 'not_enough_history' },
    ])
  )
  const rows = screen.getAllByRole('row')
  expect(rows[1]).toHaveAttribute('aria-current', 'true')
  expect(rows[2]).toHaveAttribute('title', 'Not enough history')
  expect(rows[2]).toHaveTextContent('—')
})
