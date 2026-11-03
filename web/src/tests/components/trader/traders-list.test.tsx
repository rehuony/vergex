import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, expect, it } from 'vitest'
import { TradersList } from '@/components/trader/traders-list'
import type { TraderInfo, TraderMetrics } from '@/types'

const trader = (name: string, running = false) =>
  ({
    trader_id: name,
    trader_name: name,
    agent_model: 'deepseek',
    account_id: 'binance',
    running: running,
  }) as TraderInfo

const metrics = {
  Alpha: { equity: 100 },
  Beta: { equity: 900 },
} as unknown as Record<string, TraderMetrics>

const show = (props: {
  traders: TraderInfo[]
  search?: string
  modelCount?: number
  accountCount?: number
}) =>
  render(
    <MemoryRouter>
      <TradersList
        metrics={metrics}
        failed={false}
        models={[]}
        accounts={[]}
        modelCount={1}
        accountCount={1}
        search=""
        language="en"
        {...props}
      />
    </MemoryRouter>
  )

const names = () =>
  screen
    .getAllByRole('link')
    .map((link) => link.textContent)
    .filter((text) => text !== 'Add model')

afterEach(cleanup)

it('lists running traders first, then by the chosen column', () => {
  show({ traders: [trader('Alpha'), trader('Beta'), trader('Gamma', true)] })
  expect(names()).toEqual(['Gamma', 'Alpha', 'Beta'])

  fireEvent.click(screen.getByRole('button', { name: 'Account equity' }))
  expect(names()).toEqual(['Beta', 'Alpha', 'Gamma'])

  fireEvent.click(screen.getByRole('button', { name: 'Account equity' }))
  expect(names()).toEqual(['Alpha', 'Beta', 'Gamma'])
})

it('filters by the search and says when nothing matches', () => {
  const traders = [trader('Alpha'), trader('Beta')]
  const { unmount } = show({ traders, search: 'bet' })
  expect(names()).toEqual(['Beta'])
  unmount()

  show({ traders, search: 'nothing' })
  expect(screen.getByText('No traders match this search')).toBeInTheDocument()
})

it('points a first trader at what is still missing', () => {
  show({ traders: [], modelCount: 0, accountCount: 0 })
  expect(
    screen.getByText('A trader needs a model and an account')
  ).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Add model' })).toHaveAttribute(
    'href',
    '?settings=models'
  )
})
