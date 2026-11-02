import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { MarketPicker } from '@/components/futures/market-picker'

const mocks = vi.hoisted(() => ({ api: vi.fn() }))
vi.mock('@/api/futures', () => ({ futuresAPI: mocks.api }))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
afterEach(() => {
  cleanup()
  mocks.api.mockReset()
})

const market = (symbol: string, volume: number, execution = true) => ({
  symbol,
  base: symbol,
  volume_24h: volume,
  category: 'crypto',
  price: '100',
  execution_supported: execution,
})
const board = (...markets: ReturnType<typeof market>[]) => ({
  venue: 'binance',
  markets,
  sources: ['fixture'],
  fetched_at: new Date().toISOString(),
})

const open = (select = vi.fn()) => {
  render(
    <MarketPicker
      venue="binance"
      selected="BTCUSDT"
      disabled={false}
      onSelect={select}
    />
  )
  fireEvent.click(screen.getByRole('button', { name: 'Market BTCUSDT' }))
  return select
}

it('loads markets only when opened, ranked by volume', async () => {
  mocks.api.mockResolvedValue(
    board(market('BTCUSDT', 100), market('ETHUSDT', 500))
  )
  render(
    <MarketPicker
      venue="binance"
      selected="BTCUSDT"
      disabled={false}
      onSelect={vi.fn()}
    />
  )
  expect(mocks.api).not.toHaveBeenCalled()
  cleanup()
  const select = open()
  const eth = await screen.findByRole('button', { name: /ETHUSDT/ })
  const names = screen
    .getAllByRole('button')
    .map((button) => button.textContent)
    .filter((text) => /USDT/.test(text ?? ''))
  expect(names.findIndex((n) => n?.includes('ETHUSDT'))).toBeLessThan(
    names.findIndex((n) => n?.startsWith('BTCUSDT1'))
  )
  fireEvent.click(eth)
  expect(select).toHaveBeenCalledWith('ETHUSDT')
  expect(mocks.api).toHaveBeenCalledWith('markets?venue=binance')
})

it('marks markets without a verified execution adapter as chart only', async () => {
  mocks.api.mockResolvedValue(
    board(market('ETHUSDT', 100), market('NEWUSDT', 200, false))
  )
  open()
  const listing = await screen.findByRole('button', { name: /NEWUSDT/ })
  expect(listing).toHaveTextContent('Chart only')
  fireEvent.change(screen.getByLabelText('Search exchange markets'), {
    target: { value: 'eth' },
  })
  expect(screen.getByRole('button', { name: /ETHUSDT/ })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /NEWUSDT/ })).toBeNull()
})

it('opens a typed symbol the list does not have', async () => {
  mocks.api.mockResolvedValue(board(market('ETHUSDT', 100)))
  const select = open()
  await screen.findByRole('button', { name: /ETHUSDT/ })
  const search = screen.getByLabelText('Search exchange markets')
  fireEvent.change(search, { target: { value: 'dogeusdt' } })
  expect(
    screen.getByText('Press Enter to open this symbol')
  ).toBeInTheDocument()
  fireEvent.submit(search)
  expect(select).toHaveBeenCalledWith('DOGEUSDT')
})
