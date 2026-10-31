import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { SWRConfig } from 'swr'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { ForecastRecord, ForecastSignal } from '@/api/forecast'
import { sortSignals } from '@/components/forecast/signal-table'
import { ForecastPage } from '@/pages/forecast-page'

const mocks = vi.hoisted(() => ({ signals: vi.fn() }))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
vi.mock('@/api', () => ({ api: { getForecastSignals: mocks.signals } }))
vi.mock('@/components/settings/settings-link', () => ({
  SettingsLink: ({ children }: { children: React.ReactNode }) => (
    <a href="#settings">{children}</a>
  ),
}))

const record = (patch: Partial<ForecastRecord>) =>
  ({
    id: 1,
    display_prob: 0.5,
    calibrated: false,
    direction: 'neutral',
    confidence: 10,
    expected_move_pct: 1,
    regime: 'ranging',
    open_time: 1,
    ...patch,
  }) as ForecastRecord

const signal = (
  symbol: string,
  patch: Partial<ForecastSignal> = {}
): ForecastSignal => ({
  symbol,
  interval: '1h',
  tier: 1,
  price: 100,
  record: null,
  stale: false,
  ...patch,
})

const show = () =>
  render(
    <MemoryRouter>
      <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
        <ForecastPage />
      </SWRConfig>
    </MemoryRouter>
  )

beforeEach(() => vi.clearAllMocks())
afterEach(cleanup)

it('puts strong calls first in either direction and waiting coins last', () => {
  const sorted = sortSignals([
    signal('WAITUSDT', { pending: 'waiting' }),
    signal('WEAKUSDT', { record: record({ display_prob: 0.52 }) }),
    signal('DOWNUSDT', { record: record({ display_prob: 0.2 }) }),
    signal('UPUSDT', { record: record({ display_prob: 0.7 }) }),
  ])
  expect(sorted.map((row) => row.symbol)).toEqual([
    'DOWNUSDT',
    'UPUSDT',
    'WEAKUSDT',
    'WAITUSDT',
  ])
})

it('shows a score until a forecast is calibrated, then a probability', async () => {
  mocks.signals.mockResolvedValue({
    signals: [
      signal('BTCUSDT', {
        record: record({ display_prob: 0.68, direction: 'up' }),
      }),
      signal('ETHUSDT', {
        record: record({ display_prob: 0.34, calibrated: true }),
      }),
    ],
  })
  show()
  expect(await screen.findByText('68')).toBeVisible()
  expect(screen.getByText('34%')).toBeVisible()
  expect(screen.getByText('Up')).toBeVisible()
})

it('says why a coin has no forecast yet, and filters by search', async () => {
  mocks.signals.mockResolvedValue({
    signals: [
      signal('NEWUSDT', { pending: 'not_enough_history' }),
      signal('BTCUSDT', { pending: 'fetching' }),
    ],
  })
  show()
  expect(await screen.findByText('Not enough history')).toBeVisible()
  expect(screen.getByText('Fetching candles…')).toBeVisible()

  fireEvent.change(screen.getByLabelText('Search coins'), {
    target: { value: 'new' },
  })
  expect(screen.queryByText('Fetching candles…')).toBeNull()
  expect(screen.getByText('NEW')).toBeVisible()
})

it('asks for another interval from the server', async () => {
  mocks.signals.mockResolvedValue({ signals: [] })
  show()
  expect(
    await screen.findByText('No coins are being tracked. Add some in Settings.')
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: '4h' }))
  expect(mocks.signals).toHaveBeenLastCalledWith('4h')
})
