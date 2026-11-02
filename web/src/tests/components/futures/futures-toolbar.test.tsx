import { cleanup, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, expect, it, vi } from 'vitest'
import { FuturesToolbar } from '@/components/futures/futures-toolbar'
import type { Account } from '@/types'

vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))

afterEach(cleanup)

const accounts = [{ id: 'main', name: 'Main', venue: 'binance' }] as Account[]

const trigger = (venue: string, accountID: string, loaded = true) => {
  render(
    <MemoryRouter>
      <FuturesToolbar
        venue={venue}
        loaded={loaded}
        accounts={accounts}
        accountID={accountID}
        accountCheck={null}
        leverage={5}
        busy={false}
        onVenueChange={vi.fn()}
        onAccountChange={vi.fn()}
        onLeverageChange={vi.fn()}
      />
    </MemoryRouter>
  )
  return screen.getByRole('combobox', { name: 'Trading account' })
}

it('shows the selected account on a venue that has one', () => {
  expect(trigger('binance', 'main')).toHaveTextContent('Main')
  expect(screen.queryByText('No account')).toBeNull()
})

it('says there is no account only for a venue with none', () => {
  expect(trigger('okx', '')).toHaveTextContent('No account')
  expect(screen.getByRole('link', { name: 'Add an account' })).toBeVisible()
})

it('claims nothing about accounts before they are read', () => {
  expect(trigger('binance', '', false)).toHaveTextContent('')
  expect(screen.queryByText('No account')).toBeNull()
  expect(screen.queryByRole('link', { name: 'Add an account' })).toBeNull()
})
