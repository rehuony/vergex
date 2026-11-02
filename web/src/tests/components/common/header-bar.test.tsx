import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { expect, it, vi } from 'vitest'
import { HeaderBar } from '@/components/common/header-bar'

function renderHeader(props: Partial<Parameters<typeof HeaderBar>[0]> = {}) {
  return render(
    <MemoryRouter>
      <HeaderBar
        isLoggedIn
        user={{ email: 'trader@vergex.test' }}
        language="en"
        {...props}
      />
    </MemoryRouter>
  )
}

it('keeps settings and help in the account menu, not the nav', () => {
  const onOpenSettings = vi.fn()
  renderHeader({ onOpenSettings })
  expect(screen.queryByRole('button', { name: 'Settings' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Help docs' })).toBeNull()

  fireEvent.click(screen.getByRole('button', { name: /Account menu/ }))
  expect(screen.getByText('trader@vergex.test')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Help docs' })).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Settings' }))

  expect(onOpenSettings).toHaveBeenCalledOnce()
  expect(screen.queryByText('trader@vergex.test')).toBeNull()
})

it('switches the theme from the Appearance submenu', () => {
  const onThemeChange = vi.fn()
  renderHeader({ themePreference: 'system', onThemeChange })

  fireEvent.click(screen.getByRole('button', { name: /Account menu/ }))
  const appearance = screen.getByRole('button', { name: /^Appearance/ })
  expect(appearance).toHaveTextContent('System')
  fireEvent.click(appearance)
  fireEvent.click(screen.getByRole('button', { name: 'Dark' }))

  expect(onThemeChange).toHaveBeenCalledWith('dark')
})

it('does not expose a mode switch after login', () => {
  render(
    <MemoryRouter initialEntries={['/traders']}>
      <HeaderBar
        isLoggedIn
        user={{ email: 'owner@example.com' }}
        onLogout={() => undefined}
      />
    </MemoryRouter>
  )

  fireEvent.click(screen.getByRole('button', { name: /owner@example.com/i }))

  expect(screen.queryByText(/Switch to Advanced/i)).toBeNull()
  expect(screen.queryByText(/Switch to Beginner/i)).toBeNull()
})

it('keeps desktop navigation labels on one line', () => {
  render(
    <MemoryRouter initialEntries={['/traders']}>
      <HeaderBar />
    </MemoryRouter>
  )

  expect(
    screen.getByRole('button', { name: 'Auto trading' }).className
  ).toContain('whitespace-nowrap')
})

it('holds the pages and the account in one sheet on a phone', () => {
  const onOpenSettings = vi.fn()
  renderHeader({ onOpenSettings })
  const toggle = screen.getByRole('button', { name: 'Open navigation' })
  expect(screen.queryByRole('button', { name: 'Settings' })).toBeNull()

  fireEvent.click(toggle)
  expect(toggle).toHaveAttribute('aria-expanded', 'true')
  expect(toggle).toHaveAccessibleName('Close navigation')
  fireEvent.click(screen.getByRole('button', { name: 'Settings' }))

  expect(onOpenSettings).toHaveBeenCalledOnce()
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
})

it('closes the sheet when the page you are on is chosen again', () => {
  render(
    <MemoryRouter initialEntries={['/traders']}>
      <HeaderBar isLoggedIn user={{ email: 'trader@vergex.test' }} />
    </MemoryRouter>
  )
  const toggle = screen.getByRole('button', { name: 'Open navigation' })

  fireEvent.click(toggle)
  fireEvent.click(screen.getByRole('button', { name: 'Auto trading' }))

  expect(toggle).toHaveAttribute('aria-expanded', 'false')
})

it('releases the page when the sheet closes with Escape', () => {
  renderHeader()
  const page = document.createElement('main')
  document.body.append(page)
  const toggle = screen.getByRole('button', { name: 'Open navigation' })

  fireEvent.click(toggle)
  expect(page).toHaveAttribute('inert')
  fireEvent.keyDown(document, { key: 'Escape' })

  expect(toggle).toHaveAttribute('aria-expanded', 'false')
  expect(toggle).toHaveFocus()
  expect(page).not.toHaveAttribute('inert')
  page.remove()
})
