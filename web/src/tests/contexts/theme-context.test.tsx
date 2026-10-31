import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ThemeProvider, useTheme } from '@/contexts/theme-context'

function Probe() {
  const { preference, theme, setPreference } = useTheme()
  return (
    <button onClick={() => setPreference('light')}>
      {preference}:{theme}
    </button>
  )
}

// jsdom has no matchMedia; stand in for the OS color scheme.
function mockSystem(dark: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: (query: string) =>
      ({
        matches: dark && query.includes('dark'),
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }) as unknown as MediaQueryList,
  })
}

beforeEach(() => {
  localStorage.clear()
  delete document.documentElement.dataset.theme
})
afterEach(() => {
  Reflect.deleteProperty(window, 'matchMedia')
})

it('follows the system until the user picks a theme, then remembers it', () => {
  mockSystem(true)
  render(
    <ThemeProvider>
      <Probe />
    </ThemeProvider>
  )
  expect(screen.getByRole('button')).toHaveTextContent('system:dark')
  expect(document.documentElement.dataset.theme).toBe('dark')

  act(() => screen.getByRole('button').click())
  expect(screen.getByRole('button')).toHaveTextContent('light:light')
  expect(document.documentElement.dataset.theme).toBe('light')
  expect(localStorage.getItem('vergex-theme')).toBe('light')
})
