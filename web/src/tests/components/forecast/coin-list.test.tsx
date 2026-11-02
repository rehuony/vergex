import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, expect, it, vi } from 'vitest'
import type { ForecastCoin, ForecastSignal } from '@/api/forecast'
import { CoinList } from '@/components/forecast/coin-list'

const coin = (symbol: string, prob: number | null, price = 100) =>
  ({
    symbol,
    interval: '1h',
    tier: 1,
    price,
    stale: false,
    signal:
      prob === null
        ? null
        : ({
            display_prob: prob,
            calibrated: false,
            direction: prob >= 0.5 ? 'up' : 'down',
          } as ForecastSignal),
  }) as ForecastCoin

const list = (
  coins: ForecastCoin[],
  watchlist: string[] = [],
  onToggleWatch: (symbol: string) => void = () => {}
) => (
  <MemoryRouter>
    <CoinList
      coins={coins}
      interval="1h"
      selected="AAAUSDT"
      loading={false}
      failed={false}
      watchlist={watchlist}
      onToggleWatch={onToggleWatch}
      language="en"
    />
  </MemoryRouter>
)

const coins = () =>
  screen
    .getAllByRole('row')
    .slice(1)
    .map((row) => row.querySelector('[data-coin]')?.textContent)

afterEach(() => {
  cleanup()
  localStorage.clear()
})

it('keeps rows in place while values refresh, until the sort changes', () => {
  const { rerender } = render(
    list([coin('AAAUSDT', 0.9), coin('BBBUSDT', 0.6)])
  )
  expect(coins()).toEqual(['AAA', 'BBB'])

  // BBB is now the stronger call, but it should not jump ahead by itself.
  rerender(
    list([coin('AAAUSDT', 0.51), coin('BBBUSDT', 0.95), coin('CCCUSDT', 0.99)])
  )
  expect(coins()).toEqual(['AAA', 'BBB', 'CCC'])

  fireEvent.click(screen.getByRole('button', { name: 'Score' }))
  expect(coins()).toEqual(['CCC', 'BBB', 'AAA'])
  fireEvent.click(screen.getByRole('button', { name: 'Coin' }))
  expect(coins()).toEqual(['AAA', 'BBB', 'CCC'])

  rerender(list([coin('AAAUSDT', 0.5), coin('CCCUSDT', 0.5)]))
  expect(coins()).toEqual(['AAA', 'CCC'])
})

it('sorts a coin with no value last whichever way the column runs', () => {
  render(
    list([coin('WAITUSDT', null), coin('LOWUSDT', 0.3), coin('HIGHUSDT', 0.8)])
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
      coin('AAAUSDT', 0.9),
      { ...coin('NEWUSDT', null), pending: 'not_enough_history' },
    ])
  )
  const rows = screen.getAllByRole('row')
  expect(rows[1]).toHaveAttribute('aria-current', 'true')
  expect(rows[2]).toHaveAttribute('title', 'Not enough history')
  expect(rows[2]).not.toHaveTextContent('—')
})

it('stars a coin without opening it, and filters to the watchlist and to calls', () => {
  const toggle = vi.fn()
  render(
    list(
      [coin('AAAUSDT', 0.9), coin('BBBUSDT', 0.3), coin('CCCUSDT', null)],
      ['BBBUSDT'],
      toggle
    )
  )
  expect(coins()).toEqual(['AAA', 'BBB', 'CCC'])

  fireEvent.click(
    screen.getByRole('button', { name: 'Add AAA to the watchlist' })
  )
  expect(toggle).toHaveBeenCalledWith('AAAUSDT')
  expect(
    screen.getByRole('button', { name: 'Remove BBB from the watchlist' })
  ).toHaveAttribute('aria-pressed', 'true')

  fireEvent.click(screen.getByRole('button', { name: 'Watchlist' }))
  expect(coins()).toEqual(['BBB'])
  fireEvent.click(screen.getByRole('button', { name: 'Up' }))
  expect(coins()).toEqual(['AAA'])
  fireEvent.click(screen.getByRole('button', { name: 'Down' }))
  expect(coins()).toEqual(['BBB'])
  expect(localStorage.getItem('vergex.forecast.filter')).toBe('down')
})

it('says what to do when the watchlist is empty', () => {
  localStorage.setItem('vergex.forecast.filter', 'watchlist')
  render(list([coin('AAAUSDT', 0.9)]))
  expect(screen.getByText('Star a coin to keep it here.')).toBeVisible()
})
