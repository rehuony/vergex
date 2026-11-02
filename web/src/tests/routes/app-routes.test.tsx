import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { QueryWrapper } from '@/tests/query-wrapper'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { httpClient } from '@/api/http-client'
import { invalidateSystemConfig } from '@/api/system'
import { AuthProvider } from '@/contexts/auth-context'
import { AppRoutes } from '@/routes'

vi.mock('@/pages/dashboard-page', () => ({
  DashboardPage: () => <h1>Dashboard page</h1>,
}))
vi.mock('@/pages/traders-page', () => ({
  TradersPage: () => <h1>Traders page</h1>,
}))
vi.mock('@/components/common/app-chrome', () => ({
  AppChrome: ({ children }: { children: ReactNode }) => children,
}))
vi.mock('@/api', () => ({
  api: {
    getSupportedModels: async () => [],
  },
}))

const session = {
  token: 'session-token',
  user_id: 'owner',
  email: 'owner@example.test',
}

let initialized: boolean

beforeEach(() => {
  window.scrollTo = vi.fn()
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) =>
      Response.json(url === '/api/login' ? session : { initialized })
    )
  )
  invalidateSystemConfig()
})

afterEach(() => {
  cleanup()
  sessionStorage.clear()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

function renderApp(path: string) {
  render(
    <QueryWrapper>
      <MemoryRouter initialEntries={[path]}>
        <AuthProvider>
          <AppRoutes />
        </AuthProvider>
      </MemoryRouter>
    </QueryWrapper>
  )
}

it('stays signed in after registering the first user on /login', async () => {
  initialized = false
  // A token left over from a wiped database gets a 401, which redirects here.
  sessionStorage.setItem('from401', 'true')
  vi.spyOn(httpClient, 'post').mockImplementation(async () => {
    initialized = true
    return { success: true, data: session }
  })
  renderApp('/login')

  fireEvent.change(await screen.findByPlaceholderText('you@example.com'), {
    target: { value: session.email },
  })
  fireEvent.change(screen.getByPlaceholderText('At least 8 characters'), {
    target: { value: 'StrongPass123!' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Get started' }))

  expect(
    await screen.findByRole('heading', { name: 'Dashboard page' })
  ).toBeVisible()
  expect(localStorage.getItem('auth_token')).toBe(session.token)
  expect(localStorage.getItem('user_id')).toBe(session.user_id)
  // The later login page must not claim this new session expired.
  expect(sessionStorage.getItem('from401')).toBeNull()
})

it('keeps the session when /login is opened while signed in', async () => {
  initialized = true
  localStorage.setItem('auth_token', session.token)
  localStorage.setItem(
    'auth_user',
    JSON.stringify({ id: session.user_id, email: session.email })
  )
  renderApp('/login')

  expect(
    await screen.findByRole('heading', { name: 'Dashboard page' })
  ).toBeVisible()
  expect(localStorage.getItem('auth_token')).toBe(session.token)
})

it('signing in returns to the page that asked for it', async () => {
  initialized = true
  renderApp('/traders')

  fireEvent.change(await screen.findByLabelText('Email'), {
    target: { value: session.email },
  })
  fireEvent.change(screen.getByLabelText('Password'), {
    target: { value: 'StrongPass123!' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Sign In' }))

  expect(
    await screen.findByRole('heading', { name: 'Traders page' })
  ).toBeVisible()
  await waitFor(() =>
    expect(localStorage.getItem('auth_token')).toBe(session.token)
  )
})
