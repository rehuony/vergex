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
    <MemoryRouter initialEntries={['/autopilot']}>
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
    <MemoryRouter initialEntries={['/autopilot']}>
      <HeaderBar />
    </MemoryRouter>
  )

  expect(screen.getByRole('button', { name: 'Autopilot' }).className).toContain(
    'whitespace-nowrap'
  )
})

it('keeps the full navigation visible in the desktop header', () => {
  render(
    <MemoryRouter initialEntries={['/autopilot']}>
      <HeaderBar />
    </MemoryRouter>
  )

  expect(
    screen.getByRole('button', { name: 'Autopilot' }).parentElement?.className
  ).toContain('lg:flex')
  expect(
    screen.getByRole('button', { name: 'Open navigation' }).className
  ).toContain('lg:hidden')
})
